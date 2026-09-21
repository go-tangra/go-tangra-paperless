// Package app wires the paperless service: configuration -> Freya runtime ->
// store/sealing/authz/blob/extract -> the Freya HTTP server (reached only
// through the gateway) and gateway registration. Domain services (documents,
// categories, permissions, search, extraction workers) are attached by their
// user-story phases; this foundational build starts the service with an empty
// registered API surface.
package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-freya/freya"
	"github.com/go-freya/freya/services/lcm/pkg/lcmidentity"

	authv1 "github.com/go-freya/freya/services/auth/api/proto/auth/v1"
	"github.com/go-freya/freya/services/auth/pkg/authclient"
	"github.com/go-freya/freya/services/gateway/pkg/gatewayclient"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/backup"
	"github.com/go-freya/freya/services/paperless/internal/blob"
	"github.com/go-freya/freya/services/paperless/internal/categories"
	"github.com/go-freya/freya/services/paperless/internal/config"
	"github.com/go-freya/freya/services/paperless/internal/documents"
	"github.com/go-freya/freya/services/paperless/internal/events"
	"github.com/go-freya/freya/services/paperless/internal/extract"
	"github.com/go-freya/freya/services/paperless/internal/grpcapi"
	"github.com/go-freya/freya/services/paperless/internal/httpapi"
	"github.com/go-freya/freya/services/paperless/internal/jobs"
	"github.com/go-freya/freya/services/paperless/internal/permissions"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/repo/repodb"
	"github.com/go-freya/freya/services/paperless/internal/sealed"
	"github.com/go-freya/freya/services/paperless/internal/search"
	"github.com/go-freya/freya/services/paperless/internal/stats"
	"github.com/go-freya/freya/services/paperless/internal/store"
	"github.com/go-freya/freya/services/paperless/internal/stream"
	"github.com/go-freya/freya/services/paperless/internal/stream/valkeykv"
	"github.com/go-freya/freya/services/paperless/pkg/paperlessmanifest"
)

// Options override infrastructure (tests) and attach optional parts.
type Options struct {
	Logger   slog.Handler
	KEK      []byte
	Verifier httpapi.Verifier
	Freya    []freya.Option
	Migrate  bool
	Remote   fs.FS // built federated UI remote (nil serves no remote)
}

// App is the wired service.
type App struct {
	Cfg      config.Config
	Log      *slog.Logger
	Freya    *freya.App
	Store    *store.Store
	Repo     repo.Store
	Env      *sealed.Envelope
	Blob     blob.Store
	Extract  extract.Extractor
	Authz    *authz.Authorizer
	Verifier httpapi.Verifier
	HTTP     *httpapi.Server
	Hub      *stream.Hub

	closers []func()
	workers []func(context.Context)
}

// Build wires the service.
func Build(ctx context.Context, cfg config.Config, o Options) (a *App, err error) {
	a = &App{Cfg: cfg}
	handler := o.Logger
	if handler == nil {
		handler = slog.NewJSONHandler(os.Stderr, nil)
	}
	a.Log = slog.New(handler)

	fopts := append([]freya.Option{freya.WithLogger(handler)}, o.Freya...)
	if cfg.Enroll.Enabled {
		raw, rerr := os.ReadFile(cfg.Enroll.TokenFile)
		if rerr != nil {
			return nil, fmt.Errorf("paperless: enroll token: %w", rerr)
		}
		prov, perr := lcmidentity.NewNet(ctx, lcmidentity.NetConfig{
			EnrollURL: cfg.Enroll.EnrollURL, LCMGRPCTarget: cfg.Enroll.LCMGRPCTarget,
			TenantID: cfg.Enroll.TenantID, TrustDomain: cfg.Config.TrustDomain, ServiceName: cfg.Config.ServiceName,
			EnrollmentToken: strings.TrimSpace(string(raw)), Insecure: cfg.Enroll.Insecure, StateFile: cfg.Enroll.StateFile,
		})
		if perr != nil {
			return nil, fmt.Errorf("paperless: enroll: %w", perr)
		}
		a.closers = append(a.closers, func() { _ = prov.Close() })
		fopts = append(fopts, freya.WithIdentityProvider(prov))
	}
	if a.Freya, err = freya.New(cfg.Config, fopts...); err != nil {
		return nil, err
	}
	a.closers = append(a.closers, a.Freya.Close)

	// KEK + envelope.
	kek := o.KEK
	if len(kek) == 0 {
		if kek, err = sealed.LoadKEK(cfg.KEK.Source, cfg.KEK.Path, cfg.KEK.Env); err != nil {
			return nil, fmt.Errorf("kek: %w", err)
		}
	}
	if a.Env, err = sealed.NewEnvelope(kek); err != nil {
		return nil, err
	}

	// Store (migrate then open the app pool).
	if o.Migrate {
		mdsn := cfg.DB.MigrateDSN
		if mdsn == "" {
			mdsn = cfg.DB.DSN
		}
		if err = store.Migrate(ctx, mdsn); err != nil {
			return nil, err
		}
	}
	if a.Store, err = store.Open(ctx, cfg.DB.DSN, cfg.DB.MaxConns); err != nil {
		return nil, err
	}
	a.closers = append(a.closers, a.Store.Close)
	a.Repo = repodb.New(a.Store)
	a.Authz = authz.New(a.Repo)

	// Object store (blob) + extractor.
	if a.Blob, err = blob.New(blob.Config{
		Endpoint: cfg.ObjectStore.Endpoint, Bucket: cfg.ObjectStore.Bucket, Region: cfg.ObjectStore.Region,
		AccessKey: cfg.ObjectStore.AccessKey, SecretKey: cfg.ObjectStore.SecretKey, UseSSL: cfg.ObjectStore.UseSSL,
	}); err != nil {
		return nil, fmt.Errorf("object store: %w", err)
	}
	// Self-provision the bucket (idempotent). Non-fatal: a transient object-store
	// outage should not crash-loop the service; uploads surface the error and the
	// extraction worker retries.
	if berr := a.Blob.EnsureBucket(ctx); berr != nil {
		a.Log.Warn("object store: ensure bucket", "bucket", cfg.ObjectStore.Bucket, "err", berr)
	}
	a.Extract = extract.New(extract.Config{
		TikaURL: cfg.Extract.TikaURL, GotenbergURL: cfg.Extract.GotenbergURL,
		Timeout: cfg.ExtractTimeout(), MaxBytes: cfg.Extract.MaxBytes,
	})

	// Verifier (platform token) from auth.
	a.Verifier = o.Verifier
	if a.Verifier == nil {
		conn, cerr := a.Freya.Client(ctx, "auth")
		if cerr != nil {
			return nil, fmt.Errorf("auth client: %w", cerr)
		}
		a.Verifier = authclient.New(authclient.Config{Issuer: cfg.Gateway.Issuer},
			authclient.GRPCKeys{Client: authv1.NewKeysClient(conn)},
			authclient.GRPCRevocations{Client: authv1.NewSessionsClient(conn)})
	}

	// Event bus (Valkey Streams) for realtime processing status.
	sc := valkeykv.Config{Addresses: cfg.Valkey.Addresses, Username: cfg.Valkey.Username, Password: cfg.Valkey.Password, AllowPlaintext: cfg.Valkey.AllowPlaintext}
	if cfg.Valkey.CAFile != "" {
		if sc.CAPEM, err = os.ReadFile(cfg.Valkey.CAFile); err != nil {
			return nil, fmt.Errorf("valkey ca: %w", err)
		}
	}
	streamClient, serr := valkeykv.New(sc)
	if serr != nil {
		return nil, fmt.Errorf("event bus: %w", serr)
	}
	a.Hub = stream.NewHub(streamClient, stream.Config{}, a.Log)
	a.closers = append(a.closers, a.Hub.Close)

	// HTTP (empty registered surface for now; user stories add routes).
	hopts := []httpapi.Option{httpapi.WithVerifier(a.Verifier)}
	if o.Remote != nil {
		hopts = append(hopts, httpapi.WithRemote(o.Remote))
	}
	if a.HTTP, err = httpapi.NewHandler(a.Freya, hopts...); err != nil {
		return nil, err
	}

	// Services.
	pub := events.HubPublisher{Hub: a.Hub}
	docs := documents.New(a.Repo, a.Authz, a.Blob, pub, cfg.PresignTTL())
	cats := categories.New(a.Repo, a.Authz)
	perms := permissions.New(a.Repo, a.Authz)
	srch := search.New(a.Repo, a.Authz)
	a.HTTP.Register(httpapi.Deps{
		Documents: docs, Categories: cats, Permissions: perms, Search: srch,
		Stats: stats.New(a.Repo), Backup: backup.New(a.Repo), Hub: a.Hub, MaxUpload: cfg.Uploads.MaxSizeBytes, AllowedMime: cfg.Uploads.AllowedMime,
	})
	a.Freya.HTTP().HandlePrefix("/", a.HTTP.Handler())

	// Service-to-service gRPC surface (paperless.v1), for authenticated platform
	// services on the SPIFFE mTLS channel (not gateway-proxied).
	grpcapi.Register(a.Freya.GRPC(), grpcapi.Deps{
		Documents: docs, Categories: cats, Permissions: perms, Search: srch, Stats: stats.New(a.Repo),
	})

	// Extraction worker pool.
	worker := jobs.New(a.Repo, a.Blob, a.Extract, pub, jobs.Config{
		Workers: cfg.Jobs.Workers, Interval: cfg.Interval(), Lease: cfg.Lease(), JobTimeout: cfg.JobTimeout(),
		MaxRetries: cfg.Jobs.MaxRetries, RetryDelay: cfg.RetryDelay(), Backoff: cfg.Jobs.BackoffMultiplier, Cleanup: cfg.CleanupWindow(),
	})
	a.workers = append(a.workers, func(c context.Context) { worker.Run(c, a.Log) })
	return a, nil
}

// Run starts the verifier, gateway registration, workers, and the Freya runtime.
func (a *App) Run(ctx context.Context) error {
	wctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if v, ok := a.Verifier.(*authclient.Verifier); ok {
		go func() {
			for wctx.Err() == nil {
				if err := v.Start(wctx, func(err error) { a.Log.Warn("verifier", "err", err) }); err == nil {
					return
				}
				select {
				case <-wctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}()
	}
	go a.register(wctx)
	for _, w := range a.workers {
		go w(wctx)
	}
	go func() {
		for wctx.Err() == nil && !a.Freya.Ready() {
			time.Sleep(100 * time.Millisecond)
		}
		a.seedLoop(wctx)
	}()
	return a.Freya.Run(ctx)
}

// Close releases resources.
func (a *App) Close() {
	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}
	a.closers = nil
}

// register keeps the gateway lease for the manifest.
func (a *App) register(ctx context.Context) {
	for ctx.Err() == nil && !a.Freya.Ready() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	man, err := paperlessmanifest.Manifest()
	if err != nil {
		a.Log.Error("gateway manifest", "err", err)
		return
	}
	httpEP, err := a.Freya.HTTP().Endpoint()
	if err != nil {
		a.Log.Error("gateway registration: http endpoint", "err", err)
		return
	}
	grpcEP, err := a.Freya.GRPC().Endpoint()
	if err != nil {
		a.Log.Error("gateway registration: grpc endpoint", "err", err)
		return
	}
	var client *gatewayclient.Client
	for ctx.Err() == nil && client == nil {
		conn, cerr := a.Freya.Client(ctx, a.Cfg.Gateway.Service)
		if cerr != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
		}
		client, err = gatewayclient.New(conn, gatewayclient.Options{Manifest: man, HTTPURL: "https://" + httpEP.Host, GRPCTarget: grpcEP.Host, Logger: a.Log,
			OnState: func(s gatewayclient.State) {
				a.Log.Info("gateway lease", "registered", s.Registered, "lease", s.LeaseID, "err", s.Err)
			}})
		if err != nil {
			a.Log.Error("gateway client", "err", err)
			return
		}
	}
	if client != nil {
		if err := client.Run(ctx); err != nil {
			a.Log.Error("gateway registration", "err", err)
		}
	}
}

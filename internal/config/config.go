// Package config loads and validates the paperless service configuration: the
// Freya framework config plus the module's own sections. Every value is
// explicit; insecure opt-outs are named and logged at start (Constitution I/VII).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	fconfig "github.com/go-tangra/go-tangra/v4/config"
	"gopkg.in/yaml.v3"
)

// Config is the paperless service configuration.
type Config struct {
	fconfig.Config `yaml:",inline"`

	DB          DB          `yaml:"db"`
	Valkey      Valkey      `yaml:"valkey"`
	KEK         KEK         `yaml:"kek"`
	ObjectStore ObjectStore `yaml:"object_store"`
	Extract     Extract     `yaml:"extract"`
	Uploads     Uploads     `yaml:"uploads"`
	Jobs        Jobs        `yaml:"jobs"`
	Events      Events      `yaml:"events"`
	Gateway     Gateway     `yaml:"gateway"`
	Enroll      Enroll      `yaml:"enroll"`
	Limits      Limits      `yaml:"limits_paperless"`
}

// DB configures TimescaleDB.
type DB struct {
	DSN        string `yaml:"dsn"`
	MigrateDSN string `yaml:"migrate_dsn"`
	MaxConns   int32  `yaml:"max_conns"`
}

// Valkey configures the platform event bus (Valkey Streams).
type Valkey struct {
	Addresses      []string `yaml:"addresses"`
	Username       string   `yaml:"username"`
	Password       string   `yaml:"password"`
	AllowPlaintext bool     `yaml:"allow_plaintext"`
	CAFile         string   `yaml:"ca_file"`
}

// KEK names where the 32-byte key-encryption key comes from.
type KEK struct {
	Source string `yaml:"source"` // file | env
	Path   string `yaml:"path"`
	Env    string `yaml:"env"`
}

// ObjectStore configures S3-compatible blob storage. Access/secret keys are
// sealed at rest (source: kek) or read from env; never returned.
type ObjectStore struct {
	Endpoint   string `yaml:"endpoint"`
	Bucket     string `yaml:"bucket"`
	Region     string `yaml:"region"`
	UseSSL     bool   `yaml:"use_ssl"`
	AccessKey  string `yaml:"access_key"`
	SecretKey  string `yaml:"secret_key"`
	PresignTTL int    `yaml:"presign_ttl_seconds"`
}

// Extract configures the external text-extraction/conversion services.
type Extract struct {
	TikaURL        string `yaml:"tika_url"`
	GotenbergURL   string `yaml:"gotenberg_url"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	MaxBytes       int64  `yaml:"max_bytes"`
}

// Uploads bounds inbound document uploads.
type Uploads struct {
	MaxSizeBytes int64    `yaml:"max_size_bytes"`
	AllowedMime  []string `yaml:"allowed_mime"` // empty = allow all
}

// Jobs configures the distributed extraction worker pool.
type Jobs struct {
	Workers           int     `yaml:"workers"`
	IntervalSeconds   int     `yaml:"interval_seconds"`
	LeaseSeconds      int     `yaml:"lease_seconds"`
	MaxRetries        int     `yaml:"max_retries"`
	RetryDelaySeconds int     `yaml:"retry_delay_seconds"`
	BackoffMultiplier float64 `yaml:"backoff_multiplier"`
	JobTimeoutSeconds int     `yaml:"job_timeout_seconds"`
	CleanupDays       int     `yaml:"cleanup_days"`
}

// Events toggles the realtime publisher.
type Events struct {
	Enabled bool `yaml:"enabled"`
}

// Gateway names the application gateway and the platform token issuer.
type Gateway struct {
	Service string `yaml:"service"`
	Issuer  string `yaml:"issuer"`
}

// Enroll makes the service obtain its SVID by enrolling with lcm over the
// network; app.Build injects the enroll identity provider.
type Enroll struct {
	Enabled       bool   `yaml:"enabled"`
	EnrollURL     string `yaml:"enroll_url"`
	LCMGRPCTarget string `yaml:"lcm_grpc"`
	TenantID      string `yaml:"tenant_id"`
	TokenFile     string `yaml:"token_file"`
	StateFile     string `yaml:"state_file"`
	Insecure      bool   `yaml:"insecure"`
}

// Limits bound the module's request shapes.
type Limits struct {
	BackupMaxBytes int64 `yaml:"backup_max_bytes"`
	SearchMaxLen   int   `yaml:"search_max_len"`
}

// Default returns secure defaults on top of the Freya defaults.
func Default() Config {
	return Config{
		Config:      fconfig.Default(),
		DB:          DB{MaxConns: 16},
		KEK:         KEK{Source: "file"},
		ObjectStore: ObjectStore{Region: "us-east-1", PresignTTL: 300},
		Extract:     Extract{TimeoutSeconds: 60, MaxBytes: 32 << 20},
		Uploads:     Uploads{MaxSizeBytes: 100 << 20},
		Jobs:        Jobs{Workers: 5, IntervalSeconds: 5, LeaseSeconds: 300, MaxRetries: 3, RetryDelaySeconds: 60, BackoffMultiplier: 2.0, JobTimeoutSeconds: 300, CleanupDays: 30},
		Events:      Events{Enabled: true},
		Gateway:     Gateway{Service: "gateway"},
		Limits:      Limits{BackupMaxBytes: 32 << 20, SearchMaxLen: 512},
	}
}

// Load reads YAML over Default(); unknown fields are rejected.
func Load(path string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path) // #nosec G304 -- operator-supplied config path
	if err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

// Validate checks the Freya config and every module section.
func (c Config) Validate() error {
	if err := c.Config.Validate(); err != nil {
		return err
	}
	prod := c.IsProduction()
	if c.DB.DSN == "" {
		return errors.New("config: db.dsn is required")
	}
	if prod && !strings.Contains(c.DB.DSN, "sslmode=verify-full") && !strings.Contains(c.DB.DSN, "sslmode=verify-ca") {
		return errors.New("config: db.dsn must use sslmode=verify-full (or verify-ca) in production")
	}
	if len(c.Valkey.Addresses) == 0 {
		return errors.New("config: valkey.addresses is required")
	}
	if prod && c.Valkey.AllowPlaintext {
		return errors.New("config: valkey.allow_plaintext is not permitted in production")
	}
	switch c.KEK.Source {
	case "file":
		if c.KEK.Path == "" {
			return errors.New("config: kek.path is required for kek.source file")
		}
	case "env":
		if c.KEK.Env == "" {
			return errors.New("config: kek.env is required for kek.source env")
		}
	default:
		return errors.New("config: kek.source must be file or env")
	}
	if c.ObjectStore.Endpoint == "" || c.ObjectStore.Bucket == "" {
		return errors.New("config: object_store.endpoint and object_store.bucket are required")
	}
	if c.ObjectStore.PresignTTL < 30 || c.ObjectStore.PresignTTL > 3600 {
		return errors.New("config: object_store.presign_ttl_seconds must be within [30, 3600]")
	}
	if c.Uploads.MaxSizeBytes < 1<<10 || c.Uploads.MaxSizeBytes > 5<<30 {
		return errors.New("config: uploads.max_size_bytes must be within [1 KiB, 5 GiB]")
	}
	if c.Extract.TimeoutSeconds < 1 || c.Extract.TimeoutSeconds > 3600 {
		return errors.New("config: extract.timeout_seconds must be within [1, 3600]")
	}
	if c.Jobs.Workers < 1 || c.Jobs.Workers > 64 {
		return errors.New("config: jobs.workers must be within [1, 64]")
	}
	if c.Jobs.IntervalSeconds < 1 || c.Jobs.IntervalSeconds > 60 {
		return errors.New("config: jobs.interval_seconds must be within [1, 60]")
	}
	if c.Jobs.LeaseSeconds < c.Jobs.IntervalSeconds || c.Jobs.LeaseSeconds > 3600 {
		return errors.New("config: jobs.lease_seconds must be within [interval, 3600]")
	}
	if c.Jobs.MaxRetries < 0 || c.Jobs.MaxRetries > 20 {
		return errors.New("config: jobs.max_retries must be within [0, 20]")
	}
	if c.Jobs.JobTimeoutSeconds < 10 || c.Jobs.JobTimeoutSeconds > 3600 {
		return errors.New("config: jobs.job_timeout_seconds must be within [10, 3600]")
	}
	if c.Jobs.CleanupDays < 1 || c.Jobs.CleanupDays > 3650 {
		return errors.New("config: jobs.cleanup_days must be within [1, 3650]")
	}
	if c.Gateway.Service == "" {
		return errors.New("config: gateway.service is required")
	}
	if iu, err := url.Parse(c.Gateway.Issuer); err != nil || iu.Scheme != "https" || iu.Host == "" {
		return errors.New("config: gateway.issuer must be an https origin")
	}
	if c.Limits.BackupMaxBytes < 4<<20 || c.Limits.BackupMaxBytes > 128<<20 {
		return errors.New("config: limits_paperless.backup_max_bytes must be within [4 MiB, 128 MiB]")
	}
	if c.Limits.SearchMaxLen < 16 || c.Limits.SearchMaxLen > 4096 {
		return errors.New("config: limits_paperless.search_max_len must be within [16, 4096]")
	}
	return nil
}

// Warnings lists accepted insecure opt-outs (logged at start).
func (c Config) Warnings() []string {
	w := c.Config.Warnings()
	if c.Valkey.AllowPlaintext {
		w = append(w, "valkey.allow_plaintext: event-bus traffic without TLS (development only)")
	}
	if !c.ObjectStore.UseSSL {
		w = append(w, "object_store.use_ssl=false: blob traffic without TLS (development only)")
	}
	return w
}

// Interval is the worker tick.
func (c Config) Interval() time.Duration { return time.Duration(c.Jobs.IntervalSeconds) * time.Second }

// Lease is how long a claimed job stays claimed.
func (c Config) Lease() time.Duration { return time.Duration(c.Jobs.LeaseSeconds) * time.Second }

// JobTimeout bounds a single extraction attempt.
func (c Config) JobTimeout() time.Duration {
	return time.Duration(c.Jobs.JobTimeoutSeconds) * time.Second
}

// CleanupWindow is the job retention window.
func (c Config) CleanupWindow() time.Duration {
	return time.Duration(c.Jobs.CleanupDays) * 24 * time.Hour
}

// RetryDelay is the base retry delay.
func (c Config) RetryDelay() time.Duration {
	return time.Duration(c.Jobs.RetryDelaySeconds) * time.Second
}

// ExtractTimeout bounds a Tika/Gotenberg call.
func (c Config) ExtractTimeout() time.Duration {
	return time.Duration(c.Extract.TimeoutSeconds) * time.Second
}

// PresignTTL is the lifetime of a presigned download URL.
func (c Config) PresignTTL() time.Duration {
	return time.Duration(c.ObjectStore.PresignTTL) * time.Second
}

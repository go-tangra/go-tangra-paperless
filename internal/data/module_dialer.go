package data

import (
	"context"
	"fmt"
	"os"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	grpcMD "google.golang.org/grpc/metadata"

	"github.com/go-tangra/go-tangra-common/grpcx"
	"github.com/go-tangra/go-tangra-common/registration"
)

// NewRegistrationClient creates a registration client connected to admin-service.
func NewRegistrationClient(ctx *bootstrap.Context) (*registration.Client, error) {
	adminEndpoint := os.Getenv("ADMIN_GRPC_ENDPOINT")
	if adminEndpoint == "" {
		return nil, nil
	}

	cfg := &registration.Config{
		AdminEndpoint: adminEndpoint,
		MaxRetries:    60,
	}

	return registration.NewClient(ctx.GetLogger(), cfg)
}

// NewModuleDialer creates a ModuleDialer from the registration client's admin connection.
func NewModuleDialer(ctx *bootstrap.Context, regClient *registration.Client) *grpcx.ModuleDialer {
	if regClient == nil {
		return nil
	}
	return grpcx.NewModuleDialer(ctx.GetLogger(), "paperless", regClient.AdminConn(), "")
}

// RegistrationClientCleanup returns a cleanup function for the registration client.
func RegistrationClientCleanup(client *registration.Client) func() {
	return func() {
		if client != nil {
			_ = client.Close()
		}
	}
}

// RegistrationBundle holds the registration client and logger for lifecycle management.
type RegistrationBundle struct {
	Client *registration.Client
	Logger log.Logger
}

// ForwardMetadataContext builds outgoing gRPC metadata by forwarding relevant headers
// from the incoming context (tenant ID, user ID, username, roles).
func ForwardMetadataContext(ctx context.Context, tenantID uint32) context.Context {
	outMD := grpcMD.New(map[string]string{
		"x-md-global-tenant-id": fmt.Sprintf("%d", tenantID),
	})

	if inMD, ok := grpcMD.FromIncomingContext(ctx); ok {
		for _, key := range []string{"x-md-global-user-id", "x-md-global-username", "x-md-global-roles"} {
			if vals := inMD.Get(key); len(vals) > 0 {
				outMD.Set(key, vals[0])
			}
		}
	}

	return grpcMD.NewOutgoingContext(ctx, outMD)
}

// DetachedMetadataContext extracts gRPC metadata from the incoming request context
// and builds a new outgoing context based on context.Background().
// Use this for async goroutines where the request context will be canceled.
// When no incoming auth metadata exists (e.g. public endpoints), it falls back
// to a system identity so downstream services accept the call.
func DetachedMetadataContext(ctx context.Context, tenantID uint32) context.Context {
	outMD := grpcMD.New(map[string]string{
		"x-md-global-tenant-id": fmt.Sprintf("%d", tenantID),
	})

	if inMD, ok := grpcMD.FromIncomingContext(ctx); ok {
		for _, key := range []string{"x-md-global-user-id", "x-md-global-username", "x-md-global-roles"} {
			if vals := inMD.Get(key); len(vals) > 0 {
				outMD.Set(key, vals[0])
			}
		}
	}
	// When called from unauthenticated contexts (public endpoints), no user metadata
	// is set. Downstream services should authenticate via mTLS client cert instead.

	return grpcMD.NewOutgoingContext(context.Background(), outMD)
}

// DetachedMetadataContextAs builds a detached context impersonating a specific user.
// Use this when the original request has no auth (e.g. public endpoints) but we know
// the user who should own the downstream action (e.g. the signing request creator).
func DetachedMetadataContextAs(tenantID uint32, userID uint32) context.Context {
	outMD := grpcMD.New(map[string]string{
		"x-md-global-tenant-id": fmt.Sprintf("%d", tenantID),
		"x-md-global-user-id":  fmt.Sprintf("%d", userID),
	})
	return grpcMD.NewOutgoingContext(context.Background(), outMD)
}

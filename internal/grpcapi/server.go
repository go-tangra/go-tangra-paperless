// Package grpcapi serves paperless.v1 for other platform services on the Freya
// SPIFFE mTLS channel: the caller is an authenticated service acting for the
// tenant named in the request. Nothing here is proxied by the gateway. The
// tenant comes from the request and the actor identity from the verified SPIFFE
// peer. Document/Category responses never carry extracted content or secrets.
package grpcapi

import (
	"context"
	"errors"
	"regexp"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/go-freya/freya/authn"
	paperlessv1 "github.com/go-freya/freya/services/paperless/api/proto/paperless/v1"
	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/categories"
	"github.com/go-freya/freya/services/paperless/internal/documents"
	"github.com/go-freya/freya/services/paperless/internal/permissions"
	"github.com/go-freya/freya/services/paperless/internal/search"
	"github.com/go-freya/freya/services/paperless/internal/stats"
)

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// callerFunc resolves the SPIFFE identity of a call (overridable in tests).
var callerFunc = func(ctx context.Context) (string, bool) {
	p, ok := authn.FromContext(ctx)
	if !ok {
		return "", false
	}
	return p.ID.String(), true
}

// caller returns the service subjects for the tenant named in the request; the
// tenant must be a uuid and the peer must present a SPIFFE identity.
func caller(ctx context.Context, tenantID string) (authz.Subjects, error) {
	id, ok := callerFunc(ctx)
	if !ok {
		return authz.Subjects{}, status.Error(codes.Unauthenticated, "service identity required")
	}
	if !uuidRE.MatchString(tenantID) {
		return authz.Subjects{}, status.Error(codes.InvalidArgument, "tenant_id must be a uuid")
	}
	return authz.Subjects{TenantID: tenantID, UserID: id, ActorKind: "service"}, nil
}

// grpcError maps a service error to a gRPC status.
func grpcError(err error) error {
	var ve *categories.ValidationError
	switch {
	case errors.As(err, &ve):
		return status.Error(codes.InvalidArgument, "validation_failed")
	case errors.Is(err, authz.ErrForbidden):
		return status.Error(codes.PermissionDenied, "forbidden")
	case errors.Is(err, documents.ErrNotFound),
		errors.Is(err, categories.ErrNotFound),
		errors.Is(err, permissions.ErrNotFound),
		errors.Is(err, authz.ErrNotFound):
		return status.Error(codes.NotFound, "not_found")
	case errors.Is(err, categories.ErrNotEmpty), errors.Is(err, categories.ErrCycle):
		return status.Error(codes.FailedPrecondition, "conflict")
	}
	return status.Error(codes.Unavailable, "temporarily_unavailable")
}

// Deps carries the services the paperless.v1 servers use.
type Deps struct {
	Documents   *documents.Service
	Categories  *categories.Service
	Permissions *permissions.Service
	Search      *search.Service
	Stats       *stats.Service
}

// Register registers the paperless.v1 servers on the gRPC server. Callers are
// authenticated services; nothing here is gateway-proxied.
func Register(gs grpc.ServiceRegistrar, d Deps) {
	if d.Documents != nil {
		paperlessv1.RegisterPaperlessDocumentServiceServer(gs, &DocumentServer{Docs: d.Documents, Searcher: d.Search})
	}
	if d.Categories != nil {
		paperlessv1.RegisterPaperlessCategoryServiceServer(gs, &CategoryServer{Svc: d.Categories})
	}
	if d.Permissions != nil {
		paperlessv1.RegisterPaperlessPermissionServiceServer(gs, &PermissionServer{Svc: d.Permissions})
	}
	if d.Stats != nil {
		paperlessv1.RegisterPaperlessStatisticsServiceServer(gs, &StatisticsServer{Svc: d.Stats})
	}
}

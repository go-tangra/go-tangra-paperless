package stats

import (
	"context"
	"errors"
	"testing"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// errStore injects errors into the reads used by snapshot / SystemWide.
type errStore struct {
	*memstore.Mem
	failDocumentAggregates error
	failListCategories     error
	failTenantIDs          error
}

func (s *errStore) DocumentAggregates(ctx context.Context, tenantID string) ([]store.DocAggregate, error) {
	if s.failDocumentAggregates != nil {
		return nil, s.failDocumentAggregates
	}
	return s.Mem.DocumentAggregates(ctx, tenantID)
}

func (s *errStore) ListCategories(ctx context.Context, tenantID string) ([]store.Category, error) {
	if s.failListCategories != nil {
		return nil, s.failListCategories
	}
	return s.Mem.ListCategories(ctx, tenantID)
}

func (s *errStore) TenantIDs(ctx context.Context) ([]string, error) {
	if s.failTenantIDs != nil {
		return nil, s.failTenantIDs
	}
	return s.Mem.TenantIDs(ctx)
}

func (s *errStore) Atomic(ctx context.Context, tenantID string, fn func(tx repo.Store) error) error {
	return fn(s)
}

func TestTenant_DocumentAggregatesError(t *testing.T) {
	s := &errStore{Mem: memstore.New(), failDocumentAggregates: errors.New("doc boom")}
	if _, err := New(s).Tenant(context.Background(), authz.Subjects{TenantID: "t1"}); err == nil {
		t.Fatal("Tenant should surface DocumentAggregates error")
	}
}

func TestTenant_ListCategoriesError(t *testing.T) {
	s := &errStore{Mem: memstore.New(), failListCategories: errors.New("cat boom")}
	if _, err := New(s).Tenant(context.Background(), authz.Subjects{TenantID: "t1"}); err == nil {
		t.Fatal("Tenant should surface ListCategories error")
	}
}

func TestSystemWide_TenantIDsError(t *testing.T) {
	s := &errStore{Mem: memstore.New(), failTenantIDs: errors.New("tenants boom")}
	admin := authz.Subjects{TenantID: "t1", Roles: []string{"admin"}}
	if _, err := New(s).SystemWide(context.Background(), admin); err == nil {
		t.Fatal("SystemWide should surface TenantIDs error")
	}
}

func TestSystemWide_SnapshotError(t *testing.T) {
	base := memstore.New()
	seedCat(t, base, "t1", "c1", "A", "a")
	s := &errStore{Mem: base, failDocumentAggregates: errors.New("doc boom")}
	admin := authz.Subjects{TenantID: "t1", Roles: []string{"admin"}}
	if _, err := New(s).SystemWide(context.Background(), admin); err == nil {
		t.Fatal("SystemWide should surface a per-tenant snapshot error")
	}
}

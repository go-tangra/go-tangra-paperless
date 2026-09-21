package stats

import (
	"context"
	"errors"
	"testing"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// errStore injects errors into the reads used by snapshot / SystemWide.
type errStore struct {
	*memstore.Mem
	failListDocuments  error
	failListCategories error
	failTenantIDs      error
}

func (s *errStore) ListDocuments(ctx context.Context, tenantID string, f repo.DocFilter) ([]store.Document, error) {
	if s.failListDocuments != nil {
		return nil, s.failListDocuments
	}
	return s.Mem.ListDocuments(ctx, tenantID, f)
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

func TestTenant_ListDocumentsError(t *testing.T) {
	s := &errStore{Mem: memstore.New(), failListDocuments: errors.New("doc boom")}
	if _, err := New(s).Tenant(context.Background(), authz.Subjects{TenantID: "t1"}); err == nil {
		t.Fatal("Tenant should surface ListDocuments error")
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
	s := &errStore{Mem: base, failListDocuments: errors.New("doc boom")}
	admin := authz.Subjects{TenantID: "t1", Roles: []string{"admin"}}
	if _, err := New(s).SystemWide(context.Background(), admin); err == nil {
		t.Fatal("SystemWide should surface a per-tenant snapshot error")
	}
}

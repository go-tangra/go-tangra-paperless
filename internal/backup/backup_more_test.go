package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// errStore injects errors into individual read methods used by Export.
type errStore struct {
	*memstore.Mem
	failListCategories error
	failListDocuments  error
	failListPerms      error
}

func (s *errStore) ListCategories(ctx context.Context, tenantID string) ([]store.Category, error) {
	if s.failListCategories != nil {
		return nil, s.failListCategories
	}
	return s.Mem.ListCategories(ctx, tenantID)
}

func (s *errStore) ListDocuments(ctx context.Context, tenantID string, f repo.DocFilter) ([]store.Document, error) {
	if s.failListDocuments != nil {
		return nil, s.failListDocuments
	}
	return s.Mem.ListDocuments(ctx, tenantID, f)
}

func (s *errStore) ListPermissionsByResource(ctx context.Context, tenantID, rt, rid string) ([]store.PermissionTuple, error) {
	if s.failListPerms != nil {
		return nil, s.failListPerms
	}
	return s.Mem.ListPermissionsByResource(ctx, tenantID, rt, rid)
}

func (s *errStore) Atomic(ctx context.Context, tenantID string, fn func(tx repo.Store) error) error {
	return fn(s)
}

func TestSetClock_StampsExportedAt(t *testing.T) {
	m := memstore.New()
	seedTenant(t, m, "t1")
	svc := New(m)
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	svc.SetClock(func() time.Time { return fixed })
	b, err := svc.Export(context.Background(), authz.Subjects{TenantID: "t1"}, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !b.ExportedAt.Equal(fixed) {
		t.Fatalf("ExportedAt = %v, want %v", b.ExportedAt, fixed)
	}
}

func TestExport_ErrorPaths(t *testing.T) {
	base := memstore.New()
	seedTenant(t, base, "t1")
	subj := authz.Subjects{TenantID: "t1"}

	if _, err := New(&errStore{Mem: base, failListCategories: errors.New("cat boom")}).Export(context.Background(), subj, false); err == nil {
		t.Fatal("Export should fail when ListCategories fails")
	}
	if _, err := New(&errStore{Mem: base, failListDocuments: errors.New("doc boom")}).Export(context.Background(), subj, false); err == nil {
		t.Fatal("Export should fail when ListDocuments fails")
	}
	if _, err := New(&errStore{Mem: base, failListPerms: errors.New("perm boom")}).Export(context.Background(), subj, false); err == nil {
		t.Fatal("Export should fail when ListPermissionsByResource fails")
	}
}

func TestImport_DefaultsEmptyStatusAndGrantedBy(t *testing.T) {
	ctx := context.Background()
	dst := memstore.New()
	subj := authz.Subjects{TenantID: "t2", UserID: "importer"}
	b := Backup{
		SchemaVersion: SchemaVersion,
		Documents: []DocumentExport{
			{ID: "d-empty", Name: "no-status", ObjectKey: "obj/d-empty"}, // Status omitted
		},
		Permissions: []PermissionExport{
			{ID: "p-empty", ResourceType: store.ResourceDocument, ResourceID: "d-empty",
				SubjectType: store.SubjectUser, SubjectID: "u1", Relation: store.RelationViewer}, // GrantedBy omitted
		},
	}
	res, err := New(dst).Import(ctx, subj, b, ModeSkip)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.DocumentsImported != 1 || res.PermissionsImported != 1 {
		t.Fatalf("import result = %+v, want 1 doc / 1 perm", res)
	}
	d, err := dst.GetDocument(ctx, "t2", "d-empty")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if d.Status != store.DocActive {
		t.Errorf("imported empty-status doc = %q, want defaulted to active", d.Status)
	}
	perms, _ := dst.ListPermissionsByResource(ctx, "t2", store.ResourceDocument, "d-empty")
	if len(perms) != 1 || perms[0].GrantedBy != "importer" {
		t.Errorf("perm GrantedBy = %+v, want defaulted to importer", perms)
	}
}

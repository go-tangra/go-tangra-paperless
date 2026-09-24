package stats

import (
	"context"
	"errors"
	"testing"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

func strp(s string) *string { return &s }

func seedDoc(t *testing.T, m *memstore.Mem, tenant, id, catID, status, source, mime, proc string, size int64) {
	t.Helper()
	d := store.Document{
		ID: id, TenantID: tenant, Name: id, ObjectKey: "obj/" + id,
		FileName: id + ".pdf", FileSize: size, MimeType: mime,
		Status: status, Source: source, ProcessingStatus: proc,
	}
	if catID != "" {
		d.CategoryID = strp(catID)
	}
	if err := m.InsertDocument(context.Background(), d); err != nil {
		t.Fatalf("insert doc: %v", err)
	}
}

func seedCat(t *testing.T, m *memstore.Mem, tenant, id, name, path string) {
	t.Helper()
	c := store.Category{ID: id, TenantID: tenant, Name: name, Path: path}
	if err := m.InsertCategory(context.Background(), c); err != nil {
		t.Fatalf("insert cat: %v", err)
	}
}

func TestTenantSnapshot(t *testing.T) {
	m := memstore.New()
	ctx := context.Background()
	const tn = "t1"

	seedCat(t, m, tn, "cat-a", "Invoices", "invoices")
	seedCat(t, m, tn, "cat-b", "Receipts", "receipts")

	// Two active PDFs in cat-a, one archived PNG in cat-b, one uncategorized email PDF.
	seedDoc(t, m, tn, "d1", "cat-a", store.DocActive, store.SourceUpload, "application/pdf", store.ProcCompleted, 100)
	seedDoc(t, m, tn, "d2", "cat-a", store.DocActive, store.SourceUpload, "application/pdf", store.ProcPending, 200)
	seedDoc(t, m, tn, "d3", "cat-b", store.DocArchived, store.SourceUpload, "image/png", store.ProcFailed, 50)
	seedDoc(t, m, tn, "d4", "", store.DocActive, store.SourceEmail, "application/pdf", store.ProcProcessing, 400)

	svc := New(m)
	snap, err := svc.Tenant(ctx, authz.Subjects{TenantID: tn})
	if err != nil {
		t.Fatalf("Tenant: %v", err)
	}

	if snap.DocumentsTotal != 4 {
		t.Errorf("DocumentsTotal = %d, want 4", snap.DocumentsTotal)
	}
	if got, want := snap.DocumentsByStatus[store.DocActive], 3; got != want {
		t.Errorf("DocumentsByStatus[active] = %d, want %d", got, want)
	}
	if got, want := snap.DocumentsByStatus[store.DocArchived], 1; got != want {
		t.Errorf("DocumentsByStatus[archived] = %d, want %d", got, want)
	}
	if got, want := snap.DocumentsBySource[store.SourceUpload], 3; got != want {
		t.Errorf("DocumentsBySource[upload] = %d, want %d", got, want)
	}
	if got, want := snap.DocumentsBySource[store.SourceEmail], 1; got != want {
		t.Errorf("DocumentsBySource[email] = %d, want %d", got, want)
	}
	if got, want := snap.DocumentsByMime["application/pdf"], 3; got != want {
		t.Errorf("DocumentsByMime[pdf] = %d, want %d", got, want)
	}
	if got, want := snap.DocumentsByMime["image/png"], 1; got != want {
		t.Errorf("DocumentsByMime[png] = %d, want %d", got, want)
	}
	if snap.StorageBytes != 750 {
		t.Errorf("StorageBytes = %d, want 750", snap.StorageBytes)
	}
	if got, want := snap.StorageByCategory["cat-a"], int64(300); got != want {
		t.Errorf("StorageByCategory[cat-a] = %d, want %d", got, want)
	}
	if got, want := snap.StorageByCategory["cat-b"], int64(50); got != want {
		t.Errorf("StorageByCategory[cat-b] = %d, want %d", got, want)
	}
	if got, want := snap.StorageByCategory[uncategorized], int64(400); got != want {
		t.Errorf("StorageByCategory[uncategorized] = %d, want %d", got, want)
	}
	if snap.CategoriesTotal != 2 {
		t.Errorf("CategoriesTotal = %d, want 2", snap.CategoriesTotal)
	}
	// Backlog: pending(d2), processing(d4), failed(d3); completed is not backlog.
	if got, want := snap.Backlog[store.ProcPending], 1; got != want {
		t.Errorf("Backlog[pending] = %d, want %d", got, want)
	}
	if got, want := snap.Backlog[store.ProcProcessing], 1; got != want {
		t.Errorf("Backlog[processing] = %d, want %d", got, want)
	}
	if got, want := snap.Backlog[store.ProcFailed], 1; got != want {
		t.Errorf("Backlog[failed] = %d, want %d", got, want)
	}
	if _, ok := snap.Backlog[store.ProcCompleted]; ok {
		t.Errorf("Backlog should not include completed")
	}
}

func TestSystemWideRequiresAdmin(t *testing.T) {
	m := memstore.New()
	svc := New(m)
	_, err := svc.SystemWide(context.Background(), authz.Subjects{TenantID: "t1", Roles: []string{"user"}})
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("SystemWide non-admin err = %v, want ErrForbidden", err)
	}
}

func TestSystemWideAggregatesTwoTenants(t *testing.T) {
	m := memstore.New()
	ctx := context.Background()

	seedCat(t, m, "t1", "c1", "A", "a")
	seedDoc(t, m, "t1", "t1d1", "c1", store.DocActive, store.SourceUpload, "application/pdf", store.ProcPending, 100)
	seedDoc(t, m, "t1", "t1d2", "c1", store.DocActive, store.SourceUpload, "application/pdf", store.ProcCompleted, 200)

	seedCat(t, m, "t2", "c2", "B", "b")
	seedDoc(t, m, "t2", "t2d1", "c2", store.DocArchived, store.SourceEmail, "image/png", store.ProcFailed, 300)

	admin := authz.Subjects{TenantID: "t1", Roles: []string{"admin"}}
	sys, err := New(m).SystemWide(ctx, admin)
	if err != nil {
		t.Fatalf("SystemWide: %v", err)
	}

	if len(sys.PerTenant) != 2 {
		t.Fatalf("PerTenant len = %d, want 2", len(sys.PerTenant))
	}
	if sys.PerTenant[0].TenantID != "t1" || sys.PerTenant[1].TenantID != "t2" {
		t.Errorf("PerTenant order = [%s %s], want [t1 t2]", sys.PerTenant[0].TenantID, sys.PerTenant[1].TenantID)
	}
	if sys.Snapshot.DocumentsTotal != 3 {
		t.Errorf("aggregate DocumentsTotal = %d, want 3", sys.Snapshot.DocumentsTotal)
	}
	if sys.Snapshot.CategoriesTotal != 2 {
		t.Errorf("aggregate CategoriesTotal = %d, want 2", sys.Snapshot.CategoriesTotal)
	}
	if sys.Snapshot.StorageBytes != 600 {
		t.Errorf("aggregate StorageBytes = %d, want 600", sys.Snapshot.StorageBytes)
	}
	if got := sys.Snapshot.DocumentsByStatus[store.DocActive]; got != 2 {
		t.Errorf("aggregate active = %d, want 2", got)
	}
	if got := sys.Snapshot.DocumentsByStatus[store.DocArchived]; got != 1 {
		t.Errorf("aggregate archived = %d, want 1", got)
	}
	if got := sys.Snapshot.Backlog[store.ProcPending]; got != 1 {
		t.Errorf("aggregate backlog pending = %d, want 1", got)
	}
	if got := sys.Snapshot.Backlog[store.ProcFailed]; got != 1 {
		t.Errorf("aggregate backlog failed = %d, want 1", got)
	}
}

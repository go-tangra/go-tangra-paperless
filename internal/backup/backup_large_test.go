package backup

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// seedMany stores n listed documents (some sharing a creation time, so a
// cursor walk must break ties by id) plus one soft-deleted document.
func seedMany(t *testing.T, m *memstore.Mem, tenant string, n int) {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n + 1 {
		d := store.Document{
			ID: fmt.Sprintf("018f0000-0000-7000-8000-%012d", i), TenantID: tenant, Name: fmt.Sprintf("doc-%03d", i),
			ObjectKey: fmt.Sprintf("k/%d", i), FileSize: int64(i), MimeType: "application/pdf",
			Status: store.DocActive, Source: store.SourceUpload, ProcessingStatus: store.ProcCompleted,
			CreatedAt: base.Add(time.Duration(i/3) * time.Second),
		}
		if i == n {
			d.Status = store.DocDeleted
		}
		if err := m.InsertDocument(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
}

// Regression: export and import see every document of the tenant, not the
// first 100.
func TestExportImport_MoreThan100Documents(t *testing.T) {
	const n = 250
	ctx := context.Background()
	subj := authz.Subjects{TenantID: "018f0000-0000-7000-8000-00000000aaaa", UserID: "u1", Roles: []string{"admin"}}
	src := memstore.New()
	seedMany(t, src, subj.TenantID, n)

	b, err := New(src).Export(ctx, subj, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Documents) != n {
		t.Fatalf("exported %d documents, want %d", len(b.Documents), n)
	}
	seen := map[string]bool{}
	for _, d := range b.Documents {
		seen[d.ID] = true
	}
	if len(seen) != n {
		t.Fatalf("exported %d distinct documents, want %d", len(seen), n)
	}

	// Re-importing into the same tenant finds every existing document.
	res, err := New(src).Import(ctx, subj, b, ModeSkip)
	if err != nil {
		t.Fatal(err)
	}
	if res.DocumentsSkipped != n || res.DocumentsImported != 0 {
		t.Fatalf("re-import = %+v", res)
	}

	// Round trip into an empty store.
	dst := memstore.New()
	if res, err = New(dst).Import(ctx, subj, b, ModeSkip); err != nil || res.DocumentsImported != n {
		t.Fatalf("import = %+v %v", res, err)
	}
	if b2, err := New(dst).Export(ctx, subj, false); err != nil || len(b2.Documents) != n {
		t.Fatalf("round trip exported %d %v", len(b2.Documents), err)
	}
}

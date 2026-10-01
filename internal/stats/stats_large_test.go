package stats

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// Regression: statistics count every document of the tenant, not the first 100.
func TestTenant_MoreThan100Documents(t *testing.T) {
	const n = 250
	m := memstore.New()
	tenant := "018f0000-0000-7000-8000-00000000aaaa"
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var bytes int64
	for i := range n + 1 {
		d := store.Document{
			ID: fmt.Sprintf("018f0000-0000-7000-8000-%012d", i), TenantID: tenant, Name: fmt.Sprintf("d%d", i),
			ObjectKey: fmt.Sprintf("k/%d", i), FileSize: int64(i), MimeType: []string{"application/pdf", "text/plain"}[i%2],
			Status: store.DocActive, Source: store.SourceUpload, ProcessingStatus: []string{store.ProcCompleted, store.ProcPending}[i%2],
			CreatedAt: base,
		}
		if i%5 == 0 {
			d.Status = store.DocArchived
		}
		if i == n {
			d.Status = store.DocDeleted // not counted, as before
		} else {
			bytes += d.FileSize
		}
		if err := m.InsertDocument(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := New(m).Tenant(context.Background(), authz.Subjects{TenantID: tenant})
	if err != nil {
		t.Fatal(err)
	}
	if snap.DocumentsTotal != n || snap.StorageBytes != bytes || snap.DocumentsByStatus[store.DocArchived] != 50 ||
		snap.DocumentsByStatus[store.DocActive] != 200 || snap.DocumentsByMime["text/plain"] != 125 ||
		snap.Backlog[store.ProcPending] != 125 || snap.StorageByCategory[uncategorized] != bytes || snap.DocumentsBySource[store.SourceUpload] != n {
		t.Fatalf("snapshot = %+v", snap)
	}
}

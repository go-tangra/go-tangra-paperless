//go:build integration

package repodb_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/backup"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo/repodb"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/stats"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// TestWholeTenantReads is the regression for backup and statistics reading
// only the first 100 documents: with 250 documents (creation-time ties
// included) export, import and the statistics see all of them, and the cursor
// walk breaks ties by id.
func TestWholeTenantReads(t *testing.T) {
	adminDSN, appDSN := startDB(t)
	ctx := context.Background()
	if err := store.Migrate(ctx, adminDSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := store.Open(ctx, appDSN, 4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	db := repodb.New(st)

	const n = 250
	var bytes int64
	for i := range n + 1 {
		d := store.Document{
			ID: store.NewID(), TenantID: tenantA, Name: fmt.Sprintf("doc-%03d", i), ObjectKey: fmt.Sprintf("w/%d", i),
			FileSize: int64(i), MimeType: []string{"application/pdf", "text/plain"}[i%2],
			ProcessingStatus: []string{store.ProcCompleted, store.ProcPending}[i%2],
		}
		if i%5 == 0 {
			d.Status = store.DocArchived
		}
		if i == n {
			d.Status = store.DocDeleted // hidden from backup and statistics, as before
		} else {
			bytes += d.FileSize
		}
		if err := db.InsertDocument(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.InsertDocument(ctx, store.Document{ID: store.NewID(), TenantID: tenantB, Name: "b", ObjectKey: "b/0", FileSize: 1000}); err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	// Groups of three share a creation time: page boundaries fall inside ties.
	if _, err := admin.Exec(ctx, `UPDATE paperless_documents SET created_at = '2026-01-01'::timestamptz + ((split_part(object_key,'/',2)::int / 3) * interval '1 second') WHERE tenant_id = $1`, tenantA); err != nil {
		t.Fatal(err)
	}

	// The cursor walk (gRPC List path) visits every listed document once.
	all, err := repo.AllDocuments(ctx, db, tenantA, repo.DocFilter{})
	if err != nil || len(all) != n {
		t.Fatalf("AllDocuments = %d %v", len(all), err)
	}
	seen := map[string]bool{}
	var cursor string
	for range 100 {
		docs, err := db.ListDocuments(ctx, tenantA, repo.DocFilter{Limit: 7, CursorID: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(docs) == 0 {
			break
		}
		for _, d := range docs {
			if seen[d.ID] {
				t.Fatalf("cursor walk repeated %s", d.ID)
			}
			seen[d.ID] = true
		}
		cursor = docs[len(docs)-1].ID
	}
	if len(seen) != n {
		t.Fatalf("cursor walk saw %d, want %d (no ties skipped)", len(seen), n)
	}

	subj := authz.Subjects{TenantID: tenantA, UserID: "u1", Roles: []string{"admin"}}
	snap, err := stats.New(db).Tenant(ctx, subj)
	if err != nil {
		t.Fatal(err)
	}
	if snap.DocumentsTotal != n || snap.StorageBytes != bytes || snap.DocumentsByStatus[store.DocArchived] != 50 ||
		snap.DocumentsByMime["text/plain"] != 125 || snap.Backlog[store.ProcPending] != 125 || snap.DocumentsBySource[store.SourceUpload] != n {
		t.Fatalf("stats = %+v", snap)
	}

	b, err := backup.New(db).Export(ctx, subj, false)
	if err != nil || len(b.Documents) != n {
		t.Fatalf("export = %d %v", len(b.Documents), err)
	}
	res, err := backup.New(db).Import(ctx, subj, b, backup.ModeSkip)
	if err != nil || res.DocumentsSkipped != n || res.DocumentsImported != 0 {
		t.Fatalf("re-import = %+v %v", res, err)
	}
	// Round trip: empty the tenant, import, export again.
	if _, err := admin.Exec(ctx, `DELETE FROM paperless_documents WHERE tenant_id = $1`, tenantA); err != nil {
		t.Fatal(err)
	}
	if res, err = backup.New(db).Import(ctx, subj, b, backup.ModeSkip); err != nil || res.DocumentsImported != n {
		t.Fatalf("import = %+v %v", res, err)
	}
	if b2, err := backup.New(db).Export(ctx, subj, false); err != nil || len(b2.Documents) != n {
		t.Fatalf("round trip export = %d %v", len(b2.Documents), err)
	}
}

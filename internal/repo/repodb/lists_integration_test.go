//go:build integration

package repodb_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-tangra/go-tangra/v4/listquery"
	"github.com/jackc/pgx/v5"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo/repodb"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// TestDocumentPages covers the documents list contract (go-tangra specs/
// 032-server-side-tables) against the real database: every sort field in both
// directions pages every document exactly once (id tie-breaker), more than 100
// documents are reachable, filters apply before the count, other tenants are
// never counted, a page beyond the end is clamped, and the gRPC limit/cursor
// path (ListDocuments) is unchanged.
func TestDocumentPages(t *testing.T) {
	adminDSN, appDSN := startDB(t)
	ctx := context.Background()
	if err := store.Migrate(ctx, adminDSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	st, err := store.Open(ctx, appDSN, 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(st.Close)
	db := repodb.New(st)

	catID := store.NewID()
	if err := db.InsertCategory(ctx, store.Category{ID: catID, TenantID: tenantA, Name: "c", Path: "c", CreatedBy: "u"}); err != nil {
		t.Fatal(err)
	}
	const n = 130
	mimes := []string{"application/pdf", "Text/Plain", "image/png"}
	procs := []string{store.ProcPending, store.ProcCompleted, store.ProcFailed}
	for i := range n {
		d := store.Document{
			ID: store.NewID(), TenantID: tenantA, Name: fmt.Sprintf("Doc-%02d", i%40), ObjectKey: fmt.Sprintf("a/%d", i),
			FileSize: int64(i % 7), MimeType: mimes[i%3], Source: store.SourceUpload, ProcessingStatus: procs[i%3],
			CreatedBy: "u",
		}
		if i%10 == 0 {
			d.Status = store.DocArchived
		}
		if i%4 == 0 {
			d.CategoryID = &catID
			d.Tags = map[string]string{"year": "2026"}
		}
		if i == 1 {
			d.Status = store.DocDeleted // hidden without an explicit status filter
		}
		if err := db.InsertDocument(ctx, d); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	for i := range 5 {
		if err := db.InsertDocument(ctx, store.Document{ID: store.NewID(), TenantID: tenantB, Name: "b", ObjectKey: fmt.Sprintf("b/%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	// Distinct creation times one minute apart, newest = highest i (the cursor
	// path compares created_at strictly, as it always has).
	admin, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `UPDATE paperless_documents d SET created_at = '2026-01-01'::timestamptz + (split_part(object_key,'/',2)::int * interval '1 minute') WHERE tenant_id = $1`, tenantA); err != nil {
		t.Fatal(err)
	}
	var idx int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM pg_indexes WHERE tablename = 'paperless_documents' AND indexname IN ('documents_tenant_created_id','documents_tenant_lower_name_id')`).Scan(&idx); err != nil || idx != 2 {
		t.Fatalf("list indexes = %d %v", idx, err)
	}
	_ = admin.Close(ctx)

	const listed = n - 1 // the deleted one is hidden
	page := func(f repo.DocFilter, r listquery.Request) ([]store.Document, int, listquery.Request) {
		t.Helper()
		items, total, served, err := db.PageDocuments(ctx, tenantA, f, r)
		if err != nil {
			t.Fatalf("page %+v: %v", r, err)
		}
		return items, total, served
	}

	for sort := range store.DocumentList.Fields {
		for _, dir := range []listquery.Dir{listquery.Asc, listquery.Desc} {
			seen := map[string]int{}
			var prev *store.Document
			for p := 1; p <= 3; p++ {
				items, total, served := page(repo.DocFilter{}, listquery.Request{Page: p, PageSize: 50, Sort: sort, Order: dir})
				if total != listed || served.Page != p {
					t.Fatalf("%s %s page %d: total %d served %+v", sort, dir, p, total, served)
				}
				for i := range items {
					d := items[i]
					seen[d.ID]++
					if prev != nil && !ordered(*prev, d, sort, dir) {
						t.Fatalf("%s %s: %s/%s before %s/%s", sort, dir, prev.Name, prev.ID, d.Name, d.ID)
					}
					prev = &d
				}
			}
			if len(seen) != listed {
				t.Fatalf("%s %s: %d distinct, want %d", sort, dir, len(seen), listed)
			}
			for id, c := range seen {
				if c != 1 {
					t.Fatalf("%s %s: %s seen %d times", sort, dir, id, c)
				}
			}
		}
	}

	// Default: newest first, 25 per page.
	items, total, served := page(repo.DocFilter{}, listquery.Request{})
	if total != listed || len(items) != 25 || served != (listquery.Request{Page: 1, PageSize: 25, Sort: "created_at", Order: listquery.Desc}) || items[0].ObjectKey != "a/129" {
		t.Fatalf("default: total %d items %d served %+v first %s", total, len(items), served, items[0].ObjectKey)
	}

	// Filters apply before the count.
	for name, tc := range map[string]struct {
		f    repo.DocFilter
		want int
	}{
		"archived": {repo.DocFilter{Status: store.DocArchived}, 13},
		"deleted":  {repo.DocFilter{Status: store.DocDeleted}, 1},
		"category": {repo.DocFilter{CategoryID: catID}, 33},
		"tag kv":   {repo.DocFilter{Tag: "year=2026"}, 33},
		"tag key":  {repo.DocFilter{Tag: "year"}, 33},
		"mime":     {repo.DocFilter{MimeType: "image/png"}, 43},
		"proc":     {repo.DocFilter{ProcessingStatus: store.ProcFailed}, 43},
		"none":     {repo.DocFilter{CreatedBy: "nobody"}, 0},
	} {
		items, total, served := page(tc.f, listquery.Request{PageSize: 10})
		wantItems := min(10, tc.want)
		if total != tc.want || len(items) != wantItems || served.Page != 1 {
			t.Fatalf("%s: total %d items %d page %d, want %d", name, total, len(items), served.Page, tc.want)
		}
	}

	// A page beyond the end answers the last page; tenant B never counts.
	items, total, served = page(repo.DocFilter{}, listquery.Request{Page: 50, PageSize: 50})
	if served.Page != 3 || len(items) != listed-100 || total != listed {
		t.Fatalf("clamp: page %d items %d total %d", served.Page, len(items), total)
	}
	if _, tb, _, err := db.PageDocuments(ctx, tenantB, repo.DocFilter{}, listquery.Request{}); err != nil || tb != 5 {
		t.Fatalf("tenant B total %d %v", tb, err)
	}

	// gRPC path unchanged: limit + cursor walks newest first, deleted hidden.
	var cursor string
	walked := 0
	for range 10 {
		docs, err := db.ListDocuments(ctx, tenantA, repo.DocFilter{Limit: 50, CursorID: cursor})
		if err != nil {
			t.Fatal(err)
		}
		if len(docs) == 0 {
			break
		}
		for i := 1; i < len(docs); i++ {
			if !docs[i-1].CreatedAt.After(docs[i].CreatedAt) {
				t.Fatal("cursor path not newest first")
			}
		}
		walked += len(docs)
		cursor = docs[len(docs)-1].ID
	}
	if walked != listed {
		t.Fatalf("cursor walk = %d, want %d", walked, listed)
	}
	if docs, err := db.ListDocuments(ctx, tenantA, repo.DocFilter{}); err != nil || len(docs) != 100 {
		t.Fatalf("cursor default limit = %d %v", len(docs), err)
	}
}

// ordered reports whether a may precede b under sort/dir (SQL semantics: text
// fields case-insensitive, id breaking ties in the same direction).
func ordered(a, b store.Document, sort string, dir listquery.Dir) bool {
	cmp := 0
	switch sort {
	case "name":
		cmp = cmpStr(lower(a.Name), lower(b.Name))
	case "mime_type":
		cmp = cmpStr(lower(a.MimeType), lower(b.MimeType))
	case "status":
		cmp = cmpStr(a.Status, b.Status)
	case "processing_status":
		cmp = cmpStr(a.ProcessingStatus, b.ProcessingStatus)
	case "file_size":
		cmp = cmpInt(a.FileSize, b.FileSize)
	case "created_at":
		cmp = a.CreatedAt.Compare(b.CreatedAt)
	}
	if cmp == 0 {
		cmp = cmpStr(a.ID, b.ID)
	}
	if dir == listquery.Desc {
		cmp = -cmp
	}
	return cmp < 0
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func cmpStr(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

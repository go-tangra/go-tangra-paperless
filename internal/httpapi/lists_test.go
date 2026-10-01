package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// listBody is the list contract response of GET /documents.
type listBody struct {
	Items []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		FileSize    int64  `json:"file_size"`
		Status      string `json:"status"`
		ContentText string `json:"content_text"`
	} `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	Sort     string `json:"sort"`
	Order    string `json:"order"`
}

// seedDocs stores n documents in apiTenant (every 10th archived) plus two in
// another tenant, with distinct creation times and repeating sizes/names so
// sorts have ties the id breaks.
func seedDocs(t *testing.T, f *apiFixture, n int) {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range n {
		status := store.DocActive
		if i%10 == 0 {
			status = store.DocArchived
		}
		d := store.Document{
			ID: fmt.Sprintf("018f0000-0000-7000-8000-%012d", i), TenantID: apiTenant,
			Name: fmt.Sprintf("doc-%02d", i%40), ObjectKey: fmt.Sprintf("k%d", i), FileSize: int64(i % 7),
			MimeType: "application/pdf", Status: status, Source: store.SourceUpload, ProcessingStatus: store.ProcCompleted,
			ContentText: secretText, CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := f.mem.InsertDocument(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 2 {
		if err := f.mem.InsertDocument(context.Background(), store.Document{
			ID: fmt.Sprintf("018f0000-0000-7000-9000-%012d", i), TenantID: "99999999-9999-7999-8999-999999999999",
			Name: "other", ObjectKey: fmt.Sprintf("o%d", i), Status: store.DocActive, CreatedAt: base,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func listDocs(t *testing.T, f *apiFixture, query string) listBody {
	t.Helper()
	w := f.req(t, "GET", p+"/documents"+query, "admin", "")
	if w.Code != 200 {
		t.Fatalf("GET %s = %d %s", query, w.Code, w.Body)
	}
	var b listBody
	if err := json.Unmarshal(w.Body.Bytes(), &b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDocumentsListContract(t *testing.T) {
	f := newAPI(t)
	seedDocs(t, f, 130)

	// Defaults: newest first, 25 per page, total of the tenant's listed documents.
	b := listDocs(t, f, "")
	if b.Total != 130 || b.Page != 1 || b.PageSize != 25 || b.Sort != "created_at" || b.Order != "desc" || len(b.Items) != 25 {
		t.Fatalf("defaults = total %d page %d size %d %s %s items %d", b.Total, b.Page, b.PageSize, b.Sort, b.Order, len(b.Items))
	}
	if b.Items[0].ID != "018f0000-0000-7000-8000-000000000129" {
		t.Fatalf("newest first: %s", b.Items[0].ID)
	}
	if strings.Contains(fmt.Sprint(b.Items), "TOP-SECRET") {
		t.Fatal("content_text leaked into the list")
	}

	// Regression: more than 100 documents are all reachable, each exactly once.
	for _, sort := range []string{"name", "file_size", "mime_type", "status", "processing_status", "created_at"} {
		for _, order := range []string{"asc", "desc"} {
			seen := map[string]int{}
			for page := 1; page <= 3; page++ {
				pb := listDocs(t, f, fmt.Sprintf("?page=%d&page_size=50&sort=%s&order=%s", page, sort, order))
				for _, it := range pb.Items {
					seen[it.ID]++
				}
			}
			if len(seen) != 130 {
				t.Fatalf("%s %s: %d distinct documents, want 130", sort, order, len(seen))
			}
			for id, n := range seen {
				if n != 1 {
					t.Fatalf("%s %s: %s seen %d times", sort, order, id, n)
				}
			}
		}
	}

	// Name sorts case-insensitively ascending by default, id breaking ties.
	b = listDocs(t, f, "?sort=name&page_size=5")
	if b.Order != "asc" || b.Items[0].Name != "doc-00" || b.Items[0].ID >= b.Items[1].ID {
		t.Fatalf("name asc = %+v", b.Items)
	}
	// file_size defaults to descending.
	if b = listDocs(t, f, "?sort=file_size&page_size=1"); b.Order != "desc" || b.Items[0].FileSize != 6 {
		t.Fatalf("file_size default = %s %+v", b.Order, b.Items)
	}

	// Filters apply before the count.
	if b = listDocs(t, f, "?status=archived&page_size=200"); b.Total != 13 || len(b.Items) != 13 {
		t.Fatalf("archived total = %d items %d", b.Total, len(b.Items))
	}

	// A page beyond the end answers the last page.
	if b = listDocs(t, f, "?page=99&page_size=50"); b.Page != 3 || len(b.Items) != 30 {
		t.Fatalf("clamped = page %d items %d", b.Page, len(b.Items))
	}
}

func TestDocumentsListValidation(t *testing.T) {
	f := newAPI(t)
	for _, tc := range []struct{ query, param string }{
		{"?page=0", "page"},
		{"?page=-1", "page"},
		{"?page=abc", "page"},
		{"?page_size=0", "page_size"},
		{"?page_size=201", "page_size"},
		{"?page_size=abc", "page_size"},
		{"?sort=content_text", "sort"},
		{"?sort=lower(name)%3BDROP", "sort"},
		{"?order=up", "order"},
		{"?cursor=x&page=1", "cursor"},
	} {
		w := f.req(t, "GET", p+"/documents"+tc.query, "admin", "")
		if w.Code != 422 {
			t.Fatalf("%s = %d %s", tc.query, w.Code, w.Body)
		}
		var body struct {
			Reason string         `json:"reason"`
			Detail map[string]any `json:"detail"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Reason != "validation_failed" || body.Detail["param"] != tc.param || len(body.Detail) != 1 {
			t.Fatalf("%s = %s", tc.query, w.Body)
		}
		for _, leak := range []string{"content_text", "DROP", "abc", "up\"", "lower("} {
			if strings.Contains(w.Body.String(), leak) {
				t.Fatalf("%s echoes %q: %s", tc.query, leak, w.Body)
			}
		}
	}
	// Unauthenticated callers are refused before anything is listed.
	if w := f.req(t, "GET", p+"/documents?page=1", "", ""); w.Code != 401 {
		t.Fatalf("anonymous = %d", w.Code)
	}
}

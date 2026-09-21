package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/search"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// errStore injects errors into the reads used by Search.
type errStore struct {
	*memstore.Mem
	failSearch error
}

func (s *errStore) SearchDocuments(ctx context.Context, tenantID, query string, accessible []string, all bool, limit int) ([]store.SearchResult, error) {
	if s.failSearch != nil {
		return nil, s.failSearch
	}
	return s.Mem.SearchDocuments(ctx, tenantID, query, accessible, all, limit)
}

func (s *errStore) Atomic(ctx context.Context, tenantID string, fn func(tx repo.Store) error) error {
	return fn(s)
}

func TestSearch_LimitClamp(t *testing.T) {
	ctx := context.Background()
	m := memstore.New()
	az := authz.New(m)
	svc := search.New(m, az)
	admin := authz.Subjects{TenantID: tenant, UserID: "a", Roles: []string{"admin"}, ActorKind: "user"}
	_ = m.InsertDocument(ctx, store.Document{ID: store.NewID(), TenantID: tenant, Name: "n", ContentText: "budget report", Status: store.DocActive})

	// limit <= 0 and limit > 200 both clamp to a sane default (no error).
	if _, err := svc.Search(ctx, admin, "budget", 0); err != nil {
		t.Fatalf("Search limit 0: %v", err)
	}
	if _, err := svc.Search(ctx, admin, "budget", 9999); err != nil {
		t.Fatalf("Search limit 9999: %v", err)
	}
}

func TestSearch_StoreError(t *testing.T) {
	ctx := context.Background()
	s := &errStore{Mem: memstore.New(), failSearch: errors.New("search boom")}
	az := authz.New(s)
	svc := search.New(s, az)
	admin := authz.Subjects{TenantID: tenant, UserID: "a", Roles: []string{"admin"}, ActorKind: "user"}
	if _, err := svc.Search(ctx, admin, "x", 10); err == nil {
		t.Fatal("Search should surface SearchDocuments error")
	}
}

func TestSearch_ReturnsCategoryFields(t *testing.T) {
	ctx := context.Background()
	m := memstore.New()
	az := authz.New(m)
	svc := search.New(m, az)
	admin := authz.Subjects{TenantID: tenant, UserID: "a", Roles: []string{"admin"}, ActorKind: "user"}
	cat := "cat-1"
	_ = m.InsertDocument(ctx, store.Document{
		ID: store.NewID(), TenantID: tenant, Name: "n", CategoryID: &cat, CategoryPath: "/C",
		MimeType: "application/pdf", ContentText: "budget report", Status: store.DocActive,
	})
	hits, err := svc.Search(ctx, admin, "budget", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 || hits[0].CategoryID != cat || hits[0].CategoryPath != "/C" {
		t.Fatalf("hit = %+v, want category fields populated", hits)
	}
}

// FuzzSearchQuery feeds arbitrary query strings to Service.Search (T064:
// search-query-parser fuzz) and asserts it never panics and returns clean
// errors (no error for the in-memory store).
func FuzzSearchQuery(f *testing.F) {
	ctx := context.Background()
	m := memstore.New()
	az := authz.New(m)
	svc := search.New(m, az)
	subj := authz.Subjects{TenantID: tenant, UserID: "u1", Roles: []string{"admin"}, ActorKind: "user"}
	_ = m.InsertDocument(ctx, store.Document{ID: store.NewID(), TenantID: tenant, Name: "budget", ContentText: "quarterly budget report 2026", Status: store.DocActive})

	seeds := []string{
		"",
		"budget",
		"   ",
		"quarterly budget report",
		"'; DROP TABLE paperless_documents;--",
		"%_\\ & | ! : * ( )",
		"日本語のクエリ",
		"a & b | c:* !d",
		string(make([]byte, 4096)), // very long
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, q string) {
		hits, err := svc.Search(ctx, subj, q, 20)
		if err != nil {
			t.Fatalf("Search(%q) returned error: %v", q, err)
		}
		for _, h := range hits {
			if h.ID == "" {
				t.Fatalf("Search(%q) returned a hit with empty ID", q)
			}
		}
	})
}

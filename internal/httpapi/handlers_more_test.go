package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandlersRequireCaller invokes every registered route handler directly with
// no verified identity in context, covering the `subjects(r)` guard (→ 401) that
// begins every handler and is otherwise unreachable behind the auth middleware.
func TestHandlersRequireCaller(t *testing.T) {
	f := buildAPI(t, nil)
	for _, rt := range f.s.Implemented() {
		h := f.s.handlers[rt]
		if h == nil {
			t.Fatalf("no handler for %s", rt)
		}
		path := strings.ReplaceAll(rt.Path, "{id}", "00000000-0000-7000-8000-000000000000")
		r := httptest.NewRequest(rt.Method, "https://localhost"+path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r) // no identity → subjects() fails
		if w.Code != 401 {
			t.Fatalf("%s without caller: got %d, want 401 (%s)", rt, w.Code, w.Body.String())
		}
	}
}

// TestExtraHappyPaths covers route success branches not hit elsewhere:
// system-wide statistics, get-with-content, backup export with no body, and the
// document list with query filters.
func TestExtraHappyPaths(t *testing.T) {
	f := buildAPI(t, nil)

	// Seed a document so list/get have something to return.
	up := f.upload(t, "admin", "Report", "report.txt", secretText)
	if up.Code != 201 {
		t.Fatalf("seed upload: %d %s", up.Code, up.Body.String())
	}
	id, _ := decode(t, up)["id"].(string)

	// System-wide statistics.
	if w := f.req(t, "GET", p+"/statistics", "admin", ""); w.Code != 200 {
		t.Fatalf("system stats: %d %s", w.Code, w.Body.String())
	}

	// Get with include_content=true (admin may read content; still no secret in a
	// metadata-only path unless requested — extraction is not run in this fixture).
	if w := f.req(t, "GET", p+"/documents/"+id+"?include_content=true", "admin", ""); w.Code != 200 {
		t.Fatalf("get include_content: %d %s", w.Code, w.Body.String())
	}

	// List with query filters exercises the DocFilter construction.
	if w := f.req(t, "GET", p+"/documents?status=active&mime_type=application/octet-stream&tag=x&source=upload&processing_status=pending&created_by=z", "admin", ""); w.Code != 200 {
		t.Fatalf("list filtered: %d %s", w.Code, w.Body.String())
	}

	// Backup export with no request body (ContentLength == 0 branch).
	ew := f.reqNoBody(t, "POST", p+"/backup/export", "admin")
	if ew.Code != 200 {
		t.Fatalf("export no body: %d %s", ew.Code, ew.Body.String())
	}
}

// reqNoBody drives a mutating request with a genuinely empty body (ContentLength
// 0), unlike req which sends the provided string.
func (f *apiFixture) reqNoBody(t *testing.T, method, path, tok string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://localhost"+path, nil)
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	r.Header.Set("X-CSRF-Token", "t")
	w := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(w, r)
	return w
}

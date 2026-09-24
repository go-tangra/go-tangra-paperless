package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/stream"
	"github.com/go-tangra/go-tangra/v4/freyatest/testrt"
	"github.com/go-tangra/go-tangra/v4/freyatest/testutil"
)

// TestServerAccessors exercises the read-only introspection helpers.
func TestServerAccessors(t *testing.T) {
	hub := stream.NewHub(stream.NewMemory(), stream.Config{StreamsPerUser: 2, StreamsPerTenant: 2}, nil)
	t.Cleanup(hub.Close)
	f := buildAPI(t, hub) // hub set → the stream route is also wired
	s := f.s
	if len(s.Declared()) == 0 {
		t.Fatal("no declared routes")
	}
	if len(s.Implemented()) == 0 {
		t.Fatal("no implemented routes")
	}
	// With the hub wired, every declared route (including /stream) has a handler.
	if m := s.Missing(); len(m) != 0 {
		t.Fatalf("unexpected missing routes: %v", m)
	}
	if s.Document() == nil {
		t.Fatal("nil document")
	}
	// The stream route is declared; none of the routes are public in this API.
	if s.IsPublic("GET", p+"/categories") {
		t.Fatal("categories should not be public")
	}
}

// TestHandleUndeclaredRoute rejects a route not present in the OpenAPI document.
func TestHandleUndeclaredRoute(t *testing.T) {
	f := buildAPI(t, nil)
	if err := f.s.HandleFunc("GET", "/api/paperless/v1/not-a-route", func(http.ResponseWriter, *http.Request) {}); err == nil {
		t.Fatal("expected error for undeclared route")
	}
	// MustHandle must panic on the same undeclared route.
	defer func() {
		if recover() == nil {
			t.Fatal("MustHandle did not panic on undeclared route")
		}
	}()
	f.s.MustHandle("GET", "/api/paperless/v1/not-a-route", func(http.ResponseWriter, *http.Request) {})
}

// TestPureHelpers covers the small pure helpers.
func TestPureHelpers(t *testing.T) {
	if got := jsonRaw([]byte(`{"a":1}`)); func() bool { _, ok := got.(json.RawMessage); return !ok }() {
		t.Fatalf("valid json should be RawMessage, got %T", got)
	}
	if got := jsonRaw([]byte(`not json`)); got != "not json" {
		t.Fatalf("invalid json fallback: %v", got)
	}
	if def("", "d") != "d" || def("x", "d") != "x" {
		t.Fatal("def wrong")
	}
	if (&Error{Status: 418, Reason: "teapot"}).Error() != "teapot" {
		t.Fatal("Error.Error wrong")
	}
	if got, ok := extensionInt(float64(5)); !ok || got != 5 {
		t.Fatal("extensionInt float64")
	}
	if got, ok := extensionInt(int(7)); !ok || got != 7 {
		t.Fatal("extensionInt int")
	}
	if got, ok := extensionInt(json.Number("9")); !ok || got != 9 {
		t.Fatal("extensionInt json.Number")
	}
	if _, ok := extensionInt("nope"); ok {
		t.Fatal("extensionInt string should fail")
	}
	if !extensionBool(true) || extensionBool("x") {
		t.Fatal("extensionBool wrong")
	}

	r := httptest.NewRequest("GET", "https://localhost/x", nil)
	r.Header.Set(HeaderRequestID, "req-42")
	if RequestID(r) != "req-42" {
		t.Fatal("RequestID wrong")
	}
}

// TestStatusMappings covers the Status() error → (code, reason) table and
// WriteDetail.
func TestStatusMappings(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{ErrValidation, http.StatusUnprocessableEntity},
		{store.ErrNotFound, http.StatusNotFound},
		{store.ErrConflict, http.StatusConflict},
		{errIO{}, http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		if code, _ := Status(c.err); code != c.code {
			t.Fatalf("Status(%v) = %d, want %d", c.err, code, c.code)
		}
	}
	w := httptest.NewRecorder()
	WriteDetail(w, ErrValidation, map[string]any{"field": "name"})
	if w.Code != 422 || !strings.Contains(w.Body.String(), "field") {
		t.Fatalf("WriteDetail: %d %s", w.Code, w.Body.String())
	}
}

type errIO struct{}

func (errIO) Error() string { return "io boom" }

// TestValidateMiddlewareBranches drives the OpenAPI validation middleware
// error branches: 405 method-not-allowed and 413 body-too-large.
func TestValidateMiddlewareBranches(t *testing.T) {
	f := buildAPI(t, nil)

	// PATCH on a declared path → method not allowed.
	r := httptest.NewRequest("PATCH", "https://localhost"+p+"/categories", nil)
	r.Header.Set("Authorization", "Bearer admin")
	r.Header.Set("X-CSRF-Token", "t")
	w := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method not allowed: %d %s", w.Code, w.Body.String())
	}

	// A JSON body larger than the default 64 KiB limit → 413 on a non-binary route.
	big := `{"query":"` + strings.Repeat("z", (64<<10)+100) + `"}`
	bw := f.req(t, "POST", p+"/documents/search", "admin", big)
	if bw.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized json: %d %s", bw.Code, bw.Body.String())
	}
}

// TestAuthenticateNoVerifier: a server built without a verifier refuses every
// protected route with 401.
func TestAuthenticateNoVerifier(t *testing.T) {
	rt := testrt.NewTB(t, testutil.MustCA("example.org"), "paperless")
	s, err := NewHandler(rt) // no WithVerifier
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "https://localhost"+p+"/categories", nil)
	r.Header.Set("Authorization", "Bearer whatever")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no verifier: %d %s", w.Code, w.Body.String())
	}
}

// TestRemoteHandler serves the federated remote build under the UI prefix.
func TestRemoteHandler(t *testing.T) {
	rt := testrt.NewTB(t, testutil.MustCA("example.org"), "paperless")
	dist := fstest.MapFS{
		"index.html":           {Data: []byte("<html></html>")},
		"manifest.webmanifest": {Data: []byte("{}")},
		"assets/app-123.js":    {Data: []byte("console.log(1)")},
	}
	s, err := NewHandler(rt, WithRemote(dist))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	// Hashed asset → long-lived immutable cache.
	aw := httptest.NewRecorder()
	h.ServeHTTP(aw, httptest.NewRequest("GET", "https://localhost"+RemotePrefix+"/assets/app-123.js", nil))
	if aw.Code != 200 || !strings.Contains(aw.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("asset: %d cc=%q", aw.Code, aw.Header().Get("Cache-Control"))
	}

	// A non-hashed top-level file → no-cache.
	iw := httptest.NewRecorder()
	h.ServeHTTP(iw, httptest.NewRequest("GET", "https://localhost"+RemotePrefix+"/manifest.webmanifest", nil))
	if iw.Code != 200 || iw.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("manifest: %d cc=%q", iw.Code, iw.Header().Get("Cache-Control"))
	}

	// A missing file and a directory both 404.
	mw := httptest.NewRecorder()
	h.ServeHTTP(mw, httptest.NewRequest("GET", "https://localhost"+RemotePrefix+"/missing.js", nil))
	if mw.Code != 404 {
		t.Fatalf("missing asset: %d", mw.Code)
	}
	dw := httptest.NewRecorder()
	h.ServeHTTP(dw, httptest.NewRequest("GET", "https://localhost"+RemotePrefix+"/", nil))
	if dw.Code != 404 {
		t.Fatalf("directory: %d", dw.Code)
	}
}

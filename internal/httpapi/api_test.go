package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-freya/freya/internal/testrt"
	"github.com/go-freya/freya/internal/testutil"
	"github.com/go-freya/freya/services/auth/pkg/authclient"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/backup"
	"github.com/go-freya/freya/services/paperless/internal/blob"
	"github.com/go-freya/freya/services/paperless/internal/categories"
	"github.com/go-freya/freya/services/paperless/internal/documents"
	"github.com/go-freya/freya/services/paperless/internal/events"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/permissions"
	"github.com/go-freya/freya/services/paperless/internal/search"
	"github.com/go-freya/freya/services/paperless/internal/stats"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

const (
	apiTenant = "11111111-1111-7111-8111-111111111111"
	apiAdmin  = "22222222-2222-7222-8222-222222222222"
)

// secretText is the document body that must never surface in metadata responses.
const secretText = "TOP-SECRET-CONTENT invoice total 9001"

// fakeVerifier maps a bearer token to a fixed identity.
type fakeVerifier struct {
	ids map[string]authclient.Identity
}

func (f fakeVerifier) Verify(_ context.Context, token string) (authclient.Identity, error) {
	if id, ok := f.ids[token]; ok {
		return id, nil
	}
	return authclient.Identity{}, ErrUnauthenticated
}

type apiFixture struct {
	s   *Server
	mem *memstore.Mem
}

func newAPI(t *testing.T) *apiFixture {
	t.Helper()
	mem := memstore.New()
	rt := testrt.New(t, testutil.MustCA("example.org"), "paperless")
	az := authz.New(mem)
	bs := blob.NewFake()
	pub := events.HubPublisher{} // nil hub → no-op publisher (extraction worker not run here)

	docs := documents.New(mem, az, bs, pub, time.Hour)
	cats := categories.New(mem, az)
	perms := permissions.New(mem, az)
	srch := search.New(mem, az)

	v := fakeVerifier{ids: map[string]authclient.Identity{
		"admin": {UserID: apiAdmin, TenantID: apiTenant, Roles: []string{"admin"}},
	}}
	s, err := NewHandler(rt, WithVerifier(v))
	if err != nil {
		t.Fatal(err)
	}
	s.Register(Deps{
		Documents: docs, Categories: cats, Permissions: perms, Search: srch,
		Stats: stats.New(mem), Backup: backup.New(mem), MaxUpload: 1 << 20,
	})
	return &apiFixture{s: s, mem: mem}
}

// req drives one JSON request as the caller (empty tok => no Authorization).
// Mutating methods carry the CSRF header the OpenAPI validator requires.
func (f *apiFixture) req(t *testing.T, method, path, tok, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "https://localhost"+path, strings.NewReader(body))
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	if method != "GET" {
		r.Header.Set("X-CSRF-Token", "t")
	}
	w := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(w, r)
	return w
}

// upload drives a multipart document upload. The upload route declares only the
// csrf header and a body-limit extension (no requestBody schema), so a multipart
// body satisfies request validation and reaches the handler.
func (f *apiFixture) upload(t *testing.T, tok, name, fileName, content string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteField("name", name); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("POST", "https://localhost"+p+"/documents", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	r.Header.Set("X-CSRF-Token", "t")
	w := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(w, r)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode %q: %v", w.Body.String(), err)
	}
	return m
}

const p = "/api/paperless/v1"

func TestAuthRequired(t *testing.T) {
	f := newAPI(t)
	if w := f.req(t, "GET", p+"/categories", "", ""); w.Code != 401 {
		t.Fatalf("no token: %d %s", w.Code, w.Body.String())
	}
	if w := f.req(t, "GET", p+"/categories", "bad", ""); w.Code != 401 {
		t.Fatalf("bad token: %d", w.Code)
	}
	if w := f.req(t, "GET", p+"/categories", "admin", ""); w.Code != 200 {
		t.Fatalf("admin: %d %s", w.Code, w.Body.String())
	}
}

func TestCategoriesCRUDAndTree(t *testing.T) {
	f := newAPI(t)
	// Create.
	w := f.req(t, "POST", p+"/categories", "admin", `{"name":"Invoices","description":"AP"}`)
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	id, _ := decode(t, w)["id"].(string)
	if id == "" {
		t.Fatalf("no id: %s", w.Body.String())
	}
	// List.
	if w := f.req(t, "GET", p+"/categories", "admin", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "Invoices") {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	// Tree.
	if w := f.req(t, "GET", p+"/categories/tree", "admin", ""); w.Code != 200 || !strings.Contains(w.Body.String(), "tree") {
		t.Fatalf("tree: %d %s", w.Code, w.Body.String())
	}
	// Get one.
	if w := f.req(t, "GET", p+"/categories/"+id, "admin", ""); w.Code != 200 {
		t.Fatalf("get: %d %s", w.Code, w.Body.String())
	}
}

func TestDocumentUploadAndRead(t *testing.T) {
	f := newAPI(t)
	// Multipart upload → 201, and the extracted content is never echoed back.
	w := f.upload(t, "admin", "Invoice 42", "invoice.txt", secretText)
	if w.Code != 201 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "content_text") || strings.Contains(w.Body.String(), secretText) {
		t.Fatalf("content leaked in upload response: %s", w.Body.String())
	}
	docID, _ := decode(t, w)["id"].(string)
	if docID == "" {
		t.Fatalf("no doc id: %s", w.Body.String())
	}

	// The document appears in the list, with no content_text.
	lw := f.req(t, "GET", p+"/documents", "admin", "")
	if lw.Code != 200 || !strings.Contains(lw.Body.String(), "Invoice 42") {
		t.Fatalf("list: %d %s", lw.Code, lw.Body.String())
	}
	if strings.Contains(lw.Body.String(), "content_text") || strings.Contains(lw.Body.String(), secretText) {
		t.Fatalf("content leaked in list: %s", lw.Body.String())
	}

	// Get metadata + a presigned download URL (no content in either).
	gw := f.req(t, "GET", p+"/documents/"+docID, "admin", "")
	if gw.Code != 200 || strings.Contains(gw.Body.String(), secretText) {
		t.Fatalf("get leaked or failed: %d %s", gw.Code, gw.Body.String())
	}
	uw := f.req(t, "GET", p+"/documents/"+docID+"/download-url", "admin", "")
	if uw.Code != 200 || strings.Contains(uw.Body.String(), secretText) {
		t.Fatalf("download-url: %d %s", uw.Code, uw.Body.String())
	}
	if decode(t, uw)["url"] == nil {
		t.Fatalf("no url: %s", uw.Body.String())
	}
}

func TestPermissionsGrantAndCheck(t *testing.T) {
	f := newAPI(t)
	// Grant a viewer relation to a document for a user subject.
	res := "33333333-3333-7333-8333-333333333333"
	usr := "44444444-4444-7444-8444-444444444444"
	gw := f.req(t, "POST", p+"/permissions/grant", "admin",
		`{"resource_type":"`+store.ResourceDocument+`","resource_id":"`+res+`","subject_type":"`+store.SubjectUser+`","subject_id":"`+usr+`","relation":"`+store.RelationViewer+`"}`)
	if gw.Code != 200 {
		t.Fatalf("grant: %d %s", gw.Code, gw.Body.String())
	}
	// List the grants on that resource.
	lw := f.req(t, "GET", p+"/permissions?resource_type="+store.ResourceDocument+"&resource_id="+res, "admin", "")
	if lw.Code != 200 {
		t.Fatalf("list perms: %d %s", lw.Code, lw.Body.String())
	}
	// Check access (admin short-circuits to allowed).
	cw := f.req(t, "POST", p+"/permissions/check", "admin",
		`{"resource_type":"`+store.ResourceDocument+`","resource_id":"`+res+`","permission":"read"}`)
	if cw.Code != 200 {
		t.Fatalf("check: %d %s", cw.Code, cw.Body.String())
	}
	if decode(t, cw)["allowed"] != true {
		t.Fatalf("expected allowed: %s", cw.Body.String())
	}
}

func TestSearch(t *testing.T) {
	f := newAPI(t)
	if w := f.req(t, "POST", p+"/documents/search", "admin", `{"query":"invoice","limit":10}`); w.Code != 200 {
		t.Fatalf("search: %d %s", w.Code, w.Body.String())
	}
}

func TestStatisticsAndBackup(t *testing.T) {
	f := newAPI(t)
	// Upload a document so the tenant has something to count / export.
	if w := f.upload(t, "admin", "Doc", "doc.txt", secretText); w.Code != 201 {
		t.Fatalf("seed upload: %d %s", w.Code, w.Body.String())
	}

	// Tenant statistics.
	if w := f.req(t, "GET", p+"/statistics/tenant", "admin", ""); w.Code != 200 {
		t.Fatalf("tenant stats: %d %s", w.Code, w.Body.String())
	}

	// Export must not carry document content.
	ew := f.req(t, "POST", p+"/backup/export", "admin", `{"include_links":false}`)
	if ew.Code != 200 {
		t.Fatalf("export: %d %s", ew.Code, ew.Body.String())
	}
	if strings.Contains(ew.Body.String(), "content_text") || strings.Contains(ew.Body.String(), secretText) {
		t.Fatalf("export leaked content: %s", ew.Body.String())
	}

	// Round-trip import (skip mode) accepts the exported document.
	imp := `{"mode":"skip","backup":` + ew.Body.String() + `}`
	if w := f.req(t, "POST", p+"/backup/import", "admin", imp); w.Code != 200 {
		t.Fatalf("import: %d %s", w.Code, w.Body.String())
	}
}

package httpapi

import (
	"context"
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
	"github.com/go-freya/freya/services/paperless/internal/stream"
)

// apiUser is a plain (non-admin) caller in the same tenant as apiAdmin.
const apiUser = "55555555-5555-7555-8555-555555555555"

// buildAPI builds a fixture that also authenticates a non-admin "user" token and
// optionally wires an SSE hub. It mirrors newAPI (api_test.go) but is
// configurable so the route/error-mapping tests can exercise forbidden paths and
// the realtime stream.
func buildAPI(t testing.TB, hub *stream.Hub) *apiFixture {
	t.Helper()
	mem := memstore.New()
	rt := testrt.NewTB(t, testutil.MustCA("example.org"), "paperless")
	az := authz.New(mem)
	bs := blob.NewFake()
	pub := events.HubPublisher{}

	docs := documents.New(mem, az, bs, pub, time.Hour)
	cats := categories.New(mem, az)
	perms := permissions.New(mem, az)
	srch := search.New(mem, az)

	v := fakeVerifier{ids: map[string]authclient.Identity{
		"admin": {UserID: apiAdmin, TenantID: apiTenant, Roles: []string{"admin"}},
		"user":  {UserID: apiUser, TenantID: apiTenant},
	}}
	s, err := NewHandler(rt, WithVerifier(v))
	if err != nil {
		t.Fatal(err)
	}
	s.Register(Deps{
		Documents: docs, Categories: cats, Permissions: perms, Search: srch,
		Stats: stats.New(mem), Backup: backup.New(mem), MaxUpload: 1 << 20, Hub: hub,
	})
	return &apiFixture{s: s, mem: mem}
}

// TestDocumentUpdateMoveRemoveBatch drives the document mutation routes that
// api_test.go does not: PUT update, POST move, POST remove (soft and hard), and
// batch-delete, plus the download byte stream.
func TestDocumentUpdateMoveRemoveBatch(t *testing.T) {
	f := buildAPI(t, nil)

	// Seed a category to move into.
	cw := f.req(t, "POST", p+"/categories", "admin", `{"name":"Archive"}`)
	if cw.Code != 201 {
		t.Fatalf("category: %d %s", cw.Code, cw.Body.String())
	}
	catID, _ := decode(t, cw)["id"].(string)

	// Upload two documents.
	up := f.upload(t, "admin", "Doc One", "one.txt", secretText)
	if up.Code != 201 {
		t.Fatalf("upload: %d %s", up.Code, up.Body.String())
	}
	id1, _ := decode(t, up)["id"].(string)
	up2 := f.upload(t, "admin", "Doc Two", "two.txt", "second body")
	id2, _ := decode(t, up2)["id"].(string)

	// PUT update.
	uw := f.req(t, "PUT", p+"/documents/"+id1, "admin", `{"name":"Renamed","description":"d"}`)
	if uw.Code != 200 || decode(t, uw)["name"] != "Renamed" {
		t.Fatalf("update: %d %s", uw.Code, uw.Body.String())
	}
	// Malformed update body → 400.
	if bw := f.req(t, "PUT", p+"/documents/"+id1, "admin", `{`); bw.Code != 400 {
		t.Fatalf("update malformed: %d %s", bw.Code, bw.Body.String())
	}

	// POST move into the category.
	mw := f.req(t, "POST", p+"/documents/"+id1+"/move", "admin", `{"category_id":"`+catID+`"}`)
	if mw.Code != 200 {
		t.Fatalf("move: %d %s", mw.Code, mw.Body.String())
	}

	// Download the raw bytes: the response must be the exact file content and
	// must not carry the JSON content_text metadata key.
	dw := f.req(t, "GET", p+"/documents/"+id1+"/download", "admin", "")
	if dw.Code != 200 {
		t.Fatalf("download: %d %s", dw.Code, dw.Body.String())
	}
	if dw.Body.String() != secretText {
		t.Fatalf("download bytes = %q, want %q", dw.Body.String(), secretText)
	}
	if strings.Contains(dw.Body.String(), "content_text") {
		t.Fatalf("download leaked content_text key: %s", dw.Body.String())
	}
	if ct := dw.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("download missing content-type")
	}

	// Soft remove id1, then hard remove id2 (via ?hard=true).
	if rw := f.req(t, "POST", p+"/documents/"+id1+"/remove", "admin", ""); rw.Code != 204 {
		t.Fatalf("soft remove: %d %s", rw.Code, rw.Body.String())
	}
	if rw := f.req(t, "POST", p+"/documents/"+id2+"/remove?hard=true", "admin", ""); rw.Code != 204 {
		t.Fatalf("hard remove: %d %s", rw.Code, rw.Body.String())
	}

	// Batch-delete a fresh pair.
	b1, _ := decode(t, f.upload(t, "admin", "B1", "b1.txt", "x"))["id"].(string)
	b2, _ := decode(t, f.upload(t, "admin", "B2", "b2.txt", "y"))["id"].(string)
	bd := f.req(t, "POST", p+"/documents/batch-delete", "admin", `{"ids":["`+b1+`","`+b2+`"],"hard":true}`)
	if bd.Code != 200 || !strings.Contains(bd.Body.String(), "results") {
		t.Fatalf("batch-delete: %d %s", bd.Code, bd.Body.String())
	}
}

// TestCategoryErrorMappings drives the failSvc mappings through the category
// routes: 422 validation, 404 not-found, 409 conflict (ErrNotEmpty / ErrCycle),
// and category get/put/remove/move happy paths.
func TestCategoryErrorMappings(t *testing.T) {
	f := buildAPI(t, nil)

	// 422: empty name is a categories.ValidationError.
	if w := f.req(t, "POST", p+"/categories", "admin", `{"name":""}`); w.Code != 422 {
		t.Fatalf("empty name: %d %s", w.Code, w.Body.String())
	}

	// 404: get / update / remove / move a category that does not exist.
	missing := "66666666-6666-7666-8666-666666666666"
	if w := f.req(t, "GET", p+"/categories/"+missing, "admin", ""); w.Code != 404 {
		t.Fatalf("get missing: %d %s", w.Code, w.Body.String())
	}
	if w := f.req(t, "PUT", p+"/categories/"+missing, "admin", `{"name":"x"}`); w.Code != 404 {
		t.Fatalf("update missing: %d %s", w.Code, w.Body.String())
	}
	if w := f.req(t, "POST", p+"/categories/"+missing+"/remove", "admin", ""); w.Code != 404 {
		t.Fatalf("remove missing: %d %s", w.Code, w.Body.String())
	}

	// Build parent + child to exercise conflict + happy paths.
	parent, _ := decode(t, f.req(t, "POST", p+"/categories", "admin", `{"name":"Parent"}`))["id"].(string)
	child, _ := decode(t, f.req(t, "POST", p+"/categories", "admin", `{"name":"Child","parentid":"`+parent+`"}`))["id"].(string)
	if parent == "" || child == "" {
		t.Fatalf("seed parent=%q child=%q", parent, child)
	}

	// PUT update (rename) the child.
	if w := f.req(t, "PUT", p+"/categories/"+child, "admin", `{"name":"Renamed"}`); w.Code != 200 {
		t.Fatalf("update child: %d %s", w.Code, w.Body.String())
	}

	// 409 ErrNotEmpty: delete the non-empty parent without cascade.
	if w := f.req(t, "POST", p+"/categories/"+parent+"/remove", "admin", ""); w.Code != 409 {
		t.Fatalf("delete non-empty: %d %s", w.Code, w.Body.String())
	}

	// 409 ErrCycle: move the parent under its own child.
	if w := f.req(t, "POST", p+"/categories/"+parent+"/move", "admin", `{"parent_id":"`+child+`"}`); w.Code != 409 {
		t.Fatalf("cycle move: %d %s", w.Code, w.Body.String())
	}

	// Happy-path move: reparent child to a root (empty parent_id).
	if w := f.req(t, "POST", p+"/categories/"+child+"/move", "admin", `{"parent_id":""}`); w.Code != 200 {
		t.Fatalf("move to root: %d %s", w.Code, w.Body.String())
	}

	// Cascade delete the (now empty enough) parent succeeds.
	if w := f.req(t, "POST", p+"/categories/"+parent+"/remove?cascade=true", "admin", ""); w.Code != 204 {
		t.Fatalf("cascade delete: %d %s", w.Code, w.Body.String())
	}
}

// TestForbiddenMappings exercises the 403 failSvc branch with a non-admin caller
// that has no grants on an admin-owned resource, and the collection-level create.
func TestForbiddenMappings(t *testing.T) {
	f := buildAPI(t, nil)

	// Admin owns a category; the plain user may not read it → 403.
	cat, _ := decode(t, f.req(t, "POST", p+"/categories", "admin", `{"name":"Secret"}`))["id"].(string)
	if w := f.req(t, "GET", p+"/categories/"+cat, "user", ""); w.Code != 403 {
		t.Fatalf("user get admin category: %d %s", w.Code, w.Body.String())
	}

	// The plain user cannot create a root category (no tenant-wide write) → 403.
	if w := f.req(t, "POST", p+"/categories", "user", `{"name":"Nope"}`); w.Code != 403 {
		t.Fatalf("user create root: %d %s", w.Code, w.Body.String())
	}

	// A category the user cannot see is reported as 404 (existence masked).
	missing := "77777777-7777-7777-8777-777777777777"
	if w := f.req(t, "GET", p+"/categories/"+missing, "user", ""); w.Code != 404 {
		t.Fatalf("user get missing: %d %s", w.Code, w.Body.String())
	}
}

// TestPermissionsRevokeListEffective covers the permission routes api_test.go
// leaves out: revoke, list-for-resource, and effective.
func TestPermissionsRevokeListEffective(t *testing.T) {
	f := buildAPI(t, nil)

	res := "33333333-3333-7333-8333-333333333333"
	usr := "44444444-4444-7444-8444-444444444444"
	gw := f.req(t, "POST", p+"/permissions/grant", "admin",
		`{"resource_type":"`+store.ResourceDocument+`","resource_id":"`+res+`","subject_type":"`+store.SubjectUser+`","subject_id":"`+usr+`","relation":"`+store.RelationViewer+`"}`)
	if gw.Code != 200 {
		t.Fatalf("grant: %d %s", gw.Code, gw.Body.String())
	}
	grantID, _ := decode(t, gw)["id"].(string)
	if grantID == "" {
		t.Fatalf("no grant id: %s", gw.Body.String())
	}

	// Effective permissions on the resource (admin sees owner set).
	ew := f.req(t, "GET", p+"/permissions/effective?resource_type="+store.ResourceDocument+"&resource_id="+res, "admin", "")
	if ew.Code != 200 || !strings.Contains(ew.Body.String(), "permissions") {
		t.Fatalf("effective: %d %s", ew.Code, ew.Body.String())
	}

	// Revoke the grant, then list should still succeed (now empty of that grant).
	rw := f.req(t, "POST", p+"/permissions/revoke", "admin",
		`{"resource_type":"`+store.ResourceDocument+`","resource_id":"`+res+`","id":"`+grantID+`"}`)
	if rw.Code != 200 || decode(t, rw)["revoked"] != true {
		t.Fatalf("revoke: %d %s", rw.Code, rw.Body.String())
	}
	lw := f.req(t, "GET", p+"/permissions?resource_type="+store.ResourceDocument+"&resource_id="+res, "admin", "")
	if lw.Code != 200 {
		t.Fatalf("list after revoke: %d %s", lw.Code, lw.Body.String())
	}
}

// TestBackupBadSchema drives the 422 backup.ErrBadSchema mapping.
func TestBackupBadSchema(t *testing.T) {
	f := buildAPI(t, nil)
	if w := f.req(t, "POST", p+"/backup/import", "admin", `{"mode":"skip","backup":{"schema_version":99}}`); w.Code != 422 {
		t.Fatalf("bad schema import: %d %s", w.Code, w.Body.String())
	}
}

// TestStreamRoute checks the SSE route: 501 when no hub is wired, and a 200
// text/event-stream when a real hub is set on Deps.
func TestStreamRouteNotImplemented(t *testing.T) {
	f := buildAPI(t, nil) // no hub
	w := f.req(t, "GET", p+"/stream", "admin", "")
	if w.Code != 501 {
		t.Fatalf("stream without hub: %d %s", w.Code, w.Body.String())
	}
}

func TestStreamRouteServed(t *testing.T) {
	hub := stream.NewHub(stream.NewMemory(), stream.Config{
		StreamsPerUser: 4, StreamsPerTenant: 8, ReplayWindow: time.Minute,
		ReadBlock: 10 * time.Millisecond, RetryDelay: time.Minute,
	}, nil)
	t.Cleanup(hub.Close)
	f := buildAPI(t, hub)

	// The SSE handler blocks until the request context is done; give it a short
	// deadline so the recorder captures the initial 200 + headers and returns.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	w := f.reqCtx(t, ctx, "GET", p+"/stream", "admin", "")
	if w.Code != 200 {
		t.Fatalf("stream served: %d %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("stream content-type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), "connected") {
		t.Fatalf("stream body missing connected frame: %q", w.Body.String())
	}
}

// reqCtx drives one request as the caller with a caller-supplied context (used
// to bound the long-lived SSE handler).
func (f *apiFixture) reqCtx(t *testing.T, ctx context.Context, method, path, tok, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(ctx, method, "https://localhost"+path, strings.NewReader(body))
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

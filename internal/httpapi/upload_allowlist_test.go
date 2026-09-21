package httpapi

import (
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
)

// buildAPIWithMime builds a fixture whose upload route enforces the given MIME
// allow-list (empty = allow all), so the T064 "disallowed mime" path is
// exercised against the real handler enforcement.
func buildAPIWithMime(t testing.TB, allow []string) *apiFixture {
	t.Helper()
	mem := memstore.New()
	rt := testrt.NewTB(t, testutil.MustCA("example.org"), "paperless")
	az := authz.New(mem)
	bs := blob.NewFake()
	docs := documents.New(mem, az, bs, events.HubPublisher{}, time.Hour)
	v := fakeVerifier{ids: map[string]authclient.Identity{
		"admin": {UserID: apiAdmin, TenantID: apiTenant, Roles: []string{"admin"}},
	}}
	s, err := NewHandler(rt, WithVerifier(v))
	if err != nil {
		t.Fatal(err)
	}
	s.Register(Deps{
		Documents: docs, Categories: categories.New(mem, az), Permissions: permissions.New(mem, az),
		Search: search.New(mem, az), Stats: stats.New(mem), Backup: backup.New(mem),
		MaxUpload: 1 << 20, AllowedMime: allow,
	})
	return &apiFixture{s: s, mem: mem}
}

// TestUploadMimeAllowList: with an allow-list configured, an allowed MIME
// uploads (201) and a disallowed one is refused (415) before any bytes are
// stored. This locks the T064 disallowed-mime enforcement.
func TestUploadMimeAllowList(t *testing.T) {
	f := buildAPIWithMime(t, []string{"application/pdf", "text/plain"})

	// Allowed: text/plain -> 201.
	ct, body := multipartBody(t, "file", "ok.txt", "text/plain", "hello", map[string]string{"name": "ok"})
	if w := f.callUpload(t, ct, body, 0); w.Code != 201 {
		t.Fatalf("allowed mime: want 201, got %d %s", w.Code, w.Body.String())
	}

	// Allowed with a charset parameter -> still 201 (parameters ignored).
	ct, body = multipartBody(t, "file", "p.txt", "text/plain; charset=utf-8", "hi", map[string]string{"name": "p"})
	if w := f.callUpload(t, ct, body, 0); w.Code != 201 {
		t.Fatalf("allowed mime w/ charset: want 201, got %d %s", w.Code, w.Body.String())
	}

	// Disallowed: application/zip -> 415, and nothing persisted.
	ct, body = multipartBody(t, "file", "bad.zip", "application/zip", "PK\x03\x04", map[string]string{"name": "bad"})
	w := f.callUpload(t, ct, body, 0)
	if w.Code != 415 {
		t.Fatalf("disallowed mime: want 415, got %d %s", w.Code, w.Body.String())
	}
}

// TestUploadNoAllowListAllowsAny: an empty allow-list accepts any MIME.
func TestUploadNoAllowListAllowsAny(t *testing.T) {
	f := buildAPIWithMime(t, nil)
	ct, body := multipartBody(t, "file", "any.bin", "application/octet-stream", "\x00\x01", map[string]string{"name": "any"})
	if w := f.callUpload(t, ct, body, 0); w.Code != 201 {
		t.Fatalf("no allow-list: want 201, got %d %s", w.Code, w.Body.String())
	}
}

// TestUploadOversizeRejectedByHeader: a part whose declared size exceeds
// MaxUpload is refused with 413.
func TestUploadOversizeRejectedByHeader(t *testing.T) {
	f := buildAPIWithMime(t, nil)
	big := make([]byte, (1<<20)+16) // one byte class over the 1 MiB MaxUpload
	for i := range big {
		big[i] = 'a'
	}
	ct, body := multipartBody(t, "file", "big.txt", "text/plain", string(big), map[string]string{"name": "big"})
	w := f.callUpload(t, ct, body, 0)
	if w.Code != 413 {
		t.Fatalf("oversize: want 413, got %d %s", w.Code, w.Body.String())
	}
}

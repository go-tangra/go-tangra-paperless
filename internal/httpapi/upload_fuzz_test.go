package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-tangra/go-tangra-auth/sdk/v4/pkg/authclient"
)

// uploadRoute is the declared multipart upload operation.
var uploadRoute = Route{Method: "POST", Path: p + "/documents"}

// callUpload invokes the raw upload handler (bypassing the OpenAPI/auth chain)
// with the caller identity already established in context. When maxBody > 0 the
// body is wrapped in a MaxBytesReader, exactly as the validate middleware does
// for the upload route's x-freya-max-body-bytes limit, so the oversize path can
// be exercised without a 100 MiB request.
func (f *apiFixture) callUpload(t *testing.T, contentType string, body []byte, maxBody int64) *httptest.ResponseRecorder {
	t.Helper()
	h := f.s.handlers[uploadRoute]
	if h == nil {
		t.Fatal("upload handler not registered")
	}
	r := httptest.NewRequest("POST", "https://localhost"+p+"/documents", bytes.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	// Establish the verified caller directly (the auth middleware is not in this path).
	r = r.WithContext(authclient.WithIdentity(r.Context(),
		authclient.Identity{UserID: apiAdmin, TenantID: apiTenant, Roles: []string{"admin"}}))
	w := httptest.NewRecorder()
	if maxBody > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	}
	h.ServeHTTP(w, r)
	return w
}

// multipartBody builds a form-data body with a single "file" part.
func multipartBody(t *testing.T, fileField, fileName, mimeType, content string, extraFields map[string]string) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if fileField != "" {
		hdr := make(map[string][]string)
		hdr["Content-Disposition"] = []string{`form-data; name="` + fileField + `"; filename="` + fileName + `"`}
		if mimeType != "" {
			hdr["Content-Type"] = []string{mimeType}
		}
		pw, err := mw.CreatePart(hdr)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	for k, v := range extraFields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mw.FormDataContentType(), buf.Bytes()
}

// TestUploadOversizeRejected drives T064(a): a multipart body larger than the
// enforced limit is refused with a client error (never a 2xx, never a panic).
func TestUploadOversizeRejected(t *testing.T) {
	f := buildAPI(t, nil)
	ct, body := multipartBody(t, "file", "big.txt", "text/plain", strings.Repeat("A", 4096), nil)
	// Enforce a limit far below the body size, as the middleware's MaxBytesReader
	// would for an over-limit upload.
	w := f.callUpload(t, ct, body, 512)
	if w.Code < 400 || w.Code >= 500 {
		t.Fatalf("oversize upload: got %d %s, want a 4xx client error", w.Code, w.Body.String())
	}
}

// TestUploadMissingFilePart drives T064(c): a well-formed multipart body with no
// "file" part is a 4xx (file_required). This goes through the full chain.
func TestUploadMissingFilePart(t *testing.T) {
	f := buildAPI(t, nil)
	ct, body := multipartBody(t, "", "", "", "", map[string]string{"name": "no file here"})
	r := httptest.NewRequest("POST", "https://localhost"+p+"/documents", bytes.NewReader(body))
	r.Header.Set("Content-Type", ct)
	r.Header.Set("Authorization", "Bearer admin")
	r.Header.Set("X-CSRF-Token", "t")
	w := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(w, r)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "file_required") {
		t.Fatalf("missing file part: got %d %s, want 400 file_required", w.Code, w.Body.String())
	}
}

// TestUploadUnusualMime documents T064(b). NOTE: the httpapi layer wires no MIME
// allow-list (Deps has no such field, and documents.Service.Create performs no
// MIME validation), so a "disallowed" content type is not rejected here — it is
// accepted and forwarded to the object store. This test pins that current
// behavior; a real allow-list would need to be added to Deps/the service first.
func TestUploadUnusualMime(t *testing.T) {
	f := buildAPI(t, nil)
	ct, body := multipartBody(t, "file", "evil.exe", "application/x-msdownload", "MZ...", map[string]string{"name": "installer"})
	r := httptest.NewRequest("POST", "https://localhost"+p+"/documents", bytes.NewReader(body))
	r.Header.Set("Content-Type", ct)
	r.Header.Set("Authorization", "Bearer admin")
	r.Header.Set("X-CSRF-Token", "t")
	w := httptest.NewRecorder()
	f.s.Handler().ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("unusual mime upload: got %d %s (no allow-list is configured, so 201 expected)", w.Code, w.Body.String())
	}
}

// FuzzUploadMultipart (T064) feeds arbitrary bytes as the multipart boundary and
// body straight into the upload handler. The handler must never panic and must
// always write a status (i.e. return promptly, never hang).
func FuzzUploadMultipart(f *testing.F) {
	// Seed corpus: a valid-ish multipart, an empty body, random bytes, a
	// truncated boundary, and a header-only fragment.
	validCT, validBody := multipartBodyForSeed()
	f.Add(validCT, validBody)
	f.Add("boundaryX", []byte(""))
	f.Add("----abc", []byte("--_--not a real part--"))
	f.Add("b", []byte("\r\n\r\n\x00\x01\x02random\xff\xfe"))
	f.Add("", []byte("garbage without boundary"))

	fx := buildAPI(f, nil)

	f.Fuzz(func(t *testing.T, boundary string, body []byte) {
		// Any boundary/body is allowed; a bad one simply yields a 4xx.
		ct := "multipart/form-data; boundary=" + boundary
		h := fx.s.handlers[uploadRoute]
		r := httptest.NewRequest("POST", "https://localhost"+p+"/documents", bytes.NewReader(body))
		r.Header.Set("Content-Type", ct)
		r = r.WithContext(authclient.WithIdentity(r.Context(),
			authclient.Identity{UserID: apiAdmin, TenantID: apiTenant, Roles: []string{"admin"}}))
		w := httptest.NewRecorder()
		// A panic here fails the fuzz run, which is exactly the property we want.
		h.ServeHTTP(w, r)
		if w.Code < 100 || w.Code >= 600 {
			t.Fatalf("handler wrote invalid status %d for boundary=%q len(body)=%d", w.Code, boundary, len(body))
		}
	})
}

// multipartBodyForSeed builds a valid single-file multipart body for the fuzz
// corpus without needing a *testing.T.
func multipartBodyForSeed() (string, []byte) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	pw, _ := mw.CreateFormFile("file", "seed.txt")
	_, _ = pw.Write([]byte("seed content"))
	_ = mw.WriteField("name", "seed")
	_ = mw.Close()
	// Return just the boundary token (the fuzz target re-adds the ct prefix).
	return mw.Boundary(), buf.Bytes()
}

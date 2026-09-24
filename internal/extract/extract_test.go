package extract

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTikaServer mocks the Apache Tika REST contract: PUT /tika returns plain
// text, PUT /meta returns JSON metadata. It records the method and forwarded
// body of the /tika request via the returned pointers.
func newTikaServer(t *testing.T, text, metaJSON string, gotMethod, gotBody *string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		switch r.URL.Path {
		case "/tika":
			if gotMethod != nil {
				*gotMethod = r.Method
			}
			if gotBody != nil {
				*gotBody = string(body)
			}
			w.Header().Set("Content-Type", "text/plain")
			_, _ = io.WriteString(w, text)
		case "/meta":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, metaJSON)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestExtractHappyPath(t *testing.T) {
	var gotMethod, gotBody string
	meta := `{"Content-Type":"application/pdf","dc:title":["Quarterly Report","alt"],"page-count":3}`
	srv := newTikaServer(t, "hello extracted text", meta, &gotMethod, &gotBody)
	defer srv.Close()

	e := New(Config{TikaURL: srv.URL})
	text, m, err := e.Extract(context.Background(), strings.NewReader("RAW-DOC-BYTES"), "application/pdf")
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if text != "hello extracted text" {
		t.Errorf("text = %q, want %q", text, "hello extracted text")
	}
	if got := m["Content-Type"]; got != "application/pdf" {
		t.Errorf("meta[Content-Type] = %q, want application/pdf", got)
	}
	// Array-valued metadata should collapse to its first string.
	if got := m["dc:title"]; got != "Quarterly Report" {
		t.Errorf("meta[dc:title] = %q, want Quarterly Report", got)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("tika method = %q, want PUT", gotMethod)
	}
	if gotBody != "RAW-DOC-BYTES" {
		t.Errorf("forwarded body = %q, want RAW-DOC-BYTES", gotBody)
	}
}

func TestExtractNoOpMode(t *testing.T) {
	e := New(Config{TikaURL: ""}) // no Tika configured
	text, meta, err := e.Extract(context.Background(), strings.NewReader("anything"), "text/plain")
	if err != nil {
		t.Fatalf("no-op Extract returned error: %v", err)
	}
	if text != "" {
		t.Errorf("no-op text = %q, want empty", text)
	}
	if len(meta) != 0 {
		t.Errorf("no-op meta = %v, want empty", meta)
	}
}

func TestExtractTruncatesOversizedResponse(t *testing.T) {
	// /tika returns 10 MiB of junk but MaxBytes caps the read at 1 KiB. The call
	// must not panic and must return at most MaxBytes of text.
	const maxBytes = 1024
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tika":
			junk := strings.Repeat("A", 10<<20)
			_, _ = io.WriteString(w, junk)
		case "/meta":
			// Garbage (non-JSON, oversized) metadata — must surface as an error,
			// never a panic.
			_, _ = io.WriteString(w, strings.Repeat("{", 10<<20))
		}
	}))
	defer srv.Close()

	e := New(Config{TikaURL: srv.URL, MaxBytes: maxBytes})
	text, _, err := e.Extract(context.Background(), strings.NewReader("doc"), "text/plain")
	// Metadata is garbage, so an error is expected; the key assertion is that we
	// did not panic and the text read was bounded.
	if err == nil {
		if int64(len(text)) > maxBytes {
			t.Fatalf("text length %d exceeds MaxBytes %d", len(text), maxBytes)
		}
		return
	}
	if !strings.Contains(err.Error(), "tika") {
		t.Errorf("expected tika-related error, got: %v", err)
	}
}

func TestExtractConnectionErrorSurfaces(t *testing.T) {
	// Point at a server that is already closed => connection error, not a panic.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	e := New(Config{TikaURL: url, Timeout: 200 * time.Millisecond})
	_, _, err := e.Extract(context.Background(), strings.NewReader("doc"), "text/plain")
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestExtractTimeoutSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = io.WriteString(w, "too late")
	}))
	defer srv.Close()

	e := New(Config{TikaURL: srv.URL, Timeout: 50 * time.Millisecond})
	_, _, err := e.Extract(context.Background(), strings.NewReader("doc"), "text/plain")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestCapabilities(t *testing.T) {
	cases := []struct {
		name        string
		cfg         Config
		wantTika    string
		wantGoten   string
		wantConvert bool
	}{
		{"both configured", Config{TikaURL: "http://tika:9998", GotenbergURL: "http://goten:3000"}, "http://tika:9998", "http://goten:3000", true},
		{"tika only", Config{TikaURL: "http://tika:9998"}, "http://tika:9998", "", false},
		{"neither", Config{}, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := New(tc.cfg)
			tika, goten, convert := e.Capabilities()
			if tika != tc.wantTika || goten != tc.wantGoten || convert != tc.wantConvert {
				t.Errorf("Capabilities() = (%q, %q, %v), want (%q, %q, %v)",
					tika, goten, convert, tc.wantTika, tc.wantGoten, tc.wantConvert)
			}
		})
	}
}

func TestConvertNotConfigured(t *testing.T) {
	e := New(Config{TikaURL: "http://tika:9998"}) // no Gotenberg
	_, err := e.Convert(context.Background(), strings.NewReader("doc"), "application/msword")
	if err == nil {
		t.Fatal("expected 'conversion not configured' error, got nil")
	}
	if !strings.Contains(err.Error(), "not configured") {
		t.Errorf("error = %v, want mention of 'not configured'", err)
	}
}

func TestConvertHappyPath(t *testing.T) {
	var gotMethod, gotPath, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		_ = r.ParseMultipartForm(1 << 20)
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, "%PDF-1.7 fake")
	}))
	defer srv.Close()

	e := New(Config{GotenbergURL: srv.URL})
	rc, err := e.Convert(context.Background(), strings.NewReader("word doc bytes"), "application/msword")
	if err != nil {
		t.Fatalf("Convert error: %v", err)
	}
	defer rc.Close()
	pdf, _ := io.ReadAll(rc)
	if !strings.HasPrefix(string(pdf), "%PDF") {
		t.Errorf("pdf = %q, want %%PDF prefix", string(pdf))
	}
	if gotMethod != http.MethodPost {
		t.Errorf("gotenberg method = %q, want POST", gotMethod)
	}
	if gotPath != "/forms/libreoffice/convert" {
		t.Errorf("gotenberg path = %q, want /forms/libreoffice/convert", gotPath)
	}
	if !strings.HasPrefix(gotCT, "multipart/form-data") {
		t.Errorf("gotenberg content-type = %q, want multipart/form-data", gotCT)
	}
}

func TestFakeExtractor(t *testing.T) {
	f := &Fake{Text: "canned", Meta: map[string]string{"k": "v"}}
	text, meta, err := f.Extract(context.Background(), strings.NewReader("x"), "text/plain")
	if err != nil || text != "canned" || meta["k"] != "v" {
		t.Fatalf("Fake.Extract = (%q, %v, %v)", text, meta, err)
	}
	// Nil Meta normalizes to an empty (non-nil) map.
	f2 := &Fake{Text: "t"}
	if _, m, _ := f2.Extract(context.Background(), nil, ""); m == nil {
		t.Error("Fake.Extract nil Meta should normalize to empty map")
	}
}

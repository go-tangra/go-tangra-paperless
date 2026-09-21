package extract

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errReader always fails, to exercise the document-read error path.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom read") }

func TestExtractDocumentReadError(t *testing.T) {
	srv := newTikaServer(t, "text", "{}", nil, nil)
	defer srv.Close()
	e := New(Config{TikaURL: srv.URL})
	if _, _, err := e.Extract(context.Background(), errReader{}, "text/plain"); err == nil ||
		!strings.Contains(err.Error(), "read document") {
		t.Fatalf("expected read-document error, got %v", err)
	}
}

func TestTikaTextNon200WithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/tika" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "tika exploded")
			return
		}
		_, _ = io.WriteString(w, "{}")
	}))
	defer srv.Close()
	e := New(Config{TikaURL: srv.URL})
	_, _, err := e.Extract(context.Background(), strings.NewReader("doc"), "text/plain")
	if err == nil || !strings.Contains(err.Error(), "status 500") || !strings.Contains(err.Error(), "tika exploded") {
		t.Fatalf("expected status-500 error with body, got %v", err)
	}
}

func TestTikaMetaNon200EmptyBody(t *testing.T) {
	// /tika succeeds, /meta returns a non-200 with an empty body: statusError's
	// empty-message branch.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tika":
			_, _ = io.WriteString(w, "ok text")
		case "/meta":
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	e := New(Config{TikaURL: srv.URL})
	_, _, err := e.Extract(context.Background(), strings.NewReader("doc"), "text/plain")
	if err == nil || !strings.Contains(err.Error(), "status 502") {
		t.Fatalf("expected status-502 error, got %v", err)
	}
	if strings.Contains(err.Error(), ": :") {
		t.Fatalf("empty body should produce no trailing message: %v", err)
	}
}

func TestTikaMetaValueTypes(t *testing.T) {
	meta := `{"s":"x","arr":["a","b"],"emptyarr":[],"numarr":[1,2],"num":1.5,"flag":true,"nul":null}`
	srv := newTikaServer(t, "body", meta, nil, nil)
	defer srv.Close()
	e := New(Config{TikaURL: srv.URL})
	_, m, err := e.Extract(context.Background(), strings.NewReader("doc"), "application/pdf")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if m["s"] != "x" {
		t.Errorf("string value: %q", m["s"])
	}
	if m["arr"] != "a" {
		t.Errorf("array first-string: %q", m["arr"])
	}
	if m["num"] != "1.5" {
		t.Errorf("float value: %q", m["num"])
	}
	if m["flag"] != "true" {
		t.Errorf("bool value: %q", m["flag"])
	}
	// empty array, non-string array element and null contribute no entry.
	if _, ok := m["emptyarr"]; ok {
		t.Error("empty array should not yield a key")
	}
	if _, ok := m["numarr"]; ok {
		t.Error("non-string array element should not yield a key")
	}
	if _, ok := m["nul"]; ok {
		t.Error("null should not yield a key")
	}
}

func TestConvertNon200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		_, _ = io.WriteString(w, "cannot convert")
	}))
	defer srv.Close()
	e := New(Config{GotenbergURL: srv.URL})
	_, err := e.Convert(context.Background(), strings.NewReader("doc"), "application/msword")
	if err == nil || !strings.Contains(err.Error(), "gotenberg") || !strings.Contains(err.Error(), "415") {
		t.Fatalf("expected gotenberg 415 error, got %v", err)
	}
}

func TestConvertConnectionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now refuses connections
	e := New(Config{GotenbergURL: url})
	if _, err := e.Convert(context.Background(), strings.NewReader("doc"), "application/msword"); err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestConvertBoundedBodyRespectsMaxBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = io.WriteString(w, strings.Repeat("P", 4096))
	}))
	defer srv.Close()
	e := New(Config{GotenbergURL: srv.URL, MaxBytes: 100})
	rc, err := e.Convert(context.Background(), strings.NewReader("doc"), "application/msword")
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out, _ := io.ReadAll(rc)
	if len(out) != 100 {
		t.Fatalf("bounded body should cap at 100 bytes, got %d", len(out))
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestFakeConvertAndCapabilities(t *testing.T) {
	f := &Fake{Text: "canned"}
	rc, err := f.Convert(context.Background(), strings.NewReader("x"), "text/plain")
	if err != nil {
		t.Fatalf("Fake.Convert: %v", err)
	}
	b, _ := io.ReadAll(rc)
	if string(b) != "canned" {
		t.Fatalf("Fake.Convert body: %q", b)
	}
	tika, goten, convert := f.Capabilities()
	if tika != "fake://tika" || goten != "fake://gotenberg" || !convert {
		t.Fatalf("Fake.Capabilities: %q %q %v", tika, goten, convert)
	}
}

func TestFakeErrPaths(t *testing.T) {
	boom := errors.New("fake boom")
	f := &Fake{Err: boom}
	if _, _, err := f.Extract(context.Background(), nil, ""); !errors.Is(err, boom) {
		t.Fatalf("Fake.Extract err: %v", err)
	}
	if _, err := f.Convert(context.Background(), nil, ""); !errors.Is(err, boom) {
		t.Fatalf("Fake.Convert err: %v", err)
	}
}

// FuzzExtractResponse fuzzes the Tika extraction-response parser (task T064):
// malformed/oversized/empty text and metadata bodies are served back to
// Extract; the parser must never panic and must surface errors cleanly.
func FuzzExtractResponse(f *testing.F) {
	seeds := []string{
		"{}",
		`{"dc:title":["a","b"],"n":1,"b":true}`,
		"",
		"not json at all",
		strings.Repeat("{", 4096),
		`{"k":`,
		`{"k":[]}`,
		`[1,2,3]`,
		"\x00\x01\x02",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Both the text and metadata endpoints echo the fuzz body so the
			// text reader and the JSON metadata decoder are both exercised.
			_, _ = io.WriteString(w, body)
		}))
		defer srv.Close()

		e := New(Config{TikaURL: srv.URL, MaxBytes: 1 << 16})
		// Must not panic. Any error must be a normal error value.
		text, meta, err := e.Extract(context.Background(), strings.NewReader("doc-bytes"), "application/octet-stream")
		if err != nil {
			if text != "" || meta != nil {
				t.Fatalf("on error, text/meta must be zero: %q %v", text, meta)
			}
			return
		}
		// On success the metadata map is non-nil and every value is a string.
		for k, v := range meta {
			_ = k
			_ = v // typed string by construction; presence check only
		}
	})
}

// Package extract pulls plain text and metadata out of stored documents via
// Apache Tika, and optionally renders a preview PDF via Gotenberg, over plain
// stdlib HTTP. Both dependencies are external services treated as UNTRUSTED:
// every response body is bounded by an io.LimitReader and every request carries
// a context deadline. Tika is optional in dev — with no TikaURL configured the
// Extractor becomes a no-op so documents stay stored and searchable by name
// without a running Tika (feature 009, task T014).
package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// Default bounds. MaxBytes caps how much of any (untrusted) response we read;
// Timeout caps how long any single Tika/Gotenberg call may run.
const (
	defaultMaxBytes int64 = 32 << 20 // 32 MiB
	defaultTimeout        = 60 * time.Second
)

// Extractor extracts text/metadata and optionally converts documents to PDF.
type Extractor interface {
	// Extract returns the plain text and embedded metadata of a document (Apache Tika).
	Extract(ctx context.Context, r io.Reader, contentType string) (text string, meta map[string]string, err error)
	// Convert converts a document to PDF for preview (Gotenberg); optional — may be a no-op.
	Convert(ctx context.Context, r io.Reader, contentType string) (pdf io.ReadCloser, err error)
	// Capabilities reports the configured endpoints and whether conversion is available.
	Capabilities() (tikaURL, gotenbergURL string, convert bool)
}

// Config configures a New Extractor. Zero-value URLs disable the corresponding
// service (empty TikaURL => no-op Extract; empty GotenbergURL => Convert errors).
type Config struct {
	TikaURL      string
	GotenbergURL string
	Timeout      time.Duration
	MaxBytes     int64
}

// client is the stdlib-HTTP implementation of Extractor.
type client struct {
	tikaURL      string
	gotenbergURL string
	maxBytes     int64
	timeout      time.Duration
	httpClient   *http.Client
}

// New builds an Extractor from cfg, filling in sane defaults for Timeout and
// MaxBytes. The returned value never itself performs I/O; that happens per-call.
func New(cfg Config) Extractor {
	maxBytes := cfg.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &client{
		tikaURL:      cfg.TikaURL,
		gotenbergURL: cfg.GotenbergURL,
		maxBytes:     maxBytes,
		timeout:      timeout,
		httpClient:   &http.Client{Timeout: timeout},
	}
}

// Capabilities reports the configured endpoints and whether conversion is available.
func (c *client) Capabilities() (tikaURL, gotenbergURL string, convert bool) {
	return c.tikaURL, c.gotenbergURL, c.gotenbergURL != ""
}

// Extract PUTs the document bytes to Tika's /tika (text/plain) and /meta
// (application/json) endpoints. With no TikaURL configured it is a no-op that
// returns empty text, empty metadata and a nil error so the pipeline can run
// without Tika in dev. Document bytes are never logged.
func (c *client) Extract(ctx context.Context, r io.Reader, contentType string) (string, map[string]string, error) {
	if c.tikaURL == "" {
		// No-op mode: keep documents stored/searchable-by-name without Tika.
		return "", map[string]string{}, nil
	}

	// Tika needs the body twice (text + meta) and each request may be retried
	// by the transport, so buffer the (already size-bounded upstream) document.
	body, err := io.ReadAll(io.LimitReader(r, c.maxBytes))
	if err != nil {
		return "", nil, fmt.Errorf("extract: read document: %w", err)
	}

	text, err := c.tikaText(ctx, body, contentType)
	if err != nil {
		return "", nil, err
	}
	meta, err := c.tikaMeta(ctx, body, contentType)
	if err != nil {
		return "", nil, err
	}
	return text, meta, nil
}

// tikaText: PUT {TikaURL}/tika with Accept: text/plain, body = document bytes.
func (c *client) tikaText(ctx context.Context, body []byte, contentType string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.tikaURL+"/tika", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("extract: build tika text request: %w", err)
	}
	req.Header.Set("Accept", "text/plain")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("extract: tika text request: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, c.maxBytes)
	if resp.StatusCode != http.StatusOK {
		return "", statusError("tika", resp.StatusCode, limited)
	}

	out, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("extract: read tika text response: %w", err)
	}
	return string(out), nil
}

// tikaMeta: PUT {TikaURL}/meta with Accept: application/json. Tika returns each
// value as a string or an array of strings; we take the first string of an array.
func (c *client) tikaMeta(ctx context.Context, body []byte, contentType string) (map[string]string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.tikaURL+"/meta", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("extract: build tika meta request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extract: tika meta request: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, c.maxBytes)
	if resp.StatusCode != http.StatusOK {
		return nil, statusError("tika meta", resp.StatusCode, limited)
	}

	// Read into memory (already bounded) before decoding so a truncated/garbage
	// body surfaces as a decode error rather than a panic or unbounded read.
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("extract: read tika meta response: %w", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("extract: decode tika metadata: %w", err)
	}

	meta := make(map[string]string, len(decoded))
	for k, v := range decoded {
		switch val := v.(type) {
		case string:
			meta[k] = val
		case []any:
			if len(val) > 0 {
				if s, ok := val[0].(string); ok {
					meta[k] = s
				}
			}
		case float64:
			meta[k] = fmt.Sprintf("%v", val)
		case bool:
			meta[k] = fmt.Sprintf("%v", val)
		}
	}
	return meta, nil
}

// Convert POSTs the document as a multipart form to Gotenberg's LibreOffice
// endpoint and returns the resulting PDF as a stream. With no GotenbergURL
// configured it returns an error ("conversion not configured"). The returned
// ReadCloser is bounded by MaxBytes; the caller must Close it.
func (c *client) Convert(ctx context.Context, r io.Reader, contentType string) (io.ReadCloser, error) {
	if c.gotenbergURL == "" {
		return nil, fmt.Errorf("extract: conversion not configured")
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("files", "document")
	if err != nil {
		return nil, fmt.Errorf("extract: build convert form: %w", err)
	}
	if _, err := io.Copy(part, io.LimitReader(r, c.maxBytes)); err != nil {
		return nil, fmt.Errorf("extract: copy document into form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("extract: close convert form: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.gotenbergURL+"/forms/libreoffice/convert", &buf)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("extract: build gotenberg request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("extract: gotenberg request: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		err := statusError("gotenberg", resp.StatusCode, io.LimitReader(resp.Body, c.maxBytes))
		resp.Body.Close()
		cancel()
		return nil, err
	}

	// Hand back a bounded, self-closing stream that also releases the context
	// once the caller is done reading the PDF.
	return &boundedBody{
		reader: io.LimitReader(resp.Body, c.maxBytes),
		closer: resp.Body,
		cancel: cancel,
	}, nil
}

// boundedBody caps reads at MaxBytes and, on Close, closes the underlying HTTP
// body and cancels the request context.
type boundedBody struct {
	reader io.Reader
	closer io.Closer
	cancel context.CancelFunc
}

func (b *boundedBody) Read(p []byte) (int, error) { return b.reader.Read(p) }

func (b *boundedBody) Close() error {
	err := b.closer.Close()
	if b.cancel != nil {
		b.cancel()
	}
	return err
}

// statusError builds an error for a non-200 response, including a bounded,
// scrubbed-of-nothing-sensitive snippet of the (already untrusted) body. Only
// the service's own error message is read here — never document bytes.
func statusError(service string, code int, body io.Reader) error {
	snippet, _ := io.ReadAll(io.LimitReader(body, 512))
	msg := string(bytes.TrimSpace(snippet))
	if msg == "" {
		return fmt.Errorf("extract: %s returned status %d", service, code)
	}
	return fmt.Errorf("extract: %s returned status %d: %s", service, code, msg)
}

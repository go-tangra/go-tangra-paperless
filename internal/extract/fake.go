package extract

import (
	"context"
	"io"
	"strings"
)

// Fake is a canned Extractor for tests (used by the extraction worker tests).
// It ignores its inputs and returns the configured Text/Meta/Err. A nil Meta is
// normalized to an empty map so callers need not nil-check.
type Fake struct {
	Text string
	Meta map[string]string
	Err  error
}

var _ Extractor = (*Fake)(nil)

// Extract returns the canned Text/Meta/Err.
func (f *Fake) Extract(ctx context.Context, r io.Reader, contentType string) (string, map[string]string, error) {
	if f.Err != nil {
		return "", nil, f.Err
	}
	meta := f.Meta
	if meta == nil {
		meta = map[string]string{}
	}
	return f.Text, meta, nil
}

// Convert returns a reader over the canned Text as a stand-in PDF, or Err.
func (f *Fake) Convert(ctx context.Context, r io.Reader, contentType string) (io.ReadCloser, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	return io.NopCloser(strings.NewReader(f.Text)), nil
}

// Capabilities reports the fake as convert-capable.
func (f *Fake) Capabilities() (tikaURL, gotenbergURL string, convert bool) {
	return "fake://tika", "fake://gotenberg", true
}

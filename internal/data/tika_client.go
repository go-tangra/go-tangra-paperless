package data

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
)

// TikaClient wraps Apache Tika HTTP API for text and metadata extraction
type TikaClient struct {
	endpoint   string
	httpClient *http.Client
	log        *log.Helper
}

// NewTikaClient creates a new Tika client
func NewTikaClient(ctx *bootstrap.Context) (*TikaClient, func(), error) {
	l := ctx.NewLoggerHelper("tika/data/paperless-service")

	endpoint := getEnvOrDefault("PAPERLESS_TIKA_ENDPOINT", "http://localhost:9998")

	tc := &TikaClient{
		endpoint:   endpoint,
		httpClient: &http.Client{},
		log:        l,
	}

	return tc, func() {
		tc.httpClient.CloseIdleConnections()
	}, nil
}

// ExtractText extracts plain text content from a document via Tika
func (c *TikaClient) ExtractText(ctx context.Context, content []byte, mimeType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.endpoint+"/tika", bytes.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("failed to create tika request: %w", err)
	}

	req.Header.Set("Accept", "text/plain")
	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("tika text extraction failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("tika returned status %d: %s", resp.StatusCode, string(body))
	}

	text, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read tika response: %w", err)
	}

	return string(text), nil
}

// ExtractMetadata extracts metadata from a document via Tika /meta endpoint
func (c *TikaClient) ExtractMetadata(ctx context.Context, content []byte, mimeType string) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.endpoint+"/meta", bytes.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("failed to create tika meta request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if mimeType != "" {
		req.Header.Set("Content-Type", mimeType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tika metadata extraction failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tika meta returned status %d: %s", resp.StatusCode, string(body))
	}

	// Tika returns metadata values as either strings or arrays of strings
	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode tika metadata: %w", err)
	}

	metadata := make(map[string]string, len(raw))
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			metadata[k] = val
		case []interface{}:
			if len(val) > 0 {
				if s, ok := val[0].(string); ok {
					metadata[k] = s
				}
			}
		}
	}

	return metadata, nil
}

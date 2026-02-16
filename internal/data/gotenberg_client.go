package data

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
)

// GotenbergClient wraps the Gotenberg HTTP API for document conversion
type GotenbergClient struct {
	endpoint   string
	httpClient *http.Client
	log        *log.Helper
}

// NewGotenbergClient creates a new Gotenberg client
func NewGotenbergClient(ctx *bootstrap.Context) (*GotenbergClient, func(), error) {
	l := ctx.NewLoggerHelper("gotenberg/data/paperless-service")

	endpoint := getEnvOrDefault("PAPERLESS_GOTENBERG_ENDPOINT", "http://localhost:3000")

	gc := &GotenbergClient{
		endpoint:   endpoint,
		httpClient: &http.Client{},
		log:        l,
	}

	return gc, func() {
		gc.httpClient.CloseIdleConnections()
	}, nil
}

// ConvertToPDF converts a document (DOC/DOCX) to PDF via Gotenberg's LibreOffice endpoint
func (c *GotenbergClient) ConvertToPDF(ctx context.Context, content []byte, fileName string) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("files", fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := part.Write(content); err != nil {
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/forms/libreoffice/convert", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create gotenberg request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gotenberg conversion failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return nil, fmt.Errorf("gotenberg returned status %d (failed to read response body: %w)", resp.StatusCode, readErr)
		}
		return nil, fmt.Errorf("gotenberg returned status %d: %s", resp.StatusCode, string(body))
	}

	pdfContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read gotenberg response: %w", err)
	}

	return pdfContent, nil
}

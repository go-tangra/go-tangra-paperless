package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/go-tangra/go-tangra-paperless/internal/data"
)

const (
	mimeTypePDF  = "application/pdf"
	mimeTypeDOC  = "application/msword"
	mimeTypeDOCX = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

	statusProcessing = "PROCESSING_STATUS_PROCESSING"
	statusCompleted  = "PROCESSING_STATUS_COMPLETED"
	statusFailed     = "PROCESSING_STATUS_FAILED"
	statusSkipped    = "PROCESSING_STATUS_SKIPPED"
)

// DocumentProcessor handles async document content extraction
type DocumentProcessor struct {
	log          *log.Helper
	tika         *data.TikaClient
	gotenberg    *data.GotenbergClient
	documentRepo *data.DocumentRepo
}

// NewDocumentProcessor creates a new DocumentProcessor
func NewDocumentProcessor(
	ctx *bootstrap.Context,
	tika *data.TikaClient,
	gotenberg *data.GotenbergClient,
	documentRepo *data.DocumentRepo,
) *DocumentProcessor {
	return &DocumentProcessor{
		log:          ctx.NewLoggerHelper("paperless/service/document-processor"),
		tika:         tika,
		gotenberg:    gotenberg,
		documentRepo: documentRepo,
	}
}

// ProcessDocument extracts text and metadata from a document asynchronously
func (p *DocumentProcessor) ProcessDocument(ctx context.Context, documentID string, fileContent []byte, mimeType string) {
	p.log.Infof("starting document processing: id=%s, mimeType=%s", documentID, mimeType)

	// Set status to PROCESSING
	if err := p.documentRepo.UpdateProcessingResult(ctx, documentID, "", nil, statusProcessing); err != nil {
		p.log.Errorf("failed to set processing status: %v", err)
		return
	}

	var pdfContent []byte

	switch mimeType {
	case mimeTypePDF:
		pdfContent = fileContent
	case mimeTypeDOC, mimeTypeDOCX:
		// Convert to PDF via Gotenberg first
		// Use an ASCII filename with correct extension — Gotenberg needs the extension to pick the converter
		ext := ".doc"
		if mimeType == mimeTypeDOCX {
			ext = ".docx"
		}
		converted, err := p.gotenberg.ConvertToPDF(ctx, fileContent, "document"+ext)
		if err != nil {
			p.log.Errorf("gotenberg conversion failed for document %s: %v", documentID, err)
			if updateErr := p.documentRepo.UpdateProcessingResult(ctx, documentID, "", nil, statusFailed); updateErr != nil {
				p.log.Errorf("failed to set processing status to FAILED for document %s: %v", documentID, updateErr)
			}
			return
		}
		pdfContent = converted
	default:
		p.log.Infof("skipping unsupported mime type for document %s: %s", documentID, mimeType)
		if updateErr := p.documentRepo.UpdateProcessingResult(ctx, documentID, "", nil, statusSkipped); updateErr != nil {
			p.log.Errorf("failed to set processing status to SKIPPED for document %s: %v", documentID, updateErr)
		}
		return
	}

	// Extract text via Tika
	text, err := p.tika.ExtractText(ctx, pdfContent, mimeTypePDF)
	if err != nil {
		p.log.Errorf("tika text extraction failed for document %s: %v", documentID, err)
		if updateErr := p.documentRepo.UpdateProcessingResult(ctx, documentID, "", nil, statusFailed); updateErr != nil {
			p.log.Errorf("failed to set processing status to FAILED for document %s: %v", documentID, updateErr)
		}
		return
	}

	// Extract metadata via Tika
	metadata, err := p.tika.ExtractMetadata(ctx, pdfContent, mimeTypePDF)
	if err != nil {
		p.log.Warnf("tika metadata extraction failed for document %s: %v", documentID, err)
		// Continue with text only - metadata is not critical
		metadata = nil
	}

	// Update document with extracted content
	if err := p.documentRepo.UpdateProcessingResult(ctx, documentID, text, metadata, statusCompleted); err != nil {
		p.log.Errorf("failed to update processing result for document %s: %v", documentID, err)
		return
	}

	p.log.Infof("document processing completed: id=%s, textLen=%d", documentID, len(text))
}

package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	entCrud "github.com/tx7do/go-crud/entgo"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/go-tangra/go-tangra-paperless/internal/data/ent"
	"github.com/go-tangra/go-tangra-paperless/internal/data/ent/document"
)

// DocumentStats holds aggregated document statistics
type DocumentStats struct {
	TotalCount         int64
	ByStatus           map[string]int64
	BySource           map[string]int64
	ByProcessingStatus map[string]int64
	ByMimeType         map[string]int64
	TotalStorageBytes  int64
}

// StatisticsRepo provides methods for collecting statistics
type StatisticsRepo struct {
	entClient *entCrud.EntClient[*ent.Client]
	log       *log.Helper
}

// NewStatisticsRepo creates a new StatisticsRepo
func NewStatisticsRepo(ctx *bootstrap.Context, entClient *entCrud.EntClient[*ent.Client]) *StatisticsRepo {
	return &StatisticsRepo{
		entClient: entClient,
		log:       ctx.NewLoggerHelper("paperless/statistics/repo"),
	}
}

// GetDocumentStats returns aggregated document statistics
func (r *StatisticsRepo) GetDocumentStats(ctx context.Context) (*DocumentStats, error) {
	stats := &DocumentStats{
		ByStatus:           make(map[string]int64),
		BySource:           make(map[string]int64),
		ByProcessingStatus: make(map[string]int64),
		ByMimeType:         make(map[string]int64),
	}

	client := r.entClient.Client()

	// Total count
	total, err := client.Document.Query().Count(ctx)
	if err != nil {
		return nil, err
	}
	stats.TotalCount = int64(total)

	// Count by status
	statuses := []document.Status{
		document.StatusDOCUMENT_STATUS_UNSPECIFIED,
		document.StatusDOCUMENT_STATUS_ACTIVE,
		document.StatusDOCUMENT_STATUS_ARCHIVED,
		document.StatusDOCUMENT_STATUS_DELETED,
	}
	for _, s := range statuses {
		count, err := client.Document.Query().Where(document.StatusEQ(s)).Count(ctx)
		if err != nil {
			r.log.Warnf("Failed to count documents by status %s: %v", s, err)
			continue
		}
		stats.ByStatus[string(s)] = int64(count)
	}

	// Count by source
	sources := []document.Source{
		document.SourceDOCUMENT_SOURCE_UNSPECIFIED,
		document.SourceDOCUMENT_SOURCE_UPLOAD,
		document.SourceDOCUMENT_SOURCE_EMAIL,
	}
	for _, s := range sources {
		count, err := client.Document.Query().Where(document.SourceEQ(s)).Count(ctx)
		if err != nil {
			r.log.Warnf("Failed to count documents by source %s: %v", s, err)
			continue
		}
		stats.BySource[string(s)] = int64(count)
	}

	// Count by processing status
	processingStatuses := []document.ProcessingStatus{
		document.ProcessingStatusPROCESSING_STATUS_PENDING,
		document.ProcessingStatusPROCESSING_STATUS_PROCESSING,
		document.ProcessingStatusPROCESSING_STATUS_COMPLETED,
		document.ProcessingStatusPROCESSING_STATUS_FAILED,
		document.ProcessingStatusPROCESSING_STATUS_SKIPPED,
	}
	for _, s := range processingStatuses {
		count, err := client.Document.Query().Where(document.ProcessingStatusEQ(s)).Count(ctx)
		if err != nil {
			r.log.Warnf("Failed to count documents by processing status %s: %v", s, err)
			continue
		}
		stats.ByProcessingStatus[string(s)] = int64(count)
	}

	// Sum file sizes for total storage and count by MIME type
	docs, err := client.Document.Query().All(ctx)
	if err != nil {
		r.log.Warnf("Failed to get documents for storage calculation: %v", err)
	} else {
		var totalBytes int64
		mimeTypeCounts := make(map[string]int64)
		for _, d := range docs {
			totalBytes += d.FileSize
			if d.MimeType != "" {
				mimeTypeCounts[d.MimeType]++
			}
		}
		stats.TotalStorageBytes = totalBytes
		stats.ByMimeType = mimeTypeCounts
	}

	return stats, nil
}

// GetDocumentTimeStats returns the count of documents created since the given time
func (r *StatisticsRepo) GetDocumentTimeStats(ctx context.Context, since time.Time) (int64, error) {
	client := r.entClient.Client()

	count, err := client.Document.Query().
		Where(document.CreateTimeGTE(since)).
		Count(ctx)
	if err != nil {
		return 0, err
	}

	return int64(count), nil
}

// GetCategoryStats returns the total count of categories
func (r *StatisticsRepo) GetCategoryStats(ctx context.Context) (int64, error) {
	client := r.entClient.Client()

	count, err := client.Category.Query().Count(ctx)
	if err != nil {
		return 0, err
	}

	return int64(count), nil
}

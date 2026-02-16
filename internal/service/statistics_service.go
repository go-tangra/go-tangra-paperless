package service

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/go-tangra/go-tangra-paperless/internal/data"
	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

// StatisticsService implements the PaperlessStatisticsService gRPC service
type StatisticsService struct {
	paperlessV1.UnimplementedPaperlessStatisticsServiceServer

	statsRepo *data.StatisticsRepo
	log       *log.Helper
}

// NewStatisticsService creates a new StatisticsService
func NewStatisticsService(ctx *bootstrap.Context, statsRepo *data.StatisticsRepo) *StatisticsService {
	return &StatisticsService{
		statsRepo: statsRepo,
		log:       ctx.NewLoggerHelper("paperless/service/statistics"),
	}
}

// GetStatistics returns comprehensive statistics about the Paperless system
func (s *StatisticsService) GetStatistics(ctx context.Context, req *paperlessV1.GetStatisticsRequest) (*paperlessV1.GetStatisticsResponse, error) {
	response := &paperlessV1.GetStatisticsResponse{
		GeneratedAt: timestamppb.Now(),
	}

	// Get document statistics
	docStats, err := s.statsRepo.GetDocumentStats(ctx)
	if err != nil {
		s.log.Errorf("Failed to get document stats: %v", err)
	} else {
		last24Hours := time.Now().Add(-24 * time.Hour)
		last7Days := time.Now().Add(-7 * 24 * time.Hour)

		recentUploads24h, err := s.statsRepo.GetDocumentTimeStats(ctx, last24Hours)
		if err != nil {
			s.log.Warnf("failed to get 24h document time stats: %v", err)
		}
		recentUploads7d, err := s.statsRepo.GetDocumentTimeStats(ctx, last7Days)
		if err != nil {
			s.log.Warnf("failed to get 7d document time stats: %v", err)
		}

		response.Documents = &paperlessV1.DocumentStatistics{
			TotalCount:         docStats.TotalCount,
			ByStatus:           docStats.ByStatus,
			BySource:           docStats.BySource,
			ByProcessingStatus: docStats.ByProcessingStatus,
			ByMimeType:         docStats.ByMimeType,
			TotalStorageBytes:  docStats.TotalStorageBytes,
			RecentUploads_24H:  recentUploads24h,
			RecentUploads_7D:   recentUploads7d,
		}
	}

	// Get category statistics
	categoryCount, err := s.statsRepo.GetCategoryStats(ctx)
	if err != nil {
		s.log.Errorf("Failed to get category stats: %v", err)
	} else {
		response.Categories = &paperlessV1.CategoryStatistics{
			TotalCount: categoryCount,
		}
	}

	return response, nil
}

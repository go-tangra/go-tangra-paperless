package metrics

import (
	"context"

	"github.com/go-tangra/go-tangra-paperless/internal/data"
)

// Seed loads initial gauge values from the database.
// Called once at startup so Prometheus has accurate values from the start.
func (c *Collector) Seed(ctx context.Context, statsRepo *data.StatisticsRepo) {
	c.log.Info("Seeding Prometheus metrics from database...")

	docStats, err := statsRepo.GetDocumentStats(ctx)
	if err != nil {
		c.log.Errorf("Failed to seed document stats: %v", err)
	} else {
		for status, count := range docStats.ByStatus {
			c.DocumentsByStatus.WithLabelValues(status).Set(float64(count))
		}
		for source, count := range docStats.BySource {
			c.DocumentsBySource.WithLabelValues(source).Set(float64(count))
		}
		for ps, count := range docStats.ByProcessingStatus {
			c.DocumentsByProcessingStatus.WithLabelValues(ps).Set(float64(count))
		}
		c.StorageBytesTotal.Set(float64(docStats.TotalStorageBytes))
	}

	categoryCount, err := statsRepo.GetCategoryStats(ctx)
	if err != nil {
		c.log.Errorf("Failed to seed category stats: %v", err)
	} else {
		c.CategoriesTotal.Set(float64(categoryCount))
	}

	c.log.Info("Prometheus metrics seeded successfully")
}

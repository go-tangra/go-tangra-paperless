package metrics

import (
	"context"
	"os"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	commonMetrics "github.com/go-tangra/go-tangra-common/metrics"
)

const namespace = "tangra"
const subsystem = "paperless"

// Collector holds all Prometheus metrics for the paperless module.
type Collector struct {
	log    *log.Helper
	server *commonMetrics.MetricsServer

	// Document metrics
	DocumentsByStatus           *prometheus.GaugeVec
	DocumentsBySource           *prometheus.GaugeVec
	DocumentsByProcessingStatus *prometheus.GaugeVec
	StorageBytesTotal           prometheus.Gauge

	// Category metrics
	CategoriesTotal prometheus.Gauge
}

// NewCollector creates and registers all paperless Prometheus metrics.
func NewCollector(ctx *bootstrap.Context) *Collector {
	c := &Collector{
		log: ctx.NewLoggerHelper("paperless/metrics"),

		DocumentsByStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "documents_by_status",
			Help:      "Number of documents by status.",
		}, []string{"status"}),

		DocumentsBySource: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "documents_by_source",
			Help:      "Number of documents by source.",
		}, []string{"source"}),

		DocumentsByProcessingStatus: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "documents_by_processing_status",
			Help:      "Number of documents by processing status.",
		}, []string{"processing_status"}),

		StorageBytesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "storage_bytes_total",
			Help:      "Total storage used by documents in bytes.",
		}),

		CategoriesTotal: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "categories_total",
			Help:      "Total number of categories.",
		}),
	}

	prometheus.MustRegister(
		c.DocumentsByStatus,
		c.DocumentsBySource,
		c.DocumentsByProcessingStatus,
		c.StorageBytesTotal,
		c.CategoriesTotal,
	)

	addr := os.Getenv("METRICS_ADDR")
	if addr == "" {
		addr = ":9510"
	}
	c.server = commonMetrics.NewMetricsServer(addr, nil, ctx.GetLogger())

	go func() {
		if err := c.server.Start(); err != nil {
			c.log.Errorf("Metrics server failed: %v", err)
		}
	}()

	return c
}

// Stop shuts down the metrics HTTP server.
func (c *Collector) Stop(ctx context.Context) {
	if c.server != nil {
		c.server.Stop(ctx)
	}
}

// --- Document helpers ---

// DocumentCreated increments counters for a newly created document.
func (c *Collector) DocumentCreated(status, source, processingStatus string, fileSize int64) {
	c.DocumentsByStatus.WithLabelValues(status).Inc()
	c.DocumentsBySource.WithLabelValues(source).Inc()
	c.DocumentsByProcessingStatus.WithLabelValues(processingStatus).Inc()
	c.StorageBytesTotal.Add(float64(fileSize))
}

// DocumentDeleted decrements counters for a deleted document.
func (c *Collector) DocumentDeleted(status, source, processingStatus string, fileSize int64) {
	c.DocumentsByStatus.WithLabelValues(status).Dec()
	c.DocumentsBySource.WithLabelValues(source).Dec()
	c.DocumentsByProcessingStatus.WithLabelValues(processingStatus).Dec()
	c.StorageBytesTotal.Sub(float64(fileSize))
}

// DocumentStatusChanged adjusts the status gauge when a document's status changes.
func (c *Collector) DocumentStatusChanged(oldStatus, newStatus string) {
	c.DocumentsByStatus.WithLabelValues(oldStatus).Dec()
	c.DocumentsByStatus.WithLabelValues(newStatus).Inc()
}

// DocumentProcessingStatusChanged adjusts the processing status gauge.
func (c *Collector) DocumentProcessingStatusChanged(oldStatus, newStatus string) {
	c.DocumentsByProcessingStatus.WithLabelValues(oldStatus).Dec()
	c.DocumentsByProcessingStatus.WithLabelValues(newStatus).Inc()
}

// --- Category helpers ---

// CategoryCreated increments the category counter.
func (c *Collector) CategoryCreated() {
	c.CategoriesTotal.Inc()
}

// CategoryDeleted decrements the category counter.
func (c *Collector) CategoryDeleted() {
	c.CategoriesTotal.Dec()
}

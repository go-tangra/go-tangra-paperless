// Package jobs runs the distributed document-extraction worker pool: it claims
// due extraction jobs (one winner each via the store's lease), fetches the
// document bytes from object storage, extracts text and metadata via the
// external extractor, stores the searchable content, and publishes each
// processing-status transition to the platform event bus. Failures retry with
// exponential backoff up to a maximum; a cleanup worker prunes old jobs.
package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/blob"
	"github.com/go-freya/freya/services/paperless/internal/events"
	"github.com/go-freya/freya/services/paperless/internal/extract"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// Config bounds the worker pool.
type Config struct {
	Workers    int
	Interval   time.Duration
	Lease      time.Duration
	JobTimeout time.Duration
	MaxRetries int
	RetryDelay time.Duration
	Backoff    float64
	Cleanup    time.Duration
}

// Service runs the extraction pipeline.
type Service struct {
	st  repo.Store
	bs  blob.Store
	ex  extract.Extractor
	pub events.Publisher
	cfg Config
	now func() time.Time
}

// New builds the service.
func New(st repo.Store, bs blob.Store, ex extract.Extractor, pub events.Publisher, cfg Config) *Service {
	if cfg.Workers <= 0 {
		cfg.Workers = 5
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 5 * time.Second
	}
	if cfg.Backoff < 1 {
		cfg.Backoff = 2
	}
	return &Service{st: st, bs: bs, ex: ex, pub: pub, cfg: cfg, now: time.Now}
}

// SetClock injects the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Run claims and processes due jobs until ctx ends, and prunes old jobs.
func (s *Service) Run(ctx context.Context, log *slog.Logger) {
	if log == nil {
		log = slog.Default()
	}
	var wg sync.WaitGroup
	tick := time.NewTicker(s.cfg.Interval)
	defer tick.Stop()
	cleanup := time.NewTicker(6 * time.Hour)
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-cleanup.C:
			if s.cfg.Cleanup > 0 {
				_, _ = s.st.DeleteJobsOlderThan(ctx, s.now().Add(-s.cfg.Cleanup))
			}
		case <-tick.C:
			due, err := s.st.ClaimDueJobs(ctx, s.now(), s.cfg.Lease, s.cfg.Workers)
			if err != nil {
				log.Warn("paperless: claim failed", "err", err)
				continue
			}
			for _, j := range due {
				wg.Add(1)
				go func(job store.ProcessingJob) {
					defer wg.Done()
					s.process(ctx, log, job)
				}(j)
			}
		}
	}
}

// Once claims and processes one batch synchronously (tests).
func (s *Service) Once(ctx context.Context, log *slog.Logger) int {
	if log == nil {
		log = slog.Default()
	}
	due, err := s.st.ClaimDueJobs(ctx, s.now(), s.cfg.Lease, s.cfg.Workers)
	if err != nil {
		return 0
	}
	for _, j := range due {
		s.process(ctx, log, j)
	}
	return len(due)
}

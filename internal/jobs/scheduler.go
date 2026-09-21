package jobs

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/events"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// process runs one extraction job: fetch bytes -> extract -> store content ->
// mark completed; on failure retry with backoff, else mark the document failed.
func (s *Service) process(ctx context.Context, log *slog.Logger, j store.ProcessingJob) {
	doc, err := s.st.GetDocument(ctx, j.TenantID, j.DocumentID)
	if err != nil {
		// Document gone; drop the job.
		s.finish(ctx, &j, store.ProcCompleted)
		return
	}
	// Mark processing + publish.
	_ = s.st.SetDocumentProcessing(ctx, j.TenantID, j.DocumentID, store.ProcProcessing, doc.ContentText, nil)
	s.publish(ctx, j.TenantID, events.Processing, doc)

	pctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()
	rc, gerr := s.bs.Get(pctx, doc.ObjectKey)
	if gerr != nil {
		s.retryOrFail(ctx, log, &j, doc, "fetch bytes failed")
		return
	}
	text, meta, eerr := s.ex.Extract(pctx, rc, doc.MimeType)
	_ = rc.Close()
	if eerr != nil {
		s.retryOrFail(ctx, log, &j, doc, "extraction failed")
		return
	}
	if serr := s.st.SetDocumentProcessing(ctx, j.TenantID, j.DocumentID, store.ProcCompleted, text, meta); serr != nil {
		s.retryOrFail(ctx, log, &j, doc, "store content failed")
		return
	}
	s.finish(ctx, &j, store.ProcCompleted)
	doc.ProcessingStatus = store.ProcCompleted
	s.publish(ctx, j.TenantID, events.Completed, doc)
}

// retryOrFail re-queues with backoff if retries remain, else marks failed.
func (s *Service) retryOrFail(ctx context.Context, log *slog.Logger, j *store.ProcessingJob, doc store.Document, msg string) {
	if j.RetryCount < j.MaxRetries {
		j.RetryCount++
		j.Status = store.ProcRetrying
		j.LeaseUntil = nil
		delay := time.Duration(float64(s.retryDelay()) * math.Pow(s.cfg.Backoff, float64(j.RetryCount-1)))
		next := s.now().Add(delay)
		j.NextRetryAt = &next
		j.UpdatedAt = s.now()
		_ = s.st.UpdateJob(ctx, *j)
		_ = s.st.SetDocumentProcessing(ctx, j.TenantID, j.DocumentID, store.ProcPending, doc.ContentText, nil)
		if log != nil {
			log.Info("paperless: extraction retry", "doc", j.DocumentID, "attempt", j.RetryCount, "reason", msg)
		}
		return
	}
	s.finish(ctx, j, store.ProcFailed)
	_ = s.st.SetDocumentProcessing(ctx, j.TenantID, j.DocumentID, store.ProcFailed, doc.ContentText, nil)
	doc.ProcessingStatus = store.ProcFailed
	s.publish(ctx, j.TenantID, events.Failed, doc)
}

func (s *Service) finish(ctx context.Context, j *store.ProcessingJob, status string) {
	j.Status = status
	now := s.now()
	j.CompletedAt = &now
	j.LeaseUntil = nil
	j.UpdatedAt = now
	_ = s.st.UpdateJob(ctx, *j)
}

func (s *Service) publish(ctx context.Context, tenantID, eventType string, doc store.Document) {
	if s.pub == nil {
		return
	}
	s.pub.Publish(ctx, tenantID, eventType, events.DocumentPayload(doc.ID, doc.ProcessingStatus, doc.Name))
}

func (s *Service) timeout() time.Duration {
	if s.cfg.JobTimeout > 0 {
		return s.cfg.JobTimeout
	}
	return 5 * time.Minute
}

func (s *Service) retryDelay() time.Duration {
	if s.cfg.RetryDelay > 0 {
		return s.cfg.RetryDelay
	}
	return time.Second
}

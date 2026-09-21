package jobs_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/blob"
	"github.com/go-freya/freya/services/paperless/internal/events"
	"github.com/go-freya/freya/services/paperless/internal/extract"
	"github.com/go-freya/freya/services/paperless/internal/jobs"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// jobStore wraps the in-memory store to inject failures into ClaimDueJobs and
// SetDocumentProcessing (the completed transition).
type jobStore struct {
	*memstore.Mem
	failClaim          error
	failSetOnCompleted error
}

func (s *jobStore) ClaimDueJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]store.ProcessingJob, error) {
	if s.failClaim != nil {
		return nil, s.failClaim
	}
	return s.Mem.ClaimDueJobs(ctx, now, lease, limit)
}

func (s *jobStore) SetDocumentProcessing(ctx context.Context, tenantID, id, status, content string, meta map[string]string) error {
	if s.failSetOnCompleted != nil && status == store.ProcCompleted {
		return s.failSetOnCompleted
	}
	return s.Mem.SetDocumentProcessing(ctx, tenantID, id, status, content, meta)
}

func (s *jobStore) Atomic(ctx context.Context, tenantID string, fn func(tx repo.Store) error) error {
	return fn(s)
}

func TestNew_AppliesDefaults(t *testing.T) {
	m := memstore.New()
	// Zero config exercises the default Workers/Interval/Backoff branches.
	svc := jobs.New(m, blob.NewFake(), &extract.Fake{}, nil, jobs.Config{})
	if n := svc.Once(context.Background(), nil); n != 0 {
		t.Fatalf("Once with no jobs = %d, want 0", n)
	}
}

func TestOnce_ClaimError(t *testing.T) {
	js := &jobStore{Mem: memstore.New(), failClaim: errors.New("claim boom")}
	svc := jobs.New(js, blob.NewFake(), &extract.Fake{}, nil, jobs.Config{Workers: 1, Lease: time.Minute})
	if n := svc.Once(context.Background(), nil); n != 0 {
		t.Fatalf("Once with claim error = %d, want 0", n)
	}
}

func TestProcess_DocumentGone(t *testing.T) {
	m := memstore.New()
	// A job whose document does not exist: the job is dropped (completed) cleanly.
	jobID := store.NewID()
	if err := m.InsertJob(context.Background(), store.ProcessingJob{
		ID: jobID, TenantID: tenant, DocumentID: store.NewID(), Status: store.ProcPending, MaxRetries: 1,
	}); err != nil {
		t.Fatalf("InsertJob: %v", err)
	}
	svc := jobs.New(m, blob.NewFake(), &extract.Fake{}, nil, jobs.Config{Workers: 1, Lease: time.Minute})
	if n := svc.Once(context.Background(), nil); n != 1 {
		t.Fatalf("processed %d, want 1", n)
	}
	j, _ := m.GetJob(context.Background(), tenant, jobID)
	if j.Status != store.ProcCompleted {
		t.Fatalf("job status = %q, want completed (dropped)", j.Status)
	}
}

func TestProcess_StoreContentFailureMarksFailed(t *testing.T) {
	js := &jobStore{Mem: memstore.New(), failSetOnCompleted: errors.New("store boom")}
	bs := blob.NewFake()
	ctx := context.Background()
	docID, jobID := store.NewID(), store.NewID()
	key := "tenants/" + tenant + "/documents/" + docID
	_, _ = bs.Put(ctx, key, bytes.NewReader([]byte("body")), 4, "text/plain")
	_ = js.Mem.InsertDocument(ctx, store.Document{ID: docID, TenantID: tenant, Name: "d", ObjectKey: key, MimeType: "text/plain", Status: store.DocActive, ProcessingStatus: store.ProcPending})
	// MaxRetries 0 so a store failure fails the document immediately.
	_ = js.Mem.InsertJob(ctx, store.ProcessingJob{ID: jobID, TenantID: tenant, DocumentID: docID, Status: store.ProcPending, MaxRetries: 0})

	svc := jobs.New(js, bs, &extract.Fake{Text: "body"}, nil, jobs.Config{Workers: 1, Lease: time.Minute, RetryDelay: time.Millisecond})
	svc.Once(ctx, nil)

	d, _ := js.Mem.GetDocument(ctx, tenant, docID)
	if d.ProcessingStatus != store.ProcFailed {
		t.Fatalf("doc status = %q, want failed after store-content failure", d.ProcessingStatus)
	}
}

func TestProcess_RetryWithDefaultDelay(t *testing.T) {
	m := memstore.New()
	bs := blob.NewFake()
	ctx := context.Background()
	docID, jobID := store.NewID(), store.NewID()
	key := "tenants/" + tenant + "/documents/" + docID
	_, _ = bs.Put(ctx, key, bytes.NewReader([]byte("body")), 4, "text/plain")
	_ = m.InsertDocument(ctx, store.Document{ID: docID, TenantID: tenant, Name: "d", ObjectKey: key, MimeType: "text/plain", Status: store.DocActive, ProcessingStatus: store.ProcPending})
	_ = m.InsertJob(ctx, store.ProcessingJob{ID: jobID, TenantID: tenant, DocumentID: docID, Status: store.ProcPending, MaxRetries: 2})

	// Extractor always fails; RetryDelay 0 exercises the default-delay branch.
	svc := jobs.New(m, bs, &extract.Fake{Err: errors.New("tika down")}, events.HubPublisher{}, jobs.Config{Workers: 1, Lease: time.Minute, MaxRetries: 2})
	clk := time.Now()
	svc.SetClock(func() time.Time { return clk })
	svc.Once(ctx, nil)

	j, _ := m.GetJob(ctx, tenant, jobID)
	if j.Status != store.ProcRetrying || j.NextRetryAt == nil {
		t.Fatalf("job after first failure = %q next=%v, want retrying with a next-retry time", j.Status, j.NextRetryAt)
	}
	if j.RetryCount != 1 {
		t.Fatalf("retry count = %d, want 1", j.RetryCount)
	}
}

func TestProcess_SuccessWithPublisherAndTimeout(t *testing.T) {
	m := memstore.New()
	bs := blob.NewFake()
	ctx := context.Background()
	docID, jobID := store.NewID(), store.NewID()
	key := "tenants/" + tenant + "/documents/" + docID
	_, _ = bs.Put(ctx, key, bytes.NewReader([]byte("hello world")), 11, "text/plain")
	_ = m.InsertDocument(ctx, store.Document{ID: docID, TenantID: tenant, Name: "d", ObjectKey: key, MimeType: "text/plain", Status: store.DocActive, ProcessingStatus: store.ProcPending})
	_ = m.InsertJob(ctx, store.ProcessingJob{ID: jobID, TenantID: tenant, DocumentID: docID, Status: store.ProcPending, MaxRetries: 1})

	// Non-nil publisher (nil hub) + explicit JobTimeout exercise those branches.
	svc := jobs.New(m, bs, &extract.Fake{Text: "hello world", Meta: map[string]string{"pages": "1"}}, events.HubPublisher{}, jobs.Config{Workers: 1, Lease: time.Minute, JobTimeout: time.Minute})
	if n := svc.Once(ctx, nil); n != 1 {
		t.Fatalf("processed %d, want 1", n)
	}
	d, _ := m.GetDocument(ctx, tenant, docID)
	if d.ProcessingStatus != store.ProcCompleted || d.ContentText != "hello world" {
		t.Fatalf("doc after success = %q content=%q", d.ProcessingStatus, d.ContentText)
	}
}

func TestRun_ProcessesThenStops(t *testing.T) {
	m := memstore.New()
	bs := blob.NewFake()
	ctx := context.Background()
	docID, jobID := store.NewID(), store.NewID()
	key := "tenants/" + tenant + "/documents/" + docID
	_, _ = bs.Put(ctx, key, bytes.NewReader([]byte("run bytes")), 9, "text/plain")
	_ = m.InsertDocument(ctx, store.Document{ID: docID, TenantID: tenant, Name: "d", ObjectKey: key, MimeType: "text/plain", Status: store.DocActive, ProcessingStatus: store.ProcPending})
	_ = m.InsertJob(ctx, store.ProcessingJob{ID: jobID, TenantID: tenant, DocumentID: docID, Status: store.ProcPending, MaxRetries: 1})

	svc := jobs.New(m, bs, &extract.Fake{Text: "run bytes"}, nil, jobs.Config{Workers: 2, Interval: time.Millisecond, Lease: time.Minute, Cleanup: time.Hour})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		svc.Run(runCtx, nil)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		d, _ := m.GetDocument(ctx, tenant, docID)
		if d.ProcessingStatus == store.ProcCompleted {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}

	d, _ := m.GetDocument(ctx, tenant, docID)
	if d.ProcessingStatus != store.ProcCompleted {
		t.Fatalf("doc after Run = %q, want completed", d.ProcessingStatus)
	}
}

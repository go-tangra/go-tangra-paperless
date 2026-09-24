package jobs_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/blob"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/extract"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/jobs"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

const tenant = "11111111-1111-1111-1111-111111111111"

func seed(t *testing.T, m *memstore.Mem, bs blob.Store) (docID, jobID string) {
	ctx := context.Background()
	docID, jobID = store.NewID(), store.NewID()
	key := "tenants/" + tenant + "/documents/" + docID
	_, _ = bs.Put(ctx, key, bytes.NewReader([]byte("invoice total 42")), 16, "text/plain")
	_ = m.InsertDocument(ctx, store.Document{ID: docID, TenantID: tenant, Name: "inv", ObjectKey: key, MimeType: "text/plain", Status: store.DocActive, ProcessingStatus: store.ProcPending})
	_ = m.InsertJob(ctx, store.ProcessingJob{ID: jobID, TenantID: tenant, DocumentID: docID, Status: store.ProcPending, MaxRetries: 1})
	return
}

func TestExtractionCompletes(t *testing.T) {
	m := memstore.New()
	bs := blob.NewFake()
	ex := &extract.Fake{Text: "invoice total 42", Meta: map[string]string{"pages": "1"}}
	svc := jobs.New(m, bs, ex, nil, jobs.Config{Workers: 2, Lease: time.Minute})
	doc, _ := seed(t, m, bs)

	if n := svc.Once(context.Background(), nil); n != 1 {
		t.Fatalf("processed %d, want 1", n)
	}
	d, _ := m.GetDocument(context.Background(), tenant, doc)
	if d.ProcessingStatus != store.ProcCompleted || d.ContentText != "invoice total 42" {
		t.Fatalf("doc after extraction: status=%q content=%q", d.ProcessingStatus, d.ContentText)
	}
}

func TestExtractionRetriesThenFails(t *testing.T) {
	m := memstore.New()
	bs := blob.NewFake()
	ex := &extract.Fake{Err: errors.New("tika down")}
	svc := jobs.New(m, bs, ex, nil, jobs.Config{Workers: 2, Lease: time.Minute, MaxRetries: 1, RetryDelay: time.Millisecond})
	doc, _ := seed(t, m, bs)

	clk := time.Now()
	svc.SetClock(func() time.Time { return clk })
	for i := 0; i < 6; i++ {
		if svc.Once(context.Background(), nil) == 0 && i > 0 {
			break
		}
		clk = clk.Add(time.Hour)
	}
	d, _ := m.GetDocument(context.Background(), tenant, doc)
	if d.ProcessingStatus != store.ProcFailed {
		t.Fatalf("doc processing = %q, want failed", d.ProcessingStatus)
	}
}

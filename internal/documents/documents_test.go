package documents_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/blob"
	"github.com/go-freya/freya/services/paperless/internal/documents"
	"github.com/go-freya/freya/services/paperless/internal/events"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// fixture wires a service over the in-memory store + fake blob and returns an
// admin subject.
type fixture struct {
	svc  *documents.Service
	mem  *memstore.Mem
	bs   *blob.Fake
	subj authz.Subjects
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	mem := memstore.New()
	bs := blob.NewFake()
	az := authz.New(mem)
	svc := documents.New(mem, az, bs, events.HubPublisher{}, 5*time.Minute)
	subj := authz.Subjects{TenantID: store.NewID(), UserID: "u1", Roles: []string{"admin"}, ActorKind: "user"}
	return fixture{svc: svc, mem: mem, bs: bs, subj: subj}
}

func objectKey(tenantID, id string) string {
	return "tenants/" + tenantID + "/documents/" + id
}

func mustCreate(t *testing.T, f fixture, name string, data []byte) documents.View {
	t.Helper()
	v, err := f.svc.Create(context.Background(), f.subj, documents.CreateInput{
		Name:     name,
		FileName: name + ".txt",
		MimeType: "text/plain",
		Reader:   bytes.NewReader(data),
		Size:     int64(len(data)),
	})
	if err != nil {
		t.Fatalf("Create(%q): %v", name, err)
	}
	return v
}

func TestCreate_StoresBytesChecksumOwnerJobStatus(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	data := []byte("synthetic document bytes for T025")

	v := mustCreate(t, f, "invoice", data)

	// status active / processing pending.
	if v.Status != store.DocActive {
		t.Errorf("status = %q, want %q", v.Status, store.DocActive)
	}
	if v.ProcessingStatus != store.ProcPending {
		t.Errorf("processing_status = %q, want %q", v.ProcessingStatus, store.ProcPending)
	}

	// checksum is the SHA-256 hex of the exact bytes.
	sum := sha256.Sum256(data)
	if want := hex.EncodeToString(sum[:]); v.Checksum != want {
		t.Errorf("checksum = %q, want %q", v.Checksum, want)
	}

	// bytes are retrievable from the fake blob store under the tenant object key.
	rc, err := f.bs.Get(ctx, objectKey(f.subj.TenantID, v.ID))
	if err != nil {
		t.Fatalf("blob Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if !bytes.Equal(got, data) {
		t.Errorf("stored bytes = %q, want %q", got, data)
	}

	// owner permission tuple exists for the creator.
	perms, err := f.mem.ListPermissionsByResource(ctx, f.subj.TenantID, store.ResourceDocument, v.ID)
	if err != nil {
		t.Fatalf("ListPermissionsByResource: %v", err)
	}
	var ownerFound bool
	for _, p := range perms {
		if p.SubjectType == store.SubjectUser && p.SubjectID == "u1" && p.Relation == store.RelationOwner {
			ownerFound = true
		}
	}
	if !ownerFound {
		t.Errorf("no owner permission tuple recorded for creator; got %+v", perms)
	}
}

func TestCreate_EnqueuesPendingJob(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "report", []byte("body"))

	claimed, err := f.mem.ClaimDueJobs(ctx, time.Now(), time.Minute, 100)
	if err != nil {
		t.Fatalf("ClaimDueJobs: %v", err)
	}
	var found bool
	for _, j := range claimed {
		if j.DocumentID == v.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("no processing job enqueued for document %s", v.ID)
	}
}

func TestGet_ContentRedactedUnlessIncluded(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "contract", []byte("body"))

	const secret = "TOP-SECRET-EXTRACTED-CONTENT"
	if err := f.mem.SetDocumentProcessing(ctx, f.subj.TenantID, v.ID, store.ProcCompleted, secret, map[string]string{"pages": "3"}); err != nil {
		t.Fatalf("SetDocumentProcessing: %v", err)
	}

	// without includeContent: metadata present, content redacted.
	got, err := f.svc.Get(ctx, f.subj, v.ID, false)
	if err != nil {
		t.Fatalf("Get(includeContent=false): %v", err)
	}
	if got.Name != "contract" {
		t.Errorf("name = %q, want contract", got.Name)
	}
	if got.ContentText != "" || got.ExtractedMetadata != nil {
		t.Errorf("content leaked when not included: %+v", got)
	}

	// with includeContent: content returned.
	full, err := f.svc.Get(ctx, f.subj, v.ID, true)
	if err != nil {
		t.Fatalf("Get(includeContent=true): %v", err)
	}
	if full.ContentText != secret {
		t.Errorf("content_text = %q, want %q", full.ContentText, secret)
	}
	if full.ExtractedMetadata["pages"] != "3" {
		t.Errorf("extracted_metadata = %+v, want pages=3", full.ExtractedMetadata)
	}
}

func TestList_NeverContainsContentText(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "statement", []byte("body"))

	const secret = "SECRET-LIST-CONTENT-STRING"
	if err := f.mem.SetDocumentProcessing(ctx, f.subj.TenantID, v.ID, store.ProcCompleted, secret, map[string]string{"k": "v"}); err != nil {
		t.Fatalf("SetDocumentProcessing: %v", err)
	}

	list, err := f.svc.List(ctx, f.subj, repo.DocFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List returned %d docs, want 1", len(list))
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Errorf("List JSON leaked content_text: %s", raw)
	}
	if strings.Contains(string(raw), "content_text") {
		t.Errorf("List JSON contains content_text key: %s", raw)
	}
}

func TestDelete_SoftHidesFromList(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "temp", []byte("body"))

	if err := f.svc.Delete(ctx, f.subj, v.ID, false); err != nil {
		t.Fatalf("soft Delete: %v", err)
	}
	list, err := f.svc.List(ctx, f.subj, repo.DocFilter{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("soft-deleted doc still listed: %+v", list)
	}
}

func TestDelete_HardRemovesBlob(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "purge", []byte("body"))
	key := objectKey(f.subj.TenantID, v.ID)

	// present before hard delete.
	if _, err := f.bs.Get(ctx, key); err != nil {
		t.Fatalf("expected blob present before hard delete: %v", err)
	}
	if err := f.svc.Delete(ctx, f.subj, v.ID, true); err != nil {
		t.Fatalf("hard Delete: %v", err)
	}
	if _, err := f.bs.Get(ctx, key); err == nil {
		t.Errorf("expected blob Get to error after hard delete")
	}
}

func TestDownload_ReturnsBytes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	data := []byte("downloadable payload")
	v := mustCreate(t, f, "dl", data)

	rc, doc, err := f.svc.Download(ctx, f.subj, v.ID)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer rc.Close()
	if doc.ID != v.ID {
		t.Errorf("doc.ID = %q, want %q", doc.ID, v.ID)
	}
	got, _ := io.ReadAll(rc)
	if !bytes.Equal(got, data) {
		t.Errorf("downloaded bytes = %q, want %q", got, data)
	}
}

func TestDownloadURL_NonEmpty(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "url", []byte("body"))

	url, err := f.svc.DownloadURL(ctx, f.subj, v.ID)
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	if url == "" {
		t.Errorf("DownloadURL returned empty URL")
	}
}

func TestMove_UpdatesCategory(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "movable", []byte("body"))

	cat := store.Category{ID: store.NewID(), TenantID: f.subj.TenantID, Name: "Finance", Path: "finance"}
	if err := f.mem.InsertCategory(ctx, cat); err != nil {
		t.Fatalf("InsertCategory: %v", err)
	}

	moved, err := f.svc.Move(ctx, f.subj, v.ID, cat.ID)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if moved.CategoryID != cat.ID {
		t.Errorf("category_id = %q, want %q", moved.CategoryID, cat.ID)
	}
	if moved.CategoryPath != cat.Path {
		t.Errorf("category_path = %q, want %q", moved.CategoryPath, cat.Path)
	}
}

var _ = store.DocArchived // keep store import used if constants shift

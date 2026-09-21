package documents_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

// failingStore embeds the in-memory store and can inject an error into
// InsertPermission (GrantOwner) or InsertJob without failing the earlier
// InsertDocument the way FailNext would.
type failingStore struct {
	*memstore.Mem
	failInsertPermission error
	failInsertJob        error
}

func (f *failingStore) InsertPermission(ctx context.Context, p store.PermissionTuple) error {
	if f.failInsertPermission != nil {
		return f.failInsertPermission
	}
	return f.Mem.InsertPermission(ctx, p)
}

func (f *failingStore) InsertJob(ctx context.Context, j store.ProcessingJob) error {
	if f.failInsertJob != nil {
		return f.failInsertJob
	}
	return f.Mem.InsertJob(ctx, j)
}

func (f *failingStore) Atomic(ctx context.Context, tenantID string, fn func(tx repo.Store) error) error {
	return fn(f)
}

// failBlob wraps the fake blob and can fail selected operations.
type failBlob struct {
	*blob.Fake
	failPut     error
	failGet     error
	failDelete  error
	failPresign error
}

func (b failBlob) Put(ctx context.Context, key string, r io.Reader, size int64, ct string) (string, error) {
	if b.failPut != nil {
		return "", b.failPut
	}
	return b.Fake.Put(ctx, key, r, size, ct)
}

func (b failBlob) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	if b.failGet != nil {
		return nil, b.failGet
	}
	return b.Fake.Get(ctx, key)
}

func (b failBlob) Delete(ctx context.Context, key string) error {
	if b.failDelete != nil {
		return b.failDelete
	}
	return b.Fake.Delete(ctx, key)
}

func (b failBlob) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	if b.failPresign != nil {
		return "", b.failPresign
	}
	return b.Fake.PresignGet(ctx, key, ttl)
}

func member(tenantID string) authz.Subjects {
	return authz.Subjects{TenantID: tenantID, UserID: "stranger", ActorKind: "user"}
}

func createInput(name string, data []byte) documents.CreateInput {
	return documents.CreateInput{
		Name: name, FileName: name + ".txt", MimeType: "text/plain",
		Reader: bytes.NewReader(data), Size: int64(len(data)),
	}
}

func TestSetClock_NoPanic(t *testing.T) {
	f := newFixture(t)
	f.svc.SetClock(func() time.Time { return time.Unix(0, 0) })
	// Still functional after clock injection.
	mustCreate(t, f, "clocked", []byte("x"))
}

func TestCreate_WithCategorySuccess(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cat := store.Category{ID: store.NewID(), TenantID: f.subj.TenantID, Name: "Finance", Path: "/Finance"}
	if err := f.mem.InsertCategory(ctx, cat); err != nil {
		t.Fatalf("InsertCategory: %v", err)
	}
	v, err := f.svc.Create(ctx, f.subj, documents.CreateInput{
		Name: "filed", FileName: "f.txt", MimeType: "text/plain",
		Reader: bytes.NewReader([]byte("b")), Size: 1, CategoryID: cat.ID, Source: store.SourceEmail,
	})
	if err != nil {
		t.Fatalf("Create with category: %v", err)
	}
	if v.CategoryID != cat.ID || v.CategoryPath != "/Finance" {
		t.Errorf("category not applied: %+v", v)
	}
	if v.Source != store.SourceEmail {
		t.Errorf("source = %q, want email", v.Source)
	}
}

func TestCreate_CategoryWriteForbidden(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cat := store.Category{ID: store.NewID(), TenantID: f.subj.TenantID, Name: "Locked", Path: "/Locked"}
	if err := f.mem.InsertCategory(ctx, cat); err != nil {
		t.Fatalf("InsertCategory: %v", err)
	}
	_, err := f.svc.Create(ctx, member(f.subj.TenantID), documents.CreateInput{
		Name: "x", FileName: "x.txt", MimeType: "text/plain",
		Reader: bytes.NewReader([]byte("b")), Size: 1, CategoryID: cat.ID,
	})
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Create category-write err = %v, want ErrForbidden", err)
	}
}

func TestCreate_CategoryPathNotFound(t *testing.T) {
	f := newFixture(t)
	// Admin passes the write check (short-circuit) but the category does not exist.
	_, err := f.svc.Create(context.Background(), f.subj, documents.CreateInput{
		Name: "x", FileName: "x.txt", MimeType: "text/plain",
		Reader: bytes.NewReader([]byte("b")), Size: 1, CategoryID: store.NewID(),
	})
	if !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("Create missing category err = %v, want ErrNotFound", err)
	}
}

func TestCreate_BlobPutFailure(t *testing.T) {
	mem := memstore.New()
	az := authz.New(mem)
	bs := failBlob{Fake: blob.NewFake(), failPut: errors.New("put boom")}
	svc := documents.New(mem, az, bs, events.HubPublisher{}, time.Minute)
	subj := authz.Subjects{TenantID: store.NewID(), UserID: "u1", Roles: []string{"admin"}, ActorKind: "user"}
	if _, err := svc.Create(context.Background(), subj, createInput("x", []byte("b"))); err == nil {
		t.Fatal("Create should fail when blob.Put fails")
	}
}

func TestCreate_InsertDocumentFailure(t *testing.T) {
	f := newFixture(t)
	f.mem.FailNext(errors.New("insert doc boom"))
	if _, err := f.svc.Create(context.Background(), f.subj, createInput("x", []byte("b"))); err == nil {
		t.Fatal("Create should fail when InsertDocument fails")
	}
}

func TestCreate_GrantOwnerAndJobFailures(t *testing.T) {
	tenant := store.NewID()
	subj := authz.Subjects{TenantID: tenant, UserID: "u1", Roles: []string{"admin"}, ActorKind: "user"}

	// GrantOwner (InsertPermission) fails.
	fs := &failingStore{Mem: memstore.New(), failInsertPermission: errors.New("perm boom")}
	svc := documents.New(fs, authz.New(fs), blob.NewFake(), events.HubPublisher{}, time.Minute)
	if _, err := svc.Create(context.Background(), subj, createInput("x", []byte("b"))); err == nil {
		t.Fatal("Create should fail when GrantOwner fails")
	}

	// InsertJob fails.
	fs2 := &failingStore{Mem: memstore.New(), failInsertJob: errors.New("job boom")}
	svc2 := documents.New(fs2, authz.New(fs2), blob.NewFake(), events.HubPublisher{}, time.Minute)
	if _, err := svc2.Create(context.Background(), subj, createInput("y", []byte("b"))); err == nil {
		t.Fatal("Create should fail when InsertJob fails")
	}
}

func TestGet_ForbiddenAndNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "doc", []byte("b"))

	// Existing doc, no permission -> forbidden.
	if _, err := f.svc.Get(ctx, member(f.subj.TenantID), v.ID, false); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Get forbidden err = %v, want ErrForbidden", err)
	}
	// Missing doc -> not found (existence masked).
	if _, err := f.svc.Get(ctx, member(f.subj.TenantID), store.NewID(), false); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("Get missing err = %v, want ErrNotFound", err)
	}
}

func TestList_Forbidden(t *testing.T) {
	f := newFixture(t)
	if _, err := f.svc.List(context.Background(), member(f.subj.TenantID), repo.DocFilter{}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("List forbidden err = %v, want ErrForbidden", err)
	}
}

func TestUpdate_FullAndErrors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "orig", []byte("b"))
	cat := store.Category{ID: store.NewID(), TenantID: f.subj.TenantID, Name: "Cat", Path: "/Cat"}
	if err := f.mem.InsertCategory(ctx, cat); err != nil {
		t.Fatalf("InsertCategory: %v", err)
	}

	got, err := f.svc.Update(ctx, f.subj, v.ID, documents.UpdateInput{
		Name: "renamed", Description: "desc", Tags: map[string]string{"k": "v"}, CategoryID: cat.ID,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name != "renamed" || got.Description != "desc" || got.Tags["k"] != "v" {
		t.Errorf("update fields = %+v", got)
	}
	if got.CategoryID != cat.ID || got.CategoryPath != "/Cat" {
		t.Errorf("update category = %+v", got)
	}

	// Content never present on Update projection.
	if err := f.mem.SetDocumentProcessing(ctx, f.subj.TenantID, v.ID, store.ProcCompleted, "SECRET", map[string]string{"a": "b"}); err != nil {
		t.Fatalf("SetDocumentProcessing: %v", err)
	}
	up, _ := f.svc.Update(ctx, f.subj, v.ID, documents.UpdateInput{Description: "again"})
	raw, _ := json.Marshal(up)
	if strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), "content_text") {
		t.Errorf("Update leaked content: %s", raw)
	}

	// Update to a missing category -> not found.
	if _, err := f.svc.Update(ctx, f.subj, v.ID, documents.UpdateInput{CategoryID: store.NewID()}); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("Update missing category err = %v, want ErrNotFound", err)
	}
	// Update missing doc -> not found.
	if _, err := f.svc.Update(ctx, f.subj, store.NewID(), documents.UpdateInput{Name: "z"}); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("Update missing doc err = %v, want ErrNotFound", err)
	}
	// Update forbidden.
	if _, err := f.svc.Update(ctx, member(f.subj.TenantID), v.ID, documents.UpdateInput{Name: "z"}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Update forbidden err = %v, want ErrForbidden", err)
	}
}

func TestMove_ClearAndErrors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	cat := store.Category{ID: store.NewID(), TenantID: f.subj.TenantID, Name: "Cat", Path: "/Cat"}
	if err := f.mem.InsertCategory(ctx, cat); err != nil {
		t.Fatalf("InsertCategory: %v", err)
	}
	v := mustCreate(t, f, "m", []byte("b"))
	if _, err := f.svc.Move(ctx, f.subj, v.ID, cat.ID); err != nil {
		t.Fatalf("Move into cat: %v", err)
	}
	// Move to root (empty categoryID) clears the category.
	cleared, err := f.svc.Move(ctx, f.subj, v.ID, "")
	if err != nil {
		t.Fatalf("Move to root: %v", err)
	}
	if cleared.CategoryID != "" || cleared.CategoryPath != "" {
		t.Errorf("category not cleared: %+v", cleared)
	}
	// Move to missing category.
	if _, err := f.svc.Move(ctx, f.subj, v.ID, store.NewID()); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("Move missing cat err = %v, want ErrNotFound", err)
	}
	// Move forbidden.
	if _, err := f.svc.Move(ctx, member(f.subj.TenantID), v.ID, ""); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Move forbidden err = %v, want ErrForbidden", err)
	}
	// Move missing doc.
	if _, err := f.svc.Move(ctx, f.subj, store.NewID(), ""); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("Move missing doc err = %v, want ErrNotFound", err)
	}
}

func TestDelete_ForbiddenAndNotFound(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v := mustCreate(t, f, "d", []byte("b"))
	if err := f.svc.Delete(ctx, member(f.subj.TenantID), v.ID, false); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Delete forbidden err = %v, want ErrForbidden", err)
	}
	if err := f.svc.Delete(ctx, f.subj, store.NewID(), true); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("Delete missing err = %v, want ErrNotFound", err)
	}
}

func TestBatchDelete_PerIDResults(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a := mustCreate(t, f, "a", []byte("b"))
	b := mustCreate(t, f, "b", []byte("b"))
	missing := store.NewID()

	res, err := f.svc.BatchDelete(ctx, f.subj, []string{a.ID, missing, b.ID}, false)
	if err != nil {
		t.Fatalf("BatchDelete: %v", err)
	}
	if len(res) != 3 {
		t.Fatalf("results = %d, want 3", len(res))
	}
	if !res[0].OK || res[0].ID != a.ID {
		t.Errorf("res[0] = %+v, want ok for %s", res[0], a.ID)
	}
	if res[1].OK || res[1].ID != missing || res[1].Error == "" {
		t.Errorf("res[1] = %+v, want failed for missing id", res[1])
	}
	if !res[2].OK || res[2].ID != b.ID {
		t.Errorf("res[2] = %+v, want ok for %s", res[2], b.ID)
	}
}

func TestDownload_ForbiddenAndBlobFailure(t *testing.T) {
	mem := memstore.New()
	az := authz.New(mem)
	bs := failBlob{Fake: blob.NewFake(), failGet: errors.New("get boom")}
	svc := documents.New(mem, az, bs, events.HubPublisher{}, time.Minute)
	subj := authz.Subjects{TenantID: store.NewID(), UserID: "u1", Roles: []string{"admin"}, ActorKind: "user"}
	ctx := context.Background()

	v, err := svc.Create(ctx, subj, createInput("dl", []byte("payload")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Blob Get fails.
	if _, _, err := svc.Download(ctx, subj, v.ID); err == nil {
		t.Fatal("Download should surface blob Get failure")
	}
	// Forbidden download.
	if _, _, err := svc.Download(ctx, member(subj.TenantID), v.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Download forbidden err = %v, want ErrForbidden", err)
	}
	// Missing doc.
	if _, _, err := svc.Download(ctx, subj, store.NewID()); !errors.Is(err, documents.ErrNotFound) {
		t.Fatalf("Download missing err = %v, want ErrNotFound", err)
	}
}

func TestDownloadURL_ForbiddenAndPresignFailure(t *testing.T) {
	mem := memstore.New()
	az := authz.New(mem)
	bs := failBlob{Fake: blob.NewFake(), failPresign: errors.New("presign boom")}
	svc := documents.New(mem, az, bs, events.HubPublisher{}, time.Minute)
	subj := authz.Subjects{TenantID: store.NewID(), UserID: "u1", Roles: []string{"admin"}, ActorKind: "user"}
	ctx := context.Background()

	v, err := svc.Create(ctx, subj, createInput("u", []byte("b")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.DownloadURL(ctx, subj, v.ID); err == nil {
		t.Fatal("DownloadURL should surface presign failure")
	}
	if _, err := svc.DownloadURL(ctx, member(subj.TenantID), v.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("DownloadURL forbidden err = %v, want ErrForbidden", err)
	}
}

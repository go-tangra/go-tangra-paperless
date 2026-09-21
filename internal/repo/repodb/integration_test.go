//go:build integration

// Package repodb integration test: exercises the TimescaleDB-backed paperless
// store and its repo wrappers against a real database (testcontainers), covering
// the SQL layer (store.Open/Migrate/Tx, the per-entity SQL funcs, and the repodb
// tenant/system scoping) and per-tenant row-level security. Run with:
//
//	go test -tags integration ./internal/repo/repodb/
//
// It skips cleanly when Docker/testcontainers is unavailable.
package repodb_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/repo/repodb"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

const (
	tenantA = "0190f7c2-6a3e-7c1a-9b2e-2f6f9d1b4c55"
	tenantB = "0190f7c2-6a3e-7c1a-9b2e-2f6f9d1b4c66"
)

func startDB(t *testing.T) (adminDSN, appDSN string) {
	t.Helper()
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "timescale/timescaledb:latest-pg16", ExposedPorts: []string{"5432/tcp"},
			Env:        map[string]string{"POSTGRES_PASSWORD": "test", "POSTGRES_DB": "paperless"},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(2 * time.Minute),
		}, Started: true,
	})
	if err != nil {
		t.Skipf("testcontainers unavailable: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })
	host, _ := c.Host(ctx)
	port, _ := c.MappedPort(ctx, "5432/tcp")
	adminDSN = "postgres://postgres:test@" + host + ":" + port.Port() + "/paperless?sslmode=disable"
	conn, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Exec(ctx, "CREATE ROLE paperless_app LOGIN PASSWORD 'app' NOBYPASSRLS")
	_ = conn.Close(ctx)
	appDSN = "postgres://paperless_app:app@" + host + ":" + port.Port() + "/paperless?sslmode=disable"
	return
}

func openRepo(t *testing.T) repo.Store {
	t.Helper()
	adminDSN, appDSN := startDB(t)
	ctx := context.Background()
	if err := store.Migrate(ctx, adminDSN); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := store.Migrate(ctx, adminDSN); err != nil { // idempotent
		t.Fatalf("migrate idempotent: %v", err)
	}
	st, err := store.Open(ctx, appDSN, 4)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(st.Close)
	return repodb.New(st)
}

func TestPaperlessRepo(t *testing.T) {
	db := openRepo(t)
	ctx := context.Background()

	// --- categories: insert / get / list / subtree ---
	rootID, childID := store.NewID(), store.NewID()
	if err := db.InsertCategory(ctx, store.Category{
		ID: rootID, TenantID: tenantA, Name: "finance", Path: "finance", CreatedBy: "u1",
	}); err != nil {
		t.Fatalf("insert root category: %v", err)
	}
	if err := db.InsertCategory(ctx, store.Category{
		ID: childID, TenantID: tenantA, ParentID: &rootID, Name: "invoices", Path: "finance/invoices",
		Depth: 1, CreatedBy: "u1",
	}); err != nil {
		t.Fatalf("insert child category: %v", err)
	}
	// Unique (tenant, parent, name) conflict. NULL parent_id rows are distinct in
	// Postgres, so a duplicate must share the same non-null parent to collide.
	if err := db.InsertCategory(ctx, store.Category{
		ID: store.NewID(), TenantID: tenantA, ParentID: &rootID, Name: "invoices", Path: "finance/invoices",
	}); err != store.ErrConflict {
		t.Fatalf("expected conflict on duplicate category, got %v", err)
	}
	if got, err := db.GetCategory(ctx, tenantA, childID); err != nil || got.Name != "invoices" || got.ParentID == nil || *got.ParentID != rootID {
		t.Fatalf("get category: %+v %v", got, err)
	}
	if cats, err := db.ListCategories(ctx, tenantA); err != nil || len(cats) != 2 {
		t.Fatalf("list categories: %d %v", len(cats), err)
	}
	if sub, err := db.ListSubtree(ctx, tenantA, "finance"); err != nil || len(sub) != 2 {
		t.Fatalf("subtree finance: %d %v", len(sub), err)
	}
	if sub, err := db.ListSubtree(ctx, tenantA, "finance/invoices"); err != nil || len(sub) != 1 {
		t.Fatalf("subtree invoices: %d %v", len(sub), err)
	}
	child, _ := db.GetCategory(ctx, tenantA, childID)
	child.DocumentCount = 5
	if err := db.UpdateCategory(ctx, child); err != nil {
		t.Fatalf("update category: %v", err)
	}

	// --- documents: insert / get / list(filter) / update / setprocessing / search / delete ---
	docID := store.NewID()
	if err := db.InsertDocument(ctx, store.Document{
		ID: docID, TenantID: tenantA, CategoryID: &childID, CategoryPath: "finance/invoices",
		Name: "Q3 Invoice", Description: "quarterly", ObjectKey: "obj/q3", FileName: "q3.pdf",
		FileSize: 1024, MimeType: "application/pdf", Checksum: "abc", Status: store.DocActive,
		Source: store.SourceUpload, Tags: map[string]string{"year": "2026"},
		ContentText: "acme corporation quarterly invoice pineapple", ProcessingStatus: store.ProcCompleted,
		CreatedBy: "u1",
	}); err != nil {
		t.Fatalf("insert document: %v", err)
	}
	// Unique (tenant, object_key) conflict.
	if err := db.InsertDocument(ctx, store.Document{
		ID: store.NewID(), TenantID: tenantA, Name: "dup", ObjectKey: "obj/q3",
	}); err != store.ErrConflict {
		t.Fatalf("expected conflict on duplicate object_key, got %v", err)
	}
	got, err := db.GetDocument(ctx, tenantA, docID)
	if err != nil || got.Name != "Q3 Invoice" || got.Tags["year"] != "2026" {
		t.Fatalf("get document: %+v %v", got, err)
	}
	if list, err := db.ListDocuments(ctx, tenantA, repo.DocFilter{CategoryID: childID}); err != nil || len(list) != 1 {
		t.Fatalf("list by category: %d %v", len(list), err)
	}
	if list, err := db.ListDocuments(ctx, tenantA, repo.DocFilter{Tag: "year=2026"}); err != nil || len(list) != 1 {
		t.Fatalf("list by tag: %d %v", len(list), err)
	}
	if list, err := db.ListDocuments(ctx, tenantA, repo.DocFilter{MimeType: "image/png"}); err != nil || len(list) != 0 {
		t.Fatalf("list by absent mime: %d %v", len(list), err)
	}
	got.Description = "updated"
	if err := db.UpdateDocument(ctx, got); err != nil {
		t.Fatalf("update document: %v", err)
	}
	if err := db.SetDocumentProcessing(ctx, tenantA, docID, store.ProcCompleted,
		"acme corporation quarterly invoice pineapple extracted", map[string]string{"pages": "3"}); err != nil {
		t.Fatalf("set processing: %v", err)
	}
	if got, _ := db.GetDocument(ctx, tenantA, docID); got.ExtractedMetadata["pages"] != "3" {
		t.Fatalf("processing metadata not stored: %+v", got.ExtractedMetadata)
	}
	// Full-text search on a distinctive word in content_text.
	if res, err := db.SearchDocuments(ctx, tenantA, "pineapple", nil, true, 10); err != nil || len(res) != 1 || res[0].Document.ID != docID {
		t.Fatalf("search all: %d %v", len(res), err)
	}
	if res, err := db.SearchDocuments(ctx, tenantA, "pineapple", []string{docID}, false, 10); err != nil || len(res) != 1 {
		t.Fatalf("search accessible: %d %v", len(res), err)
	}
	if res, err := db.SearchDocuments(ctx, tenantA, "pineapple", []string{store.NewID()}, false, 10); err != nil || len(res) != 0 {
		t.Fatalf("search inaccessible should be empty: %d %v", len(res), err)
	}
	if res, err := db.SearchDocuments(ctx, tenantA, "zzznotpresent", nil, true, 10); err != nil || len(res) != 0 {
		t.Fatalf("search miss: %d %v", len(res), err)
	}

	// --- permissions: insert / list / grantsfor ---
	permID := store.NewID()
	if err := db.InsertPermission(ctx, store.PermissionTuple{
		ID: permID, TenantID: tenantA, ResourceType: store.ResourceDocument, ResourceID: docID,
		SubjectType: store.SubjectUser, SubjectID: "u1", Relation: store.RelationOwner, GrantedBy: "admin",
	}); err != nil {
		t.Fatalf("insert permission: %v", err)
	}
	if err := db.InsertPermission(ctx, store.PermissionTuple{
		ID: store.NewID(), TenantID: tenantA, ResourceType: store.ResourceDocument, ResourceID: docID,
		SubjectType: store.SubjectRole, SubjectID: "editors", Relation: store.RelationEditor,
	}); err != nil {
		t.Fatalf("insert role permission: %v", err)
	}
	expired := time.Now().Add(-time.Hour)
	if err := db.InsertPermission(ctx, store.PermissionTuple{
		ID: store.NewID(), TenantID: tenantA, ResourceType: store.ResourceDocument, ResourceID: docID,
		SubjectType: store.SubjectUser, SubjectID: "u1", Relation: store.RelationViewer, ExpiresAt: &expired,
	}); err != nil {
		t.Fatalf("insert expired permission: %v", err)
	}
	if pr, err := db.ListPermissionsByResource(ctx, tenantA, store.ResourceDocument, docID); err != nil || len(pr) != 3 {
		t.Fatalf("list perms by resource: %d %v", len(pr), err)
	}
	if ps, err := db.ListPermissionsBySubject(ctx, tenantA, store.SubjectUser, "u1"); err != nil || len(ps) != 2 {
		t.Fatalf("list perms by subject: %d %v", len(ps), err)
	}
	// GrantsForSubjects: the user tuple + the editors-role tuple; the expired one is excluded.
	if g, err := db.GrantsForSubjects(ctx, tenantA, "u1", []string{"editors"}, time.Now()); err != nil || len(g) != 2 {
		t.Fatalf("grants for subjects: %d %v", len(g), err)
	}
	if err := db.DeletePermission(ctx, tenantA, permID); err != nil {
		t.Fatalf("delete permission: %v", err)
	}

	// --- jobs: insert / claim (FOR UPDATE SKIP LOCKED) / update / delete-old ---
	jobID := store.NewID()
	if err := db.InsertJob(ctx, store.ProcessingJob{
		ID: jobID, TenantID: tenantA, DocumentID: docID, Status: store.ProcPending, MaxRetries: 3,
		Result: []byte(`{"ok":true}`),
	}); err != nil {
		t.Fatalf("insert job: %v", err)
	}
	due, err := db.ClaimDueJobs(ctx, time.Now(), time.Minute, 10)
	if err != nil || len(due) != 1 || due[0].ID != jobID || due[0].Status != store.ProcProcessing {
		t.Fatalf("claim due jobs: %d %v", len(due), err)
	}
	// A second immediate claim finds nothing (lease held).
	if again, err := db.ClaimDueJobs(ctx, time.Now(), time.Minute, 10); err != nil || len(again) != 0 {
		t.Fatalf("second claim should be empty: %d %v", len(again), err)
	}
	j, _ := db.GetJob(ctx, tenantA, jobID)
	j.Status = store.ProcCompleted
	completed := time.Now()
	j.CompletedAt = &completed
	if err := db.UpdateJob(ctx, j); err != nil {
		t.Fatalf("update job: %v", err)
	}
	if n, err := db.DeleteJobsOlderThan(ctx, time.Now().Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("delete old jobs: %d %v", n, err)
	}

	// --- audit insert (system scope) ---
	if err := db.InsertAuditRows(ctx, []store.AuditRow{{
		TS: time.Now(), TenantID: tenantA, EventType: "document_created", ActorKind: "user", ActorID: "u1",
		SubjectKind: "document", SubjectID: docID, Outcome: "ok", Details: []byte(`{"k":"v"}`),
	}}); err != nil {
		t.Fatalf("audit insert: %v", err)
	}

	// --- Exists + TenantIDs ---
	if ok, err := db.Exists(ctx, tenantA, store.ResourceDocument, docID); err != nil || !ok {
		t.Fatalf("exists document: %v %v", ok, err)
	}
	if ok, err := db.Exists(ctx, tenantA, store.ResourceCategory, rootID); err != nil || !ok {
		t.Fatalf("exists category: %v %v", ok, err)
	}
	if ok, err := db.Exists(ctx, tenantA, store.ResourceDocument, store.NewID()); err != nil || ok {
		t.Fatalf("exists absent should be false: %v %v", ok, err)
	}
	if ids, err := db.TenantIDs(ctx); err != nil || len(ids) == 0 {
		t.Fatalf("tenant ids: %v %v", ids, err)
	}

	// --- RLS: tenant B cannot read tenant A's document ---
	if _, err := db.GetDocument(ctx, tenantB, docID); err == nil {
		t.Fatal("RLS breach: tenant B read tenant A document")
	}
	if list, err := db.ListDocuments(ctx, tenantB, repo.DocFilter{}); err != nil || len(list) != 0 {
		t.Fatalf("RLS breach: tenant B listed tenant A documents: %d %v", len(list), err)
	}

	// --- delete document ---
	if err := db.DeleteDocument(ctx, tenantA, docID); err != nil {
		t.Fatalf("delete document: %v", err)
	}
	if _, err := db.GetDocument(ctx, tenantA, docID); err == nil {
		t.Fatal("document still present after delete")
	}
}

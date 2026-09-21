// Package repodb binds repo.Store to the database: tenant-scoped calls run in a
// tenant transaction (RLS), system-scoped calls (job claim, cleanup, audit,
// tenant enumeration) in a system transaction.
package repodb

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// DB implements repo.Store over *store.Store.
type DB struct {
	St *store.Store
	tx pgx.Tx // set inside Atomic
}

// New wraps the store.
func New(st *store.Store) *DB { return &DB{St: st} }

func (d *DB) run(ctx context.Context, scope store.Scope, fn func(tx pgx.Tx) error) error {
	if d.tx != nil {
		return fn(d.tx)
	}
	return d.St.Tx(ctx, scope, fn)
}
func (d *DB) tenant(ctx context.Context, tid string, fn func(tx pgx.Tx) error) error {
	return d.run(ctx, store.Scope{TenantID: tid}, fn)
}
func (d *DB) system(ctx context.Context, fn func(tx pgx.Tx) error) error {
	return d.run(ctx, store.Scope{System: true}, fn)
}

// Atomic runs fn in one tenant transaction.
func (d *DB) Atomic(ctx context.Context, tenantID string, fn func(repo.Store) error) error {
	if d.tx != nil {
		return fn(d)
	}
	return d.St.Tx(ctx, store.Scope{TenantID: tenantID}, func(tx pgx.Tx) error { return fn(&DB{St: d.St, tx: tx}) })
}

func (d *DB) Close() { d.St.Close() }

// --- documents ---

func (d *DB) InsertDocument(ctx context.Context, doc store.Document) error {
	return d.tenant(ctx, doc.TenantID, func(tx pgx.Tx) error { return store.InsertDocument(ctx, tx, doc) })
}
func (d *DB) GetDocument(ctx context.Context, tid, id string) (out store.Document, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error { out, err = store.GetDocument(ctx, tx, tid, id); return err })
	return
}
func (d *DB) ListDocuments(ctx context.Context, tid string, f repo.DocFilter) (out []store.Document, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error { out, err = store.ListDocuments(ctx, tx, tid, f); return err })
	return
}
func (d *DB) UpdateDocument(ctx context.Context, doc store.Document) error {
	return d.tenant(ctx, doc.TenantID, func(tx pgx.Tx) error { return store.UpdateDocument(ctx, tx, doc) })
}
func (d *DB) SetDocumentProcessing(ctx context.Context, tid, id, status, contentText string, meta map[string]string) error {
	return d.tenant(ctx, tid, func(tx pgx.Tx) error {
		return store.SetDocumentProcessing(ctx, tx, tid, id, status, contentText, meta)
	})
}
func (d *DB) DeleteDocument(ctx context.Context, tid, id string) error {
	return d.tenant(ctx, tid, func(tx pgx.Tx) error { return store.DeleteDocument(ctx, tx, tid, id) })
}
func (d *DB) SearchDocuments(ctx context.Context, tid, query string, accessible []string, all bool, limit int) (out []store.SearchResult, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error {
		out, err = store.SearchDocuments(ctx, tx, tid, query, accessible, all, limit)
		return err
	})
	return
}

// --- categories ---

func (d *DB) InsertCategory(ctx context.Context, c store.Category) error {
	return d.tenant(ctx, c.TenantID, func(tx pgx.Tx) error { return store.InsertCategory(ctx, tx, c) })
}
func (d *DB) GetCategory(ctx context.Context, tid, id string) (out store.Category, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error { out, err = store.GetCategory(ctx, tx, tid, id); return err })
	return
}
func (d *DB) ListCategories(ctx context.Context, tid string) (out []store.Category, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error { out, err = store.ListCategories(ctx, tx, tid); return err })
	return
}
func (d *DB) ListSubtree(ctx context.Context, tid, pathPrefix string) (out []store.Category, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error { out, err = store.ListSubtree(ctx, tx, tid, pathPrefix); return err })
	return
}
func (d *DB) UpdateCategory(ctx context.Context, c store.Category) error {
	return d.tenant(ctx, c.TenantID, func(tx pgx.Tx) error { return store.UpdateCategory(ctx, tx, c) })
}
func (d *DB) DeleteCategory(ctx context.Context, tid, id string) error {
	return d.tenant(ctx, tid, func(tx pgx.Tx) error { return store.DeleteCategory(ctx, tx, tid, id) })
}

// --- permissions ---

func (d *DB) InsertPermission(ctx context.Context, p store.PermissionTuple) error {
	return d.tenant(ctx, p.TenantID, func(tx pgx.Tx) error { return store.InsertPermission(ctx, tx, p) })
}
func (d *DB) DeletePermission(ctx context.Context, tid, id string) error {
	return d.tenant(ctx, tid, func(tx pgx.Tx) error { return store.DeletePermission(ctx, tx, tid, id) })
}
func (d *DB) ListPermissionsByResource(ctx context.Context, tid, resourceType, resourceID string) (out []store.PermissionTuple, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error {
		out, err = store.ListPermissionsByResource(ctx, tx, tid, resourceType, resourceID)
		return err
	})
	return
}
func (d *DB) ListPermissionsBySubject(ctx context.Context, tid, subjectType, subjectID string) (out []store.PermissionTuple, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error {
		out, err = store.ListPermissionsBySubject(ctx, tx, tid, subjectType, subjectID)
		return err
	})
	return
}
func (d *DB) GrantsForSubjects(ctx context.Context, tid, userID string, roles []string, now time.Time) (out []store.PermissionTuple, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error {
		out, err = store.GrantsForSubjects(ctx, tx, tid, userID, roles, now)
		return err
	})
	return
}

// --- jobs ---

func (d *DB) InsertJob(ctx context.Context, j store.ProcessingJob) error {
	return d.tenant(ctx, j.TenantID, func(tx pgx.Tx) error { return store.InsertJob(ctx, tx, j) })
}
func (d *DB) GetJob(ctx context.Context, tid, id string) (out store.ProcessingJob, err error) {
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error { out, err = store.GetJob(ctx, tx, tid, id); return err })
	return
}
func (d *DB) UpdateJob(ctx context.Context, j store.ProcessingJob) error {
	return d.tenant(ctx, j.TenantID, func(tx pgx.Tx) error { return store.UpdateJob(ctx, tx, j) })
}
func (d *DB) ClaimDueJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) (out []store.ProcessingJob, err error) {
	err = d.system(ctx, func(tx pgx.Tx) error { out, err = store.ClaimDueJobs(ctx, tx, now, lease, limit); return err })
	return
}
func (d *DB) DeleteJobsOlderThan(ctx context.Context, cutoff time.Time) (n int, err error) {
	err = d.system(ctx, func(tx pgx.Tx) error { n, err = store.DeleteJobsOlderThan(ctx, tx, cutoff); return err })
	return
}

// --- audit ---

func (d *DB) InsertAuditRows(ctx context.Context, rows []store.AuditRow) error {
	return d.system(ctx, func(tx pgx.Tx) error { return store.InsertAuditRows(ctx, tx, rows) })
}

// --- existence (authz) ---

func (d *DB) Exists(ctx context.Context, tid, resourceType, id string) (ok bool, err error) {
	table := map[string]string{
		store.ResourceDocument: "paperless_documents",
		store.ResourceCategory: "paperless_categories",
	}[resourceType]
	if table == "" {
		return false, nil
	}
	err = d.tenant(ctx, tid, func(tx pgx.Tx) error { ok, err = store.Exists(ctx, tx, tid, table, id); return err })
	return
}

// TenantIDs lists every tenant with paperless data (system scope).
func (d *DB) TenantIDs(ctx context.Context) (out []string, err error) {
	err = d.system(ctx, func(tx pgx.Tx) error { out, err = store.TenantIDs(ctx, tx); return err })
	return
}

var _ repo.Store = (*DB)(nil)

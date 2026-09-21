// Package repo is the paperless storage interface. Services depend on this
// interface; the TimescaleDB implementation (internal/repo/repodb over
// internal/store) and the in-memory fake (internal/memstore) both satisfy it.
// Every method is tenant-scoped — the concrete store enforces per-tenant RLS,
// the fake filters by tenant.
package repo

import (
	"context"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/store"
)

// DocFilter selects documents.
type DocFilter = store.DocumentFilter

// Store is the paperless persistence surface.
type Store interface {
	// Atomic runs fn in a transaction scoped to tenantID.
	Atomic(ctx context.Context, tenantID string, fn func(tx Store) error) error

	// Documents.
	InsertDocument(ctx context.Context, d store.Document) error
	GetDocument(ctx context.Context, tenantID, id string) (store.Document, error)
	ListDocuments(ctx context.Context, tenantID string, f DocFilter) ([]store.Document, error)
	UpdateDocument(ctx context.Context, d store.Document) error
	SetDocumentProcessing(ctx context.Context, tenantID, id, status, contentText string, meta map[string]string) error
	DeleteDocument(ctx context.Context, tenantID, id string) error
	SearchDocuments(ctx context.Context, tenantID, query string, accessible []string, all bool, limit int) ([]store.SearchResult, error)

	// Categories.
	InsertCategory(ctx context.Context, c store.Category) error
	GetCategory(ctx context.Context, tenantID, id string) (store.Category, error)
	ListCategories(ctx context.Context, tenantID string) ([]store.Category, error)
	ListSubtree(ctx context.Context, tenantID, pathPrefix string) ([]store.Category, error)
	UpdateCategory(ctx context.Context, c store.Category) error
	DeleteCategory(ctx context.Context, tenantID, id string) error

	// Permissions.
	InsertPermission(ctx context.Context, p store.PermissionTuple) error
	DeletePermission(ctx context.Context, tenantID, id string) error
	ListPermissionsByResource(ctx context.Context, tenantID, resourceType, resourceID string) ([]store.PermissionTuple, error)
	ListPermissionsBySubject(ctx context.Context, tenantID, subjectType, subjectID string) ([]store.PermissionTuple, error)
	// GrantsForSubjects returns all non-expired tuples that could apply to the
	// subject (its user/role tuples + tenant-wide), for the authz engine to
	// resolve against a resource and its ancestor category paths.
	GrantsForSubjects(ctx context.Context, tenantID, userID string, roles []string, now time.Time) ([]store.PermissionTuple, error)

	// Processing jobs.
	InsertJob(ctx context.Context, j store.ProcessingJob) error
	GetJob(ctx context.Context, tenantID, id string) (store.ProcessingJob, error)
	UpdateJob(ctx context.Context, j store.ProcessingJob) error
	ClaimDueJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]store.ProcessingJob, error)
	DeleteJobsOlderThan(ctx context.Context, cutoff time.Time) (int, error)

	// Audit (batch insert, system scope).
	InsertAuditRows(ctx context.Context, rows []store.AuditRow) error

	// Exists reports whether a resource (document|category) exists in the tenant.
	Exists(ctx context.Context, tenantID, resourceType, id string) (bool, error)
	// TenantIDs returns every tenant with paperless data (system scope).
	TenantIDs(ctx context.Context) ([]string, error)

	Close()
}

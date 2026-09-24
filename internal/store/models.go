// Package store models mirror the paperless_* tables (see
// specs/009-paperless-service/data-model.md). Document bytes are never stored
// here; only metadata, extracted content, and the object key.
package store

import "time"

// Document statuses.
const (
	DocActive   = "active"
	DocArchived = "archived"
	DocDeleted  = "deleted"
)

// Document sources.
const (
	SourceUpload = "upload"
	SourceEmail  = "email"
)

// Processing statuses.
const (
	ProcPending    = "pending"
	ProcProcessing = "processing"
	ProcCompleted  = "completed"
	ProcFailed     = "failed"
	ProcRetrying   = "retrying"
)

// Permission relations and resource/subject kinds.
const (
	RelationOwner  = "owner"
	RelationEditor = "editor"
	RelationViewer = "viewer"
	RelationSharer = "sharer"

	ResourceDocument = "document"
	ResourceCategory = "category"

	SubjectUser   = "user"
	SubjectRole   = "role"
	SubjectTenant = "tenant"
)

// Document is a stored document's metadata + extracted content. content_text and
// ExtractedMetadata are redacted from list/log/audit projections.
type Document struct {
	ID                string
	TenantID          string
	CategoryID        *string
	CategoryPath      string
	Name              string
	Description       string
	ObjectKey         string
	FileName          string
	FileSize          int64
	MimeType          string
	Checksum          string
	Status            string
	Source            string
	Tags              map[string]string
	ContentText       string            // redacted from read projections
	ExtractedMetadata map[string]string // redacted
	ProcessingStatus  string
	CreatedBy         string
	UpdatedBy         string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Category is a folder in the tenant's tree, with a materialized path.
type Category struct {
	ID               string
	TenantID         string
	ParentID         *string
	Name             string
	Path             string
	Description      string
	Depth            int
	SortOrder        int
	DocumentCount    int
	SubcategoryCount int
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// PermissionTuple binds a subject to a resource with a relation, optionally expiring.
type PermissionTuple struct {
	ID           string
	TenantID     string
	ResourceType string
	ResourceID   string
	SubjectType  string
	SubjectID    string
	Relation     string
	GrantedBy    string
	GrantedAt    time.Time
	ExpiresAt    *time.Time
}

// ProcessingJob is one document's extraction work.
type ProcessingJob struct {
	ID          string
	TenantID    string
	DocumentID  string
	Status      string
	RetryCount  int
	MaxRetries  int
	LeaseUntil  *time.Time
	NextRetryAt *time.Time
	Result      []byte // JSON, non-secret
	StartedAt   *time.Time
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// AuditRow is one append-only audit entry (schema shared across modules).
type AuditRow struct {
	TS            time.Time
	TenantID      string
	EventType     string
	ActorKind     string
	ActorID       string
	SubjectKind   string
	SubjectID     string
	SubjectName   string
	Outcome       string
	Reason        string
	CorrelationID string
	Details       []byte
}

// DocumentFilter selects documents for listing.
type DocumentFilter struct {
	CategoryID       string
	Status           string
	MimeType         string
	Source           string
	ProcessingStatus string
	Tag              string // key=value or key
	CreatedBy        string
	Limit            int
	CursorID         string
}

// SearchResult is one full-text hit.
type SearchResult struct {
	Document Document
	Rank     float64
	Snippet  string
}

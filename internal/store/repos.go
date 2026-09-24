package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- helpers ----

// marshalStrMap encodes a string map to jsonb bytes, defaulting to an empty
// object so the NOT NULL jsonb columns always receive valid JSON.
func marshalStrMap(m map[string]string) []byte {
	if len(m) == 0 {
		return []byte("{}")
	}
	b, err := json.Marshal(m)
	if err != nil || len(b) == 0 {
		return []byte("{}")
	}
	return b
}

// unmarshalStrMap decodes jsonb bytes into a string map (nil on empty/invalid).
func unmarshalStrMap(b []byte) map[string]string {
	if len(b) == 0 {
		return nil
	}
	m := map[string]string{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

func defaultStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func defaultInt(n, d int) int {
	if n == 0 {
		return d
	}
	return n
}

// ---- documents ----

const docCols = "id, tenant_id, category_id, category_path, name, description, object_key, " +
	"file_name, file_size, mime_type, checksum, status, source, tags, content_text, " +
	"extracted_metadata, processing_status, created_by, updated_by, created_at, updated_at"

// docDest returns the scan destinations for docCols; tags/meta land in raw byte
// slices so the jsonb columns can be decoded into string maps by the caller.
func docDest(d *Document, tags, meta *[]byte) []any {
	return []any{
		&d.ID, &d.TenantID, &d.CategoryID, &d.CategoryPath, &d.Name, &d.Description, &d.ObjectKey,
		&d.FileName, &d.FileSize, &d.MimeType, &d.Checksum, &d.Status, &d.Source, tags, &d.ContentText,
		meta, &d.ProcessingStatus, &d.CreatedBy, &d.UpdatedBy, &d.CreatedAt, &d.UpdatedAt,
	}
}

func scanDoc(row pgx.Row) (Document, error) {
	var d Document
	var tags, meta []byte
	if err := row.Scan(docDest(&d, &tags, &meta)...); err != nil {
		return Document{}, notFound(err)
	}
	d.Tags = unmarshalStrMap(tags)
	d.ExtractedMetadata = unmarshalStrMap(meta)
	return d, nil
}

func scanDocs(rows pgx.Rows) ([]Document, error) {
	var out []Document
	for rows.Next() {
		d, err := scanDoc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// InsertDocument stores a document (search_tsv is generated, never written).
func InsertDocument(ctx context.Context, tx pgx.Tx, d Document) error {
	_, err := tx.Exec(ctx, `INSERT INTO paperless_documents
		(id, tenant_id, category_id, category_path, name, description, object_key, file_name, file_size,
		 mime_type, checksum, status, source, tags, content_text, extracted_metadata, processing_status,
		 created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$18)`,
		d.ID, d.TenantID, d.CategoryID, d.CategoryPath, d.Name, d.Description, d.ObjectKey, d.FileName, d.FileSize,
		d.MimeType, d.Checksum, defaultStr(d.Status, DocActive), defaultStr(d.Source, SourceUpload),
		marshalStrMap(d.Tags), d.ContentText, marshalStrMap(d.ExtractedMetadata),
		defaultStr(d.ProcessingStatus, ProcPending), d.CreatedBy)
	return conflict(err)
}

// GetDocument by tenant + id.
func GetDocument(ctx context.Context, tx pgx.Tx, tenantID, id string) (Document, error) {
	return scanDoc(tx.QueryRow(ctx, "SELECT "+docCols+" FROM paperless_documents WHERE tenant_id=$1 AND id=$2", tenantID, id))
}

// ListDocuments applies the filter (empty fields ignored) newest-first. When no
// explicit status filter is set, soft-deleted documents are hidden.
func ListDocuments(ctx context.Context, tx pgx.Tx, tenantID string, f DocumentFilter) ([]Document, error) {
	where := []string{"tenant_id = $1"}
	args := []any{tenantID}
	add := func(clause string, val any) {
		args = append(args, val)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if f.Status != "" {
		add("status = $%d", f.Status)
	} else {
		where = append(where, "status <> '"+DocDeleted+"'")
	}
	if f.CategoryID != "" {
		add("category_id = $%d::uuid", f.CategoryID)
	}
	if f.MimeType != "" {
		add("mime_type = $%d", f.MimeType)
	}
	if f.Source != "" {
		add("source = $%d", f.Source)
	}
	if f.ProcessingStatus != "" {
		add("processing_status = $%d", f.ProcessingStatus)
	}
	if f.CreatedBy != "" {
		add("created_by = $%d", f.CreatedBy)
	}
	if f.Tag != "" {
		if k, v, ok := strings.Cut(f.Tag, "="); ok {
			args = append(args, k, v)
			where = append(where, fmt.Sprintf("tags->>$%d = $%d", len(args)-1, len(args)))
		} else {
			add("tags ? $%d", f.Tag)
		}
	}
	if f.CursorID != "" {
		add("created_at < (SELECT created_at FROM paperless_documents WHERE id = $%d::uuid AND tenant_id = $1)", f.CursorID)
	}
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	args = append(args, limit)
	q := "SELECT " + docCols + " FROM paperless_documents WHERE " + strings.Join(where, " AND ") +
		" ORDER BY created_at DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDocs(rows)
}

// UpdateDocument rewrites the mutable columns.
func UpdateDocument(ctx context.Context, tx pgx.Tx, d Document) error {
	ct, err := tx.Exec(ctx, `UPDATE paperless_documents SET category_id=$3, category_path=$4, name=$5,
		description=$6, object_key=$7, file_name=$8, file_size=$9, mime_type=$10, checksum=$11, status=$12,
		source=$13, tags=$14, content_text=$15, extracted_metadata=$16, processing_status=$17, updated_by=$18,
		updated_at=now() WHERE tenant_id=$1 AND id=$2`,
		d.TenantID, d.ID, d.CategoryID, d.CategoryPath, d.Name, d.Description, d.ObjectKey, d.FileName, d.FileSize,
		d.MimeType, d.Checksum, d.Status, d.Source, marshalStrMap(d.Tags), d.ContentText,
		marshalStrMap(d.ExtractedMetadata), d.ProcessingStatus, d.UpdatedBy)
	if err != nil {
		return conflict(err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetDocumentProcessing records extraction output; a nil meta leaves the stored
// extracted_metadata untouched.
func SetDocumentProcessing(ctx context.Context, tx pgx.Tx, tenantID, id, status, contentText string, meta map[string]string) error {
	var metaArg any
	if meta != nil {
		metaArg = marshalStrMap(meta)
	}
	ct, err := tx.Exec(ctx, `UPDATE paperless_documents SET processing_status=$3, content_text=$4,
		extracted_metadata=COALESCE($5::jsonb, extracted_metadata), updated_at=now()
		WHERE tenant_id=$1 AND id=$2`,
		tenantID, id, status, contentText, metaArg)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDocument removes a document.
func DeleteDocument(ctx context.Context, tx pgx.Tx, tenantID, id string) error {
	ct, err := tx.Exec(ctx, "DELETE FROM paperless_documents WHERE tenant_id=$1 AND id=$2", tenantID, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SearchDocuments runs a tenant-scoped full-text search. When !all the results
// are confined to the accessible id set; an empty query matches every document.
func SearchDocuments(ctx context.Context, tx pgx.Tx, tenantID, query string, accessible []string, all bool, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	args := []any{tenantID}
	where := []string{"tenant_id = $1", "status <> '" + DocDeleted + "'"}
	rankExpr := "1.0::float4"
	snippetExpr := "left(content_text, 200)"
	if q := strings.TrimSpace(query); q != "" {
		args = append(args, q)
		tsq := fmt.Sprintf("websearch_to_tsquery('english', $%d)", len(args))
		where = append(where, "search_tsv @@ "+tsq)
		rankExpr = "ts_rank(search_tsv, " + tsq + ")"
		snippetExpr = "ts_headline('english', content_text, " + tsq + ")"
	}
	if !all {
		args = append(args, accessible)
		where = append(where, fmt.Sprintf("id = ANY($%d::uuid[])", len(args)))
	}
	args = append(args, limit)
	q := "SELECT " + docCols + ", " + rankExpr + " AS rank, " + snippetExpr + " AS snippet " +
		"FROM paperless_documents WHERE " + strings.Join(where, " AND ") +
		" ORDER BY rank DESC, created_at DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var d Document
		var tags, meta []byte
		var r SearchResult
		dest := append(docDest(&d, &tags, &meta), &r.Rank, &r.Snippet)
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		d.Tags = unmarshalStrMap(tags)
		d.ExtractedMetadata = unmarshalStrMap(meta)
		r.Document = d
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- categories ----

const catCols = "id, tenant_id, parent_id, name, path, description, depth, sort_order, " +
	"document_count, subcategory_count, created_by, created_at, updated_at"

func scanCategory(row pgx.Row) (Category, error) {
	var c Category
	err := row.Scan(&c.ID, &c.TenantID, &c.ParentID, &c.Name, &c.Path, &c.Description, &c.Depth,
		&c.SortOrder, &c.DocumentCount, &c.SubcategoryCount, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Category{}, notFound(err)
	}
	return c, nil
}

func scanCategories(rows pgx.Rows) ([]Category, error) {
	var out []Category
	for rows.Next() {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// InsertCategory stores a category node.
func InsertCategory(ctx context.Context, tx pgx.Tx, c Category) error {
	_, err := tx.Exec(ctx, `INSERT INTO paperless_categories
		(id, tenant_id, parent_id, name, path, description, depth, sort_order, document_count, subcategory_count, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		c.ID, c.TenantID, c.ParentID, c.Name, c.Path, c.Description, c.Depth, c.SortOrder,
		c.DocumentCount, c.SubcategoryCount, c.CreatedBy)
	return conflict(err)
}

// GetCategory by tenant + id.
func GetCategory(ctx context.Context, tx pgx.Tx, tenantID, id string) (Category, error) {
	return scanCategory(tx.QueryRow(ctx, "SELECT "+catCols+" FROM paperless_categories WHERE tenant_id=$1 AND id=$2", tenantID, id))
}

// ListCategories returns a tenant's categories ordered by path.
func ListCategories(ctx context.Context, tx pgx.Tx, tenantID string) ([]Category, error) {
	rows, err := tx.Query(ctx, "SELECT "+catCols+" FROM paperless_categories WHERE tenant_id=$1 ORDER BY path", tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCategories(rows)
}

// ListSubtree returns the category at pathPrefix and all its descendants.
func ListSubtree(ctx context.Context, tx pgx.Tx, tenantID, pathPrefix string) ([]Category, error) {
	rows, err := tx.Query(ctx, `SELECT `+catCols+` FROM paperless_categories
		WHERE tenant_id=$1 AND (path = $2 OR path LIKE $2 || '/%') ORDER BY path`, tenantID, pathPrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCategories(rows)
}

// UpdateCategory rewrites the mutable columns.
func UpdateCategory(ctx context.Context, tx pgx.Tx, c Category) error {
	ct, err := tx.Exec(ctx, `UPDATE paperless_categories SET parent_id=$3, name=$4, path=$5, description=$6,
		depth=$7, sort_order=$8, document_count=$9, subcategory_count=$10, updated_at=now()
		WHERE tenant_id=$1 AND id=$2`,
		c.TenantID, c.ID, c.ParentID, c.Name, c.Path, c.Description, c.Depth, c.SortOrder,
		c.DocumentCount, c.SubcategoryCount)
	if err != nil {
		return conflict(err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCategory removes a category (children cascade in the schema).
func DeleteCategory(ctx context.Context, tx pgx.Tx, tenantID, id string) error {
	ct, err := tx.Exec(ctx, "DELETE FROM paperless_categories WHERE tenant_id=$1 AND id=$2", tenantID, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- permissions ----

const permCols = "id, tenant_id, resource_type, resource_id, subject_type, subject_id, relation, " +
	"COALESCE(granted_by, ''), granted_at, expires_at"

func scanPerm(row pgx.Row) (PermissionTuple, error) {
	var p PermissionTuple
	err := row.Scan(&p.ID, &p.TenantID, &p.ResourceType, &p.ResourceID, &p.SubjectType, &p.SubjectID,
		&p.Relation, &p.GrantedBy, &p.GrantedAt, &p.ExpiresAt)
	if err != nil {
		return PermissionTuple{}, notFound(err)
	}
	return p, nil
}

func scanPerms(rows pgx.Rows) ([]PermissionTuple, error) {
	var out []PermissionTuple
	for rows.Next() {
		p, err := scanPerm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// InsertPermission stores an access tuple.
func InsertPermission(ctx context.Context, tx pgx.Tx, p PermissionTuple) error {
	var gat *time.Time
	if !p.GrantedAt.IsZero() {
		gat = &p.GrantedAt
	}
	_, err := tx.Exec(ctx, `INSERT INTO paperless_permissions
		(id, tenant_id, resource_type, resource_id, subject_type, subject_id, relation, granted_by, granted_at, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,COALESCE($9, now()),$10)`,
		p.ID, p.TenantID, p.ResourceType, p.ResourceID, p.SubjectType, p.SubjectID, p.Relation,
		p.GrantedBy, gat, p.ExpiresAt)
	return conflict(err)
}

// DeletePermission removes an access tuple.
func DeletePermission(ctx context.Context, tx pgx.Tx, tenantID, id string) error {
	ct, err := tx.Exec(ctx, "DELETE FROM paperless_permissions WHERE tenant_id=$1 AND id=$2", tenantID, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPermissionsByResource returns every tuple on a resource.
func ListPermissionsByResource(ctx context.Context, tx pgx.Tx, tenantID, resourceType, resourceID string) ([]PermissionTuple, error) {
	rows, err := tx.Query(ctx, "SELECT "+permCols+` FROM paperless_permissions
		WHERE tenant_id=$1 AND resource_type=$2 AND resource_id=$3 ORDER BY granted_at`, tenantID, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPerms(rows)
}

// ListPermissionsBySubject returns every tuple held by a subject.
func ListPermissionsBySubject(ctx context.Context, tx pgx.Tx, tenantID, subjectType, subjectID string) ([]PermissionTuple, error) {
	rows, err := tx.Query(ctx, "SELECT "+permCols+` FROM paperless_permissions
		WHERE tenant_id=$1 AND subject_type=$2 AND subject_id=$3 ORDER BY granted_at`, tenantID, subjectType, subjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPerms(rows)
}

// GrantsForSubjects returns all non-expired tuples that could apply to the
// subject: its user tuples, its role tuples, and tenant-wide grants.
func GrantsForSubjects(ctx context.Context, tx pgx.Tx, tenantID, userID string, roles []string, now time.Time) ([]PermissionTuple, error) {
	rows, err := tx.Query(ctx, "SELECT "+permCols+` FROM paperless_permissions
		WHERE tenant_id=$1 AND (expires_at IS NULL OR expires_at > $2)
		AND (
			(subject_type='`+SubjectUser+`' AND subject_id=$3)
			OR (subject_type='`+SubjectRole+`' AND subject_id = ANY($4))
			OR subject_type='`+SubjectTenant+`'
		) ORDER BY granted_at`, tenantID, now, userID, roles)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPerms(rows)
}

// ---- processing jobs ----

const jobCols = "id, tenant_id, document_id, status, retry_count, max_retries, lease_until, " +
	"next_retry_at, result, started_at, completed_at, created_at, updated_at"

func scanJob(row pgx.Row) (ProcessingJob, error) {
	var j ProcessingJob
	err := row.Scan(&j.ID, &j.TenantID, &j.DocumentID, &j.Status, &j.RetryCount, &j.MaxRetries,
		&j.LeaseUntil, &j.NextRetryAt, &j.Result, &j.StartedAt, &j.CompletedAt, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return ProcessingJob{}, notFound(err)
	}
	return j, nil
}

func scanJobs(rows pgx.Rows) ([]ProcessingJob, error) {
	var out []ProcessingJob
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// InsertJob stores a processing job.
func InsertJob(ctx context.Context, tx pgx.Tx, j ProcessingJob) error {
	_, err := tx.Exec(ctx, `INSERT INTO paperless_jobs
		(id, tenant_id, document_id, status, retry_count, max_retries, lease_until, next_retry_at, result, started_at, completed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		j.ID, j.TenantID, j.DocumentID, defaultStr(j.Status, ProcPending), j.RetryCount, defaultInt(j.MaxRetries, 3),
		j.LeaseUntil, j.NextRetryAt, j.Result, j.StartedAt, j.CompletedAt)
	return err
}

// GetJob by tenant + id.
func GetJob(ctx context.Context, tx pgx.Tx, tenantID, id string) (ProcessingJob, error) {
	return scanJob(tx.QueryRow(ctx, "SELECT "+jobCols+" FROM paperless_jobs WHERE tenant_id=$1 AND id=$2", tenantID, id))
}

// UpdateJob rewrites the mutable columns.
func UpdateJob(ctx context.Context, tx pgx.Tx, j ProcessingJob) error {
	ct, err := tx.Exec(ctx, `UPDATE paperless_jobs SET status=$3, retry_count=$4, max_retries=$5, lease_until=$6,
		next_retry_at=$7, result=$8, started_at=$9, completed_at=$10, updated_at=now()
		WHERE tenant_id=$1 AND id=$2`,
		j.TenantID, j.ID, j.Status, j.RetryCount, j.MaxRetries, j.LeaseUntil, j.NextRetryAt, j.Result,
		j.StartedAt, j.CompletedAt)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimDueJobs leases due pending/retrying jobs (system scope), one winner each.
func ClaimDueJobs(ctx context.Context, tx pgx.Tx, now time.Time, lease time.Duration, limit int) ([]ProcessingJob, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := tx.Query(ctx, `WITH due AS (
			SELECT id FROM paperless_jobs
			WHERE status IN ('`+ProcPending+`','`+ProcRetrying+`')
			AND (next_retry_at IS NULL OR next_retry_at <= $1)
			AND (lease_until IS NULL OR lease_until <= $1)
			ORDER BY created_at LIMIT $3 FOR UPDATE SKIP LOCKED)
		UPDATE paperless_jobs j SET status='`+ProcProcessing+`', lease_until=$2, updated_at=now()
		FROM due WHERE j.id=due.id RETURNING `+jobColsPrefixed("j"), now, now.Add(lease), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanJobs(rows)
}

// DeleteJobsOlderThan prunes jobs created before cutoff (system scope).
func DeleteJobsOlderThan(ctx context.Context, tx pgx.Tx, cutoff time.Time) (int, error) {
	ct, err := tx.Exec(ctx, "DELETE FROM paperless_jobs WHERE created_at < $1", cutoff)
	if err != nil {
		return 0, err
	}
	return int(ct.RowsAffected()), nil
}

func jobColsPrefixed(alias string) string {
	cols := []string{"id", "tenant_id", "document_id", "status", "retry_count", "max_retries", "lease_until",
		"next_retry_at", "result", "started_at", "completed_at", "created_at", "updated_at"}
	out := ""
	for i, c := range cols {
		if i > 0 {
			out += ", "
		}
		out += alias + "." + c
	}
	return out
}

// ---- audit ----

// InsertAuditRows batch-inserts audit entries (system scope).
func InsertAuditRows(ctx context.Context, tx pgx.Tx, rows []AuditRow) error {
	for _, r := range rows {
		if _, err := tx.Exec(ctx, `INSERT INTO paperless_audit_events
			(ts, tenant_id, event_type, actor_kind, actor_id, subject_kind, subject_id, outcome, reason, correlation_id, details)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			r.TS, r.TenantID, r.EventType, r.ActorKind, r.ActorID, r.SubjectKind, r.SubjectID, r.Outcome,
			r.Reason, r.CorrelationID, r.Details); err != nil {
			return err
		}
	}
	return nil
}

// ---- existence (authz) ----

// Exists reports whether a resource exists in the tenant.
func Exists(ctx context.Context, tx pgx.Tx, tenantID, table, id string) (bool, error) {
	var one int
	err := tx.QueryRow(ctx, "SELECT 1 FROM "+table+" WHERE tenant_id=$1 AND id=$2", tenantID, id).Scan(&one)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// TenantIDs returns every distinct tenant across documents and categories.
// Runs under a system-scoped transaction so RLS admits the cross-tenant read.
func TenantIDs(ctx context.Context, tx pgx.Tx) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT tenant_id FROM paperless_documents
		UNION SELECT tenant_id FROM paperless_categories`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

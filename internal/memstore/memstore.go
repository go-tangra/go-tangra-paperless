// Package memstore is an in-memory repo.Store for tests and dev. It filters by
// tenant (mirroring RLS), supports the full paperless surface, and offers simple
// error injection.
package memstore

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// Mem is an in-memory store.
type Mem struct {
	docs  map[string]store.Document
	cats  map[string]store.Category
	perms map[string]store.PermissionTuple
	jobs  map[string]store.ProcessingJob
	audit []store.AuditRow
	fail  error // injected: next mutating op fails
	Now   func() time.Time
}

// New builds an empty store.
func New() *Mem {
	return &Mem{
		docs: map[string]store.Document{}, cats: map[string]store.Category{},
		perms: map[string]store.PermissionTuple{}, jobs: map[string]store.ProcessingJob{},
		Now: time.Now,
	}
}

// FailNext injects an error into the next mutating operation.
func (m *Mem) FailNext(err error) { m.fail = err }

func (m *Mem) take() error {
	if m.fail != nil {
		e := m.fail
		m.fail = nil
		return e
	}
	return nil
}

var errNotFound = store.ErrNotFound

// Atomic runs fn against the same store (no real transaction).
func (m *Mem) Atomic(ctx context.Context, _ string, fn func(tx repo.Store) error) error {
	return fn(m)
}

func (m *Mem) Close() {}

// ---- Documents

func (m *Mem) InsertDocument(_ context.Context, d store.Document) error {
	if err := m.take(); err != nil {
		return err
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = m.Now()
	}
	d.UpdatedAt = m.Now()
	m.docs[d.ID] = d
	return nil
}

func (m *Mem) GetDocument(_ context.Context, tenantID, id string) (store.Document, error) {
	d, ok := m.docs[id]
	if !ok || d.TenantID != tenantID {
		return store.Document{}, errNotFound
	}
	return d, nil
}

func (m *Mem) ListDocuments(_ context.Context, tenantID string, f repo.DocFilter) ([]store.Document, error) {
	var out []store.Document
	for _, d := range m.docs {
		if d.TenantID != tenantID {
			continue
		}
		if f.Status != "" && d.Status != f.Status {
			continue
		}
		if f.Status == "" && d.Status == store.DocDeleted {
			continue // hide soft-deleted from normal listings
		}
		if f.CategoryID != "" && (d.CategoryID == nil || *d.CategoryID != f.CategoryID) {
			continue
		}
		if f.MimeType != "" && d.MimeType != f.MimeType {
			continue
		}
		if f.Source != "" && d.Source != f.Source {
			continue
		}
		if f.ProcessingStatus != "" && d.ProcessingStatus != f.ProcessingStatus {
			continue
		}
		if f.CreatedBy != "" && d.CreatedBy != f.CreatedBy {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (m *Mem) UpdateDocument(_ context.Context, d store.Document) error {
	if err := m.take(); err != nil {
		return err
	}
	old, ok := m.docs[d.ID]
	if !ok || old.TenantID != d.TenantID {
		return errNotFound
	}
	d.CreatedAt = old.CreatedAt
	d.UpdatedAt = m.Now()
	m.docs[d.ID] = d
	return nil
}

func (m *Mem) SetDocumentProcessing(_ context.Context, tenantID, id, status, content string, meta map[string]string) error {
	d, ok := m.docs[id]
	if !ok || d.TenantID != tenantID {
		return errNotFound
	}
	d.ProcessingStatus = status
	d.ContentText = content
	if meta != nil {
		d.ExtractedMetadata = meta
	}
	d.UpdatedAt = m.Now()
	m.docs[id] = d
	return nil
}

func (m *Mem) DeleteDocument(_ context.Context, tenantID, id string) error {
	d, ok := m.docs[id]
	if !ok || d.TenantID != tenantID {
		return errNotFound
	}
	delete(m.docs, id)
	return nil
}

func (m *Mem) SearchDocuments(_ context.Context, tenantID, query string, accessible []string, all bool, limit int) ([]store.SearchResult, error) {
	acc := map[string]bool{}
	for _, id := range accessible {
		acc[id] = true
	}
	q := strings.ToLower(strings.TrimSpace(query))
	var out []store.SearchResult
	for _, d := range m.docs {
		if d.TenantID != tenantID || d.Status == store.DocDeleted {
			continue
		}
		if !all && !acc[d.ID] {
			continue
		}
		hay := strings.ToLower(d.Name + " " + d.Description + " " + d.ContentText)
		if q != "" && !strings.Contains(hay, q) {
			continue
		}
		out = append(out, store.SearchResult{Document: d, Rank: 1, Snippet: snippet(d.ContentText, q)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Document.CreatedAt.After(out[j].Document.CreatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func snippet(s, q string) string {
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// ---- Categories

func (m *Mem) InsertCategory(_ context.Context, c store.Category) error {
	if err := m.take(); err != nil {
		return err
	}
	for _, x := range m.cats {
		if x.TenantID == c.TenantID && eqPtr(x.ParentID, c.ParentID) && x.Name == c.Name {
			return store.ErrConflict
		}
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = m.Now()
	}
	c.UpdatedAt = m.Now()
	m.cats[c.ID] = c
	return nil
}

func (m *Mem) GetCategory(_ context.Context, tenantID, id string) (store.Category, error) {
	c, ok := m.cats[id]
	if !ok || c.TenantID != tenantID {
		return store.Category{}, errNotFound
	}
	return c, nil
}

func (m *Mem) ListCategories(_ context.Context, tenantID string) ([]store.Category, error) {
	var out []store.Category
	for _, c := range m.cats {
		if c.TenantID == tenantID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (m *Mem) ListSubtree(_ context.Context, tenantID, prefix string) ([]store.Category, error) {
	var out []store.Category
	for _, c := range m.cats {
		if c.TenantID == tenantID && (c.Path == prefix || strings.HasPrefix(c.Path, prefix+"/")) {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (m *Mem) UpdateCategory(_ context.Context, c store.Category) error {
	if err := m.take(); err != nil {
		return err
	}
	old, ok := m.cats[c.ID]
	if !ok || old.TenantID != c.TenantID {
		return errNotFound
	}
	c.CreatedAt = old.CreatedAt
	c.UpdatedAt = m.Now()
	m.cats[c.ID] = c
	return nil
}

func (m *Mem) DeleteCategory(_ context.Context, tenantID, id string) error {
	c, ok := m.cats[id]
	if !ok || c.TenantID != tenantID {
		return errNotFound
	}
	delete(m.cats, id)
	return nil
}

// ---- Permissions

func (m *Mem) InsertPermission(_ context.Context, p store.PermissionTuple) error {
	if err := m.take(); err != nil {
		return err
	}
	if p.GrantedAt.IsZero() {
		p.GrantedAt = m.Now()
	}
	m.perms[p.ID] = p
	return nil
}

func (m *Mem) DeletePermission(_ context.Context, tenantID, id string) error {
	p, ok := m.perms[id]
	if !ok || p.TenantID != tenantID {
		return errNotFound
	}
	delete(m.perms, id)
	return nil
}

func (m *Mem) ListPermissionsByResource(_ context.Context, tenantID, rt, rid string) ([]store.PermissionTuple, error) {
	var out []store.PermissionTuple
	for _, p := range m.perms {
		if p.TenantID == tenantID && p.ResourceType == rt && p.ResourceID == rid {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *Mem) ListPermissionsBySubject(_ context.Context, tenantID, st, sid string) ([]store.PermissionTuple, error) {
	var out []store.PermissionTuple
	for _, p := range m.perms {
		if p.TenantID == tenantID && p.SubjectType == st && p.SubjectID == sid {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *Mem) GrantsForSubjects(_ context.Context, tenantID, userID string, roles []string, now time.Time) ([]store.PermissionTuple, error) {
	roleset := map[string]bool{}
	for _, r := range roles {
		roleset[r] = true
	}
	var out []store.PermissionTuple
	for _, p := range m.perms {
		if p.TenantID != tenantID {
			continue
		}
		if p.ExpiresAt != nil && !p.ExpiresAt.After(now) {
			continue
		}
		switch p.SubjectType {
		case store.SubjectUser:
			if p.SubjectID == userID {
				out = append(out, p)
			}
		case store.SubjectRole:
			if roleset[p.SubjectID] {
				out = append(out, p)
			}
		case store.SubjectTenant:
			out = append(out, p)
		}
	}
	return out, nil
}

// ---- Jobs

func (m *Mem) InsertJob(_ context.Context, j store.ProcessingJob) error {
	if err := m.take(); err != nil {
		return err
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = m.Now()
	}
	j.UpdatedAt = m.Now()
	m.jobs[j.ID] = j
	return nil
}

func (m *Mem) GetJob(_ context.Context, tenantID, id string) (store.ProcessingJob, error) {
	j, ok := m.jobs[id]
	if !ok || j.TenantID != tenantID {
		return store.ProcessingJob{}, errNotFound
	}
	return j, nil
}

func (m *Mem) UpdateJob(_ context.Context, j store.ProcessingJob) error {
	old, ok := m.jobs[j.ID]
	if !ok || old.TenantID != j.TenantID {
		return errNotFound
	}
	j.CreatedAt = old.CreatedAt
	j.UpdatedAt = m.Now()
	m.jobs[j.ID] = j
	return nil
}

func (m *Mem) ClaimDueJobs(_ context.Context, now time.Time, lease time.Duration, limit int) ([]store.ProcessingJob, error) {
	var out []store.ProcessingJob
	for id, j := range m.jobs {
		if j.Status != store.ProcPending && j.Status != store.ProcRetrying {
			continue
		}
		if j.NextRetryAt != nil && j.NextRetryAt.After(now) {
			continue
		}
		if j.LeaseUntil != nil && j.LeaseUntil.After(now) {
			continue
		}
		u := now.Add(lease)
		j.LeaseUntil = &u
		j.Status = store.ProcProcessing
		m.jobs[id] = j
		out = append(out, j)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (m *Mem) DeleteJobsOlderThan(_ context.Context, cutoff time.Time) (int, error) {
	n := 0
	for id, j := range m.jobs {
		if j.CreatedAt.Before(cutoff) {
			delete(m.jobs, id)
			n++
		}
	}
	return n, nil
}

// ---- Audit / misc

func (m *Mem) InsertAuditRows(_ context.Context, rows []store.AuditRow) error {
	m.audit = append(m.audit, rows...)
	return nil
}

func (m *Mem) Exists(_ context.Context, tenantID, rt, id string) (bool, error) {
	switch rt {
	case store.ResourceDocument:
		d, ok := m.docs[id]
		return ok && d.TenantID == tenantID, nil
	case store.ResourceCategory:
		c, ok := m.cats[id]
		return ok && c.TenantID == tenantID, nil
	}
	return false, nil
}

func (m *Mem) TenantIDs(_ context.Context) ([]string, error) {
	set := map[string]bool{}
	for _, d := range m.docs {
		set[d.TenantID] = true
	}
	for _, c := range m.cats {
		set[c.TenantID] = true
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func eqPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

var _ repo.Store = (*Mem)(nil)
var _ = errors.New

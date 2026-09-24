// Package stats computes paperless statistics for the dashboard and reporting:
// documents by status, source, and MIME type; total and per-category storage
// usage; category counts; the processing backlog (pending/processing/failed);
// and — for administrators — a system-wide aggregate with a per-tenant
// breakdown. It reads through the repo interface; the concrete store enforces
// per-tenant RLS.
package stats

import (
	"context"
	"sort"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// uncategorized keys storage for documents with no category.
const uncategorized = "uncategorized"

// Snapshot is the statistics for one scope (a tenant, or the whole system).
type Snapshot struct {
	DocumentsByStatus map[string]int   `json:"documents_by_status"`
	DocumentsBySource map[string]int   `json:"documents_by_source"`
	DocumentsByMime   map[string]int   `json:"documents_by_mime"`
	StorageBytes      int64            `json:"storage_bytes"`
	StorageByCategory map[string]int64 `json:"storage_by_category"`
	CategoriesTotal   int              `json:"categories_total"`
	DocumentsTotal    int              `json:"documents_total"`
	Backlog           map[string]int   `json:"backlog"`
}

// TenantSnapshot ties a snapshot to its tenant (system breakdown).
type TenantSnapshot struct {
	TenantID string   `json:"tenant_id"`
	Snapshot Snapshot `json:"snapshot"`
}

// System is the administrator's system-wide view.
type System struct {
	Snapshot  Snapshot         `json:"snapshot"`
	PerTenant []TenantSnapshot `json:"per_tenant"`
}

// Service computes statistics.
type Service struct {
	st repo.Store
}

// New builds the service.
func New(st repo.Store) *Service { return &Service{st: st} }

// Tenant returns the caller's tenant statistics.
func (s *Service) Tenant(ctx context.Context, subj authz.Subjects) (Snapshot, error) {
	return s.snapshot(ctx, subj.TenantID)
}

// SystemWide returns the system aggregate plus a per-tenant breakdown. Admin
// only — non-admins get ErrForbidden.
func (s *Service) SystemWide(ctx context.Context, subj authz.Subjects) (System, error) {
	if !subj.IsAdmin() {
		return System{}, authz.ErrForbidden
	}
	ids, err := s.st.TenantIDs(ctx)
	if err != nil {
		return System{}, err
	}
	sort.Strings(ids)
	out := System{Snapshot: emptySnapshot(), PerTenant: []TenantSnapshot{}}
	for _, id := range ids {
		snap, err := s.snapshot(ctx, id)
		if err != nil {
			return System{}, err
		}
		out.PerTenant = append(out.PerTenant, TenantSnapshot{TenantID: id, Snapshot: snap})
		merge(&out.Snapshot, snap)
	}
	return out, nil
}

func (s *Service) snapshot(ctx context.Context, tenantID string) (Snapshot, error) {
	snap := emptySnapshot()

	docs, err := s.st.ListDocuments(ctx, tenantID, repo.DocFilter{})
	if err != nil {
		return snap, err
	}
	for _, d := range docs {
		snap.DocumentsTotal++
		snap.DocumentsByStatus[d.Status]++
		snap.DocumentsBySource[d.Source]++
		snap.DocumentsByMime[d.MimeType]++
		snap.StorageBytes += d.FileSize
		snap.StorageByCategory[categoryKey(d)] += d.FileSize
		switch d.ProcessingStatus {
		case store.ProcPending, store.ProcProcessing, store.ProcFailed:
			snap.Backlog[d.ProcessingStatus]++
		}
	}

	cats, err := s.st.ListCategories(ctx, tenantID)
	if err != nil {
		return snap, err
	}
	snap.CategoriesTotal = len(cats)

	return snap, nil
}

// categoryKey is the per-category storage bucket for a document: its category id,
// or "uncategorized" when unfiled.
func categoryKey(d store.Document) string {
	if d.CategoryID != nil && *d.CategoryID != "" {
		return *d.CategoryID
	}
	return uncategorized
}

func emptySnapshot() Snapshot {
	return Snapshot{
		DocumentsByStatus: map[string]int{},
		DocumentsBySource: map[string]int{},
		DocumentsByMime:   map[string]int{},
		StorageByCategory: map[string]int64{},
		Backlog:           map[string]int{},
	}
}

func merge(dst *Snapshot, src Snapshot) {
	dst.DocumentsTotal += src.DocumentsTotal
	dst.CategoriesTotal += src.CategoriesTotal
	dst.StorageBytes += src.StorageBytes
	for k, v := range src.DocumentsByStatus {
		dst.DocumentsByStatus[k] += v
	}
	for k, v := range src.DocumentsBySource {
		dst.DocumentsBySource[k] += v
	}
	for k, v := range src.DocumentsByMime {
		dst.DocumentsByMime[k] += v
	}
	for k, v := range src.StorageByCategory {
		dst.StorageByCategory[k] += v
	}
	for k, v := range src.Backlog {
		dst.Backlog[k] += v
	}
}

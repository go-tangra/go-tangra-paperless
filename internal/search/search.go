// Package search runs tenant-scoped full-text search over documents, restricted
// to the documents the caller may read (direct or inherited permission). It
// never returns extracted content beyond a highlighted snippet.
package search

import (
	"context"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
)

// Service runs full-text search.
type Service struct {
	st repo.Store
	az *authz.Authorizer
}

// New builds the service.
func New(st repo.Store, az *authz.Authorizer) *Service { return &Service{st: st, az: az} }

// Hit is one search result (no full content — only a snippet).
type Hit struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	CategoryID   string  `json:"category_id,omitempty"`
	CategoryPath string  `json:"category_path,omitempty"`
	MimeType     string  `json:"mime_type"`
	Status       string  `json:"status"`
	Rank         float64 `json:"rank"`
	Snippet      string  `json:"snippet"`
}

// Search returns ranked, permission-filtered matches for query.
func (s *Service) Search(ctx context.Context, subj authz.Subjects, query string, limit int) ([]Hit, error) {
	ids, all, err := s.az.ListAccessible(ctx, subj, authz.Document)
	if err != nil {
		return nil, err
	}
	accessible := make([]string, 0, len(ids))
	for id := range ids {
		accessible = append(accessible, id)
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.st.SearchDocuments(ctx, subj.TenantID, query, accessible, all, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Hit, 0, len(rows))
	for _, r := range rows {
		d := r.Document
		cat := ""
		if d.CategoryID != nil {
			cat = *d.CategoryID
		}
		out = append(out, Hit{
			ID: d.ID, Name: d.Name, CategoryID: cat, CategoryPath: d.CategoryPath,
			MimeType: d.MimeType, Status: d.Status, Rank: r.Rank, Snippet: r.Snippet,
		})
	}
	return out, nil
}

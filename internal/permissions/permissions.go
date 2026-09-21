// Package permissions is the sharing service: it grants and revokes access to
// documents and categories and answers access questions, delegating the
// effective-permission maths (including category->document inheritance) to the
// authz engine.
package permissions

import (
	"context"
	"errors"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// ErrNotFound is returned when a grant is absent.
var ErrNotFound = errors.New("permissions: not found")

// Service manages permission tuples.
type Service struct {
	st repo.Store
	az *authz.Authorizer
}

// New builds the service.
func New(st repo.Store, az *authz.Authorizer) *Service { return &Service{st: st, az: az} }

// GrantInput describes a new grant.
type GrantInput struct {
	ResourceType string
	ResourceID   string
	SubjectType  string
	SubjectID    string
	Relation     string
	ExpiresAt    *time.Time
}

// Tuple is a grant as returned to clients.
type Tuple struct {
	ID           string     `json:"id"`
	ResourceType string     `json:"resource_type"`
	ResourceID   string     `json:"resource_id"`
	SubjectType  string     `json:"subject_type"`
	SubjectID    string     `json:"subject_id"`
	Relation     string     `json:"relation"`
	GrantedBy    string     `json:"granted_by,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

func valid(rt, st_, rel string) bool {
	okR := rt == store.ResourceDocument || rt == store.ResourceCategory
	okS := st_ == store.SubjectUser || st_ == store.SubjectRole || st_ == store.SubjectTenant
	okRel := rel == store.RelationOwner || rel == store.RelationEditor || rel == store.RelationViewer || rel == store.RelationSharer
	return okR && okS && okRel
}

// Grant shares a resource with a subject. The caller must be able to share it.
func (s *Service) Grant(ctx context.Context, subj authz.Subjects, in GrantInput) (Tuple, error) {
	if !valid(in.ResourceType, in.SubjectType, in.Relation) {
		return Tuple{}, errors.New("permissions: invalid grant")
	}
	if err := s.az.Check(ctx, subj, in.ResourceType, in.ResourceID, authz.Share); err != nil {
		return Tuple{}, err
	}
	t := store.PermissionTuple{
		ID: store.NewID(), TenantID: subj.TenantID, ResourceType: in.ResourceType, ResourceID: in.ResourceID,
		SubjectType: in.SubjectType, SubjectID: in.SubjectID, Relation: in.Relation, GrantedBy: subj.ActorID(),
		ExpiresAt: in.ExpiresAt,
	}
	if err := s.st.InsertPermission(ctx, t); err != nil {
		return Tuple{}, err
	}
	return view(t), nil
}

// Revoke removes a grant. The caller must be able to share the resource.
func (s *Service) Revoke(ctx context.Context, subj authz.Subjects, resourceType, resourceID, id string) error {
	if err := s.az.Check(ctx, subj, resourceType, resourceID, authz.Share); err != nil {
		return err
	}
	if err := s.st.DeletePermission(ctx, subj.TenantID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// ListForResource lists a resource's grants (caller must read the resource).
func (s *Service) ListForResource(ctx context.Context, subj authz.Subjects, resourceType, resourceID string) ([]Tuple, error) {
	if err := s.az.Check(ctx, subj, resourceType, resourceID, authz.Read); err != nil {
		return nil, err
	}
	rows, err := s.st.ListPermissionsByResource(ctx, subj.TenantID, resourceType, resourceID)
	if err != nil {
		return nil, err
	}
	return views(rows), nil
}

// Check answers whether the caller holds an action on a resource.
func (s *Service) Check(ctx context.Context, subj authz.Subjects, resourceType, resourceID, action string) (bool, error) {
	err := s.az.Check(ctx, subj, resourceType, resourceID, action)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, authz.ErrForbidden) || errors.Is(err, authz.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// Effective returns the caller's effective permissions on a resource plus the
// contributing grants.
func (s *Service) Effective(ctx context.Context, subj authz.Subjects, resourceType, resourceID string) (authz.Permissions, []Tuple, error) {
	perms, err := s.az.Effective(ctx, subj, resourceType, resourceID)
	if err != nil {
		return authz.Permissions{}, nil, err
	}
	rows, _ := s.st.ListPermissionsByResource(ctx, subj.TenantID, resourceType, resourceID)
	return perms, views(rows), nil
}

// ListAccessible returns the resource ids the caller may read.
func (s *Service) ListAccessible(ctx context.Context, subj authz.Subjects, resourceType string) ([]string, bool, error) {
	ids, all, err := s.az.ListAccessible(ctx, subj, resourceType)
	if err != nil {
		return nil, false, err
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	return out, all, nil
}

func view(t store.PermissionTuple) Tuple {
	return Tuple{ID: t.ID, ResourceType: t.ResourceType, ResourceID: t.ResourceID, SubjectType: t.SubjectType,
		SubjectID: t.SubjectID, Relation: t.Relation, GrantedBy: t.GrantedBy, ExpiresAt: t.ExpiresAt}
}
func views(ts []store.PermissionTuple) []Tuple {
	out := make([]Tuple, 0, len(ts))
	for _, t := range ts {
		out = append(out, view(t))
	}
	return out
}

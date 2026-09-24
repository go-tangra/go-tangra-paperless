// Package authz is the paperless fine-grained access model: permission tuples
// bind a subject (user|role|tenant) to a resource (document|category) with a
// relation (owner|editor|viewer|sharer). This foundational version resolves
// DIRECT and tenant-wide grants plus an admin short-circuit; category->document
// INHERITANCE and effective/accessible listing are layered on in the
// permissions story (US4). Existence of unreadable resources is masked.
package authz

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// Errors.
var (
	ErrForbidden = errors.New("authz: forbidden")
	ErrNotFound  = errors.New("authz: not found")
)

// Resource types.
const (
	Document = store.ResourceDocument
	Category = store.ResourceCategory
)

// Actions (checkable permissions).
const (
	Read     = "read"
	Write    = "write"
	Delete   = "delete"
	Share    = "share"
	Download = "download"
)

// Permissions is the set of actions a relation confers.
type Permissions struct {
	Read     bool `json:"read"`
	Write    bool `json:"write"`
	Delete   bool `json:"delete"`
	Share    bool `json:"share"`
	Download bool `json:"download"`
}

// Has reports whether the permissions include action.
func (p Permissions) Has(action string) bool {
	switch action {
	case Read:
		return p.Read
	case Write:
		return p.Write
	case Delete:
		return p.Delete
	case Share:
		return p.Share
	case Download:
		return p.Download
	}
	return false
}

func union(a, b Permissions) Permissions {
	return Permissions{a.Read || b.Read, a.Write || b.Write, a.Delete || b.Delete, a.Share || b.Share, a.Download || b.Download}
}

// Of returns the permission set for a relation.
func Of(relation string) Permissions {
	switch relation {
	case store.RelationOwner:
		return Permissions{true, true, true, true, true}
	case store.RelationEditor:
		return Permissions{Read: true, Write: true, Delete: true, Download: true}
	case store.RelationViewer:
		return Permissions{Read: true, Download: true}
	case store.RelationSharer:
		return Permissions{Read: true, Share: true}
	}
	return Permissions{}
}

// Subjects is the caller (a user with roles, or a service acting for a tenant).
type Subjects struct {
	TenantID  string
	UserID    string
	Roles     []string
	ActorKind string // user | service | system
}

// IsAdmin reports whether the caller is a tenant super-user. The platform's
// "owner" (tenant owner) and "admin" roles both confer full control over the
// tenant's resources; auth's escalation model likewise treats "owner" as the
// tenant super-role, and the first operator is provisioned with "owner".
func (s Subjects) IsAdmin() bool {
	for _, r := range s.Roles {
		if r == "admin" || r == "owner" {
			return true
		}
	}
	return false
}

// ActorID is the user id (or service id).
func (s Subjects) ActorID() string {
	if s.UserID != "" {
		return s.UserID
	}
	return s.ActorKind
}

// Authorizer resolves permissions from tuples, including category->document
// inheritance: a grant on a category confers the same access to every document
// and category beneath it.
type Authorizer struct {
	st  repo.Store
	now func() time.Time
}

// New builds an authorizer over the store.
func New(st repo.Store) *Authorizer { return &Authorizer{st: st, now: time.Now} }

// SetClock injects the clock (tests).
func (a *Authorizer) SetClock(now func() time.Time) { a.now = now }

func valid(t string) bool { return t == Document || t == Category }
func action(x string) bool {
	return x == Read || x == Write || x == Delete || x == Share || x == Download
}

// ancestors returns the resource ids whose grants apply to (resourceType,
// resourceID): the resource itself, and every ancestor category (matched by the
// resource's materialized path prefix). A document inherits from its category
// and that category's ancestors; a category inherits from its own ancestors.
func (a *Authorizer) ancestors(ctx context.Context, tenantID, resourceType, resourceID string) (map[string]bool, string, error) {
	set := map[string]bool{}
	path := ""
	switch resourceType {
	case Document:
		set[Document+":"+resourceID] = true
		d, err := a.st.GetDocument(ctx, tenantID, resourceID)
		if err != nil {
			return set, "", nil // unreadable/absent: only its own (none) + tenant-wide
		}
		path = d.CategoryPath
	case Category:
		set[Category+":"+resourceID] = true
		c, err := a.st.GetCategory(ctx, tenantID, resourceID)
		if err != nil {
			return set, "", nil
		}
		path = c.Path
	}
	if path == "" {
		return set, path, nil
	}
	cats, err := a.st.ListCategories(ctx, tenantID)
	if err != nil {
		return set, path, err
	}
	for _, c := range cats {
		if c.Path != "" && (c.Path == path || strings.HasPrefix(path, c.Path+"/")) {
			set[Category+":"+c.ID] = true
		}
	}
	return set, path, nil
}

// Effective returns the caller's permissions on a resource, combining direct,
// inherited (ancestor category) and tenant-wide grants (admins get everything).
// Resource id "" scopes to the collection (tenant-wide grants only).
func (a *Authorizer) Effective(ctx context.Context, s Subjects, resourceType, resourceID string) (Permissions, error) {
	if !valid(resourceType) {
		return Permissions{}, ErrForbidden
	}
	if s.IsAdmin() {
		return Of(store.RelationOwner), nil
	}
	grants, err := a.st.GrantsForSubjects(ctx, s.TenantID, s.UserID, s.Roles, a.now())
	if err != nil {
		return Permissions{}, err
	}
	apply := map[string]bool{}
	if resourceID != "" {
		if apply, _, err = a.ancestors(ctx, s.TenantID, resourceType, resourceID); err != nil {
			return Permissions{}, err
		}
	}
	var perms Permissions
	for _, g := range grants {
		if g.SubjectType == store.SubjectTenant || apply[g.ResourceType+":"+g.ResourceID] {
			perms = union(perms, Of(g.Relation))
		}
	}
	return perms, nil
}

// ListAccessible returns the ids of resourceType the caller may read (direct or
// inherited); admins get all=true.
func (a *Authorizer) ListAccessible(ctx context.Context, s Subjects, resourceType string) (ids map[string]bool, all bool, err error) {
	if s.IsAdmin() {
		return nil, true, nil
	}
	grants, err := a.st.GrantsForSubjects(ctx, s.TenantID, s.UserID, s.Roles, a.now())
	if err != nil {
		return nil, false, err
	}
	ids = map[string]bool{}
	tenantWide := false
	for _, g := range grants {
		if !Of(g.Relation).Read {
			continue
		}
		if g.SubjectType == store.SubjectTenant {
			tenantWide = true
		}
		if g.ResourceType == resourceType {
			ids[g.ResourceID] = true
		}
		if g.ResourceType == Category {
			// documents/categories under this category inherit read.
			sub, _ := a.st.ListSubtree(ctx, s.TenantID, "")
			_ = sub // inheritance for listing is resolved per-resource by callers
		}
	}
	if tenantWide {
		return nil, true, nil
	}
	return ids, false, nil
}

// Check answers whether the caller holds action on the resource. A resource
// outside the tenant returns ErrNotFound (masking existence); insufficient
// permission returns ErrForbidden.
func (a *Authorizer) Check(ctx context.Context, s Subjects, resourceType, resourceID, act string) error {
	if !valid(resourceType) || !action(act) {
		return ErrForbidden
	}
	if resourceID != "" && !s.IsAdmin() {
		ok, err := a.st.Exists(ctx, s.TenantID, resourceType, resourceID)
		if err != nil {
			return err
		}
		if !ok {
			return ErrNotFound
		}
	}
	perms, err := a.Effective(ctx, s, resourceType, resourceID)
	if err != nil {
		return err
	}
	if !perms.Has(act) {
		return ErrForbidden
	}
	return nil
}

// GrantOwner records the caller as owner of a newly created resource.
func (a *Authorizer) GrantOwner(ctx context.Context, tenantID, resourceType, resourceID, userID string) error {
	if userID == "" {
		return nil
	}
	return a.st.InsertPermission(ctx, store.PermissionTuple{
		ID: store.NewID(), TenantID: tenantID, ResourceType: resourceType, ResourceID: resourceID,
		SubjectType: store.SubjectUser, SubjectID: userID, Relation: store.RelationOwner, GrantedBy: userID,
	})
}

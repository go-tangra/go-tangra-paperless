// Package categories is the folder-tree service (US3): CRUD over a tenant's
// category hierarchy with materialized paths. Each node carries a Path
// (e.g. "/Finance/Invoices") and Depth; moving or renaming a node rewrites the
// materialized paths of its whole subtree. Parent/child integrity is tracked in
// SubcategoryCount. Access is mediated by the authz permission model: creating a
// child needs write on the parent, and every new node grants its creator owner.
package categories

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// Errors.
var (
	// ErrNotFound is returned for a missing (or unreadable) category.
	ErrNotFound = errors.New("categories: not found")
	// ErrNotEmpty is returned by Delete for a category with children or
	// documents when cascade was not requested.
	ErrNotEmpty = errors.New("categories: not empty")
	// ErrCycle is returned by Move when the destination is the category itself
	// or one of its descendants.
	ErrCycle = errors.New("categories: cannot move under itself or a descendant")
)

// ValidationError names the offending field.
type ValidationError struct{ Field, Msg string }

func (e *ValidationError) Error() string { return "categories: " + e.Field + ": " + e.Msg }
func invalid(field, msg string) error    { return &ValidationError{Field: field, Msg: msg} }

// Service manages the tenant's category tree.
type Service struct {
	st repo.Store
	az *authz.Authorizer
}

// New builds the service.
func New(st repo.Store, az *authz.Authorizer) *Service { return &Service{st: st, az: az} }

// Input is a create/update request.
type Input struct {
	ParentID    string
	Name        string
	Description string
	SortOrder   int
}

// View is a category as returned to clients.
type View struct {
	ID               string  `json:"id"`
	ParentID         *string `json:"parent_id"`
	Name             string  `json:"name"`
	Path             string  `json:"path"`
	Description      string  `json:"description"`
	Depth            int     `json:"depth"`
	SortOrder        int     `json:"sort_order"`
	DocumentCount    int     `json:"document_count"`
	SubcategoryCount int     `json:"subcategory_count"`
}

// TreeNode is a category plus its ordered children (GetTree).
type TreeNode struct {
	Category View       `json:"category"`
	Children []TreeNode `json:"children"`
}

func toView(c store.Category) View {
	return View{
		ID: c.ID, ParentID: c.ParentID, Name: c.Name, Path: c.Path, Description: c.Description,
		Depth: c.Depth, SortOrder: c.SortOrder, DocumentCount: c.DocumentCount, SubcategoryCount: c.SubcategoryCount,
	}
}

// Create stores a category. With a parent it needs write on that parent; a root
// category needs tenant-wide (member) write. Path and Depth are derived from the
// parent, the caller becomes owner, and the parent's SubcategoryCount is bumped.
func (s *Service) Create(ctx context.Context, subj authz.Subjects, in Input) (View, error) {
	parentID := strings.TrimSpace(in.ParentID)
	if parentID != "" {
		if err := s.az.Check(ctx, subj, authz.Category, parentID, authz.Write); err != nil {
			return View{}, mapNF(err)
		}
	} else if err := s.az.Check(ctx, subj, authz.Category, "", authz.Write); err != nil {
		return View{}, err
	}

	name := strings.TrimSpace(in.Name)
	if l := len(name); l < 1 || l > 200 {
		return View{}, invalid("name", "name must be 1-200 characters")
	}

	var parent store.Category
	haveParent := false
	path := "/" + name
	depth := 0
	var parentPtr *string
	if parentID != "" {
		p, err := s.st.GetCategory(ctx, subj.TenantID, parentID)
		if err != nil {
			return View{}, mapNF(err)
		}
		parent, haveParent = p, true
		path = p.Path + "/" + name
		depth = p.Depth + 1
		pid := p.ID
		parentPtr = &pid
	}

	c := store.Category{
		ID: store.NewID(), TenantID: subj.TenantID, ParentID: parentPtr, Name: name, Path: path,
		Description: in.Description, Depth: depth, SortOrder: in.SortOrder, CreatedBy: subj.ActorID(),
	}
	if err := s.st.InsertCategory(ctx, c); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return View{}, invalid("name", "a category with this name already exists under the parent")
		}
		return View{}, err
	}
	if err := s.az.GrantOwner(ctx, subj.TenantID, authz.Category, c.ID, subj.UserID); err != nil {
		return View{}, err
	}
	if haveParent {
		parent.SubcategoryCount++
		if err := s.st.UpdateCategory(ctx, parent); err != nil {
			return View{}, err
		}
	}
	return toView(c), nil
}

// Get returns a category the caller may read.
func (s *Service) Get(ctx context.Context, subj authz.Subjects, id string) (View, error) {
	if err := s.az.Check(ctx, subj, authz.Category, id, authz.Read); err != nil {
		return View{}, mapNF(err)
	}
	c, err := s.st.GetCategory(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	return toView(c), nil
}

// List returns every category in the tenant.
func (s *Service) List(ctx context.Context, subj authz.Subjects) ([]View, error) {
	if err := s.az.Check(ctx, subj, authz.Category, "", authz.Read); err != nil {
		return nil, err
	}
	rows, err := s.st.ListCategories(ctx, subj.TenantID)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, c := range rows {
		out = append(out, toView(c))
	}
	return out, nil
}

// Update changes a category's name, description, and sort order. Renaming a node
// rewrites its own materialized path and every descendant's.
func (s *Service) Update(ctx context.Context, subj authz.Subjects, id string, in Input) (View, error) {
	if err := s.az.Check(ctx, subj, authz.Category, id, authz.Write); err != nil {
		return View{}, mapNF(err)
	}
	c, err := s.st.GetCategory(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}

	oldName := c.Name
	oldPath := c.Path
	if in.Name != "" {
		c.Name = in.Name
	}
	c.Description = in.Description
	c.SortOrder = in.SortOrder

	if c.Name != oldName {
		parentPath := strings.TrimSuffix(oldPath, "/"+oldName)
		newPath := parentPath + "/" + c.Name
		c.Path = newPath
		sub, err := s.st.ListSubtree(ctx, subj.TenantID, oldPath)
		if err != nil {
			return View{}, err
		}
		for _, d := range sub {
			if d.ID == c.ID {
				continue // self is written below with all its field changes
			}
			d.Path = newPath + strings.TrimPrefix(d.Path, oldPath)
			if err := s.st.UpdateCategory(ctx, d); err != nil {
				return View{}, err
			}
		}
	}
	if err := s.st.UpdateCategory(ctx, c); err != nil {
		return View{}, err
	}
	return toView(c), nil
}

// Delete removes a category. A non-empty category (children or documents)
// requires cascade; with cascade its whole subtree is deleted. The parent's
// SubcategoryCount is decremented.
func (s *Service) Delete(ctx context.Context, subj authz.Subjects, id string, cascade bool) error {
	if err := s.az.Check(ctx, subj, authz.Category, id, authz.Delete); err != nil {
		return mapNF(err)
	}
	c, err := s.st.GetCategory(ctx, subj.TenantID, id)
	if err != nil {
		return mapNF(err)
	}
	if (c.SubcategoryCount > 0 || c.DocumentCount > 0) && !cascade {
		return ErrNotEmpty
	}
	if cascade {
		sub, err := s.st.ListSubtree(ctx, subj.TenantID, c.Path)
		if err != nil {
			return err
		}
		// Deepest paths first so parents outlive their children.
		sort.Slice(sub, func(i, j int) bool { return sub[i].Path > sub[j].Path })
		for _, d := range sub {
			if err := s.st.DeleteCategory(ctx, subj.TenantID, d.ID); err != nil {
				return err
			}
		}
	} else if err := s.st.DeleteCategory(ctx, subj.TenantID, id); err != nil {
		return mapNF(err)
	}
	if c.ParentID != nil {
		if p, err := s.st.GetCategory(ctx, subj.TenantID, *c.ParentID); err == nil {
			if p.SubcategoryCount > 0 {
				p.SubcategoryCount--
			}
			if err := s.st.UpdateCategory(ctx, p); err != nil {
				return err
			}
		}
	}
	return nil
}

// Move reparents a category (empty newParentID moves it to a root). It refuses a
// move under the node itself or any of its descendants (cycle), then rewrites the
// Path and Depth of the node and its whole subtree and fixes both parents' counts.
func (s *Service) Move(ctx context.Context, subj authz.Subjects, id, newParentID string) (View, error) {
	if err := s.az.Check(ctx, subj, authz.Category, id, authz.Write); err != nil {
		return View{}, mapNF(err)
	}
	c, err := s.st.GetCategory(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	oldPath := c.Path
	oldParentID := c.ParentID

	newParentID = strings.TrimSpace(newParentID)
	newPath := "/" + c.Name
	newDepth := 0
	var newParentPtr *string
	var newParent store.Category
	haveNewParent := false
	if newParentID != "" {
		np, err := s.st.GetCategory(ctx, subj.TenantID, newParentID)
		if err != nil {
			return View{}, mapNF(err)
		}
		// Cycle guard: the destination cannot be the node or a descendant.
		if np.ID == c.ID || np.Path == oldPath || strings.HasPrefix(np.Path, oldPath+"/") {
			return View{}, ErrCycle
		}
		newParent, haveNewParent = np, true
		newPath = np.Path + "/" + c.Name
		newDepth = np.Depth + 1
		pid := np.ID
		newParentPtr = &pid
	}

	sub, err := s.st.ListSubtree(ctx, subj.TenantID, oldPath)
	if err != nil {
		return View{}, err
	}
	depthDelta := newDepth - c.Depth
	for _, d := range sub {
		d.Path = newPath + strings.TrimPrefix(d.Path, oldPath)
		d.Depth += depthDelta
		if d.ID == c.ID {
			d.ParentID = newParentPtr
		}
		if err := s.st.UpdateCategory(ctx, d); err != nil {
			return View{}, err
		}
	}

	if oldParentID != nil {
		if p, err := s.st.GetCategory(ctx, subj.TenantID, *oldParentID); err == nil {
			if p.SubcategoryCount > 0 {
				p.SubcategoryCount--
			}
			if err := s.st.UpdateCategory(ctx, p); err != nil {
				return View{}, err
			}
		}
	}
	if haveNewParent {
		newParent.SubcategoryCount++
		if err := s.st.UpdateCategory(ctx, newParent); err != nil {
			return View{}, err
		}
	}

	moved, err := s.st.GetCategory(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	return toView(moved), nil
}

// GetTree returns the tenant's categories as a nested forest, each level ordered
// by SortOrder then Name.
func (s *Service) GetTree(ctx context.Context, subj authz.Subjects) ([]TreeNode, error) {
	if err := s.az.Check(ctx, subj, authz.Category, "", authz.Read); err != nil {
		return nil, err
	}
	rows, err := s.st.ListCategories(ctx, subj.TenantID)
	if err != nil {
		return nil, err
	}
	byParent := map[string][]store.Category{}
	for _, c := range rows {
		key := ""
		if c.ParentID != nil {
			key = *c.ParentID
		}
		byParent[key] = append(byParent[key], c)
	}
	var build func(parentID string) []TreeNode
	build = func(parentID string) []TreeNode {
		kids := byParent[parentID]
		sort.Slice(kids, func(i, j int) bool {
			if kids[i].SortOrder != kids[j].SortOrder {
				return kids[i].SortOrder < kids[j].SortOrder
			}
			return kids[i].Name < kids[j].Name
		})
		out := make([]TreeNode, 0, len(kids))
		for _, k := range kids {
			out = append(out, TreeNode{Category: toView(k), Children: build(k.ID)})
		}
		return out
	}
	return build(""), nil
}

func mapNF(err error) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, authz.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

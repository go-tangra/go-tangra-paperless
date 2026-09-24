package categories_test

import (
	"context"
	"errors"
	"testing"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/categories"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

const tenant = "t-1"

func newSvc(t *testing.T) (*categories.Service, *memstore.Mem) {
	t.Helper()
	mem := memstore.New()
	az := authz.New(mem)
	return categories.New(mem, az), mem
}

func admin() authz.Subjects {
	return authz.Subjects{TenantID: tenant, UserID: "u-admin", Roles: []string{"admin"}, ActorKind: "user"}
}

func mustCreate(t *testing.T, s *categories.Service, parentID, name string) categories.View {
	t.Helper()
	v, err := s.Create(context.Background(), admin(), categories.Input{ParentID: parentID, Name: name})
	if err != nil {
		t.Fatalf("create %q: %v", name, err)
	}
	return v
}

func TestCreateRootAndChild(t *testing.T) {
	ctx := context.Background()
	s, mem := newSvc(t)

	root := mustCreate(t, s, "", "Finance")
	if root.Path != "/Finance" {
		t.Fatalf("root path = %q, want /Finance", root.Path)
	}
	if root.Depth != 0 {
		t.Fatalf("root depth = %d, want 0", root.Depth)
	}
	if root.ParentID != nil {
		t.Fatalf("root parent = %v, want nil", *root.ParentID)
	}

	child := mustCreate(t, s, root.ID, "Invoices")
	if child.Path != "/Finance/Invoices" {
		t.Fatalf("child path = %q, want /Finance/Invoices", child.Path)
	}
	if child.Depth != 1 {
		t.Fatalf("child depth = %d, want 1", child.Depth)
	}
	if child.ParentID == nil || *child.ParentID != root.ID {
		t.Fatalf("child parent = %v, want %s", child.ParentID, root.ID)
	}

	// Parent's SubcategoryCount was incremented.
	got, err := s.Get(ctx, admin(), root.ID)
	if err != nil {
		t.Fatalf("get root: %v", err)
	}
	if got.SubcategoryCount != 1 {
		t.Fatalf("root subcategory count = %d, want 1", got.SubcategoryCount)
	}

	// Owner tuple was written for the creator.
	tuples, err := mem.ListPermissionsByResource(ctx, tenant, store.ResourceCategory, root.ID)
	if err != nil {
		t.Fatalf("list perms: %v", err)
	}
	if len(tuples) != 1 || tuples[0].Relation != store.RelationOwner || tuples[0].SubjectID != "u-admin" {
		t.Fatalf("owner tuple = %+v, want one owner for u-admin", tuples)
	}
}

func TestCreateDuplicateNameConflict(t *testing.T) {
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Finance")
	mustCreate(t, s, root.ID, "Invoices")

	// Same name under the same parent is a conflict.
	_, err := s.Create(context.Background(), admin(), categories.Input{ParentID: root.ID, Name: "Invoices"})
	var ve *categories.ValidationError
	if !errors.As(err, &ve) || ve.Field != "name" {
		t.Fatalf("duplicate child name err = %v, want ValidationError on name", err)
	}

	// Same name under a different parent (another root) is allowed.
	if _, err := s.Create(context.Background(), admin(), categories.Input{Name: "Invoices"}); err != nil {
		t.Fatalf("root Invoices should be allowed: %v", err)
	}
}

func TestGetTreeNesting(t *testing.T) {
	s, _ := newSvc(t)
	// Two roots with children; SortOrder controls ordering.
	b, _ := s.Create(context.Background(), admin(), categories.Input{Name: "Beta", SortOrder: 2})
	a, _ := s.Create(context.Background(), admin(), categories.Input{Name: "Alpha", SortOrder: 1})
	mustCreate(t, s, a.ID, "Child")

	tree, err := s.GetTree(context.Background(), admin())
	if err != nil {
		t.Fatalf("get tree: %v", err)
	}
	if len(tree) != 2 {
		t.Fatalf("roots = %d, want 2", len(tree))
	}
	if tree[0].Category.Name != "Alpha" || tree[1].Category.Name != "Beta" {
		t.Fatalf("root order = %s,%s want Alpha,Beta", tree[0].Category.Name, tree[1].Category.Name)
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("Alpha children = %d, want 1", len(tree[0].Children))
	}
	if got := tree[0].Children[0].Category.Path; got != "/Alpha/Child" {
		t.Fatalf("nested path = %q, want /Alpha/Child", got)
	}
	_ = b
}

func TestMoveRecomputesSubtree(t *testing.T) {
	ctx := context.Background()
	s, _ := newSvc(t)

	root := mustCreate(t, s, "", "Root")
	child := mustCreate(t, s, root.ID, "Child")
	grand := mustCreate(t, s, child.ID, "Grand")
	dest := mustCreate(t, s, "", "Dest")

	moved, err := s.Move(ctx, admin(), child.ID, dest.ID)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if moved.Path != "/Dest/Child" || moved.Depth != 1 {
		t.Fatalf("moved = %q d=%d, want /Dest/Child d=1", moved.Path, moved.Depth)
	}
	if moved.ParentID == nil || *moved.ParentID != dest.ID {
		t.Fatalf("moved parent = %v, want %s", moved.ParentID, dest.ID)
	}

	// Descendant path and depth were rewritten.
	g, err := s.Get(ctx, admin(), grand.ID)
	if err != nil {
		t.Fatalf("get grand: %v", err)
	}
	if g.Path != "/Dest/Child/Grand" || g.Depth != 2 {
		t.Fatalf("grand = %q d=%d, want /Dest/Child/Grand d=2", g.Path, g.Depth)
	}

	// Counts moved from old parent to new parent.
	oldParent, _ := s.Get(ctx, admin(), root.ID)
	if oldParent.SubcategoryCount != 0 {
		t.Fatalf("old parent count = %d, want 0", oldParent.SubcategoryCount)
	}
	newParent, _ := s.Get(ctx, admin(), dest.ID)
	if newParent.SubcategoryCount != 1 {
		t.Fatalf("new parent count = %d, want 1", newParent.SubcategoryCount)
	}
}

func TestMoveRejectsCycle(t *testing.T) {
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")
	child := mustCreate(t, s, root.ID, "Child")

	// Moving a parent under its own child must be refused.
	if _, err := s.Move(context.Background(), admin(), root.ID, child.ID); !errors.Is(err, categories.ErrCycle) {
		t.Fatalf("move under descendant err = %v, want ErrCycle", err)
	}
	// Moving under itself is likewise refused.
	if _, err := s.Move(context.Background(), admin(), root.ID, root.ID); !errors.Is(err, categories.ErrCycle) {
		t.Fatalf("move under self err = %v, want ErrCycle", err)
	}
}

func TestDeleteNonEmptyRequiresCascade(t *testing.T) {
	ctx := context.Background()
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")
	child := mustCreate(t, s, root.ID, "Child")

	// Non-empty without cascade is refused.
	if err := s.Delete(ctx, admin(), root.ID, false); !errors.Is(err, categories.ErrNotEmpty) {
		t.Fatalf("delete non-empty err = %v, want ErrNotEmpty", err)
	}

	// With cascade the whole subtree is removed.
	if err := s.Delete(ctx, admin(), root.ID, true); err != nil {
		t.Fatalf("cascade delete: %v", err)
	}
	if _, err := s.Get(ctx, admin(), root.ID); !errors.Is(err, categories.ErrNotFound) {
		t.Fatalf("root after cascade err = %v, want ErrNotFound", err)
	}
	if _, err := s.Get(ctx, admin(), child.ID); !errors.Is(err, categories.ErrNotFound) {
		t.Fatalf("child after cascade err = %v, want ErrNotFound", err)
	}
}

func TestUpdateRenameRecomputesDescendantPaths(t *testing.T) {
	ctx := context.Background()
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")
	child := mustCreate(t, s, root.ID, "Child")
	grand := mustCreate(t, s, child.ID, "Grand")

	renamed, err := s.Update(ctx, admin(), root.ID, categories.Input{Name: "Renamed", Description: "d"})
	if err != nil {
		t.Fatalf("update rename: %v", err)
	}
	if renamed.Path != "/Renamed" || renamed.Description != "d" {
		t.Fatalf("renamed = %q desc=%q, want /Renamed d", renamed.Path, renamed.Description)
	}

	c, _ := s.Get(ctx, admin(), child.ID)
	if c.Path != "/Renamed/Child" {
		t.Fatalf("child path = %q, want /Renamed/Child", c.Path)
	}
	g, _ := s.Get(ctx, admin(), grand.ID)
	if g.Path != "/Renamed/Child/Grand" {
		t.Fatalf("grand path = %q, want /Renamed/Child/Grand", g.Path)
	}
}

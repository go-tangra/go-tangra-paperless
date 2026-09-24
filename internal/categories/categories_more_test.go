package categories_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/categories"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

func member() authz.Subjects {
	return authz.Subjects{TenantID: tenant, UserID: "member", ActorKind: "user"}
}

func TestValidationError_Message(t *testing.T) {
	s, _ := newSvc(t)
	_, err := s.Create(context.Background(), admin(), categories.Input{Name: ""})
	var ve *categories.ValidationError
	if !errors.As(err, &ve) || ve.Field != "name" {
		t.Fatalf("empty name err = %v, want ValidationError on name", err)
	}
	if msg := ve.Error(); !strings.Contains(msg, "name") {
		t.Fatalf("Error() = %q, want it to mention the field", msg)
	}

	long := strings.Repeat("x", 201)
	if _, err := s.Create(context.Background(), admin(), categories.Input{Name: long}); !errors.As(err, &ve) {
		t.Fatalf("long name err = %v, want ValidationError", err)
	}
}

func TestCreateRoot_ForbiddenForNonMember(t *testing.T) {
	s, _ := newSvc(t)
	if _, err := s.Create(context.Background(), member(), categories.Input{Name: "X"}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("non-member root create err = %v, want ErrForbidden", err)
	}
}

func TestCreateChild_MissingParent(t *testing.T) {
	s, _ := newSvc(t)
	// Admin passes the write check but the parent does not exist.
	if _, err := s.Create(context.Background(), admin(), categories.Input{ParentID: store.NewID(), Name: "C"}); !errors.Is(err, categories.ErrNotFound) {
		t.Fatalf("missing parent err = %v, want ErrNotFound", err)
	}
}

func TestList(t *testing.T) {
	s, _ := newSvc(t)
	mustCreate(t, s, "", "A")
	mustCreate(t, s, "", "B")
	rows, err := s.List(context.Background(), admin())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("List = %d, want 2", len(rows))
	}
	// Forbidden for a non-member.
	if _, err := s.List(context.Background(), member()); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("List non-member err = %v, want ErrForbidden", err)
	}
}

func TestGetTree_Forbidden(t *testing.T) {
	s, _ := newSvc(t)
	if _, err := s.GetTree(context.Background(), member()); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("GetTree non-member err = %v, want ErrForbidden", err)
	}
}

func TestGet_MissingAndForbidden(t *testing.T) {
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")
	if _, err := s.Get(context.Background(), admin(), store.NewID()); !errors.Is(err, categories.ErrNotFound) {
		t.Fatalf("Get missing err = %v, want ErrNotFound", err)
	}
	if _, err := s.Get(context.Background(), member(), root.ID); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Get forbidden err = %v, want ErrForbidden", err)
	}
}

func TestDelete_EmptyLeafDecrementsParent(t *testing.T) {
	ctx := context.Background()
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")
	child := mustCreate(t, s, root.ID, "Child")

	// Delete the empty leaf without cascade.
	if err := s.Delete(ctx, admin(), child.ID, false); err != nil {
		t.Fatalf("delete leaf: %v", err)
	}
	got, _ := s.Get(ctx, admin(), root.ID)
	if got.SubcategoryCount != 0 {
		t.Fatalf("parent count = %d, want 0 after child delete", got.SubcategoryCount)
	}
	// Deleting a missing category -> not found.
	if err := s.Delete(ctx, admin(), store.NewID(), false); !errors.Is(err, categories.ErrNotFound) {
		t.Fatalf("delete missing err = %v, want ErrNotFound", err)
	}
	// Forbidden delete.
	if err := s.Delete(ctx, member(), root.ID, false); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("delete forbidden err = %v, want ErrForbidden", err)
	}
}

func TestDelete_CascadeDecrementsParent(t *testing.T) {
	ctx := context.Background()
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")
	child := mustCreate(t, s, root.ID, "Child")
	mustCreate(t, s, child.ID, "Grand")

	// Cascade-delete the non-empty child; root's count should drop.
	if err := s.Delete(ctx, admin(), child.ID, true); err != nil {
		t.Fatalf("cascade delete child: %v", err)
	}
	got, _ := s.Get(ctx, admin(), root.ID)
	if got.SubcategoryCount != 0 {
		t.Fatalf("root count = %d, want 0 after cascade child delete", got.SubcategoryCount)
	}
}

func TestMove_ToRootRecomputes(t *testing.T) {
	ctx := context.Background()
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")
	child := mustCreate(t, s, root.ID, "Child")
	mustCreate(t, s, child.ID, "Grand")

	moved, err := s.Move(ctx, admin(), child.ID, "")
	if err != nil {
		t.Fatalf("move to root: %v", err)
	}
	if moved.Path != "/Child" || moved.Depth != 0 || moved.ParentID != nil {
		t.Fatalf("moved-to-root = %q d=%d parent=%v", moved.Path, moved.Depth, moved.ParentID)
	}
	g, _ := s.Get(ctx, admin(), findByName(t, s, "Grand"))
	if g.Path != "/Child/Grand" || g.Depth != 1 {
		t.Fatalf("grand after move = %q d=%d, want /Child/Grand d=1", g.Path, g.Depth)
	}
	// Old parent's count decremented.
	r, _ := s.Get(ctx, admin(), root.ID)
	if r.SubcategoryCount != 0 {
		t.Fatalf("old root count = %d, want 0", r.SubcategoryCount)
	}
}

func TestMove_Errors(t *testing.T) {
	ctx := context.Background()
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")

	// Move a missing category.
	if _, err := s.Move(ctx, admin(), store.NewID(), root.ID); !errors.Is(err, categories.ErrNotFound) {
		t.Fatalf("move missing err = %v, want ErrNotFound", err)
	}
	// Move to a missing new parent.
	if _, err := s.Move(ctx, admin(), root.ID, store.NewID()); !errors.Is(err, categories.ErrNotFound) {
		t.Fatalf("move to missing parent err = %v, want ErrNotFound", err)
	}
	// Forbidden move.
	if _, err := s.Move(ctx, member(), root.ID, ""); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("move forbidden err = %v, want ErrForbidden", err)
	}
}

func TestUpdate_MissingAndForbidden(t *testing.T) {
	ctx := context.Background()
	s, _ := newSvc(t)
	root := mustCreate(t, s, "", "Root")
	if _, err := s.Update(ctx, admin(), store.NewID(), categories.Input{Name: "Z"}); !errors.Is(err, categories.ErrNotFound) {
		t.Fatalf("update missing err = %v, want ErrNotFound", err)
	}
	if _, err := s.Update(ctx, member(), root.ID, categories.Input{Name: "Z"}); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("update forbidden err = %v, want ErrForbidden", err)
	}
}

// findByName looks up a category id by name via the admin list (test helper).
func findByName(t *testing.T, s *categories.Service, name string) string {
	t.Helper()
	rows, err := s.List(context.Background(), admin())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, r := range rows {
		if r.Name == name {
			return r.ID
		}
	}
	t.Fatalf("category %q not found", name)
	return ""
}

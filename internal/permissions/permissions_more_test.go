package permissions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/permissions"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

func setup(t *testing.T) (*permissions.Service, *memstore.Mem, string, authz.Subjects) {
	t.Helper()
	ctx := context.Background()
	m := memstore.New()
	az := authz.New(m)
	svc := permissions.New(m, az)
	docID := store.NewID()
	if err := m.InsertDocument(ctx, store.Document{ID: docID, TenantID: tenant, Name: "d", Status: store.DocActive}); err != nil {
		t.Fatalf("InsertDocument: %v", err)
	}
	owner := authz.Subjects{TenantID: tenant, UserID: "owner", ActorKind: "user"}
	if err := az.GrantOwner(ctx, tenant, authz.Document, docID, "owner"); err != nil {
		t.Fatalf("GrantOwner: %v", err)
	}
	return svc, m, docID, owner
}

func TestGrant_InvalidInput(t *testing.T) {
	svc, _, docID, owner := setup(t)
	_, err := svc.Grant(context.Background(), owner, permissions.GrantInput{
		ResourceType: "bogus", ResourceID: docID, SubjectType: store.SubjectUser, SubjectID: "z", Relation: store.RelationViewer,
	})
	if err == nil {
		t.Fatal("Grant with invalid resource type should error")
	}
}

func TestGrant_ForbiddenForNonSharer(t *testing.T) {
	svc, _, docID, _ := setup(t)
	stranger := authz.Subjects{TenantID: tenant, UserID: "stranger", ActorKind: "user"}
	_, err := svc.Grant(context.Background(), stranger, permissions.GrantInput{
		ResourceType: authz.Document, ResourceID: docID, SubjectType: store.SubjectUser, SubjectID: "z", Relation: store.RelationViewer,
	})
	if !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Grant by non-sharer err = %v, want ErrForbidden", err)
	}
}

func TestRevoke_NotFoundAndForbidden(t *testing.T) {
	svc, _, docID, owner := setup(t)
	// Revoking a non-existent grant id -> ErrNotFound.
	if err := svc.Revoke(context.Background(), owner, authz.Document, docID, store.NewID()); !errors.Is(err, permissions.ErrNotFound) {
		t.Fatalf("Revoke missing err = %v, want ErrNotFound", err)
	}
	// Non-sharer cannot revoke.
	stranger := authz.Subjects{TenantID: tenant, UserID: "stranger", ActorKind: "user"}
	if err := svc.Revoke(context.Background(), stranger, authz.Document, docID, store.NewID()); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("Revoke by non-sharer err = %v, want ErrForbidden", err)
	}
}

func TestListForResource(t *testing.T) {
	svc, _, docID, owner := setup(t)
	ctx := context.Background()
	// Owner can list; there is at least the owner tuple.
	rows, err := svc.ListForResource(ctx, owner, authz.Document, docID)
	if err != nil {
		t.Fatalf("ListForResource: %v", err)
	}
	if len(rows) != 1 || rows[0].Relation != store.RelationOwner {
		t.Fatalf("rows = %+v, want a single owner tuple", rows)
	}
	// A stranger without read cannot list.
	stranger := authz.Subjects{TenantID: tenant, UserID: "stranger", ActorKind: "user"}
	if _, err := svc.ListForResource(ctx, stranger, authz.Document, docID); !errors.Is(err, authz.ErrForbidden) {
		t.Fatalf("ListForResource stranger err = %v, want ErrForbidden", err)
	}
}

func TestCheck_ErrorPassesThrough(t *testing.T) {
	svc, _, docID, _ := setup(t)
	ctx := context.Background()
	stranger := authz.Subjects{TenantID: tenant, UserID: "stranger", ActorKind: "user"}
	// Forbidden collapses to (false, nil).
	ok, err := svc.Check(ctx, stranger, authz.Document, docID, authz.Write)
	if err != nil || ok {
		t.Fatalf("Check forbidden = (%v,%v), want (false,nil)", ok, err)
	}
	// Not-found (masked) collapses to (false, nil).
	ok, err = svc.Check(ctx, stranger, authz.Document, store.NewID(), authz.Read)
	if err != nil || ok {
		t.Fatalf("Check missing = (%v,%v), want (false,nil)", ok, err)
	}
}

func TestEffective_Grants(t *testing.T) {
	svc, _, docID, owner := setup(t)
	perms, grants, err := svc.Effective(context.Background(), owner, authz.Document, docID)
	if err != nil {
		t.Fatalf("Effective: %v", err)
	}
	if !perms.Write || !perms.Share || len(grants) == 0 {
		t.Fatalf("owner effective = %+v grants=%d", perms, len(grants))
	}
}

func TestExpiry_ExpiredGrantIgnored(t *testing.T) {
	svc, _, docID, owner := setup(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)

	// Expired viewer grant to u2: no access.
	if _, err := svc.Grant(ctx, owner, permissions.GrantInput{
		ResourceType: authz.Document, ResourceID: docID, SubjectType: store.SubjectUser,
		SubjectID: "u2", Relation: store.RelationViewer, ExpiresAt: &past,
	}); err != nil {
		t.Fatalf("Grant expired: %v", err)
	}
	u2 := authz.Subjects{TenantID: tenant, UserID: "u2", ActorKind: "user"}
	if ok, _ := svc.Check(ctx, u2, authz.Document, docID, authz.Read); ok {
		t.Fatal("expired grant should not confer read")
	}

	// Still-valid viewer grant to u3: access.
	if _, err := svc.Grant(ctx, owner, permissions.GrantInput{
		ResourceType: authz.Document, ResourceID: docID, SubjectType: store.SubjectUser,
		SubjectID: "u3", Relation: store.RelationViewer, ExpiresAt: &future,
	}); err != nil {
		t.Fatalf("Grant future: %v", err)
	}
	u3 := authz.Subjects{TenantID: tenant, UserID: "u3", ActorKind: "user"}
	if ok, _ := svc.Check(ctx, u3, authz.Document, docID, authz.Read); !ok {
		t.Fatal("valid grant should confer read")
	}
}

func TestListAccessible(t *testing.T) {
	svc, m, docID, _ := setup(t)
	ctx := context.Background()

	// A direct viewer grant to u2 on the document.
	if err := m.InsertPermission(ctx, store.PermissionTuple{
		ID: store.NewID(), TenantID: tenant, ResourceType: store.ResourceDocument, ResourceID: docID,
		SubjectType: store.SubjectUser, SubjectID: "u2", Relation: store.RelationViewer,
	}); err != nil {
		t.Fatalf("InsertPermission: %v", err)
	}
	u2 := authz.Subjects{TenantID: tenant, UserID: "u2", ActorKind: "user"}
	ids, all, err := svc.ListAccessible(ctx, u2, authz.Document)
	if err != nil {
		t.Fatalf("ListAccessible: %v", err)
	}
	if all {
		t.Fatal("non-admin should not get all=true")
	}
	found := false
	for _, id := range ids {
		if id == docID {
			found = true
		}
	}
	if !found {
		t.Fatalf("ListAccessible = %v, want to include %s", ids, docID)
	}

	// Admin gets all=true.
	admin := authz.Subjects{TenantID: tenant, UserID: "a", Roles: []string{"admin"}, ActorKind: "user"}
	if _, allAdmin, err := svc.ListAccessible(ctx, admin, authz.Document); err != nil || !allAdmin {
		t.Fatalf("admin ListAccessible all = %v err=%v, want true", allAdmin, err)
	}
}

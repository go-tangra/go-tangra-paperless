package authz_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

const tenant = "11111111-1111-1111-1111-111111111111"

func sp(s string) *string { return &s }

// seedCatDoc creates /Finance and /Finance/2026 with a document in 2026.
func seedCatDoc(t *testing.T, m *memstore.Mem) (finID, childID, docID string) {
	ctx := context.Background()
	finID, childID, docID = store.NewID(), store.NewID(), store.NewID()
	_ = m.InsertCategory(ctx, store.Category{ID: finID, TenantID: tenant, Name: "Finance", Path: "/Finance"})
	_ = m.InsertCategory(ctx, store.Category{ID: childID, TenantID: tenant, ParentID: sp(finID), Name: "2026", Path: "/Finance/2026"})
	_ = m.InsertDocument(ctx, store.Document{ID: docID, TenantID: tenant, CategoryID: sp(childID), CategoryPath: "/Finance/2026", Name: "q1", Status: store.DocActive})
	return
}

func viewer(uid string) authz.Subjects {
	return authz.Subjects{TenantID: tenant, UserID: uid, ActorKind: "user"}
}

func TestInheritanceGrantOnParentCategoryReachesDocument(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	fin, _, doc := seedCatDoc(t, m)
	// Grant viewer on the /Finance category to user u2.
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: fin, SubjectType: store.SubjectUser, SubjectID: "u2", Relation: store.RelationViewer})
	// u2 can read the document inside /Finance/2026 by inheritance.
	if err := az.Check(context.Background(), viewer("u2"), authz.Document, doc, authz.Read); err != nil {
		t.Fatalf("inherited read denied: %v", err)
	}
	// but not write (viewer has no write).
	if err := az.Check(context.Background(), viewer("u2"), authz.Document, doc, authz.Write); err != authz.ErrForbidden {
		t.Fatalf("inherited write should be forbidden: %v", err)
	}
	// u3 with no grant is denied (masked as not found since the doc exists but no read).
	if err := az.Check(context.Background(), viewer("u3"), authz.Document, doc, authz.Read); err != authz.ErrForbidden {
		t.Fatalf("no-grant read: %v", err)
	}
}

func TestExpiredGrantConfersNoAccess(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	fin, _, doc := seedCatDoc(t, m)
	past := time.Now().Add(-time.Hour)
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: fin, SubjectType: store.SubjectUser, SubjectID: "u2", Relation: store.RelationViewer, ExpiresAt: &past})
	if err := az.Check(context.Background(), viewer("u2"), authz.Document, doc, authz.Read); err != authz.ErrForbidden {
		t.Fatalf("expired grant should not confer access: %v", err)
	}
}

func TestTenantWideAndAdmin(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	_, _, doc := seedCatDoc(t, m)
	// tenant-wide viewer.
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: "", SubjectType: store.SubjectTenant, Relation: store.RelationViewer})
	if err := az.Check(context.Background(), viewer("anyone"), authz.Document, doc, authz.Read); err != nil {
		t.Fatalf("tenant-wide read denied: %v", err)
	}
	// admin gets everything.
	admin := authz.Subjects{TenantID: tenant, UserID: "a", Roles: []string{"admin"}, ActorKind: "user"}
	if err := az.Check(context.Background(), admin, authz.Document, doc, authz.Delete); err != nil {
		t.Fatalf("admin delete denied: %v", err)
	}
}

func TestCrossTenantDenied(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	_, _, doc := seedCatDoc(t, m)
	other := authz.Subjects{TenantID: "22222222-2222-2222-2222-222222222222", UserID: "x", ActorKind: "user"}
	if err := az.Check(context.Background(), other, authz.Document, doc, authz.Read); err != authz.ErrNotFound {
		t.Fatalf("cross-tenant should be masked not-found: %v", err)
	}
}

func TestGrantOwnerAndEffective(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	_, _, doc := seedCatDoc(t, m)
	if err := az.GrantOwner(context.Background(), tenant, authz.Document, doc, "owner1"); err != nil {
		t.Fatal(err)
	}
	perms, err := az.Effective(context.Background(), viewer("owner1"), authz.Document, doc)
	if err != nil || !perms.Read || !perms.Write || !perms.Delete || !perms.Share || !perms.Download {
		t.Fatalf("owner effective: %+v %v", perms, err)
	}
}

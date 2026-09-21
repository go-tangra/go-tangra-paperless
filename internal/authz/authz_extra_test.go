package authz_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// errStore wraps memstore and injects errors on selected read methods so the
// error branches of the authorizer can be exercised deterministically.
type errStore struct {
	*memstore.Mem
	grantsErr   error
	listCatsErr error
	existsErr   error
}

func (e *errStore) GrantsForSubjects(ctx context.Context, tenantID, userID string, roles []string, now time.Time) ([]store.PermissionTuple, error) {
	if e.grantsErr != nil {
		return nil, e.grantsErr
	}
	return e.Mem.GrantsForSubjects(ctx, tenantID, userID, roles, now)
}

func (e *errStore) ListCategories(ctx context.Context, tenantID string) ([]store.Category, error) {
	if e.listCatsErr != nil {
		return nil, e.listCatsErr
	}
	return e.Mem.ListCategories(ctx, tenantID)
}

func (e *errStore) Exists(ctx context.Context, tenantID, rt, id string) (bool, error) {
	if e.existsErr != nil {
		return false, e.existsErr
	}
	return e.Mem.Exists(ctx, tenantID, rt, id)
}

var _ repo.Store = (*errStore)(nil)

// --- Permissions.Has covers every action plus an unknown action.

func TestPermissionsHasAllActions(t *testing.T) {
	full := authz.Of(store.RelationOwner)
	for _, act := range []string{authz.Read, authz.Write, authz.Delete, authz.Share, authz.Download} {
		if !full.Has(act) {
			t.Fatalf("owner should have %q", act)
		}
	}
	if full.Has("bogus") {
		t.Fatal("unknown action must be false")
	}
	// A relation missing specific actions returns false for those.
	viewerP := authz.Of(store.RelationViewer)
	if viewerP.Has(authz.Write) || viewerP.Has(authz.Delete) || viewerP.Has(authz.Share) {
		t.Fatalf("viewer perms too broad: %+v", viewerP)
	}
	if !viewerP.Has(authz.Read) || !viewerP.Has(authz.Download) {
		t.Fatalf("viewer should read+download: %+v", viewerP)
	}
}

// --- Of covers every relation and the unknown/default case.

func TestOfRelations(t *testing.T) {
	owner := authz.Of(store.RelationOwner)
	if !(owner.Read && owner.Write && owner.Delete && owner.Share && owner.Download) {
		t.Fatalf("owner: %+v", owner)
	}
	editor := authz.Of(store.RelationEditor)
	if !(editor.Read && editor.Write && editor.Delete && editor.Download) || editor.Share {
		t.Fatalf("editor: %+v", editor)
	}
	viewerP := authz.Of(store.RelationViewer)
	if !(viewerP.Read && viewerP.Download) || viewerP.Write || viewerP.Delete || viewerP.Share {
		t.Fatalf("viewer: %+v", viewerP)
	}
	sharer := authz.Of(store.RelationSharer)
	if !(sharer.Read && sharer.Share) || sharer.Write || sharer.Delete || sharer.Download {
		t.Fatalf("sharer: %+v", sharer)
	}
	empty := authz.Of("nonsense")
	if empty.Read || empty.Write || empty.Delete || empty.Share || empty.Download {
		t.Fatalf("unknown relation must be empty: %+v", empty)
	}
}

// --- Subjects helpers.

func TestActorIDAndIsAdmin(t *testing.T) {
	withUser := authz.Subjects{UserID: "u1", ActorKind: "user"}
	if withUser.ActorID() != "u1" {
		t.Fatalf("ActorID with user: %q", withUser.ActorID())
	}
	svc := authz.Subjects{ActorKind: "service"}
	if svc.ActorID() != "service" {
		t.Fatalf("ActorID falls back to kind: %q", svc.ActorID())
	}
	if (authz.Subjects{Roles: []string{"x", "admin"}}).IsAdmin() != true {
		t.Fatal("admin role not detected")
	}
	if (authz.Subjects{Roles: []string{"user"}}).IsAdmin() != false {
		t.Fatal("non-admin flagged admin")
	}
}

// --- SetClock makes expiry deterministic: a grant valid at a fixed "now".

func TestSetClockExpiryDeterministic(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	fixed := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	az.SetClock(func() time.Time { return fixed })
	fin, _, doc := seedCatDoc(t, m)

	// Grant expires one hour after the fixed clock: still valid.
	future := fixed.Add(time.Hour)
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: fin, SubjectType: store.SubjectUser, SubjectID: "u2",
		Relation: store.RelationViewer, ExpiresAt: &future})
	if err := az.Check(context.Background(), viewer("u2"), authz.Document, doc, authz.Read); err != nil {
		t.Fatalf("grant valid at fixed clock should permit read: %v", err)
	}

	// Advance the clock past expiry: same grant now denies.
	az.SetClock(func() time.Time { return fixed.Add(2 * time.Hour) })
	if err := az.Check(context.Background(), viewer("u2"), authz.Document, doc, authz.Read); err != authz.ErrForbidden {
		t.Fatalf("grant expired at advanced clock should deny: %v", err)
	}
}

// --- Effective / ancestors edge cases.

func TestEffectiveInvalidResourceType(t *testing.T) {
	az := authz.New(memstore.New())
	if _, err := az.Effective(context.Background(), viewer("u"), "widget", "x"); err != authz.ErrForbidden {
		t.Fatalf("invalid resource type: %v", err)
	}
}

func TestEffectiveCollectionScopeTenantWide(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: "", SubjectType: store.SubjectTenant, Relation: store.RelationEditor})
	// resourceID "" => collection scope, ancestors not consulted, tenant-wide applies.
	perms, err := az.Effective(context.Background(), viewer("anyone"), authz.Document, "")
	if err != nil || !perms.Write {
		t.Fatalf("collection-scope tenant-wide editor: %+v %v", perms, err)
	}
}

func TestEffectiveOnCategoryResource(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	fin, child, _ := seedCatDoc(t, m)
	// grant on parent category, checked against child category (inheritance).
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: fin, SubjectType: store.SubjectUser, SubjectID: "u2", Relation: store.RelationEditor})
	perms, err := az.Effective(context.Background(), viewer("u2"), authz.Category, child)
	if err != nil || !perms.Write {
		t.Fatalf("category inheritance editor: %+v %v", perms, err)
	}
}

func TestEffectiveAbsentDocumentAndCategory(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	// tenant-wide grant so a non-empty perm set is returned even though the
	// resource itself is absent (ancestors returns early with only own id).
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: "", SubjectType: store.SubjectTenant, Relation: store.RelationViewer})
	if p, err := az.Effective(context.Background(), viewer("u"), authz.Document, "ghost-doc"); err != nil || !p.Read {
		t.Fatalf("absent document ancestors: %+v %v", p, err)
	}
	if p, err := az.Effective(context.Background(), viewer("u"), authz.Category, "ghost-cat"); err != nil || !p.Read {
		t.Fatalf("absent category ancestors: %+v %v", p, err)
	}
}

func TestEffectiveDocumentWithEmptyPath(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	// A document with no category path: ancestors returns early at path=="".
	docID := store.NewID()
	_ = m.InsertDocument(context.Background(), store.Document{ID: docID, TenantID: tenant, CategoryPath: "", Name: "loose", Status: store.DocActive})
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceDocument, ResourceID: docID, SubjectType: store.SubjectUser, SubjectID: "u2", Relation: store.RelationOwner})
	if p, err := az.Effective(context.Background(), viewer("u2"), authz.Document, docID); err != nil || !p.Delete {
		t.Fatalf("empty-path doc direct owner: %+v %v", p, err)
	}
}

func TestEffectiveGrantsError(t *testing.T) {
	boom := errors.New("grants boom")
	es := &errStore{Mem: memstore.New(), grantsErr: boom}
	az := authz.New(es)
	if _, err := az.Effective(context.Background(), viewer("u"), authz.Document, ""); !errors.Is(err, boom) {
		t.Fatalf("grants error should propagate: %v", err)
	}
}

func TestEffectiveAncestorsError(t *testing.T) {
	boom := errors.New("listcats boom")
	m := memstore.New()
	es := &errStore{Mem: m, listCatsErr: boom}
	az := authz.New(es)
	_, _, doc := seedCatDoc(t, m) // seeds via underlying Mem (path set)
	if _, err := az.Effective(context.Background(), viewer("u"), authz.Document, doc); !errors.Is(err, boom) {
		t.Fatalf("ancestors ListCategories error should propagate: %v", err)
	}
}

// --- ListAccessible.

func TestListAccessibleAdmin(t *testing.T) {
	az := authz.New(memstore.New())
	admin := authz.Subjects{TenantID: tenant, Roles: []string{"admin"}}
	ids, all, err := az.ListAccessible(context.Background(), admin, authz.Document)
	if err != nil || !all || ids != nil {
		t.Fatalf("admin all-access: ids=%v all=%v err=%v", ids, all, err)
	}
}

func TestListAccessibleDirectAndCategory(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	fin, _, doc := seedCatDoc(t, m)
	// direct viewer on a document.
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceDocument, ResourceID: doc, SubjectType: store.SubjectUser, SubjectID: "u2", Relation: store.RelationViewer})
	// viewer on a category (exercises the ListSubtree branch).
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: fin, SubjectType: store.SubjectUser, SubjectID: "u2", Relation: store.RelationViewer})
	// a non-readable relation tuple: exercises the "no read" continue path.
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceDocument, ResourceID: "other", SubjectType: store.SubjectUser, SubjectID: "u2", Relation: "noaccess"})

	ids, all, err := az.ListAccessible(context.Background(), viewer("u2"), authz.Document)
	if err != nil || all {
		t.Fatalf("expected scoped access: all=%v err=%v", all, err)
	}
	if !ids[doc] {
		t.Fatalf("direct document not accessible: %v", ids)
	}
	if ids["other"] {
		t.Fatalf("no-read tuple should not grant access: %v", ids)
	}

	// listing categories returns the directly-granted category id.
	cids, _, err := az.ListAccessible(context.Background(), viewer("u2"), authz.Category)
	if err != nil || !cids[fin] {
		t.Fatalf("category id not listed: %v %v", cids, err)
	}
}

func TestListAccessibleTenantWide(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: "", SubjectType: store.SubjectTenant, Relation: store.RelationViewer})
	ids, all, err := az.ListAccessible(context.Background(), viewer("anyone"), authz.Document)
	if err != nil || !all || ids != nil {
		t.Fatalf("tenant-wide should be all-access: ids=%v all=%v err=%v", ids, all, err)
	}
}

func TestListAccessibleGrantsError(t *testing.T) {
	boom := errors.New("grants boom")
	es := &errStore{Mem: memstore.New(), grantsErr: boom}
	az := authz.New(es)
	if _, _, err := az.ListAccessible(context.Background(), viewer("u"), authz.Document); !errors.Is(err, boom) {
		t.Fatalf("grants error should propagate: %v", err)
	}
}

// --- Check edge cases.

func TestCheckInvalidTypeOrAction(t *testing.T) {
	az := authz.New(memstore.New())
	if err := az.Check(context.Background(), viewer("u"), "widget", "x", authz.Read); err != authz.ErrForbidden {
		t.Fatalf("invalid type: %v", err)
	}
	if err := az.Check(context.Background(), viewer("u"), authz.Document, "x", "fly"); err != authz.ErrForbidden {
		t.Fatalf("invalid action: %v", err)
	}
}

func TestCheckCollectionScopeSkipsExists(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	_ = m.InsertPermission(context.Background(), store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: "", SubjectType: store.SubjectTenant, Relation: store.RelationViewer})
	// resourceID "" => Exists is not consulted; tenant-wide viewer permits read.
	if err := az.Check(context.Background(), viewer("u"), authz.Document, "", authz.Read); err != nil {
		t.Fatalf("collection read denied: %v", err)
	}
}

func TestCheckExistsError(t *testing.T) {
	boom := errors.New("exists boom")
	m := memstore.New()
	es := &errStore{Mem: m, existsErr: boom}
	az := authz.New(es)
	_, _, doc := seedCatDoc(t, m)
	if err := az.Check(context.Background(), viewer("u"), authz.Document, doc, authz.Read); !errors.Is(err, boom) {
		t.Fatalf("exists error should propagate: %v", err)
	}
}

func TestCheckEffectiveError(t *testing.T) {
	boom := errors.New("grants boom")
	m := memstore.New()
	es := &errStore{Mem: m, grantsErr: boom}
	az := authz.New(es)
	_, _, doc := seedCatDoc(t, m) // exists via Mem, but grants errors in Effective
	if err := az.Check(context.Background(), viewer("u"), authz.Document, doc, authz.Read); !errors.Is(err, boom) {
		t.Fatalf("effective error should propagate: %v", err)
	}
}

// --- GrantOwner edge cases.

func TestGrantOwnerEmptyUserNoop(t *testing.T) {
	az := authz.New(memstore.New())
	if err := az.GrantOwner(context.Background(), tenant, authz.Document, "d1", ""); err != nil {
		t.Fatalf("empty user should be a no-op: %v", err)
	}
}

func TestGrantOwnerInsertError(t *testing.T) {
	boom := errors.New("insert boom")
	m := memstore.New()
	m.FailNext(boom)
	az := authz.New(m)
	if err := az.GrantOwner(context.Background(), tenant, authz.Document, "d1", "u1"); !errors.Is(err, boom) {
		t.Fatalf("insert error should propagate: %v", err)
	}
}

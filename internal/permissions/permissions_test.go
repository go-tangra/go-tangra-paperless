package permissions_test

import (
	"context"
	"testing"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/permissions"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

const tenant = "11111111-1111-1111-1111-111111111111"

func sp(s string) *string { return &s }

func TestGrantRevokeCheckEffective(t *testing.T) {
	ctx := context.Background()
	m := memstore.New()
	az := authz.New(m)
	svc := permissions.New(m, az)

	docID := store.NewID()
	_ = m.InsertDocument(ctx, store.Document{ID: docID, TenantID: tenant, Name: "d", Status: store.DocActive})
	owner := authz.Subjects{TenantID: tenant, UserID: "owner", ActorKind: "user"}
	_ = az.GrantOwner(ctx, tenant, authz.Document, docID, "owner")

	// Owner grants viewer to u2.
	tup, err := svc.Grant(ctx, owner, permissions.GrantInput{ResourceType: authz.Document, ResourceID: docID, SubjectType: store.SubjectUser, SubjectID: "u2", Relation: store.RelationViewer})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	// u2 can read but not write.
	u2 := authz.Subjects{TenantID: tenant, UserID: "u2", ActorKind: "user"}
	if ok, _ := svc.Check(ctx, u2, authz.Document, docID, authz.Read); !ok {
		t.Fatal("u2 should read")
	}
	if ok, _ := svc.Check(ctx, u2, authz.Document, docID, authz.Write); ok {
		t.Fatal("u2 should not write")
	}
	// Effective for the owner is full.
	perms, grants, err := svc.Effective(ctx, owner, authz.Document, docID)
	if err != nil || !perms.Write || len(grants) == 0 {
		t.Fatalf("effective: %+v %v", perms, err)
	}
	// Revoke removes u2's access.
	if err := svc.Revoke(ctx, owner, authz.Document, docID, tup.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if ok, _ := svc.Check(ctx, u2, authz.Document, docID, authz.Read); ok {
		t.Fatal("u2 read should be revoked")
	}
	// A non-sharer cannot grant.
	stranger := authz.Subjects{TenantID: tenant, UserID: "x", ActorKind: "user"}
	if _, err := svc.Grant(ctx, stranger, permissions.GrantInput{ResourceType: authz.Document, ResourceID: docID, SubjectType: store.SubjectUser, SubjectID: "z", Relation: store.RelationViewer}); err == nil {
		t.Fatal("stranger should not be able to grant")
	}
}

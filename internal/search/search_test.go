package search_test

import (
	"context"
	"testing"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/search"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

const tenant = "11111111-1111-1111-1111-111111111111"

func TestSearchPermissionFiltered(t *testing.T) {
	ctx := context.Background()
	m := memstore.New()
	az := authz.New(m)
	svc := search.New(m, az)

	mine, theirs := store.NewID(), store.NewID()
	_ = m.InsertDocument(ctx, store.Document{ID: mine, TenantID: tenant, Name: "a", ContentText: "quarterly budget report", Status: store.DocActive})
	_ = m.InsertDocument(ctx, store.Document{ID: theirs, TenantID: tenant, Name: "b", ContentText: "quarterly budget report", Status: store.DocActive})
	// Grant the caller viewer only on "mine".
	_ = m.InsertPermission(ctx, store.PermissionTuple{ID: store.NewID(), TenantID: tenant, ResourceType: store.ResourceDocument, ResourceID: mine, SubjectType: store.SubjectUser, SubjectID: "u1", Relation: store.RelationViewer})

	subj := authz.Subjects{TenantID: tenant, UserID: "u1", ActorKind: "user"}
	hits, err := svc.Search(ctx, subj, "budget", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != mine {
		t.Fatalf("search returned %d hits, want only the readable one: %+v", len(hits), hits)
	}

	// Admin sees both.
	admin := authz.Subjects{TenantID: tenant, UserID: "a", Roles: []string{"admin"}, ActorKind: "user"}
	if hits, _ := svc.Search(ctx, admin, "budget", 10); len(hits) != 2 {
		t.Fatalf("admin search = %d, want 2", len(hits))
	}
}

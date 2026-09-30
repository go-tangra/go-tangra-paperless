package authz_test

import (
	"context"
	"testing"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
)

// A platform service (mesh-policy gated, no roles, no tuples) may create root
// folders and read the folder tree, but gets no collection access to documents
// and no access to a specific folder or document it holds no tuple on. A user
// without grants gets neither.
func TestServiceCollectionScopeIsCategoryOnly(t *testing.T) {
	m := memstore.New()
	az := authz.New(m)
	fin, _, doc := seedCatDoc(t, m)
	ctx := context.Background()
	svc := authz.Subjects{TenantID: tenant, UserID: "spiffe://example.org/svc/asset", ActorKind: authz.ActorService}

	p, err := az.Effective(ctx, svc, authz.Category, "")
	if err != nil || !p.Read || !p.Write || p.Delete || p.Share || p.Download {
		t.Fatalf("service category collection perms: %+v %v", p, err)
	}
	for _, act := range []string{authz.Read, authz.Write} {
		if err := az.Check(ctx, svc, authz.Category, "", act); err != nil {
			t.Fatalf("service collection %s: %v", act, err)
		}
	}
	if err := az.Check(ctx, svc, authz.Category, "", authz.Delete); err != authz.ErrForbidden {
		t.Fatalf("service collection delete: %v", err)
	}
	if err := az.Check(ctx, svc, authz.Document, "", authz.Read); err != authz.ErrForbidden {
		t.Fatalf("service document collection read: %v", err)
	}
	if err := az.Check(ctx, svc, authz.Category, fin, authz.Write); err != authz.ErrForbidden {
		t.Fatalf("service write on a foreign folder: %v", err)
	}
	if err := az.Check(ctx, svc, authz.Document, doc, authz.Read); err != authz.ErrForbidden {
		t.Fatalf("service read of a foreign document: %v", err)
	}
	if err := az.Check(ctx, viewer("u9"), authz.Category, "", authz.Write); err != authz.ErrForbidden {
		t.Fatalf("user without grants creating a root folder: %v", err)
	}
}

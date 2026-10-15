package authz_test

import (
	"context"
	"testing"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

// "Everyone in the tenant" on one item is scoped to that item (and, for a
// category, what is beneath it). It used to apply to every document and
// category in the tenant and to open the collections.
func TestTenantGrantOnOneResourceStaysOnIt(t *testing.T) {
	ctx := context.Background()
	m := memstore.New()
	az := authz.New(m)
	fin, child, doc := seedCatDoc(t, m)
	hr, other := store.NewID(), store.NewID()
	_ = m.InsertCategory(ctx, store.Category{ID: hr, TenantID: tenant, Name: "HR", Path: "/HR"})
	_ = m.InsertDocument(ctx, store.Document{ID: other, TenantID: tenant, CategoryID: sp(hr), CategoryPath: "/HR", Name: "salaries", Status: store.DocActive})
	// Share the Finance category with everyone in the tenant as editor.
	_ = m.InsertPermission(ctx, store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceCategory, ResourceID: fin, SubjectType: store.SubjectTenant, Relation: store.RelationEditor})
	u := viewer("anyone")

	for _, c := range []struct {
		name, rt, id string
		read         bool
	}{
		{"the shared category", authz.Category, fin, true},
		{"a category beneath it", authz.Category, child, true},
		{"a document beneath it", authz.Document, doc, true},
		{"another category", authz.Category, hr, false},
		{"a document elsewhere", authz.Document, other, false},
	} {
		p, err := az.Effective(ctx, u, c.rt, c.id)
		if err != nil || p.Read != c.read || p.Write != c.read {
			t.Errorf("%s: %+v %v, want read/write %v", c.name, p, err, c.read)
		}
	}
	if err := az.Check(ctx, u, authz.Document, other, authz.Read); err != authz.ErrForbidden {
		t.Errorf("document elsewhere: %v, want forbidden", err)
	}
	// No collection access: listing or creating at the top level stays closed.
	for _, rt := range []string{authz.Document, authz.Category} {
		if p, err := az.Effective(ctx, u, rt, ""); err != nil || p.Read || p.Write {
			t.Errorf("%s collection: %+v %v", rt, p, err)
		}
	}
	ids, all, err := az.ListAccessible(ctx, u, authz.Category)
	if err != nil || all || !ids[fin] || ids[hr] {
		t.Errorf("ListAccessible categories: %v all=%v %v", ids, all, err)
	}
	ids, all, err = az.ListAccessible(ctx, u, authz.Document)
	if err != nil || all || ids[other] {
		t.Errorf("ListAccessible documents: %v all=%v %v", ids, all, err)
	}
}

// A tenant grant with no resource id is still tenant-wide.
func TestTenantGrantWithoutResourceIsTenantWide(t *testing.T) {
	ctx := context.Background()
	m := memstore.New()
	az := authz.New(m)
	_, _, doc := seedCatDoc(t, m)
	_ = m.InsertPermission(ctx, store.PermissionTuple{ID: store.NewID(), TenantID: tenant,
		ResourceType: store.ResourceDocument, ResourceID: "", SubjectType: store.SubjectTenant, Relation: store.RelationViewer})
	if err := az.Check(ctx, viewer("anyone"), authz.Document, doc, authz.Read); err != nil {
		t.Fatalf("tenant-wide read: %v", err)
	}
	if _, all, err := az.ListAccessible(ctx, viewer("anyone"), authz.Document); err != nil || !all {
		t.Fatalf("tenant-wide listing: all=%v %v", all, err)
	}
}

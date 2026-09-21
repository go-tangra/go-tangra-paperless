package grpcapi

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paperlessv1 "github.com/go-freya/freya/services/paperless/api/proto/paperless/v1"
)

// TestRegisterGRPC registers all four servers on a real gRPC server.
func TestRegisterGRPC(t *testing.T) {
	s := kit(t)
	gs := grpc.NewServer()
	Register(gs, Deps{
		Documents:   s.docs.Docs,
		Categories:  s.cats.Svc,
		Permissions: s.perms.Svc,
		Search:      s.docs.Searcher,
		Stats:       s.stats.Svc,
	})
	// Nil deps must be skipped without panicking.
	Register(grpc.NewServer(), Deps{})
	gs.Stop()
}

// TestCategoryGetGRPC covers CategoryServer.Get (happy path).
func TestCategoryGetGRPC(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()
	c, err := s.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, Name: "Gettable"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.cats.Get(ctx, &paperlessv1.GetCategoryRequest{TenantId: tenant, Id: c.GetId()})
	if err != nil || got.GetName() != "Gettable" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
}

// TestEnumMappersRoundTrip drives the relation/subject/permission converters by
// granting each relation for each subject type and checking each permission.
func TestEnumMappersRoundTrip(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()

	doc, err := s.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{TenantId: tenant, Name: "M", Content: []byte("z")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	relations := []paperlessv1.Relation{
		paperlessv1.Relation_RELATION_OWNER, paperlessv1.Relation_RELATION_EDITOR,
		paperlessv1.Relation_RELATION_VIEWER, paperlessv1.Relation_RELATION_SHARER,
	}
	subjects := []paperlessv1.SubjectType{
		paperlessv1.SubjectType_SUBJECT_TYPE_USER, paperlessv1.SubjectType_SUBJECT_TYPE_ROLE,
		paperlessv1.SubjectType_SUBJECT_TYPE_TENANT,
	}
	for _, rel := range relations {
		for _, sub := range subjects {
			out, err := s.perms.GrantAccess(ctx, &paperlessv1.GrantAccessRequest{
				TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, ResourceId: doc.GetId(),
				SubjectType: sub, SubjectId: "sid", Relation: rel,
			})
			if err != nil {
				t.Fatalf("grant rel=%v sub=%v: %v", rel, sub, err)
			}
			if out.GetRelation() != rel || out.GetSubjectType() != sub {
				t.Fatalf("round-trip rel=%v→%v sub=%v→%v", rel, out.GetRelation(), sub, out.GetSubjectType())
			}
		}
	}

	perms := []paperlessv1.Permission{
		paperlessv1.Permission_PERMISSION_READ, paperlessv1.Permission_PERMISSION_WRITE,
		paperlessv1.Permission_PERMISSION_DELETE, paperlessv1.Permission_PERMISSION_SHARE,
		paperlessv1.Permission_PERMISSION_DOWNLOAD,
	}
	for _, pm := range perms {
		if _, err := s.perms.CheckAccess(ctx, &paperlessv1.CheckAccessRequest{
			TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_CATEGORY, ResourceId: "",
			Permission: pm,
		}); err != nil {
			t.Fatalf("check perm=%v: %v", pm, err)
		}
	}
}

// TestRPCsRequireCaller drives the caller() guard (→ Unauthenticated) that opens
// every RPC, exercising the otherwise-unreached error return in each method.
func TestRPCsRequireCaller(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "", false) // no SPIFFE peer
	ctx := context.Background()
	req := func(name string, err error) {
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("%s: code = %v, want Unauthenticated", name, status.Code(err))
		}
	}

	_, e := s.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{TenantId: tenant})
	req("doc.Create", e)
	_, e = s.docs.Get(ctx, &paperlessv1.GetDocumentRequest{TenantId: tenant})
	req("doc.Get", e)
	_, e = s.docs.List(ctx, &paperlessv1.ListDocumentsRequest{TenantId: tenant})
	req("doc.List", e)
	_, e = s.docs.Update(ctx, &paperlessv1.UpdateDocumentRequest{TenantId: tenant})
	req("doc.Update", e)
	_, e = s.docs.Delete(ctx, &paperlessv1.DeleteDocumentRequest{TenantId: tenant})
	req("doc.Delete", e)
	_, e = s.docs.Move(ctx, &paperlessv1.MoveDocumentRequest{TenantId: tenant})
	req("doc.Move", e)
	_, e = s.docs.Download(ctx, &paperlessv1.DownloadDocumentRequest{TenantId: tenant})
	req("doc.Download", e)
	_, e = s.docs.GetDownloadUrl(ctx, &paperlessv1.GetDownloadUrlRequest{TenantId: tenant})
	req("doc.GetDownloadUrl", e)
	_, e = s.docs.Search(ctx, &paperlessv1.SearchDocumentsRequest{TenantId: tenant})
	req("doc.Search", e)
	_, e = s.docs.BatchDelete(ctx, &paperlessv1.BatchDeleteRequest{TenantId: tenant})
	req("doc.BatchDelete", e)

	_, e = s.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant})
	req("cat.Create", e)
	_, e = s.cats.Get(ctx, &paperlessv1.GetCategoryRequest{TenantId: tenant})
	req("cat.Get", e)
	_, e = s.cats.List(ctx, &paperlessv1.ListCategoriesRequest{TenantId: tenant})
	req("cat.List", e)
	_, e = s.cats.Update(ctx, &paperlessv1.UpdateCategoryRequest{TenantId: tenant})
	req("cat.Update", e)
	_, e = s.cats.Delete(ctx, &paperlessv1.DeleteCategoryRequest{TenantId: tenant})
	req("cat.Delete", e)
	_, e = s.cats.Move(ctx, &paperlessv1.MoveCategoryRequest{TenantId: tenant})
	req("cat.Move", e)
	_, e = s.cats.GetTree(ctx, &paperlessv1.GetTreeRequest{TenantId: tenant})
	req("cat.GetTree", e)

	_, e = s.perms.GrantAccess(ctx, &paperlessv1.GrantAccessRequest{TenantId: tenant})
	req("perm.Grant", e)
	_, e = s.perms.RevokeAccess(ctx, &paperlessv1.RevokeAccessRequest{TenantId: tenant})
	req("perm.Revoke", e)
	_, e = s.perms.ListPermissions(ctx, &paperlessv1.ListPermissionsRequest{TenantId: tenant})
	req("perm.List", e)
	_, e = s.perms.CheckAccess(ctx, &paperlessv1.CheckAccessRequest{TenantId: tenant})
	req("perm.Check", e)
	_, e = s.perms.ListAccessibleResources(ctx, &paperlessv1.ListAccessibleResourcesRequest{TenantId: tenant})
	req("perm.ListAccessible", e)
	_, e = s.perms.GetEffectivePermissions(ctx, &paperlessv1.GetEffectivePermissionsRequest{TenantId: tenant})
	req("perm.Effective", e)

	_, e = s.stats.GetStatistics(ctx, &paperlessv1.GetStatisticsRequest{TenantId: tenant})
	req("stats.Get", e)
}

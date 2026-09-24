package grpcapi

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paperlessv1 "github.com/go-tangra/go-tangra-paperless/v4/api/proto/paperless/v1"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/blob"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/categories"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/documents"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/events"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/extract"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/permissions"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/search"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/stats"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

const tenant = "11111111-1111-1111-1111-111111111111"

type servers struct {
	docs  *DocumentServer
	cats  *CategoryServer
	perms *PermissionServer
	stats *StatisticsServer
	mem   *memstore.Mem
}

// kit builds the four servers over an in-memory store, seeding a tenant-wide
// owner grant so the acting service has full access within the tenant.
func kit(t *testing.T) servers {
	t.Helper()
	mem := memstore.New()
	az := authz.New(mem)
	bs := blob.NewFake()
	ex := &extract.Fake{Text: "canned"}
	_ = ex                       // extraction is exercised by the worker, not this gRPC surface
	pub := events.HubPublisher{} // nil hub: no-op publisher

	if err := mem.InsertPermission(context.Background(), store.PermissionTuple{
		ID: store.NewID(), TenantID: tenant, ResourceType: store.ResourceDocument, ResourceID: "",
		SubjectType: store.SubjectTenant, SubjectID: tenant, Relation: store.RelationOwner,
	}); err != nil {
		t.Fatalf("seed tenant grant: %v", err)
	}

	docs := documents.New(mem, az, bs, pub, time.Minute)
	return servers{
		docs:  &DocumentServer{Docs: docs, Searcher: search.New(mem, az)},
		cats:  &CategoryServer{Svc: categories.New(mem, az)},
		perms: &PermissionServer{Svc: permissions.New(mem, az)},
		stats: &StatisticsServer{Svc: stats.New(mem)},
		mem:   mem,
	}
}

// withFakeCaller overrides the SPIFFE resolver for the test and restores it.
func withFakeCaller(t *testing.T, id string, ok bool) {
	t.Helper()
	prev := callerFunc
	callerFunc = func(context.Context) (string, bool) { return id, ok }
	t.Cleanup(func() { callerFunc = prev })
}

func TestDocumentCreateGetSearch(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()

	created, err := s.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{
		TenantId: tenant, Name: "Quarterly Report", MimeType: "text/plain",
		FileName: "q3.txt", Content: []byte("hello world"), Source: paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.GetId() == "" || created.GetName() != "Quarterly Report" || created.GetFileSize() != 11 {
		t.Fatalf("created: %+v", created)
	}
	if created.GetStatus() != paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE {
		t.Fatalf("status: %v", created.GetStatus())
	}

	got, err := s.docs.Get(ctx, &paperlessv1.GetDocumentRequest{TenantId: tenant, Id: created.GetId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetName() != "Quarterly Report" {
		t.Fatalf("get: %+v", got)
	}

	list, err := s.docs.List(ctx, &paperlessv1.ListDocumentsRequest{TenantId: tenant})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.GetDocuments()) != 1 {
		t.Fatalf("list = %d, want 1", len(list.GetDocuments()))
	}

	hits, err := s.docs.Search(ctx, &paperlessv1.SearchDocumentsRequest{TenantId: tenant, Query: "quarterly"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits.GetHits()) != 1 || hits.GetHits()[0].GetId() != created.GetId() {
		t.Fatalf("search hits: %+v", hits.GetHits())
	}
}

func TestCategoryCreateGetTree(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()

	root, err := s.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, Name: "Finance"})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	if root.GetPath() != "/Finance" {
		t.Fatalf("root path: %+v", root)
	}
	child, err := s.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, ParentId: root.GetId(), Name: "Invoices"})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if child.GetPath() != "/Finance/Invoices" || child.GetDepth() != 1 {
		t.Fatalf("child: %+v", child)
	}

	tree, err := s.cats.GetTree(ctx, &paperlessv1.GetTreeRequest{TenantId: tenant})
	if err != nil {
		t.Fatalf("tree: %v", err)
	}
	if len(tree.GetNodes()) != 1 || tree.GetNodes()[0].GetCategory().GetName() != "Finance" {
		t.Fatalf("tree roots: %+v", tree.GetNodes())
	}
	if kids := tree.GetNodes()[0].GetChildren(); len(kids) != 1 || kids[0].GetCategory().GetName() != "Invoices" {
		t.Fatalf("tree children: %+v", tree.GetNodes()[0].GetChildren())
	}
}

func TestPermissionGrantCheckEffective(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()

	doc, err := s.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{
		TenantId: tenant, Name: "Shared", MimeType: "text/plain", Content: []byte("x"),
	})
	if err != nil {
		t.Fatalf("create doc: %v", err)
	}

	granted, err := s.perms.GrantAccess(ctx, &paperlessv1.GrantAccessRequest{
		TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, ResourceId: doc.GetId(),
		SubjectType: paperlessv1.SubjectType_SUBJECT_TYPE_USER, SubjectId: "user-x",
		Relation: paperlessv1.Relation_RELATION_VIEWER,
	})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if granted.GetId() == "" || granted.GetRelation() != paperlessv1.Relation_RELATION_VIEWER {
		t.Fatalf("granted: %+v", granted)
	}

	chk, err := s.perms.CheckAccess(ctx, &paperlessv1.CheckAccessRequest{
		TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, ResourceId: doc.GetId(),
		Permission: paperlessv1.Permission_PERMISSION_READ,
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !chk.GetAllowed() {
		t.Fatalf("check allowed = false, want true")
	}

	eff, err := s.perms.GetEffectivePermissions(ctx, &paperlessv1.GetEffectivePermissionsRequest{
		TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, ResourceId: doc.GetId(),
	})
	if err != nil {
		t.Fatalf("effective: %v", err)
	}
	if p := eff.GetPermissions(); p == nil || !p.GetRead() || !p.GetWrite() {
		t.Fatalf("effective perms: %+v", eff.GetPermissions())
	}
}

func TestStatisticsGetStatistics(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()

	if _, err := s.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{
		TenantId: tenant, Name: "Doc", MimeType: "text/plain", Content: []byte("abcde"),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	st, err := s.stats.GetStatistics(ctx, &paperlessv1.GetStatisticsRequest{TenantId: tenant})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if st.GetDocumentsTotal() != 1 {
		t.Fatalf("documents_total = %d, want 1", st.GetDocumentsTotal())
	}
	if st.GetStorageBytes() != 5 {
		t.Fatalf("storage_bytes = %d, want 5", st.GetStorageBytes())
	}
	if st.GetDocumentsByStatus()[store.DocActive] != 1 {
		t.Fatalf("documents_by_status: %+v", st.GetDocumentsByStatus())
	}
}

func TestUnauthenticatedAndBadTenant(t *testing.T) {
	s := kit(t)

	// No SPIFFE peer -> Unauthenticated.
	withFakeCaller(t, "", false)
	_, err := s.docs.List(context.Background(), &paperlessv1.ListDocumentsRequest{TenantId: tenant})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no peer: code = %v, want Unauthenticated", status.Code(err))
	}

	// Peer present but tenant is not a uuid -> InvalidArgument.
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	_, err = s.docs.List(context.Background(), &paperlessv1.ListDocumentsRequest{TenantId: "not-a-uuid"})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("bad tenant: code = %v, want InvalidArgument", status.Code(err))
	}
}

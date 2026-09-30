package grpcapi

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	paperlessv1 "github.com/go-tangra/go-tangra-paperless/sdk/v4/api/proto/paperless/v1"
	"github.com/go-tangra/go-tangra-paperless/sdk/v4/pkg/paperlessclient"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/blob"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/categories"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/documents"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/events"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/memstore"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/permissions"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/search"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/stats"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
)

const assetSvc = "spiffe://example.org/svc/asset"

// bareKit wires the real services over an in-memory store WITHOUT any seeded
// grant: the calling service starts with no tuples and no roles, exactly like a
// module calling paperless in a fresh tenant.
type bareKit struct {
	mem    *memstore.Mem
	docs   *documents.Service
	search *search.Service
	client *paperlessclient.Client
}

func newBareKit(t *testing.T) bareKit {
	t.Helper()
	mem := memstore.New()
	az := authz.New(mem)
	docs := documents.New(mem, az, blob.NewFake(), events.HubPublisher{}, time.Minute)
	srch := search.New(mem, az)

	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	Register(gs, Deps{
		Documents: docs, Categories: categories.New(mem, az), Permissions: permissions.New(mem, az),
		Search: srch, Stats: stats.New(mem),
	})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return bareKit{mem: mem, docs: docs, search: srch, client: paperlessclient.New(conn)}
}

func ownerTuple(t *testing.T, mem *memstore.Mem, subject, resourceType, resourceID string) bool {
	t.Helper()
	grants, err := mem.GrantsForSubjects(context.Background(), tenant, subject, nil, time.Now())
	if err != nil {
		t.Fatalf("grants: %v", err)
	}
	for _, g := range grants {
		if g.SubjectType == store.SubjectUser && g.ResourceType == resourceType && g.ResourceID == resourceID &&
			g.Relation == store.RelationOwner {
			return true
		}
	}
	return false
}

// The asset module's flow over the real paperless.v1 servers, as a service
// subject (ActorKind service, UserID = SPIFFE id, no roles): resolve-or-create
// /Assets/<tag>, store documents in it, read, download, search, delete.
func TestServiceCallerAssetDocumentsFlow(t *testing.T) {
	k := newBareKit(t)
	withFakeCaller(t, assetSvc, true)
	ctx := context.Background()
	c := k.client

	catID, err := c.EnsureCategory(ctx, tenant, []string{"Assets", "AT-000123"})
	if err != nil {
		t.Fatalf("ensure category: %v", err)
	}
	again, err := c.EnsureCategory(ctx, tenant, []string{"Assets", "AT-000123"})
	if err != nil || again != catID {
		t.Fatalf("ensure is not idempotent: %s vs %s (%v)", again, catID, err)
	}
	cats, err := k.mem.ListCategories(ctx, tenant)
	if err != nil || len(cats) != 2 {
		t.Fatalf("categories: %d %v", len(cats), err)
	}
	for _, cat := range cats {
		if !ownerTuple(t, k.mem, assetSvc, store.ResourceCategory, cat.ID) {
			t.Fatalf("service is not owner of category %s", cat.Path)
		}
	}

	manual, err := c.CreateDocument(ctx, tenant, paperlessclient.CreateInput{
		CategoryID: catID, Name: "Warranty manual", FileName: "manual.pdf", MimeType: "application/pdf",
		Tags: map[string]string{"asset": "AT-000123"}, Content: []byte("%PDF-1.7 warranty"),
	})
	if err != nil {
		t.Fatalf("create document: %v", err)
	}
	invoice, err := c.CreateDocument(ctx, tenant, paperlessclient.CreateInput{
		CategoryID: catID, Name: "Purchase invoice", FileName: "invoice.pdf", MimeType: "application/pdf",
		Content: []byte("%PDF-1.7 invoice"),
	})
	if err != nil {
		t.Fatalf("create second document: %v", err)
	}
	if manual.CategoryPath != "/Assets/AT-000123" || manual.CreatedBy != assetSvc {
		t.Fatalf("document: %+v", manual)
	}
	for _, id := range []string{manual.ID, invoice.ID} {
		if !ownerTuple(t, k.mem, assetSvc, store.ResourceDocument, id) {
			t.Fatalf("service is not owner of document %s", id)
		}
	}

	got, err := c.GetDocument(ctx, tenant, manual.ID)
	if err != nil || got.Name != "Warranty manual" || got.Tags["asset"] != "AT-000123" {
		t.Fatalf("get: %+v %v", got, err)
	}
	content, fileName, mimeType, err := c.DownloadDocument(ctx, tenant, manual.ID)
	if err != nil || string(content) != "%PDF-1.7 warranty" || fileName != "manual.pdf" || mimeType != "application/pdf" {
		t.Fatalf("download: %q %q %q %v", content, fileName, mimeType, err)
	}

	hits, err := c.Search(ctx, tenant, "warranty", 10)
	if err != nil || len(hits) != 1 || hits[0].ID != manual.ID || hits[0].CategoryID != catID ||
		hits[0].CategoryPath != "/Assets/AT-000123" {
		t.Fatalf("search: %+v %v", hits, err)
	}
	if hits, err = c.Search(ctx, tenant, "", 10); err != nil || len(hits) != 2 {
		t.Fatalf("search all own: %+v %v", hits, err)
	}

	// Documents are NOT collection-scoped for services: listing needs
	// tenant-wide read, which a service does not hold.
	if _, err := c.ListDocuments(ctx, tenant, catID, ""); !errors.Is(err, paperlessclient.ErrForbidden) {
		t.Fatalf("service list documents: want ErrForbidden, got %v", err)
	}

	if err := c.DeleteDocument(ctx, tenant, invoice.ID, true); err != nil {
		t.Fatalf("hard delete: %v", err)
	}
	if _, err := c.GetDocument(ctx, tenant, invoice.ID); !errors.Is(err, paperlessclient.ErrNotFound) {
		t.Fatalf("get after hard delete: want ErrNotFound, got %v", err)
	}
	if err := c.DeleteDocument(ctx, tenant, manual.ID, false); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if hits, err = c.Search(ctx, tenant, "", 10); err != nil || len(hits) != 0 {
		t.Fatalf("search after delete: %+v %v", hits, err)
	}
}

// Another service (or the same service in another tenant) sees nothing the
// asset service created; a tenant admin user sees everything.
func TestServiceCallerIsolationAndAdminVisibility(t *testing.T) {
	k := newBareKit(t)
	withFakeCaller(t, assetSvc, true)
	ctx := context.Background()

	catID, err := k.client.EnsureCategory(ctx, tenant, []string{"Assets", "AT-000200"})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	doc, err := k.client.CreateDocument(ctx, tenant, paperlessclient.CreateInput{
		CategoryID: catID, Name: "Service contract", FileName: "c.pdf", MimeType: "application/pdf", Content: []byte("x"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Tenant admin (HTTP user with the admin role) sees the document in search
	// and in the document list.
	admin := authz.Subjects{TenantID: tenant, UserID: "user-admin", Roles: []string{"admin"}, ActorKind: "user"}
	hits, err := k.search.Search(ctx, admin, "contract", 10)
	if err != nil || len(hits) != 1 || hits[0].ID != doc.ID {
		t.Fatalf("admin search: %+v %v", hits, err)
	}
	list, err := k.docs.List(ctx, admin, repo.DocFilter{CategoryID: catID})
	if err != nil || len(list) != 1 || list[0].ID != doc.ID {
		t.Fatalf("admin list: %+v %v", list, err)
	}
	// A plain member without grants does not.
	member := authz.Subjects{TenantID: tenant, UserID: "user-member", ActorKind: "user"}
	if hits, err := k.search.Search(ctx, member, "contract", 10); err != nil || len(hits) != 0 {
		t.Fatalf("member search: %+v %v", hits, err)
	}

	// A different service: its search is empty and the document is masked.
	withFakeCaller(t, "spiffe://example.org/svc/other", true)
	if hits, err := k.client.Search(ctx, tenant, "contract", 10); err != nil || len(hits) != 0 {
		t.Fatalf("other service search: %+v %v", hits, err)
	}
	if _, err := k.client.GetDocument(ctx, tenant, doc.ID); !errors.Is(err, paperlessclient.ErrForbidden) {
		t.Fatalf("other service get: want ErrForbidden, got %v", err)
	}
	if err := k.client.DeleteDocument(ctx, tenant, doc.ID, true); !errors.Is(err, paperlessclient.ErrForbidden) {
		t.Fatalf("other service delete: want ErrForbidden, got %v", err)
	}
	// It may see the folder tree (names only) and resolve the same folders, but
	// cannot file documents into a folder it does not hold write on.
	sameID, err := k.client.EnsureCategory(ctx, tenant, []string{"Assets", "AT-000200"})
	if err != nil || sameID != catID {
		t.Fatalf("other service resolve: %s %v", sameID, err)
	}
	if _, err := k.client.CreateDocument(ctx, tenant, paperlessclient.CreateInput{CategoryID: catID, Name: "x", Content: []byte("x")}); !errors.Is(err, paperlessclient.ErrForbidden) {
		t.Fatalf("other service create in foreign folder: want ErrForbidden, got %v", err)
	}
	// Tenant isolation: the asset service in another tenant sees nothing.
	withFakeCaller(t, assetSvc, true)
	const otherTenant = "22222222-2222-2222-2222-222222222222"
	if hits, err := k.client.Search(ctx, otherTenant, "contract", 10); err != nil || len(hits) != 0 {
		t.Fatalf("cross-tenant search: %+v %v", hits, err)
	}
	if _, err := k.client.GetDocument(ctx, otherTenant, doc.ID); !errors.Is(err, paperlessclient.ErrNotFound) {
		t.Fatalf("cross-tenant get: want ErrNotFound, got %v", err)
	}
}

// A duplicate sibling surfaces as AlreadyExists (not InvalidArgument) so
// resolve-or-create callers can re-read; a blank name stays InvalidArgument.
func TestCategoryDuplicateIsAlreadyExists(t *testing.T) {
	k := newBareKit(t)
	withFakeCaller(t, assetSvc, true)
	cats := &CategoryServer{Svc: categories.New(k.mem, authz.New(k.mem))}
	ctx := context.Background()
	if _, err := cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, Name: "Assets"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err := cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, Name: "Assets"})
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("duplicate: want AlreadyExists, got %v", err)
	}
	_, err = cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, Name: "  "})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("blank: want InvalidArgument, got %v", err)
	}
}

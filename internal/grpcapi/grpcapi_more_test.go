package grpcapi

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paperlessv1 "github.com/go-freya/freya/services/paperless/api/proto/paperless/v1"
	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/blob"
	"github.com/go-freya/freya/services/paperless/internal/categories"
	"github.com/go-freya/freya/services/paperless/internal/documents"
	"github.com/go-freya/freya/services/paperless/internal/events"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/permissions"
	"github.com/go-freya/freya/services/paperless/internal/search"
	"github.com/go-freya/freya/services/paperless/internal/stats"
	"time"
)

// kitNoGrant builds the servers WITHOUT the tenant-wide owner seed, so callers
// only hold the grants they create — used to drive PermissionDenied.
func kitNoGrant(t *testing.T) servers {
	t.Helper()
	mem := memstore.New()
	az := authz.New(mem)
	bs := blob.NewFake()
	pub := events.HubPublisher{}
	docs := documents.New(mem, az, bs, pub, time.Minute)
	return servers{
		docs:  &DocumentServer{Docs: docs, Searcher: search.New(mem, az)},
		cats:  &CategoryServer{Svc: categories.New(mem, az)},
		perms: &PermissionServer{Svc: permissions.New(mem, az)},
		stats: &StatisticsServer{Svc: stats.New(mem)},
		mem:   mem,
	}
}

func TestDocumentLifecycleGRPC(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()

	// Create a category to move into.
	cat, err := s.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, Name: "Bin"})
	if err != nil {
		t.Fatalf("category: %v", err)
	}

	// Create an email-sourced document (round-trips the source enum).
	doc, err := s.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{
		TenantId: tenant, Name: "Letter", MimeType: "text/plain", FileName: "l.txt",
		Content: []byte("dear sir"), Source: paperlessv1.DocumentSource_DOCUMENT_SOURCE_EMAIL,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if doc.GetSource() != paperlessv1.DocumentSource_DOCUMENT_SOURCE_EMAIL {
		t.Fatalf("source round-trip: %v", doc.GetSource())
	}

	// Update metadata.
	upd, err := s.docs.Update(ctx, &paperlessv1.UpdateDocumentRequest{
		TenantId: tenant, Id: doc.GetId(), Name: "Renamed Letter", Description: "d",
	})
	if err != nil || upd.GetName() != "Renamed Letter" {
		t.Fatalf("update: %+v err=%v", upd, err)
	}

	// Move into the category.
	mv, err := s.docs.Move(ctx, &paperlessv1.MoveDocumentRequest{TenantId: tenant, Id: doc.GetId(), CategoryId: cat.GetId()})
	if err != nil || mv.GetCategoryId() != cat.GetId() {
		t.Fatalf("move: %+v err=%v", mv, err)
	}

	// Download returns the bytes, mime, and file name.
	dl, err := s.docs.Download(ctx, &paperlessv1.DownloadDocumentRequest{TenantId: tenant, Id: doc.GetId()})
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if string(dl.GetContent()) != "dear sir" || dl.GetFileName() != "l.txt" || dl.GetMimeType() != "text/plain" {
		t.Fatalf("download payload: %+v", dl)
	}

	// Presigned URL.
	url, err := s.docs.GetDownloadUrl(ctx, &paperlessv1.GetDownloadUrlRequest{TenantId: tenant, Id: doc.GetId()})
	if err != nil || url.GetUrl() == "" {
		t.Fatalf("download url: %+v err=%v", url, err)
	}

	// List with status/source/processing filters (round-trips those enums).
	list, err := s.docs.List(ctx, &paperlessv1.ListDocumentsRequest{
		TenantId: tenant, Status: paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE,
		Source: paperlessv1.DocumentSource_DOCUMENT_SOURCE_EMAIL, CategoryId: cat.GetId(),
		ProcessingStatus: paperlessv1.ProcessingStatus_PROCESSING_STATUS_PENDING, Limit: 10,
	})
	if err != nil || len(list.GetDocuments()) != 1 {
		t.Fatalf("filtered list: %d err=%v", len(list.GetDocuments()), err)
	}

	// A second document to batch-delete.
	doc2, err := s.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{TenantId: tenant, Name: "Two", Content: []byte("y")})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}
	bd, err := s.docs.BatchDelete(ctx, &paperlessv1.BatchDeleteRequest{TenantId: tenant, Ids: []string{doc2.GetId()}, Hard: true})
	if err != nil || len(bd.GetResults()) != 1 || !bd.GetResults()[0].GetOk() {
		t.Fatalf("batch delete: %+v err=%v", bd.GetResults(), err)
	}

	// Soft delete the first document.
	if _, err := s.docs.Delete(ctx, &paperlessv1.DeleteDocumentRequest{TenantId: tenant, Id: doc.GetId()}); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestCategoryLifecycleAndErrorsGRPC(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()

	root, err := s.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, Name: "Root"})
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	child, err := s.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, ParentId: root.GetId(), Name: "Child"})
	if err != nil {
		t.Fatalf("child: %v", err)
	}

	// Update.
	if _, err := s.cats.Update(ctx, &paperlessv1.UpdateCategoryRequest{TenantId: tenant, Id: child.GetId(), Name: "Kid"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	// List.
	list, err := s.cats.List(ctx, &paperlessv1.ListCategoriesRequest{TenantId: tenant})
	if err != nil || len(list.GetCategories()) != 2 {
		t.Fatalf("list: %d err=%v", len(list.GetCategories()), err)
	}

	// InvalidArgument: empty name is a ValidationError.
	if _, err := s.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{TenantId: tenant, Name: ""}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("empty name: code = %v", status.Code(err))
	}

	// FailedPrecondition: delete a non-empty category without cascade (ErrNotEmpty).
	if _, err := s.cats.Delete(ctx, &paperlessv1.DeleteCategoryRequest{TenantId: tenant, Id: root.GetId()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("delete non-empty: code = %v", status.Code(err))
	}

	// FailedPrecondition: move root under its own child (ErrCycle).
	if _, err := s.cats.Move(ctx, &paperlessv1.MoveCategoryRequest{TenantId: tenant, Id: root.GetId(), NewParentId: child.GetId()}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("cycle move: code = %v", status.Code(err))
	}

	// Happy-path move child back to a root, then cascade-delete root.
	if _, err := s.cats.Move(ctx, &paperlessv1.MoveCategoryRequest{TenantId: tenant, Id: child.GetId(), NewParentId: ""}); err != nil {
		t.Fatalf("move to root: %v", err)
	}
	if _, err := s.cats.Delete(ctx, &paperlessv1.DeleteCategoryRequest{TenantId: tenant, Id: root.GetId(), Cascade: true}); err != nil {
		t.Fatalf("cascade delete: %v", err)
	}
}

func TestPermissionRPCsGRPC(t *testing.T) {
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	ctx := context.Background()

	doc, err := s.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{TenantId: tenant, Name: "Shared", Content: []byte("x")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	granted, err := s.perms.GrantAccess(ctx, &paperlessv1.GrantAccessRequest{
		TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, ResourceId: doc.GetId(),
		SubjectType: paperlessv1.SubjectType_SUBJECT_TYPE_USER, SubjectId: "user-y",
		Relation: paperlessv1.Relation_RELATION_EDITOR,
	})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}

	// ListPermissions returns the grant just made (plus the creator's owner tuple).
	lp, err := s.perms.ListPermissions(ctx, &paperlessv1.ListPermissionsRequest{
		TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, ResourceId: doc.GetId(),
	})
	if err != nil || len(lp.GetPermissions()) == 0 {
		t.Fatalf("list permissions: %+v err=%v", lp.GetPermissions(), err)
	}

	// ListAccessibleResources: the tenant-wide owner seed makes everything accessible.
	lar, err := s.perms.ListAccessibleResources(ctx, &paperlessv1.ListAccessibleResourcesRequest{
		TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT,
	})
	if err != nil || !lar.GetAll() {
		t.Fatalf("list accessible: all=%v err=%v", lar.GetAll(), err)
	}

	// Revoke the grant by id.
	if _, err := s.perms.RevokeAccess(ctx, &paperlessv1.RevokeAccessRequest{
		TenantId: tenant, ResourceType: paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, ResourceId: doc.GetId(),
		Id: granted.GetId(),
	}); err != nil {
		t.Fatalf("revoke: %v", err)
	}
}

func TestGRPCErrorMappings(t *testing.T) {
	ctx := context.Background()

	// NotFound: a document that does not exist (caller is not admin, existence
	// check fails → authz.ErrNotFound → documents.ErrNotFound).
	s := kit(t)
	withFakeCaller(t, "spiffe://example.org/svc/mailroom", true)
	if _, err := s.docs.Get(ctx, &paperlessv1.GetDocumentRequest{
		TenantId: tenant, Id: "88888888-8888-7888-8888-888888888888",
	}); status.Code(err) != codes.NotFound {
		t.Fatalf("get missing: code = %v, want NotFound", status.Code(err))
	}

	// PermissionDenied: caller B reads a document owned by caller A with no
	// tenant-wide grant in play.
	ng := kitNoGrant(t)
	withFakeCaller(t, "spiffe://example.org/svc/alice", true)
	doc, err := ng.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{TenantId: tenant, Name: "As", Content: []byte("x")})
	if err != nil {
		t.Fatalf("create as alice: %v", err)
	}
	withFakeCaller(t, "spiffe://example.org/svc/bob", true)
	if _, err := ng.docs.Get(ctx, &paperlessv1.GetDocumentRequest{TenantId: tenant, Id: doc.GetId()}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("bob reads alice doc: code = %v, want PermissionDenied", status.Code(err))
	}
}

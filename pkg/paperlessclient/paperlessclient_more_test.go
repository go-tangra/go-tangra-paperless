package paperlessclient_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	paperlessv1 "github.com/go-tangra/go-tangra-paperless/v4/api/proto/paperless/v1"
	"github.com/go-tangra/go-tangra-paperless/v4/pkg/paperlessclient"
)

// echoDocs records the last List request and can return a fixed doc set plus a
// canned download; when errCode != OK every method fails with it.
type echoDocs struct {
	paperlessv1.UnimplementedPaperlessDocumentServiceServer
	lastList *paperlessv1.ListDocumentsRequest
	errCode  codes.Code
}

func (s *echoDocs) List(_ context.Context, req *paperlessv1.ListDocumentsRequest) (*paperlessv1.ListDocumentsResponse, error) {
	if s.errCode != codes.OK {
		return nil, status.Error(s.errCode, "denied")
	}
	s.lastList = req
	// One document per status/source/processing value so the string mappers are
	// exercised on decode.
	mk := func(id string, st paperlessv1.DocumentStatus, src paperlessv1.DocumentSource, pr paperlessv1.ProcessingStatus) *paperlessv1.Document {
		return &paperlessv1.Document{Id: id, Name: id, Status: st, Source: src, ProcessingStatus: pr}
	}
	return &paperlessv1.ListDocumentsResponse{Documents: []*paperlessv1.Document{
		mk("a", paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE, paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD, paperlessv1.ProcessingStatus_PROCESSING_STATUS_PENDING),
		mk("b", paperlessv1.DocumentStatus_DOCUMENT_STATUS_ARCHIVED, paperlessv1.DocumentSource_DOCUMENT_SOURCE_EMAIL, paperlessv1.ProcessingStatus_PROCESSING_STATUS_PROCESSING),
		mk("c", paperlessv1.DocumentStatus_DOCUMENT_STATUS_DELETED, paperlessv1.DocumentSource_DOCUMENT_SOURCE_UNSPECIFIED, paperlessv1.ProcessingStatus_PROCESSING_STATUS_COMPLETED),
		mk("d", paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE, paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD, paperlessv1.ProcessingStatus_PROCESSING_STATUS_FAILED),
	}}, nil
}

func (s *echoDocs) Download(_ context.Context, req *paperlessv1.DownloadDocumentRequest) (*paperlessv1.DownloadDocumentResponse, error) {
	if s.errCode != codes.OK {
		return nil, status.Error(s.errCode, "denied")
	}
	return &paperlessv1.DownloadDocumentResponse{
		Content: []byte("file-bytes"), MimeType: "application/pdf", FileName: "doc.pdf",
	}, nil
}

// recPerms records the enum encodings the client produced.
type recPerms struct {
	paperlessv1.UnimplementedPaperlessPermissionServiceServer
	lastGrant *paperlessv1.GrantAccessRequest
	lastCheck *paperlessv1.CheckAccessRequest
}

func (s *recPerms) GrantAccess(_ context.Context, req *paperlessv1.GrantAccessRequest) (*paperlessv1.PermissionTuple, error) {
	s.lastGrant = req
	return &paperlessv1.PermissionTuple{Id: "p"}, nil
}

func (s *recPerms) CheckAccess(_ context.Context, req *paperlessv1.CheckAccessRequest) (*paperlessv1.CheckAccessResponse, error) {
	s.lastCheck = req
	return &paperlessv1.CheckAccessResponse{Allowed: true}, nil
}

func dialWith(t *testing.T, docs paperlessv1.PaperlessDocumentServiceServer, perms paperlessv1.PaperlessPermissionServiceServer) *paperlessclient.Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	if docs != nil {
		paperlessv1.RegisterPaperlessDocumentServiceServer(gs, docs)
	}
	if perms != nil {
		paperlessv1.RegisterPaperlessPermissionServiceServer(gs, perms)
	}
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return paperlessclient.New(conn)
}

// TestListDocumentsStatusEncoding checks the status filter enum encoding and the
// status/source/processing string mappers on the returned documents.
func TestListDocumentsStatusEncoding(t *testing.T) {
	ctx := context.Background()
	stub := &echoDocs{}
	c := dialWith(t, stub, nil)

	cases := map[string]paperlessv1.DocumentStatus{
		"active":   paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE,
		"archived": paperlessv1.DocumentStatus_DOCUMENT_STATUS_ARCHIVED,
		"deleted":  paperlessv1.DocumentStatus_DOCUMENT_STATUS_DELETED,
		"":         paperlessv1.DocumentStatus_DOCUMENT_STATUS_UNSPECIFIED,
	}
	for in, want := range cases {
		docs, err := c.ListDocuments(ctx, "t1", "cat-1", in)
		if err != nil {
			t.Fatalf("list %q: %v", in, err)
		}
		if stub.lastList.GetStatus() != want {
			t.Fatalf("status %q encoded as %v, want %v", in, stub.lastList.GetStatus(), want)
		}
		if stub.lastList.GetCategoryId() != "cat-1" {
			t.Fatalf("category filter not forwarded: %q", stub.lastList.GetCategoryId())
		}
		if len(docs) != 4 {
			t.Fatalf("want 4 docs, got %d", len(docs))
		}
	}

	// Verify the decoded status/source/processing strings across the doc set.
	docs, _ := c.ListDocuments(ctx, "t1", "", "")
	byID := map[string]paperlessclient.Document{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	if byID["a"].Status != "active" || byID["a"].Source != "upload" || byID["a"].ProcessingStatus != "pending" {
		t.Fatalf("doc a: %+v", byID["a"])
	}
	if byID["b"].Status != "archived" || byID["b"].Source != "email" || byID["b"].ProcessingStatus != "processing" {
		t.Fatalf("doc b: %+v", byID["b"])
	}
	if byID["c"].Status != "deleted" || byID["c"].ProcessingStatus != "completed" {
		t.Fatalf("doc c: %+v", byID["c"])
	}
	if byID["d"].ProcessingStatus != "failed" {
		t.Fatalf("doc d: %+v", byID["d"])
	}
}

// TestDownloadBytes checks Download returns the content, mime, and file name.
func TestDownloadBytes(t *testing.T) {
	c := dialWith(t, &echoDocs{}, nil)
	content, mime, name, err := c.Download(context.Background(), "t1", "doc-1")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if string(content) != "file-bytes" || mime != "application/pdf" || name != "doc.pdf" {
		t.Fatalf("download: %q %q %q", content, mime, name)
	}
}

// TestGrantCheckEnumEncoding round-trips every resource/subject/relation and
// permission value through the request builders.
func TestGrantCheckEnumEncoding(t *testing.T) {
	ctx := context.Background()
	perms := &recPerms{}
	c := dialWith(t, &echoDocs{}, perms)

	relCases := map[string]paperlessv1.Relation{
		"owner":  paperlessv1.Relation_RELATION_OWNER,
		"editor": paperlessv1.Relation_RELATION_EDITOR,
		"viewer": paperlessv1.Relation_RELATION_VIEWER,
		"sharer": paperlessv1.Relation_RELATION_SHARER,
	}
	subCases := map[string]paperlessv1.SubjectType{
		"user":   paperlessv1.SubjectType_SUBJECT_TYPE_USER,
		"role":   paperlessv1.SubjectType_SUBJECT_TYPE_ROLE,
		"tenant": paperlessv1.SubjectType_SUBJECT_TYPE_TENANT,
	}
	resCases := map[string]paperlessv1.ResourceType{
		"document": paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT,
		"category": paperlessv1.ResourceType_RESOURCE_TYPE_CATEGORY,
	}
	for rs, wantRes := range resCases {
		for sub, wantSub := range subCases {
			for rel, wantRel := range relCases {
				if err := c.GrantAccess(ctx, paperlessclient.GrantInput{
					TenantID: "t1", ResourceType: rs, ResourceID: "r", SubjectType: sub, SubjectID: "s", Relation: rel,
				}); err != nil {
					t.Fatalf("grant %s/%s/%s: %v", rs, sub, rel, err)
				}
				g := perms.lastGrant
				if g.GetResourceType() != wantRes || g.GetSubjectType() != wantSub || g.GetRelation() != wantRel {
					t.Fatalf("encoded res=%v sub=%v rel=%v for %s/%s/%s", g.GetResourceType(), g.GetSubjectType(), g.GetRelation(), rs, sub, rel)
				}
			}
		}
	}

	permCases := map[string]paperlessv1.Permission{
		"read":     paperlessv1.Permission_PERMISSION_READ,
		"write":    paperlessv1.Permission_PERMISSION_WRITE,
		"delete":   paperlessv1.Permission_PERMISSION_DELETE,
		"share":    paperlessv1.Permission_PERMISSION_SHARE,
		"download": paperlessv1.Permission_PERMISSION_DOWNLOAD,
	}
	for act, want := range permCases {
		if _, err := c.CheckAccess(ctx, "t1", "document", "r", act); err != nil {
			t.Fatalf("check %s: %v", act, err)
		}
		if perms.lastCheck.GetPermission() != want {
			t.Fatalf("permission %s encoded as %v", act, perms.lastCheck.GetPermission())
		}
	}
}

// TestErrorSurfacing checks a server-side PermissionDenied reaches the caller.
func TestErrorSurfacing(t *testing.T) {
	c := dialWith(t, &echoDocs{errCode: codes.PermissionDenied}, nil)
	if _, err := c.ListDocuments(context.Background(), "t1", "", ""); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("list: want PermissionDenied, got %v", status.Code(err))
	}
	if _, _, _, err := c.Download(context.Background(), "t1", "x"); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("download: want PermissionDenied, got %v", status.Code(err))
	}
}

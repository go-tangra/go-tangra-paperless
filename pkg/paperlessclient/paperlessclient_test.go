package paperlessclient_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	paperlessv1 "github.com/go-tangra/go-tangra-paperless/v4/api/proto/paperless/v1"
	"github.com/go-tangra/go-tangra-paperless/v4/pkg/paperlessclient"
)

// stubDocs is a canned in-process PaperlessDocumentService. It records the last
// request seen so tests can assert the encoding the client produced.
type stubDocs struct {
	paperlessv1.UnimplementedPaperlessDocumentServiceServer
	lastCreate *paperlessv1.CreateDocumentRequest
	lastSearch *paperlessv1.SearchDocumentsRequest
	failGet    bool
}

func (s *stubDocs) Create(_ context.Context, req *paperlessv1.CreateDocumentRequest) (*paperlessv1.Document, error) {
	s.lastCreate = req
	return &paperlessv1.Document{
		Id: "doc-1", Name: req.GetName(), FileName: req.GetFileName(),
		MimeType: req.GetMimeType(), FileSize: req.GetSize(), Checksum: "abc",
		Status: paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE,
		Source: req.GetSource(), Tags: req.GetTags(),
		ProcessingStatus: paperlessv1.ProcessingStatus_PROCESSING_STATUS_PENDING,
		CreatedAt:        time.Now().Unix(),
	}, nil
}

func (s *stubDocs) Get(_ context.Context, req *paperlessv1.GetDocumentRequest) (*paperlessv1.Document, error) {
	if s.failGet {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &paperlessv1.Document{Id: req.GetId(), Name: "hello"}, nil
}

func (s *stubDocs) Search(_ context.Context, req *paperlessv1.SearchDocumentsRequest) (*paperlessv1.SearchDocumentsResponse, error) {
	s.lastSearch = req
	return &paperlessv1.SearchDocumentsResponse{Hits: []*paperlessv1.SearchHit{
		{Id: "doc-1", Name: "hello", Rank: 0.9, Snippet: "…hello…"},
	}}, nil
}

func (s *stubDocs) GetDownloadUrl(_ context.Context, _ *paperlessv1.GetDownloadUrlRequest) (*paperlessv1.GetDownloadUrlResponse, error) {
	return &paperlessv1.GetDownloadUrlResponse{Url: "https://example/presigned"}, nil
}

type stubPerms struct {
	paperlessv1.UnimplementedPaperlessPermissionServiceServer
	lastGrant *paperlessv1.GrantAccessRequest
	lastCheck *paperlessv1.CheckAccessRequest
}

func (s *stubPerms) GrantAccess(_ context.Context, req *paperlessv1.GrantAccessRequest) (*paperlessv1.PermissionTuple, error) {
	s.lastGrant = req
	return &paperlessv1.PermissionTuple{Id: "perm-1"}, nil
}

func (s *stubPerms) CheckAccess(_ context.Context, req *paperlessv1.CheckAccessRequest) (*paperlessv1.CheckAccessResponse, error) {
	s.lastCheck = req
	return &paperlessv1.CheckAccessResponse{Allowed: req.GetPermission() == paperlessv1.Permission_PERMISSION_READ}, nil
}

func (s *stubPerms) GetEffectivePermissions(_ context.Context, _ *paperlessv1.GetEffectivePermissionsRequest) (*paperlessv1.GetEffectivePermissionsResponse, error) {
	return &paperlessv1.GetEffectivePermissionsResponse{Permissions: &paperlessv1.PermissionSet{Read: true, Download: true}}, nil
}

type stubStats struct {
	paperlessv1.UnimplementedPaperlessStatisticsServiceServer
}

func (s *stubStats) GetStatistics(_ context.Context, _ *paperlessv1.GetStatisticsRequest) (*paperlessv1.Statistics, error) {
	return &paperlessv1.Statistics{
		DocumentsTotal: 3, StorageBytes: 4096,
		DocumentsByStatus: map[string]int64{"active": 3},
		Backlog:           map[string]int64{"pending": 1},
	}, nil
}

func dial(t *testing.T) *paperlessclient.Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	paperlessv1.RegisterPaperlessDocumentServiceServer(gs, &stubDocs{})
	paperlessv1.RegisterPaperlessPermissionServiceServer(gs, &stubPerms{})
	paperlessv1.RegisterPaperlessStatisticsServiceServer(gs, &stubStats{})
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

func TestDocumentCreateGetSearchDownloadURL(t *testing.T) {
	ctx := context.Background()
	c := dial(t)

	doc, err := c.CreateDocument(ctx, paperlessclient.CreateDocumentInput{
		TenantID: "t1", Name: "report", FileName: "r.pdf", MimeType: "application/pdf",
		Content: []byte("hello world"), Tags: map[string]string{"k": "v"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if doc.ID != "doc-1" || doc.Status != "active" || doc.Source != "upload" || doc.ProcessingStatus != "pending" {
		t.Fatalf("decoded doc wrong: %+v", doc)
	}
	if doc.FileSize != int64(len("hello world")) {
		t.Fatalf("size not derived from content: %d", doc.FileSize)
	}

	got, err := c.GetDocument(ctx, "t1", "doc-1")
	if err != nil || got.Name != "hello" {
		t.Fatalf("get: %+v err=%v", got, err)
	}

	hits, err := c.Search(ctx, "t1", "hello", 10)
	if err != nil || len(hits) != 1 || hits[0].Snippet == "" {
		t.Fatalf("search: %+v err=%v", hits, err)
	}

	url, err := c.DownloadURL(ctx, "t1", "doc-1")
	if err != nil || url == "" {
		t.Fatalf("download url: %q err=%v", url, err)
	}
}

func TestGetNotFoundSurfacesError(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	paperlessv1.RegisterPaperlessDocumentServiceServer(gs, &stubDocs{failGet: true})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)
	conn, _ := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	t.Cleanup(func() { _ = conn.Close() })

	_, err := paperlessclient.New(conn).GetDocument(context.Background(), "t1", "missing")
	if status.Code(err) != codes.NotFound {
		t.Fatalf("want NotFound, got %v", err)
	}
}

func TestPermissionsAndStatistics(t *testing.T) {
	ctx := context.Background()
	c := dial(t)

	exp := time.Now().Add(time.Hour)
	if err := c.GrantAccess(ctx, paperlessclient.GrantInput{
		TenantID: "t1", ResourceType: "document", ResourceID: "doc-1",
		SubjectType: "user", SubjectID: "u1", Relation: "viewer", ExpiresAt: &exp,
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	ok, err := c.CheckAccess(ctx, "t1", "document", "doc-1", "read")
	if err != nil || !ok {
		t.Fatalf("check read: ok=%v err=%v", ok, err)
	}
	no, err := c.CheckAccess(ctx, "t1", "document", "doc-1", "delete")
	if err != nil || no {
		t.Fatalf("check delete: no=%v err=%v", no, err)
	}

	eff, err := c.EffectivePermissions(ctx, "t1", "document", "doc-1")
	if err != nil || !eff.Read || !eff.Download || eff.Write {
		t.Fatalf("effective: %+v err=%v", eff, err)
	}

	st, err := c.Statistics(ctx, "t1")
	if err != nil || st.DocumentsTotal != 3 || st.StorageBytes != 4096 || st.Backlog["pending"] != 1 {
		t.Fatalf("stats: %+v err=%v", st, err)
	}
}

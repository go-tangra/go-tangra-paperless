package paperlessclient_test

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	paperlessv1 "github.com/go-tangra/go-tangra-paperless/sdk/v4/api/proto/paperless/v1"
	"github.com/go-tangra/go-tangra-paperless/sdk/v4/pkg/paperlessclient"
)

// serverMaxBytes mirrors the paperless server default limits.max_request_bytes.
const serverMaxBytes = 33 << 20

// fakeDocs is an in-memory PaperlessDocumentService. When fail != OK every
// method returns that code. It records the last request of each kind.
type fakeDocs struct {
	paperlessv1.UnimplementedPaperlessDocumentServiceServer
	mu         sync.Mutex
	fail       codes.Code
	docs       map[string]*paperlessv1.Document
	content    map[string][]byte
	seq        int
	lastCreate *paperlessv1.CreateDocumentRequest
	lastDelete *paperlessv1.DeleteDocumentRequest
	lastList   *paperlessv1.ListDocumentsRequest
	lastSearch *paperlessv1.SearchDocumentsRequest
}

func newFakeDocs() *fakeDocs {
	return &fakeDocs{docs: map[string]*paperlessv1.Document{}, content: map[string][]byte{}}
}

func (f *fakeDocs) err() error {
	if f.fail != codes.OK {
		return status.Error(f.fail, "fake failure")
	}
	return nil
}

func (f *fakeDocs) Create(_ context.Context, req *paperlessv1.CreateDocumentRequest) (*paperlessv1.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.err(); err != nil {
		return nil, err
	}
	f.lastCreate = req
	f.seq++
	d := &paperlessv1.Document{
		Id: fmt.Sprintf("doc-%d", f.seq), TenantId: req.GetTenantId(), CategoryId: req.GetCategoryId(),
		CategoryPath: "/Assets/AT-1", Name: req.GetName(), Description: req.GetDescription(),
		FileName: req.GetFileName(), FileSize: req.GetSize(), MimeType: req.GetMimeType(), Checksum: "sum",
		Status: paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE, Source: req.GetSource(), Tags: req.GetTags(),
		ProcessingStatus: paperlessv1.ProcessingStatus_PROCESSING_STATUS_PENDING,
		CreatedBy:        "spiffe://example.org/svc/asset", CreatedAt: 1700000000, UpdatedAt: 1700000001,
	}
	f.docs[d.Id] = d
	f.content[d.Id] = req.GetContent()
	return d, nil
}

func (f *fakeDocs) Get(_ context.Context, req *paperlessv1.GetDocumentRequest) (*paperlessv1.Document, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.err(); err != nil {
		return nil, err
	}
	d, ok := f.docs[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return d, nil
}

func (f *fakeDocs) Delete(_ context.Context, req *paperlessv1.DeleteDocumentRequest) (*paperlessv1.DeleteDocumentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.err(); err != nil {
		return nil, err
	}
	f.lastDelete = req
	if _, ok := f.docs[req.GetId()]; !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	delete(f.docs, req.GetId())
	return &paperlessv1.DeleteDocumentResponse{}, nil
}

func (f *fakeDocs) Download(_ context.Context, req *paperlessv1.DownloadDocumentRequest) (*paperlessv1.DownloadDocumentResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.err(); err != nil {
		return nil, err
	}
	d, ok := f.docs[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "not_found")
	}
	return &paperlessv1.DownloadDocumentResponse{Content: f.content[d.Id], MimeType: d.MimeType, FileName: d.FileName}, nil
}

func (f *fakeDocs) GetDownloadUrl(_ context.Context, req *paperlessv1.GetDownloadUrlRequest) (*paperlessv1.GetDownloadUrlResponse, error) {
	if err := f.err(); err != nil {
		return nil, err
	}
	return &paperlessv1.GetDownloadUrlResponse{Url: "https://example/presigned/" + req.GetId()}, nil
}

func (f *fakeDocs) List(_ context.Context, req *paperlessv1.ListDocumentsRequest) (*paperlessv1.ListDocumentsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.err(); err != nil {
		return nil, err
	}
	f.lastList = req
	// One document per status/source/processing value so the string mappers
	// are exercised on decode.
	mk := func(id string, st paperlessv1.DocumentStatus, src paperlessv1.DocumentSource, pr paperlessv1.ProcessingStatus) *paperlessv1.Document {
		return &paperlessv1.Document{Id: id, Name: id, Status: st, Source: src, ProcessingStatus: pr}
	}
	return &paperlessv1.ListDocumentsResponse{Documents: []*paperlessv1.Document{
		mk("a", paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE, paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD, paperlessv1.ProcessingStatus_PROCESSING_STATUS_PENDING),
		mk("b", paperlessv1.DocumentStatus_DOCUMENT_STATUS_ARCHIVED, paperlessv1.DocumentSource_DOCUMENT_SOURCE_EMAIL, paperlessv1.ProcessingStatus_PROCESSING_STATUS_PROCESSING),
		mk("c", paperlessv1.DocumentStatus_DOCUMENT_STATUS_DELETED, paperlessv1.DocumentSource_DOCUMENT_SOURCE_UNSPECIFIED, paperlessv1.ProcessingStatus_PROCESSING_STATUS_COMPLETED),
		mk("d", paperlessv1.DocumentStatus_DOCUMENT_STATUS_UNSPECIFIED, paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD, paperlessv1.ProcessingStatus_PROCESSING_STATUS_FAILED),
		mk("e", paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE, paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD, paperlessv1.ProcessingStatus_PROCESSING_STATUS_RETRYING),
		mk("f", paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE, paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD, paperlessv1.ProcessingStatus_PROCESSING_STATUS_UNSPECIFIED),
	}}, nil
}

func (f *fakeDocs) Search(_ context.Context, req *paperlessv1.SearchDocumentsRequest) (*paperlessv1.SearchDocumentsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.err(); err != nil {
		return nil, err
	}
	f.lastSearch = req
	var hits []*paperlessv1.SearchHit
	for _, d := range f.docs {
		if d.GetName() == req.GetQuery() {
			hits = append(hits, &paperlessv1.SearchHit{
				Id: d.Id, Name: d.Name, CategoryId: d.CategoryId, CategoryPath: d.CategoryPath,
				MimeType: d.MimeType, Status: d.Status, Rank: 0.75, Snippet: "…" + d.Name + "…",
			})
		}
	}
	return &paperlessv1.SearchDocumentsResponse{Hits: hits}, nil
}

// fakeCats is an in-memory PaperlessCategoryService enforcing unique sibling
// names (roots too, unless allowDupRoots) like paperless does.
type fakeCats struct {
	paperlessv1.UnimplementedPaperlessCategoryServiceServer
	mu            sync.Mutex
	cats          []*paperlessv1.Category
	seq           int
	allowDupRoots bool
	listCalls     int
	failListAt    map[int]bool // 1-based List call numbers that fail Unavailable
	createFail    codes.Code
	creates       int
	hide          map[string]bool // names List never returns
	// onCreate runs (under the lock) before a Create is applied, e.g. to
	// simulate a concurrent creator winning the race.
	onCreate func(f *fakeCats, req *paperlessv1.CreateCategoryRequest)
}

func (f *fakeCats) add(id, parent, name string) *paperlessv1.Category {
	c := &paperlessv1.Category{Id: id, ParentId: parent, Name: name, Path: "/" + name}
	f.cats = append(f.cats, c)
	return c
}

func (f *fakeCats) List(_ context.Context, req *paperlessv1.ListCategoriesRequest) (*paperlessv1.ListCategoriesResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	if f.failListAt[f.listCalls] {
		return nil, status.Error(codes.Unavailable, "fake list failure")
	}
	out := &paperlessv1.ListCategoriesResponse{}
	for _, c := range f.cats {
		if !f.hide[c.GetName()] {
			out.Categories = append(out.Categories, c)
		}
	}
	return out, nil
}

func (f *fakeCats) Create(_ context.Context, req *paperlessv1.CreateCategoryRequest) (*paperlessv1.Category, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates++
	if f.createFail != codes.OK {
		return nil, status.Error(f.createFail, "fake create failure")
	}
	if f.onCreate != nil {
		f.onCreate(f, req)
	}
	for _, c := range f.cats {
		if c.GetParentId() == req.GetParentId() && c.GetName() == req.GetName() &&
			(req.GetParentId() != "" || !f.allowDupRoots) {
			return nil, status.Error(codes.AlreadyExists, "already_exists")
		}
	}
	f.seq++
	return f.add(fmt.Sprintf("cat-%03d", f.seq), req.GetParentId(), req.GetName()), nil
}

// fakePerms records the enum encodings the client produced.
type fakePerms struct {
	paperlessv1.UnimplementedPaperlessPermissionServiceServer
	fail      codes.Code
	lastGrant *paperlessv1.GrantAccessRequest
	lastCheck *paperlessv1.CheckAccessRequest
}

func (f *fakePerms) GrantAccess(_ context.Context, req *paperlessv1.GrantAccessRequest) (*paperlessv1.PermissionTuple, error) {
	if f.fail != codes.OK {
		return nil, status.Error(f.fail, "fake failure")
	}
	f.lastGrant = req
	return &paperlessv1.PermissionTuple{Id: "perm-1"}, nil
}

func (f *fakePerms) CheckAccess(_ context.Context, req *paperlessv1.CheckAccessRequest) (*paperlessv1.CheckAccessResponse, error) {
	if f.fail != codes.OK {
		return nil, status.Error(f.fail, "fake failure")
	}
	f.lastCheck = req
	return &paperlessv1.CheckAccessResponse{Allowed: req.GetPermission() == paperlessv1.Permission_PERMISSION_READ}, nil
}

func (f *fakePerms) GetEffectivePermissions(_ context.Context, _ *paperlessv1.GetEffectivePermissionsRequest) (*paperlessv1.GetEffectivePermissionsResponse, error) {
	if f.fail != codes.OK {
		return nil, status.Error(f.fail, "fake failure")
	}
	return &paperlessv1.GetEffectivePermissionsResponse{Permissions: &paperlessv1.PermissionSet{Read: true, Download: true}}, nil
}

type fakeStats struct {
	paperlessv1.UnimplementedPaperlessStatisticsServiceServer
	fail codes.Code
}

func (f *fakeStats) GetStatistics(_ context.Context, _ *paperlessv1.GetStatisticsRequest) (*paperlessv1.Statistics, error) {
	if f.fail != codes.OK {
		return nil, status.Error(f.fail, "fake failure")
	}
	return &paperlessv1.Statistics{
		DocumentsTotal: 3, StorageBytes: 4096, CategoriesTotal: 2,
		DocumentsByStatus: map[string]int64{"active": 3},
		Backlog:           map[string]int64{"pending": 1},
	}, nil
}

type fakes struct {
	docs  *fakeDocs
	cats  *fakeCats
	perms *fakePerms
	stats *fakeStats
}

func newFakes() *fakes {
	return &fakes{docs: newFakeDocs(), cats: &fakeCats{}, perms: &fakePerms{}, stats: &fakeStats{}}
}

// dial serves the fakes over bufconn (server accepting serverMaxBytes, like
// paperless by default) and returns a client built with opts.
func dial(t *testing.T, f *fakes, opts ...paperlessclient.Option) *paperlessclient.Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer(grpc.MaxRecvMsgSize(serverMaxBytes))
	paperlessv1.RegisterPaperlessDocumentServiceServer(gs, f.docs)
	paperlessv1.RegisterPaperlessCategoryServiceServer(gs, f.cats)
	paperlessv1.RegisterPaperlessPermissionServiceServer(gs, f.perms)
	paperlessv1.RegisterPaperlessStatisticsServiceServer(gs, f.stats)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return paperlessclient.New(conn, opts...)
}

package paperlessclient_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paperlessv1 "github.com/go-tangra/go-tangra-paperless/sdk/v4/api/proto/paperless/v1"
	"github.com/go-tangra/go-tangra-paperless/sdk/v4/pkg/paperlessclient"
)

const tenant = "11111111-1111-1111-1111-111111111111"

func TestDocumentLifecycle(t *testing.T) {
	f := newFakes()
	c := dial(t, f)
	ctx := context.Background()

	doc, err := c.CreateDocument(ctx, tenant, paperlessclient.CreateInput{
		CategoryID: "cat-1", Name: "invoice", Description: "d", FileName: "inv.pdf",
		MimeType: "application/pdf", Tags: map[string]string{"asset": "AT-1"}, Content: []byte("%PDF"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	req := f.docs.lastCreate
	if req.GetTenantId() != tenant || req.GetSize() != 4 || req.GetCategoryId() != "cat-1" ||
		req.GetSource() != paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD || req.GetTags()["asset"] != "AT-1" {
		t.Fatalf("create request encoding: %+v", req)
	}
	if doc.ID == "" || doc.Status != "active" || doc.Source != "upload" || doc.ProcessingStatus != "pending" ||
		doc.FileSize != 4 || doc.CategoryPath != "/Assets/AT-1" || doc.CreatedAt.IsZero() || doc.UpdatedAt.IsZero() ||
		doc.CreatedBy == "" || doc.Checksum != "sum" || doc.Description != "d" {
		t.Fatalf("create decoding: %+v", doc)
	}

	got, err := c.GetDocument(ctx, tenant, doc.ID)
	if err != nil || got.ID != doc.ID || got.Name != "invoice" {
		t.Fatalf("get: %+v %v", got, err)
	}

	content, fileName, mimeType, err := c.DownloadDocument(ctx, tenant, doc.ID)
	if err != nil || string(content) != "%PDF" || fileName != "inv.pdf" || mimeType != "application/pdf" {
		t.Fatalf("download: %q %q %q %v", content, fileName, mimeType, err)
	}

	url, err := c.DownloadURL(ctx, tenant, doc.ID)
	if err != nil || url != "https://example/presigned/"+doc.ID {
		t.Fatalf("download url: %q %v", url, err)
	}

	hits, err := c.Search(ctx, tenant, "invoice", 10)
	if err != nil || len(hits) != 1 {
		t.Fatalf("search: %+v %v", hits, err)
	}
	h := hits[0]
	if h.ID != doc.ID || h.Name != "invoice" || h.CategoryID != "cat-1" || h.CategoryPath != "/Assets/AT-1" ||
		h.MimeType != "application/pdf" || h.Rank != 0.75 || h.Snippet == "" {
		t.Fatalf("hit decoding: %+v", h)
	}
	if f.docs.lastSearch.GetLimit() != 10 || f.docs.lastSearch.GetTenantId() != tenant {
		t.Fatalf("search request: %+v", f.docs.lastSearch)
	}
	// A negative limit asks for the server default (0 on the wire).
	if _, err := c.Search(ctx, tenant, "invoice", -5); err != nil || f.docs.lastSearch.GetLimit() != 0 {
		t.Fatalf("negative limit: %v %d", err, f.docs.lastSearch.GetLimit())
	}

	if err := c.DeleteDocument(ctx, tenant, doc.ID, true); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !f.docs.lastDelete.GetHard() || f.docs.lastDelete.GetId() != doc.ID {
		t.Fatalf("delete request: %+v", f.docs.lastDelete)
	}
	if _, err := c.GetDocument(ctx, tenant, doc.ID); !errors.Is(err, paperlessclient.ErrNotFound) {
		t.Fatalf("get after delete: want ErrNotFound, got %v", err)
	}
	if err := c.DeleteDocument(ctx, tenant, doc.ID, false); !errors.Is(err, paperlessclient.ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
	if _, _, _, err := c.DownloadDocument(ctx, tenant, doc.ID); !errors.Is(err, paperlessclient.ErrNotFound) {
		t.Fatalf("download missing: want ErrNotFound, got %v", err)
	}
	// The gRPC status stays in the chain.
	if _, err := c.GetDocument(ctx, tenant, "missing"); status.Code(err) != codes.NotFound {
		t.Fatalf("status code lost: %v", err)
	}
}

func TestListDocumentsEncodesFiltersAndDecodesEnums(t *testing.T) {
	f := newFakes()
	c := dial(t, f)
	ctx := context.Background()
	want := map[string]paperlessv1.DocumentStatus{
		"active":   paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE,
		"archived": paperlessv1.DocumentStatus_DOCUMENT_STATUS_ARCHIVED,
		"deleted":  paperlessv1.DocumentStatus_DOCUMENT_STATUS_DELETED,
		"":         paperlessv1.DocumentStatus_DOCUMENT_STATUS_UNSPECIFIED,
	}
	for s, enum := range want {
		docs, err := c.ListDocuments(ctx, tenant, "cat-1", s)
		if err != nil {
			t.Fatalf("list %q: %v", s, err)
		}
		if f.docs.lastList.GetStatus() != enum || f.docs.lastList.GetCategoryId() != "cat-1" {
			t.Fatalf("list %q encoding: %+v", s, f.docs.lastList)
		}
		got := map[string]string{}
		for _, d := range docs {
			got[d.ID] = d.Status + "/" + d.Source + "/" + d.ProcessingStatus
		}
		exp := map[string]string{
			"a": "active/upload/pending", "b": "archived/email/processing", "c": "deleted//completed",
			"d": "/upload/failed", "e": "active/upload/retrying", "f": "active/upload/",
		}
		for id, v := range exp {
			if got[id] != v {
				t.Fatalf("doc %s decoded %q want %q", id, got[id], v)
			}
		}
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		code codes.Code
		want error
	}{
		{codes.NotFound, paperlessclient.ErrNotFound},
		{codes.PermissionDenied, paperlessclient.ErrForbidden},
		{codes.Unavailable, paperlessclient.ErrUnavailable},
		{codes.DeadlineExceeded, paperlessclient.ErrUnavailable},
		{codes.InvalidArgument, nil}, // passed through unchanged
	}
	for _, tc := range cases {
		f := newFakes()
		f.docs.fail, f.perms.fail, f.stats.fail = tc.code, tc.code, tc.code
		f.cats.createFail = tc.code
		c := dial(t, f)
		ctx := context.Background()
		calls := map[string]error{}
		_, calls["create"] = c.CreateDocument(ctx, tenant, paperlessclient.CreateInput{Name: "x"})
		_, calls["get"] = c.GetDocument(ctx, tenant, "x")
		calls["delete"] = c.DeleteDocument(ctx, tenant, "x", false)
		_, _, _, calls["download"] = c.DownloadDocument(ctx, tenant, "x")
		_, calls["downloadURL"] = c.DownloadURL(ctx, tenant, "x")
		_, calls["list"] = c.ListDocuments(ctx, tenant, "", "")
		_, calls["search"] = c.Search(ctx, tenant, "x", 0)
		calls["grant"] = c.GrantAccess(ctx, tenant, paperlessclient.GrantInput{})
		_, calls["check"] = c.CheckAccess(ctx, tenant, "document", "x", "read")
		_, calls["effective"] = c.EffectivePermissions(ctx, tenant, "document", "x")
		_, calls["stats"] = c.Statistics(ctx, tenant)
		if tc.code != codes.InvalidArgument { // InvalidArgument on create is treated as a race by EnsureCategory
			_, calls["ensure"] = c.EnsureCategory(ctx, tenant, []string{"Assets"})
		}
		for name, err := range calls {
			if err == nil {
				t.Fatalf("%s/%v: expected an error", name, tc.code)
			}
			if status.Code(err) != tc.code {
				t.Fatalf("%s/%v: status lost: %v", name, tc.code, err)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("%s/%v: want %v, got %v", name, tc.code, tc.want, err)
			}
			for _, s := range []error{paperlessclient.ErrNotFound, paperlessclient.ErrForbidden, paperlessclient.ErrUnavailable} {
				if s != tc.want && errors.Is(err, s) {
					t.Fatalf("%s/%v: unexpectedly matches %v", name, tc.code, s)
				}
			}
		}
	}
}

func TestPermissionsEncodeEnums(t *testing.T) {
	f := newFakes()
	c := dial(t, f)
	ctx := context.Background()
	exp := time.Unix(1800000000, 0)

	for _, tc := range []struct {
		res, subj, rel string
		wantRes        paperlessv1.ResourceType
		wantSubj       paperlessv1.SubjectType
		wantRel        paperlessv1.Relation
	}{
		{"document", "user", "owner", paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, paperlessv1.SubjectType_SUBJECT_TYPE_USER, paperlessv1.Relation_RELATION_OWNER},
		{"category", "role", "editor", paperlessv1.ResourceType_RESOURCE_TYPE_CATEGORY, paperlessv1.SubjectType_SUBJECT_TYPE_ROLE, paperlessv1.Relation_RELATION_EDITOR},
		{"x", "tenant", "viewer", paperlessv1.ResourceType_RESOURCE_TYPE_UNSPECIFIED, paperlessv1.SubjectType_SUBJECT_TYPE_TENANT, paperlessv1.Relation_RELATION_VIEWER},
		{"document", "x", "sharer", paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, paperlessv1.SubjectType_SUBJECT_TYPE_UNSPECIFIED, paperlessv1.Relation_RELATION_SHARER},
		{"document", "user", "x", paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT, paperlessv1.SubjectType_SUBJECT_TYPE_USER, paperlessv1.Relation_RELATION_UNSPECIFIED},
	} {
		if err := c.GrantAccess(ctx, tenant, paperlessclient.GrantInput{
			ResourceType: tc.res, ResourceID: "r1", SubjectType: tc.subj, SubjectID: "s1", Relation: tc.rel, ExpiresAt: &exp,
		}); err != nil {
			t.Fatalf("grant: %v", err)
		}
		g := f.perms.lastGrant
		if g.GetResourceType() != tc.wantRes || g.GetSubjectType() != tc.wantSubj || g.GetRelation() != tc.wantRel ||
			g.GetExpiresAt() != exp.Unix() || g.GetTenantId() != tenant {
			t.Fatalf("grant encoding %+v: %+v", tc, g)
		}
	}
	if err := c.GrantAccess(ctx, tenant, paperlessclient.GrantInput{ResourceType: "document"}); err != nil || f.perms.lastGrant.GetExpiresAt() != 0 {
		t.Fatalf("grant without expiry: %v %+v", err, f.perms.lastGrant)
	}

	for action, enum := range map[string]paperlessv1.Permission{
		"read": paperlessv1.Permission_PERMISSION_READ, "write": paperlessv1.Permission_PERMISSION_WRITE,
		"delete": paperlessv1.Permission_PERMISSION_DELETE, "share": paperlessv1.Permission_PERMISSION_SHARE,
		"download": paperlessv1.Permission_PERMISSION_DOWNLOAD, "x": paperlessv1.Permission_PERMISSION_UNSPECIFIED,
	} {
		ok, err := c.CheckAccess(ctx, tenant, "document", "d1", action)
		if err != nil || f.perms.lastCheck.GetPermission() != enum || ok != (action == "read") {
			t.Fatalf("check %s: %v %v %+v", action, ok, err, f.perms.lastCheck)
		}
	}

	eff, err := c.EffectivePermissions(ctx, tenant, "document", "d1")
	if err != nil || !eff.Read || !eff.Download || eff.Write || eff.Delete || eff.Share {
		t.Fatalf("effective: %+v %v", eff, err)
	}
}

func TestStatistics(t *testing.T) {
	c := dial(t, newFakes())
	s, err := c.Statistics(context.Background(), tenant)
	if err != nil || s.DocumentsTotal != 3 || s.StorageBytes != 4096 || s.CategoriesTotal != 2 ||
		s.DocumentsByStatus["active"] != 3 || s.Backlog["pending"] != 1 {
		t.Fatalf("statistics: %+v %v", s, err)
	}
}

// A 20 MiB attachment (the asset module's limit) must go up and come back:
// grpc-go's default 4 MiB receive bound would refuse the download, so this
// proves the client's per-call size options are applied.
func TestLargeDocumentRoundTrip(t *testing.T) {
	f := newFakes()
	c := dial(t, f)
	ctx := context.Background()
	payload := bytes.Repeat([]byte{0xAB}, 20<<20)
	doc, err := c.CreateDocument(ctx, tenant, paperlessclient.CreateInput{Name: "big", FileName: "big.bin", MimeType: "application/octet-stream", Content: payload})
	if err != nil {
		t.Fatalf("create 20 MiB: %v", err)
	}
	got, _, _, err := c.DownloadDocument(ctx, tenant, doc.ID)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("download 20 MiB: len=%d err=%v", len(got), err)
	}
}

func TestMaxMessageBytesOption(t *testing.T) {
	f := newFakes()
	c := dial(t, f, paperlessclient.WithMaxMessageBytes(1024), paperlessclient.WithMaxMessageBytes(0))
	ctx := context.Background()
	_, err := c.CreateDocument(ctx, tenant, paperlessclient.CreateInput{Name: "big", Content: make([]byte, 4096)})
	if status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("oversized send: want ResourceExhausted, got %v", err)
	}
	if f.docs.lastCreate != nil {
		t.Fatal("oversized request must not reach the server")
	}
	// Receive side: a small upload through a default client, then a download
	// through the bounded one is refused.
	doc, err := dial(t, f).CreateDocument(ctx, tenant, paperlessclient.CreateInput{Name: "mid", Content: make([]byte, 4096)})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, _, err := c.DownloadDocument(ctx, tenant, doc.ID); status.Code(err) != codes.ResourceExhausted {
		t.Fatalf("oversized receive: want ResourceExhausted, got %v", err)
	}
}

func TestEnsureCategoryCreatesThenReuses(t *testing.T) {
	f := newFakes()
	c := dial(t, f)
	ctx := context.Background()
	id, err := c.EnsureCategory(ctx, tenant, []string{" Assets ", "AT-000123"})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if f.cats.creates != 2 || len(f.cats.cats) != 2 {
		t.Fatalf("want 2 creates, got %d (%d cats)", f.cats.creates, len(f.cats.cats))
	}
	root, leaf := f.cats.cats[0], f.cats.cats[1]
	if root.GetName() != "Assets" || root.GetParentId() != "" || leaf.GetParentId() != root.GetId() || id != leaf.GetId() {
		t.Fatalf("tree: root=%+v leaf=%+v id=%s", root, leaf, id)
	}
	again, err := c.EnsureCategory(ctx, tenant, []string{"Assets", "AT-000123"})
	if err != nil || again != id || f.cats.creates != 2 {
		t.Fatalf("idempotent: %s %v creates=%d", again, err, f.cats.creates)
	}
	sibling, err := c.EnsureCategory(ctx, tenant, []string{"Assets", "AT-000124"})
	if err != nil || sibling == id || f.cats.creates != 3 {
		t.Fatalf("sibling: %s %v creates=%d", sibling, err, f.cats.creates)
	}
	cats, err := c.ListCategories(ctx, tenant)
	if err != nil || len(cats) != 3 || cats[1].ParentID != root.GetId() || cats[0].Path != "/Assets" {
		t.Fatalf("list categories: %+v %v", cats, err)
	}
}

func TestEnsureCategoryRejectsEmptyPath(t *testing.T) {
	c := dial(t, newFakes())
	if _, err := c.EnsureCategory(context.Background(), tenant, nil); err == nil {
		t.Fatal("empty path must be refused")
	}
	if _, err := c.EnsureCategory(context.Background(), tenant, []string{"Assets", "  "}); err == nil {
		t.Fatal("blank segment must be refused")
	}
}

func TestEnsureCategoryListFailures(t *testing.T) {
	ctx := context.Background()
	// Initial read fails.
	f := newFakes()
	f.cats.failListAt = map[int]bool{1: true}
	if _, err := dial(t, f).EnsureCategory(ctx, tenant, []string{"Assets"}); !errors.Is(err, paperlessclient.ErrUnavailable) {
		t.Fatalf("initial list: want ErrUnavailable, got %v", err)
	}
	if _, err := dial(t, f).ListCategories(ctx, tenant); err != nil {
		t.Fatalf("list recovers: %v", err)
	}
	f.cats.failListAt = map[int]bool{3: true}
	if _, err := dial(t, f).ListCategories(ctx, tenant); !errors.Is(err, paperlessclient.ErrUnavailable) {
		t.Fatalf("list categories: want ErrUnavailable, got %v", err)
	}

	// Re-read after creating a root fails.
	f = newFakes()
	f.cats.failListAt = map[int]bool{2: true}
	if _, err := dial(t, f).EnsureCategory(ctx, tenant, []string{"Assets"}); !errors.Is(err, paperlessclient.ErrUnavailable) {
		t.Fatalf("root re-read: want ErrUnavailable, got %v", err)
	}

	// Re-read after losing a race fails.
	f = newFakes()
	f.cats.failListAt = map[int]bool{2: true}
	f.cats.onCreate = func(fc *fakeCats, req *paperlessv1.CreateCategoryRequest) {
		fc.add("cat-000", req.GetParentId(), req.GetName())
	}
	if _, err := dial(t, f).EnsureCategory(ctx, tenant, []string{"Assets"}); !errors.Is(err, paperlessclient.ErrUnavailable) {
		t.Fatalf("race re-read: want ErrUnavailable, got %v", err)
	}
}

func TestEnsureCategoryLosesRace(t *testing.T) {
	ctx := context.Background()
	f := newFakes()
	f.cats.add("cat-root", "", "Assets")
	// A concurrent creator wins the child between our read and our create.
	f.cats.onCreate = func(fc *fakeCats, req *paperlessv1.CreateCategoryRequest) {
		fc.onCreate = nil
		fc.add("cat-winner", req.GetParentId(), req.GetName())
	}
	id, err := dial(t, f).EnsureCategory(ctx, tenant, []string{"Assets", "AT-1"})
	if err != nil || id != "cat-winner" {
		t.Fatalf("race: %s %v", id, err)
	}

	// The create reports a conflict but the node is not visible on re-read: the
	// conflict is returned.
	f = newFakes()
	f.cats.createFail = codes.AlreadyExists
	if _, err := dial(t, f).EnsureCategory(ctx, tenant, []string{"Assets"}); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("invisible conflict: want AlreadyExists, got %v", err)
	}
	// Older servers report duplicates as InvalidArgument: also re-read.
	f = newFakes()
	f.cats.createFail = codes.InvalidArgument
	if _, err := dial(t, f).EnsureCategory(ctx, tenant, []string{"Assets"}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("invalid argument: got %v", err)
	}
}

// Without the root-name unique index two roots can both be created; every
// caller converges on the lowest id.
func TestEnsureCategoryConvergesOnDuplicateRoots(t *testing.T) {
	ctx := context.Background()
	f := newFakes()
	f.cats.allowDupRoots = true
	f.cats.onCreate = func(fc *fakeCats, req *paperlessv1.CreateCategoryRequest) {
		fc.onCreate = nil
		fc.add("cat-000", "", req.GetName()) // the concurrent winner sorts first
	}
	id, err := dial(t, f).EnsureCategory(ctx, tenant, []string{"Assets", "AT-1"})
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	leaf := f.cats.cats[len(f.cats.cats)-1]
	if leaf.GetParentId() != "cat-000" || id != leaf.GetId() {
		t.Fatalf("did not converge on the lowest root: leaf=%+v id=%s", leaf, id)
	}

	// If the re-read does not show the new root, the created id is used.
	f = newFakes()
	f.cats.hide = map[string]bool{"Hidden": true}
	id, err = dial(t, f).EnsureCategory(ctx, tenant, []string{"Hidden"})
	if err != nil || id != "cat-001" {
		t.Fatalf("hidden root: %s %v", id, err)
	}
}

func TestEnsureCategoryConcurrentCallers(t *testing.T) {
	f := newFakes()
	c := dial(t, f)
	const n = 16
	ids := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids[i], errs[i] = c.EnsureCategory(context.Background(), tenant, []string{"Assets", "AT-7"})
		}()
	}
	wg.Wait()
	for i := range n {
		if errs[i] != nil || ids[i] != ids[0] {
			t.Fatalf("caller %d: %s %v (want %s)", i, ids[i], errs[i], ids[0])
		}
	}
	if len(f.cats.cats) != 2 {
		t.Fatalf("want exactly 2 categories, got %d", len(f.cats.cats))
	}
}

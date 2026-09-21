package backup

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/memstore"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

func strp(s string) *string { return &s }

const secretContent = "SUPER-SECRET-EXTRACTED-TEXT"

// seedTenant populates a store with one category, one document (carrying
// content_text), and permission tuples on both.
func seedTenant(t *testing.T, m *memstore.Mem, tenant string) {
	t.Helper()
	ctx := context.Background()
	cat := store.Category{ID: "cat-1", TenantID: tenant, Name: "Invoices", Path: "invoices", SortOrder: 3}
	if err := m.InsertCategory(ctx, cat); err != nil {
		t.Fatalf("seed cat: %v", err)
	}
	doc := store.Document{
		ID: "doc-1", TenantID: tenant, CategoryID: strp("cat-1"), CategoryPath: "invoices",
		Name: "invoice-2026.pdf", ObjectKey: "obj/doc-1", FileName: "invoice-2026.pdf",
		FileSize: 1234, MimeType: "application/pdf", Checksum: "abc", Status: store.DocActive,
		Source: store.SourceUpload, Tags: map[string]string{"year": "2026"},
		ContentText:       secretContent,
		ExtractedMetadata: map[string]string{"pages": "3"},
	}
	if err := m.InsertDocument(ctx, doc); err != nil {
		t.Fatalf("seed doc: %v", err)
	}
	perms := []store.PermissionTuple{
		{ID: "perm-doc", TenantID: tenant, ResourceType: store.ResourceDocument, ResourceID: "doc-1",
			SubjectType: store.SubjectUser, SubjectID: "u1", Relation: store.RelationOwner, GrantedBy: "u1"},
		{ID: "perm-cat", TenantID: tenant, ResourceType: store.ResourceCategory, ResourceID: "cat-1",
			SubjectType: store.SubjectRole, SubjectID: "auditor", Relation: store.RelationViewer, GrantedBy: "u1"},
	}
	for _, p := range perms {
		if err := m.InsertPermission(ctx, p); err != nil {
			t.Fatalf("seed perm: %v", err)
		}
	}
}

func TestExportExcludesContentText(t *testing.T) {
	m := memstore.New()
	seedTenant(t, m, "t1")

	b, err := New(m).Export(context.Background(), authz.Subjects{TenantID: "t1"}, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(b.Documents) != 1 {
		t.Fatalf("Documents len = %d, want 1", len(b.Documents))
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), secretContent) {
		t.Errorf("export leaked content_text: %s", raw)
	}
	if strings.Contains(string(raw), "pages") {
		t.Errorf("export leaked extracted metadata")
	}
	// Bytes are referenced by object key only.
	if b.Documents[0].ObjectKey != "obj/doc-1" {
		t.Errorf("ObjectKey = %q, want obj/doc-1", b.Documents[0].ObjectKey)
	}
}

func TestExportIncludeLinksPlaceholder(t *testing.T) {
	m := memstore.New()
	seedTenant(t, m, "t1")
	b, err := New(m).Export(context.Background(), authz.Subjects{TenantID: "t1"}, true)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if !b.IncludeLinks {
		t.Errorf("IncludeLinks = false, want true")
	}
	if b.Documents[0].Link != nil {
		t.Errorf("Link = %v, want nil placeholder", b.Documents[0].Link)
	}
}

func TestRoundTripIntoEmptyTenant(t *testing.T) {
	ctx := context.Background()
	src := memstore.New()
	seedTenant(t, src, "t1")

	b, err := New(src).Export(ctx, authz.Subjects{TenantID: "t1"}, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst := memstore.New()
	res, err := New(dst).Import(ctx, authz.Subjects{TenantID: "t2", UserID: "importer"}, b, ModeSkip)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res.CategoriesImported != 1 || res.DocumentsImported != 1 || res.PermissionsImported != 2 {
		t.Fatalf("import result = %+v, want 1 cat / 1 doc / 2 perms", res)
	}

	// Categories recreated preserving id.
	gotCat, err := dst.GetCategory(ctx, "t2", "cat-1")
	if err != nil {
		t.Fatalf("GetCategory: %v", err)
	}
	if gotCat.Name != "Invoices" || gotCat.Path != "invoices" || gotCat.SortOrder != 3 {
		t.Errorf("category = %+v", gotCat)
	}

	// Documents recreated preserving id, without content_text.
	gotDoc, err := dst.GetDocument(ctx, "t2", "doc-1")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if gotDoc.Name != "invoice-2026.pdf" || gotDoc.FileSize != 1234 {
		t.Errorf("doc = %+v", gotDoc)
	}
	if gotDoc.ContentText != "" {
		t.Errorf("imported doc carried content_text: %q", gotDoc.ContentText)
	}
	if gotDoc.CategoryPath != "invoices" {
		t.Errorf("doc CategoryPath = %q, want invoices", gotDoc.CategoryPath)
	}

	// Permissions recreated preserving ids.
	docPerms, err := dst.ListPermissionsByResource(ctx, "t2", store.ResourceDocument, "doc-1")
	if err != nil {
		t.Fatalf("ListPermissionsByResource: %v", err)
	}
	if len(docPerms) != 1 || docPerms[0].ID != "perm-doc" || docPerms[0].Relation != store.RelationOwner {
		t.Errorf("doc perms = %+v", docPerms)
	}
	catPerms, _ := dst.ListPermissionsByResource(ctx, "t2", store.ResourceCategory, "cat-1")
	if len(catPerms) != 1 || catPerms[0].ID != "perm-cat" {
		t.Errorf("cat perms = %+v", catPerms)
	}
}

func TestImportSkipVsOverwrite(t *testing.T) {
	ctx := context.Background()
	src := memstore.New()
	seedTenant(t, src, "t1")
	b, err := New(src).Export(ctx, authz.Subjects{TenantID: "t1"}, false)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	dst := memstore.New()
	subj := authz.Subjects{TenantID: "t2", UserID: "importer"}
	if _, err := New(dst).Import(ctx, subj, b, ModeSkip); err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Second import in skip mode: everything already present, so all skipped.
	res, err := New(dst).Import(ctx, subj, b, ModeSkip)
	if err != nil {
		t.Fatalf("skip import: %v", err)
	}
	if res.CategoriesImported != 0 || res.DocumentsImported != 0 || res.PermissionsImported != 0 {
		t.Errorf("skip import imported new rows: %+v", res)
	}
	if res.CategoriesSkipped != 1 || res.DocumentsSkipped != 1 || res.PermissionsSkipped != 2 {
		t.Errorf("skip import skipped = %+v, want 1/1/2", res)
	}

	// Third import in overwrite mode: everything replaced.
	res, err = New(dst).Import(ctx, subj, b, ModeOverwrite)
	if err != nil {
		t.Fatalf("overwrite import: %v", err)
	}
	if res.CategoriesImported != 1 || res.DocumentsImported != 1 || res.PermissionsImported != 2 {
		t.Errorf("overwrite import = %+v, want 1/1/2 imported", res)
	}
	// Still exactly one of each (no duplicates left behind).
	cats, _ := dst.ListCategories(ctx, "t2")
	if len(cats) != 1 {
		t.Errorf("categories after overwrite = %d, want 1", len(cats))
	}
	docs, _ := dst.ListDocuments(ctx, "t2", store.DocumentFilter{})
	if len(docs) != 1 {
		t.Errorf("documents after overwrite = %d, want 1", len(docs))
	}
}

func TestImportBadSchema(t *testing.T) {
	dst := memstore.New()
	b := Backup{SchemaVersion: 999}
	_, err := New(dst).Import(context.Background(), authz.Subjects{TenantID: "t2"}, b, ModeSkip)
	if !errors.Is(err, ErrBadSchema) {
		t.Fatalf("Import bad schema err = %v, want ErrBadSchema", err)
	}
}

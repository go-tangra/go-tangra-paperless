package grpcapi

import (
	"context"
	"os"
	"testing"

	fauthz "github.com/go-tangra/go-tangra/v4/authz"
	"github.com/go-tangra/go-tangra/v4/identity"
)

// deploy/policy.yaml: the asset-documents rule lets svc/asset reach exactly the
// document/category methods its attachments need, and nothing else.
func TestPolicyAssetDocuments(t *testing.T) {
	f, err := os.Open("../../deploy/policy.yaml")
	if err != nil {
		t.Fatalf("open policy: %v", err)
	}
	defer f.Close()
	pol, err := fauthz.Load(f)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	asset, err := identity.ParseSPIFFEID(assetSvc)
	if err != nil {
		t.Fatalf("spiffe id: %v", err)
	}
	other, _ := identity.ParseSPIFFEID("spiffe://example.org/svc/ipam")
	ctx := context.Background()
	allowed := []string{
		"/paperless.v1.PaperlessDocumentService/Create", "/paperless.v1.PaperlessDocumentService/Get",
		"/paperless.v1.PaperlessDocumentService/Download", "/paperless.v1.PaperlessDocumentService/Delete",
		"/paperless.v1.PaperlessDocumentService/Search", "/paperless.v1.PaperlessDocumentService/List",
		"/paperless.v1.PaperlessCategoryService/Create", "/paperless.v1.PaperlessCategoryService/Get",
		"/paperless.v1.PaperlessCategoryService/List", "/paperless.v1.PaperlessCategoryService/GetTree",
	}
	for _, op := range allowed {
		if d := pol.Authorize(ctx, asset, "paperless", op); !d.Allowed || d.RuleID != "asset-documents" {
			t.Fatalf("asset %s: %+v", op, d)
		}
		if d := pol.Authorize(ctx, other, "paperless", op); d.Allowed {
			t.Fatalf("other service %s must be denied: %+v", op, d)
		}
	}
	for _, op := range []string{
		"/paperless.v1.PaperlessDocumentService/Update", "/paperless.v1.PaperlessDocumentService/BatchDelete",
		"/paperless.v1.PaperlessDocumentService/Move", "/paperless.v1.PaperlessDocumentService/GetDownloadUrl",
		"/paperless.v1.PaperlessCategoryService/Delete", "/paperless.v1.PaperlessCategoryService/Update",
		"/paperless.v1.PaperlessPermissionService/GrantAccess", "/paperless.v1.PaperlessStatisticsService/GetStatistics",
	} {
		if d := pol.Authorize(ctx, asset, "paperless", op); d.Allowed {
			t.Fatalf("asset %s must be denied: %+v", op, d)
		}
	}
}

package grpcapi

import (
	"bytes"
	"context"
	"io"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paperlessv1 "github.com/go-freya/freya/services/paperless/api/proto/paperless/v1"
	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/categories"
	"github.com/go-freya/freya/services/paperless/internal/documents"
	"github.com/go-freya/freya/services/paperless/internal/permissions"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/search"
	"github.com/go-freya/freya/services/paperless/internal/stats"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// ---- PaperlessDocumentService

// DocumentServer implements paperless.v1.PaperlessDocumentService.
type DocumentServer struct {
	paperlessv1.UnimplementedPaperlessDocumentServiceServer
	Docs     *documents.Service
	Searcher *search.Service
}

func (s *DocumentServer) Create(ctx context.Context, req *paperlessv1.CreateDocumentRequest) (*paperlessv1.Document, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	size := req.GetSize()
	if size == 0 {
		size = int64(len(req.GetContent()))
	}
	v, err := s.Docs.Create(ctx, subj, documents.CreateInput{
		Name: req.GetName(), Description: req.GetDescription(), CategoryID: req.GetCategoryId(),
		Tags: req.GetTags(), FileName: req.GetFileName(), MimeType: req.GetMimeType(),
		Reader: bytes.NewReader(req.GetContent()), Size: size, Source: docSourceFromPB(req.GetSource()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return toDocumentPB(v), nil
}

func (s *DocumentServer) Get(ctx context.Context, req *paperlessv1.GetDocumentRequest) (*paperlessv1.Document, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	// content_text is never returned over this surface: includeContent is false.
	v, err := s.Docs.Get(ctx, subj, req.GetId(), false)
	if err != nil {
		return nil, grpcError(err)
	}
	return toDocumentPB(v), nil
}

func (s *DocumentServer) List(ctx context.Context, req *paperlessv1.ListDocumentsRequest) (*paperlessv1.ListDocumentsResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	items, err := s.Docs.List(ctx, subj, repo.DocFilter{
		CategoryID: req.GetCategoryId(), Status: docStatusFromPB(req.GetStatus()), MimeType: req.GetMimeType(),
		Source: docSourceFromPB(req.GetSource()), ProcessingStatus: procStatusFromPB(req.GetProcessingStatus()),
		Tag: req.GetTag(), CreatedBy: req.GetCreatedBy(), Limit: int(req.GetLimit()), CursorID: req.GetCursorId(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	out := &paperlessv1.ListDocumentsResponse{Documents: make([]*paperlessv1.Document, 0, len(items))}
	for _, v := range items {
		out.Documents = append(out.Documents, toDocumentPB(v))
	}
	return out, nil
}

func (s *DocumentServer) Update(ctx context.Context, req *paperlessv1.UpdateDocumentRequest) (*paperlessv1.Document, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	v, err := s.Docs.Update(ctx, subj, req.GetId(), documents.UpdateInput{
		Name: req.GetName(), Description: req.GetDescription(), CategoryID: req.GetCategoryId(), Tags: req.GetTags(),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return toDocumentPB(v), nil
}

func (s *DocumentServer) Delete(ctx context.Context, req *paperlessv1.DeleteDocumentRequest) (*paperlessv1.DeleteDocumentResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := s.Docs.Delete(ctx, subj, req.GetId(), req.GetHard()); err != nil {
		return nil, grpcError(err)
	}
	return &paperlessv1.DeleteDocumentResponse{}, nil
}

func (s *DocumentServer) Move(ctx context.Context, req *paperlessv1.MoveDocumentRequest) (*paperlessv1.Document, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	v, err := s.Docs.Move(ctx, subj, req.GetId(), req.GetCategoryId())
	if err != nil {
		return nil, grpcError(err)
	}
	return toDocumentPB(v), nil
}

func (s *DocumentServer) Download(ctx context.Context, req *paperlessv1.DownloadDocumentRequest) (*paperlessv1.DownloadDocumentResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	rc, d, err := s.Docs.Download(ctx, subj, req.GetId())
	if err != nil {
		return nil, grpcError(err)
	}
	defer rc.Close()
	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, status.Error(codes.Unavailable, "read_failed")
	}
	return &paperlessv1.DownloadDocumentResponse{Content: content, MimeType: d.MimeType, FileName: d.FileName}, nil
}

func (s *DocumentServer) GetDownloadUrl(ctx context.Context, req *paperlessv1.GetDownloadUrlRequest) (*paperlessv1.GetDownloadUrlResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	url, err := s.Docs.DownloadURL(ctx, subj, req.GetId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &paperlessv1.GetDownloadUrlResponse{Url: url}, nil
}

func (s *DocumentServer) Search(ctx context.Context, req *paperlessv1.SearchDocumentsRequest) (*paperlessv1.SearchDocumentsResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if s.Searcher == nil {
		return nil, status.Error(codes.Unimplemented, "search not configured")
	}
	hits, err := s.Searcher.Search(ctx, subj, req.GetQuery(), int(req.GetLimit()))
	if err != nil {
		return nil, grpcError(err)
	}
	out := &paperlessv1.SearchDocumentsResponse{Hits: make([]*paperlessv1.SearchHit, 0, len(hits))}
	for _, h := range hits {
		out.Hits = append(out.Hits, &paperlessv1.SearchHit{
			Id: h.ID, Name: h.Name, CategoryId: h.CategoryID, CategoryPath: h.CategoryPath,
			MimeType: h.MimeType, Status: docStatusToPB(h.Status), Rank: h.Rank, Snippet: h.Snippet,
		})
	}
	return out, nil
}

func (s *DocumentServer) BatchDelete(ctx context.Context, req *paperlessv1.BatchDeleteRequest) (*paperlessv1.BatchDeleteResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	results, err := s.Docs.BatchDelete(ctx, subj, req.GetIds(), req.GetHard())
	if err != nil {
		return nil, grpcError(err)
	}
	out := &paperlessv1.BatchDeleteResponse{Results: make([]*paperlessv1.BatchResult, 0, len(results))}
	for _, r := range results {
		out.Results = append(out.Results, &paperlessv1.BatchResult{Id: r.ID, Ok: r.OK, Error: r.Error})
	}
	return out, nil
}

// ---- PaperlessCategoryService

// CategoryServer implements paperless.v1.PaperlessCategoryService.
type CategoryServer struct {
	paperlessv1.UnimplementedPaperlessCategoryServiceServer
	Svc *categories.Service
}

func (s *CategoryServer) Create(ctx context.Context, req *paperlessv1.CreateCategoryRequest) (*paperlessv1.Category, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	v, err := s.Svc.Create(ctx, subj, categories.Input{
		ParentID: req.GetParentId(), Name: req.GetName(), Description: req.GetDescription(), SortOrder: int(req.GetSortOrder()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return toCategoryPB(v), nil
}

func (s *CategoryServer) Get(ctx context.Context, req *paperlessv1.GetCategoryRequest) (*paperlessv1.Category, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	v, err := s.Svc.Get(ctx, subj, req.GetId())
	if err != nil {
		return nil, grpcError(err)
	}
	return toCategoryPB(v), nil
}

func (s *CategoryServer) List(ctx context.Context, req *paperlessv1.ListCategoriesRequest) (*paperlessv1.ListCategoriesResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	items, err := s.Svc.List(ctx, subj)
	if err != nil {
		return nil, grpcError(err)
	}
	out := &paperlessv1.ListCategoriesResponse{Categories: make([]*paperlessv1.Category, 0, len(items))}
	for _, v := range items {
		out.Categories = append(out.Categories, toCategoryPB(v))
	}
	return out, nil
}

func (s *CategoryServer) Update(ctx context.Context, req *paperlessv1.UpdateCategoryRequest) (*paperlessv1.Category, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	v, err := s.Svc.Update(ctx, subj, req.GetId(), categories.Input{
		Name: req.GetName(), Description: req.GetDescription(), SortOrder: int(req.GetSortOrder()),
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return toCategoryPB(v), nil
}

func (s *CategoryServer) Delete(ctx context.Context, req *paperlessv1.DeleteCategoryRequest) (*paperlessv1.DeleteCategoryResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := s.Svc.Delete(ctx, subj, req.GetId(), req.GetCascade()); err != nil {
		return nil, grpcError(err)
	}
	return &paperlessv1.DeleteCategoryResponse{}, nil
}

func (s *CategoryServer) Move(ctx context.Context, req *paperlessv1.MoveCategoryRequest) (*paperlessv1.Category, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	v, err := s.Svc.Move(ctx, subj, req.GetId(), req.GetNewParentId())
	if err != nil {
		return nil, grpcError(err)
	}
	return toCategoryPB(v), nil
}

func (s *CategoryServer) GetTree(ctx context.Context, req *paperlessv1.GetTreeRequest) (*paperlessv1.GetTreeResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	tree, err := s.Svc.GetTree(ctx, subj)
	if err != nil {
		return nil, grpcError(err)
	}
	return &paperlessv1.GetTreeResponse{Nodes: toTreeNodesPB(tree)}, nil
}

// ---- PaperlessPermissionService

// PermissionServer implements paperless.v1.PaperlessPermissionService.
type PermissionServer struct {
	paperlessv1.UnimplementedPaperlessPermissionServiceServer
	Svc *permissions.Service
}

func (s *PermissionServer) GrantAccess(ctx context.Context, req *paperlessv1.GrantAccessRequest) (*paperlessv1.PermissionTuple, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	in := permissions.GrantInput{
		ResourceType: resourceTypeFromPB(req.GetResourceType()), ResourceID: req.GetResourceId(),
		SubjectType: subjectTypeFromPB(req.GetSubjectType()), SubjectID: req.GetSubjectId(),
		Relation: relationFromPB(req.GetRelation()),
	}
	if req.GetExpiresAt() != 0 {
		t := time.Unix(req.GetExpiresAt(), 0).UTC()
		in.ExpiresAt = &t
	}
	t, err := s.Svc.Grant(ctx, subj, in)
	if err != nil {
		return nil, grpcError(err)
	}
	return toPermissionPB(t), nil
}

func (s *PermissionServer) RevokeAccess(ctx context.Context, req *paperlessv1.RevokeAccessRequest) (*paperlessv1.RevokeAccessResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	if err := s.Svc.Revoke(ctx, subj, resourceTypeFromPB(req.GetResourceType()), req.GetResourceId(), req.GetId()); err != nil {
		return nil, grpcError(err)
	}
	return &paperlessv1.RevokeAccessResponse{}, nil
}

func (s *PermissionServer) ListPermissions(ctx context.Context, req *paperlessv1.ListPermissionsRequest) (*paperlessv1.ListPermissionsResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	rows, err := s.Svc.ListForResource(ctx, subj, resourceTypeFromPB(req.GetResourceType()), req.GetResourceId())
	if err != nil {
		return nil, grpcError(err)
	}
	out := &paperlessv1.ListPermissionsResponse{Permissions: make([]*paperlessv1.PermissionTuple, 0, len(rows))}
	for _, t := range rows {
		out.Permissions = append(out.Permissions, toPermissionPB(t))
	}
	return out, nil
}

func (s *PermissionServer) CheckAccess(ctx context.Context, req *paperlessv1.CheckAccessRequest) (*paperlessv1.CheckAccessResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	ok, err := s.Svc.Check(ctx, subj, resourceTypeFromPB(req.GetResourceType()), req.GetResourceId(), permissionToAction(req.GetPermission()))
	if err != nil {
		return nil, grpcError(err)
	}
	return &paperlessv1.CheckAccessResponse{Allowed: ok}, nil
}

func (s *PermissionServer) ListAccessibleResources(ctx context.Context, req *paperlessv1.ListAccessibleResourcesRequest) (*paperlessv1.ListAccessibleResourcesResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	ids, all, err := s.Svc.ListAccessible(ctx, subj, resourceTypeFromPB(req.GetResourceType()))
	if err != nil {
		return nil, grpcError(err)
	}
	return &paperlessv1.ListAccessibleResourcesResponse{ResourceIds: ids, All: all}, nil
}

func (s *PermissionServer) GetEffectivePermissions(ctx context.Context, req *paperlessv1.GetEffectivePermissionsRequest) (*paperlessv1.GetEffectivePermissionsResponse, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	perms, grants, err := s.Svc.Effective(ctx, subj, resourceTypeFromPB(req.GetResourceType()), req.GetResourceId())
	if err != nil {
		return nil, grpcError(err)
	}
	out := &paperlessv1.GetEffectivePermissionsResponse{
		Permissions: &paperlessv1.PermissionSet{
			Read: perms.Read, Write: perms.Write, Delete: perms.Delete, Share: perms.Share, Download: perms.Download,
		},
		Grants: make([]*paperlessv1.PermissionTuple, 0, len(grants)),
	}
	for _, t := range grants {
		out.Grants = append(out.Grants, toPermissionPB(t))
	}
	return out, nil
}

// ---- PaperlessStatisticsService

// StatisticsServer implements paperless.v1.PaperlessStatisticsService.
type StatisticsServer struct {
	paperlessv1.UnimplementedPaperlessStatisticsServiceServer
	Svc *stats.Service
}

func (s *StatisticsServer) GetStatistics(ctx context.Context, req *paperlessv1.GetStatisticsRequest) (*paperlessv1.Statistics, error) {
	subj, err := caller(ctx, req.GetTenantId())
	if err != nil {
		return nil, err
	}
	snap, err := s.Svc.Tenant(ctx, subj)
	if err != nil {
		return nil, grpcError(err)
	}
	return &paperlessv1.Statistics{
		DocumentsByStatus: intMapToInt64(snap.DocumentsByStatus),
		DocumentsBySource: intMapToInt64(snap.DocumentsBySource),
		DocumentsByMime:   intMapToInt64(snap.DocumentsByMime),
		StorageBytes:      snap.StorageBytes,
		StorageByCategory: snap.StorageByCategory,
		CategoriesTotal:   int64(snap.CategoriesTotal),
		DocumentsTotal:    int64(snap.DocumentsTotal),
		Backlog:           intMapToInt64(snap.Backlog),
	}, nil
}

// ---- mappers

func toDocumentPB(v documents.View) *paperlessv1.Document {
	return &paperlessv1.Document{
		Id: v.ID, TenantId: v.TenantID, CategoryId: v.CategoryID, CategoryPath: v.CategoryPath,
		Name: v.Name, Description: v.Description, FileName: v.FileName, FileSize: v.FileSize,
		MimeType: v.MimeType, Checksum: v.Checksum, Status: docStatusToPB(v.Status),
		Source: docSourceToPB(v.Source), Tags: v.Tags, ProcessingStatus: procStatusToPB(v.ProcessingStatus),
		CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt.Unix(), UpdatedAt: v.UpdatedAt.Unix(),
	}
}

func toCategoryPB(v categories.View) *paperlessv1.Category {
	c := &paperlessv1.Category{
		Id: v.ID, Name: v.Name, Path: v.Path, Description: v.Description, Depth: int64(v.Depth),
		SortOrder: int64(v.SortOrder), DocumentCount: int64(v.DocumentCount), SubcategoryCount: int64(v.SubcategoryCount),
	}
	if v.ParentID != nil {
		c.ParentId = *v.ParentID
	}
	return c
}

func toTreeNodesPB(nodes []categories.TreeNode) []*paperlessv1.CategoryTreeNode {
	out := make([]*paperlessv1.CategoryTreeNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, &paperlessv1.CategoryTreeNode{Category: toCategoryPB(n.Category), Children: toTreeNodesPB(n.Children)})
	}
	return out
}

func toPermissionPB(t permissions.Tuple) *paperlessv1.PermissionTuple {
	p := &paperlessv1.PermissionTuple{
		Id: t.ID, ResourceType: resourceTypeToPB(t.ResourceType), ResourceId: t.ResourceID,
		SubjectType: subjectTypeToPB(t.SubjectType), SubjectId: t.SubjectID,
		Relation: relationToPB(t.Relation), GrantedBy: t.GrantedBy,
	}
	if t.ExpiresAt != nil {
		p.ExpiresAt = t.ExpiresAt.Unix()
	}
	return p
}

func intMapToInt64(m map[string]int) map[string]int64 {
	out := make(map[string]int64, len(m))
	for k, v := range m {
		out[k] = int64(v)
	}
	return out
}

// ---- enum converters

func docStatusToPB(s string) paperlessv1.DocumentStatus {
	switch s {
	case store.DocActive:
		return paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE
	case store.DocArchived:
		return paperlessv1.DocumentStatus_DOCUMENT_STATUS_ARCHIVED
	case store.DocDeleted:
		return paperlessv1.DocumentStatus_DOCUMENT_STATUS_DELETED
	}
	return paperlessv1.DocumentStatus_DOCUMENT_STATUS_UNSPECIFIED
}

func docStatusFromPB(s paperlessv1.DocumentStatus) string {
	switch s {
	case paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE:
		return store.DocActive
	case paperlessv1.DocumentStatus_DOCUMENT_STATUS_ARCHIVED:
		return store.DocArchived
	case paperlessv1.DocumentStatus_DOCUMENT_STATUS_DELETED:
		return store.DocDeleted
	}
	return ""
}

func docSourceToPB(s string) paperlessv1.DocumentSource {
	switch s {
	case store.SourceUpload:
		return paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD
	case store.SourceEmail:
		return paperlessv1.DocumentSource_DOCUMENT_SOURCE_EMAIL
	}
	return paperlessv1.DocumentSource_DOCUMENT_SOURCE_UNSPECIFIED
}

func docSourceFromPB(s paperlessv1.DocumentSource) string {
	switch s {
	case paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD:
		return store.SourceUpload
	case paperlessv1.DocumentSource_DOCUMENT_SOURCE_EMAIL:
		return store.SourceEmail
	}
	return ""
}

func procStatusToPB(s string) paperlessv1.ProcessingStatus {
	switch s {
	case store.ProcPending:
		return paperlessv1.ProcessingStatus_PROCESSING_STATUS_PENDING
	case store.ProcProcessing:
		return paperlessv1.ProcessingStatus_PROCESSING_STATUS_PROCESSING
	case store.ProcCompleted:
		return paperlessv1.ProcessingStatus_PROCESSING_STATUS_COMPLETED
	case store.ProcFailed:
		return paperlessv1.ProcessingStatus_PROCESSING_STATUS_FAILED
	case store.ProcRetrying:
		return paperlessv1.ProcessingStatus_PROCESSING_STATUS_RETRYING
	}
	return paperlessv1.ProcessingStatus_PROCESSING_STATUS_UNSPECIFIED
}

func procStatusFromPB(s paperlessv1.ProcessingStatus) string {
	switch s {
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_PENDING:
		return store.ProcPending
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_PROCESSING:
		return store.ProcProcessing
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_COMPLETED:
		return store.ProcCompleted
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_FAILED:
		return store.ProcFailed
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_RETRYING:
		return store.ProcRetrying
	}
	return ""
}

func resourceTypeToPB(s string) paperlessv1.ResourceType {
	switch s {
	case store.ResourceDocument:
		return paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT
	case store.ResourceCategory:
		return paperlessv1.ResourceType_RESOURCE_TYPE_CATEGORY
	}
	return paperlessv1.ResourceType_RESOURCE_TYPE_UNSPECIFIED
}

func resourceTypeFromPB(s paperlessv1.ResourceType) string {
	switch s {
	case paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT:
		return store.ResourceDocument
	case paperlessv1.ResourceType_RESOURCE_TYPE_CATEGORY:
		return store.ResourceCategory
	}
	return ""
}

func relationToPB(s string) paperlessv1.Relation {
	switch s {
	case store.RelationOwner:
		return paperlessv1.Relation_RELATION_OWNER
	case store.RelationEditor:
		return paperlessv1.Relation_RELATION_EDITOR
	case store.RelationViewer:
		return paperlessv1.Relation_RELATION_VIEWER
	case store.RelationSharer:
		return paperlessv1.Relation_RELATION_SHARER
	}
	return paperlessv1.Relation_RELATION_UNSPECIFIED
}

func relationFromPB(s paperlessv1.Relation) string {
	switch s {
	case paperlessv1.Relation_RELATION_OWNER:
		return store.RelationOwner
	case paperlessv1.Relation_RELATION_EDITOR:
		return store.RelationEditor
	case paperlessv1.Relation_RELATION_VIEWER:
		return store.RelationViewer
	case paperlessv1.Relation_RELATION_SHARER:
		return store.RelationSharer
	}
	return ""
}

func subjectTypeToPB(s string) paperlessv1.SubjectType {
	switch s {
	case store.SubjectUser:
		return paperlessv1.SubjectType_SUBJECT_TYPE_USER
	case store.SubjectRole:
		return paperlessv1.SubjectType_SUBJECT_TYPE_ROLE
	case store.SubjectTenant:
		return paperlessv1.SubjectType_SUBJECT_TYPE_TENANT
	}
	return paperlessv1.SubjectType_SUBJECT_TYPE_UNSPECIFIED
}

func subjectTypeFromPB(s paperlessv1.SubjectType) string {
	switch s {
	case paperlessv1.SubjectType_SUBJECT_TYPE_USER:
		return store.SubjectUser
	case paperlessv1.SubjectType_SUBJECT_TYPE_ROLE:
		return store.SubjectRole
	case paperlessv1.SubjectType_SUBJECT_TYPE_TENANT:
		return store.SubjectTenant
	}
	return ""
}

func permissionToAction(p paperlessv1.Permission) string {
	switch p {
	case paperlessv1.Permission_PERMISSION_READ:
		return authz.Read
	case paperlessv1.Permission_PERMISSION_WRITE:
		return authz.Write
	case paperlessv1.Permission_PERMISSION_DELETE:
		return authz.Delete
	case paperlessv1.Permission_PERMISSION_SHARE:
		return authz.Share
	case paperlessv1.Permission_PERMISSION_DOWNLOAD:
		return authz.Download
	}
	return ""
}

// Package paperlessclient is a thin, typed Go client for the paperless.v1
// service-to-service gRPC API, for other Freya modules that need to store,
// fetch, search, or authorize documents. It wraps the generated gRPC stubs so
// callers deal in ordinary Go values, not protobuf messages. The caller supplies
// a connected, SPIFFE-mTLS *grpc.ClientConn (e.g. from freya.App.Client(ctx,
// "paperless")); this package does not dial or manage the connection. Extracted
// content and object-store credentials are never returned by the service and so
// never appear here.
package paperlessclient

import (
	"context"
	"time"

	"google.golang.org/grpc"

	paperlessv1 "github.com/go-freya/freya/services/paperless/api/proto/paperless/v1"
)

// Client calls the paperless.v1 API over a caller-provided gRPC connection.
type Client struct {
	docs  paperlessv1.PaperlessDocumentServiceClient
	perms paperlessv1.PaperlessPermissionServiceClient
	stats paperlessv1.PaperlessStatisticsServiceClient
}

// New builds a client from a connected (SPIFFE-mTLS) gRPC connection to the
// paperless service.
func New(conn grpc.ClientConnInterface) *Client {
	return &Client{
		docs:  paperlessv1.NewPaperlessDocumentServiceClient(conn),
		perms: paperlessv1.NewPaperlessPermissionServiceClient(conn),
		stats: paperlessv1.NewPaperlessStatisticsServiceClient(conn),
	}
}

// Document is a document's metadata as returned by paperless. Extracted content
// and metadata are never included.
type Document struct {
	ID               string
	CategoryID       string
	CategoryPath     string
	Name             string
	Description      string
	FileName         string
	FileSize         int64
	MimeType         string
	Checksum         string
	Status           string
	Source           string
	ProcessingStatus string
	Tags             map[string]string
	CreatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CreateDocumentInput describes a new document. Content is the raw bytes; the
// service stores them in object storage, checksums them, and enqueues async
// extraction.
type CreateDocumentInput struct {
	TenantID    string
	Name        string
	Description string
	CategoryID  string
	Tags        map[string]string
	FileName    string
	MimeType    string
	Content     []byte
}

// CreateDocument uploads a document and returns its metadata.
func (c *Client) CreateDocument(ctx context.Context, in CreateDocumentInput) (Document, error) {
	pb, err := c.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{
		TenantId: in.TenantID, Name: in.Name, Description: in.Description,
		CategoryId: in.CategoryID, Tags: in.Tags, FileName: in.FileName,
		MimeType: in.MimeType, Content: in.Content, Size: int64(len(in.Content)),
		Source: paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD,
	})
	if err != nil {
		return Document{}, err
	}
	return toDocument(pb), nil
}

// GetDocument returns one document's metadata by id.
func (c *Client) GetDocument(ctx context.Context, tenantID, id string) (Document, error) {
	pb, err := c.docs.Get(ctx, &paperlessv1.GetDocumentRequest{TenantId: tenantID, Id: id})
	if err != nil {
		return Document{}, err
	}
	return toDocument(pb), nil
}

// ListDocuments lists a tenant's documents, optionally filtered by category and
// status (empty filters match all).
func (c *Client) ListDocuments(ctx context.Context, tenantID, categoryID, status string) ([]Document, error) {
	resp, err := c.docs.List(ctx, &paperlessv1.ListDocumentsRequest{
		TenantId: tenantID, CategoryId: categoryID, Status: docStatusEnum(status),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Document, 0, len(resp.GetDocuments()))
	for _, pb := range resp.GetDocuments() {
		out = append(out, toDocument(pb))
	}
	return out, nil
}

// SearchHit is one ranked full-text search result.
type SearchHit struct {
	ID           string
	Name         string
	CategoryPath string
	MimeType     string
	Rank         float64
	Snippet      string
}

// Search runs a permission-filtered full-text search over the tenant's
// documents. limit <= 0 uses the server default.
func (c *Client) Search(ctx context.Context, tenantID, query string, limit int) ([]SearchHit, error) {
	resp, err := c.docs.Search(ctx, &paperlessv1.SearchDocumentsRequest{
		TenantId: tenantID, Query: query, Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]SearchHit, 0, len(resp.GetHits()))
	for _, h := range resp.GetHits() {
		out = append(out, SearchHit{
			ID: h.GetId(), Name: h.GetName(), CategoryPath: h.GetCategoryPath(),
			MimeType: h.GetMimeType(), Rank: h.GetRank(), Snippet: h.GetSnippet(),
		})
	}
	return out, nil
}

// Download returns a document's bytes plus its MIME type and file name.
func (c *Client) Download(ctx context.Context, tenantID, id string) (content []byte, mimeType, fileName string, err error) {
	resp, err := c.docs.Download(ctx, &paperlessv1.DownloadDocumentRequest{TenantId: tenantID, Id: id})
	if err != nil {
		return nil, "", "", err
	}
	return resp.GetContent(), resp.GetMimeType(), resp.GetFileName(), nil
}

// DownloadURL returns a short-lived presigned URL for a document's bytes.
func (c *Client) DownloadURL(ctx context.Context, tenantID, id string) (string, error) {
	resp, err := c.docs.GetDownloadUrl(ctx, &paperlessv1.GetDownloadUrlRequest{TenantId: tenantID, Id: id})
	if err != nil {
		return "", err
	}
	return resp.GetUrl(), nil
}

// GrantInput binds a subject to a resource with a relation. Types are the
// lowercase domain strings ("document"/"category", "user"/"role"/"tenant",
// "owner"/"editor"/"viewer"/"sharer").
type GrantInput struct {
	TenantID     string
	ResourceType string
	ResourceID   string
	SubjectType  string
	SubjectID    string
	Relation     string
	ExpiresAt    *time.Time
}

// GrantAccess creates a permission tuple. The caller must hold share on the
// resource.
func (c *Client) GrantAccess(ctx context.Context, in GrantInput) error {
	req := &paperlessv1.GrantAccessRequest{
		TenantId: in.TenantID, ResourceType: resourceEnum(in.ResourceType), ResourceId: in.ResourceID,
		SubjectType: subjectEnum(in.SubjectType), SubjectId: in.SubjectID, Relation: relationEnum(in.Relation),
	}
	if in.ExpiresAt != nil {
		req.ExpiresAt = in.ExpiresAt.Unix()
	}
	_, err := c.perms.GrantAccess(ctx, req)
	return err
}

// CheckAccess reports whether the calling subject (derived from the mTLS caller
// identity) may perform action (read/write/delete/share/download) on a resource.
func (c *Client) CheckAccess(ctx context.Context, tenantID, resourceType, resourceID, action string) (bool, error) {
	resp, err := c.perms.CheckAccess(ctx, &paperlessv1.CheckAccessRequest{
		TenantId: tenantID, ResourceType: resourceEnum(resourceType), ResourceId: resourceID,
		Permission: permissionEnum(action),
	})
	if err != nil {
		return false, err
	}
	return resp.GetAllowed(), nil
}

// EffectivePermissions is the caller's strongest set of actions on a resource.
type EffectivePermissions struct {
	Read, Write, Delete, Share, Download bool
}

// EffectivePermissions returns the caller's effective permissions on a resource.
func (c *Client) EffectivePermissions(ctx context.Context, tenantID, resourceType, resourceID string) (EffectivePermissions, error) {
	resp, err := c.perms.GetEffectivePermissions(ctx, &paperlessv1.GetEffectivePermissionsRequest{
		TenantId: tenantID, ResourceType: resourceEnum(resourceType), ResourceId: resourceID,
	})
	if err != nil {
		return EffectivePermissions{}, err
	}
	p := resp.GetPermissions()
	return EffectivePermissions{
		Read: p.GetRead(), Write: p.GetWrite(), Delete: p.GetDelete(),
		Share: p.GetShare(), Download: p.GetDownload(),
	}, nil
}

// Statistics is a tenant's document statistics snapshot.
type Statistics struct {
	DocumentsByStatus map[string]int64
	DocumentsBySource map[string]int64
	DocumentsByMime   map[string]int64
	StorageBytes      int64
	StorageByCategory map[string]int64
	CategoriesTotal   int64
	DocumentsTotal    int64
	Backlog           map[string]int64
}

// Statistics returns a tenant's document statistics.
func (c *Client) Statistics(ctx context.Context, tenantID string) (Statistics, error) {
	pb, err := c.stats.GetStatistics(ctx, &paperlessv1.GetStatisticsRequest{TenantId: tenantID})
	if err != nil {
		return Statistics{}, err
	}
	return Statistics{
		DocumentsByStatus: pb.GetDocumentsByStatus(), DocumentsBySource: pb.GetDocumentsBySource(),
		DocumentsByMime: pb.GetDocumentsByMime(), StorageBytes: pb.GetStorageBytes(),
		StorageByCategory: pb.GetStorageByCategory(), CategoriesTotal: pb.GetCategoriesTotal(),
		DocumentsTotal: pb.GetDocumentsTotal(), Backlog: pb.GetBacklog(),
	}, nil
}

func toDocument(pb *paperlessv1.Document) Document {
	d := Document{
		ID: pb.GetId(), CategoryID: pb.GetCategoryId(), CategoryPath: pb.GetCategoryPath(),
		Name: pb.GetName(), Description: pb.GetDescription(), FileName: pb.GetFileName(),
		FileSize: pb.GetFileSize(), MimeType: pb.GetMimeType(), Checksum: pb.GetChecksum(),
		Status: docStatusString(pb.GetStatus()), Source: docSourceString(pb.GetSource()),
		ProcessingStatus: procStatusString(pb.GetProcessingStatus()), Tags: pb.GetTags(),
		CreatedBy: pb.GetCreatedBy(),
	}
	if ts := pb.GetCreatedAt(); ts != 0 {
		d.CreatedAt = time.Unix(ts, 0).UTC()
	}
	if ts := pb.GetUpdatedAt(); ts != 0 {
		d.UpdatedAt = time.Unix(ts, 0).UTC()
	}
	return d
}

func resourceEnum(s string) paperlessv1.ResourceType {
	switch s {
	case "document":
		return paperlessv1.ResourceType_RESOURCE_TYPE_DOCUMENT
	case "category":
		return paperlessv1.ResourceType_RESOURCE_TYPE_CATEGORY
	}
	return paperlessv1.ResourceType_RESOURCE_TYPE_UNSPECIFIED
}

func subjectEnum(s string) paperlessv1.SubjectType {
	switch s {
	case "user":
		return paperlessv1.SubjectType_SUBJECT_TYPE_USER
	case "role":
		return paperlessv1.SubjectType_SUBJECT_TYPE_ROLE
	case "tenant":
		return paperlessv1.SubjectType_SUBJECT_TYPE_TENANT
	}
	return paperlessv1.SubjectType_SUBJECT_TYPE_UNSPECIFIED
}

func relationEnum(s string) paperlessv1.Relation {
	switch s {
	case "owner":
		return paperlessv1.Relation_RELATION_OWNER
	case "editor":
		return paperlessv1.Relation_RELATION_EDITOR
	case "viewer":
		return paperlessv1.Relation_RELATION_VIEWER
	case "sharer":
		return paperlessv1.Relation_RELATION_SHARER
	}
	return paperlessv1.Relation_RELATION_UNSPECIFIED
}

func permissionEnum(s string) paperlessv1.Permission {
	switch s {
	case "read":
		return paperlessv1.Permission_PERMISSION_READ
	case "write":
		return paperlessv1.Permission_PERMISSION_WRITE
	case "delete":
		return paperlessv1.Permission_PERMISSION_DELETE
	case "share":
		return paperlessv1.Permission_PERMISSION_SHARE
	case "download":
		return paperlessv1.Permission_PERMISSION_DOWNLOAD
	}
	return paperlessv1.Permission_PERMISSION_UNSPECIFIED
}

func docStatusEnum(s string) paperlessv1.DocumentStatus {
	switch s {
	case "active":
		return paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE
	case "archived":
		return paperlessv1.DocumentStatus_DOCUMENT_STATUS_ARCHIVED
	case "deleted":
		return paperlessv1.DocumentStatus_DOCUMENT_STATUS_DELETED
	}
	return paperlessv1.DocumentStatus_DOCUMENT_STATUS_UNSPECIFIED
}

func docStatusString(s paperlessv1.DocumentStatus) string {
	switch s {
	case paperlessv1.DocumentStatus_DOCUMENT_STATUS_ACTIVE:
		return "active"
	case paperlessv1.DocumentStatus_DOCUMENT_STATUS_ARCHIVED:
		return "archived"
	case paperlessv1.DocumentStatus_DOCUMENT_STATUS_DELETED:
		return "deleted"
	}
	return ""
}

func docSourceString(s paperlessv1.DocumentSource) string {
	switch s {
	case paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD:
		return "upload"
	case paperlessv1.DocumentSource_DOCUMENT_SOURCE_EMAIL:
		return "email"
	}
	return ""
}

func procStatusString(s paperlessv1.ProcessingStatus) string {
	switch s {
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_PENDING:
		return "pending"
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_PROCESSING:
		return "processing"
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_COMPLETED:
		return "completed"
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_FAILED:
		return "failed"
	}
	return ""
}

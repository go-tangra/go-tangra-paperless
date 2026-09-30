// Package paperlessclient is a thin, typed Go client for the paperless.v1
// service-to-service gRPC API, for other go-tangra modules that need to store,
// fetch, search, or authorize documents (for example the asset module's
// attachments). It wraps the generated gRPC stubs so callers deal in ordinary Go
// values, never protobuf messages.
//
// The caller supplies a connected, SPIFFE-mTLS grpc.ClientConnInterface (e.g.
// from freya.App.Client(ctx, "paperless")); this package does not dial or manage
// the connection. Every method takes the tenant explicitly: paperless acts for
// the tenant named in the request and derives the actor from the verified
// SPIFFE peer, so a service caller owns (and later finds) what it creates.
//
// Documents cross the wire whole, so each call carries MaxCallRecvMsgSize and
// MaxCallSendMsgSize call options (DefaultMaxMessageBytes unless overridden with
// WithMaxMessageBytes); the paperless server bounds inbound messages with its
// limits.max_request_bytes (default 33 MiB).
//
// Errors: NotFound, PermissionDenied and Unavailable/DeadlineExceeded statuses
// are reported as ErrNotFound, ErrForbidden and ErrUnavailable (errors.Is); the
// original gRPC status stays in the chain (status.Code still works). Extracted
// content and object-store credentials are never returned by the service and so
// never appear here.
package paperlessclient

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	paperlessv1 "github.com/go-tangra/go-tangra-paperless/sdk/v4/api/proto/paperless/v1"
)

// Errors mapped from gRPC status codes.
var (
	// ErrNotFound: the resource does not exist, or the caller may not see it
	// (paperless masks the existence of unreadable resources).
	ErrNotFound = errors.New("paperless: not found")
	// ErrForbidden: the caller is known but lacks the permission.
	ErrForbidden = errors.New("paperless: forbidden")
	// ErrUnavailable: paperless (or its database/object store) is temporarily
	// unavailable, or the call timed out; retrying later may succeed.
	ErrUnavailable = errors.New("paperless: unavailable")
)

// DefaultMaxMessageBytes bounds one request or response message: 32 MiB of
// document content plus 1 MiB of envelope. It matches the paperless server's
// default limits.max_request_bytes.
const DefaultMaxMessageBytes = 33 << 20

// Client calls the paperless.v1 API over a caller-provided gRPC connection.
type Client struct {
	docs     paperlessv1.PaperlessDocumentServiceClient
	cats     paperlessv1.PaperlessCategoryServiceClient
	perms    paperlessv1.PaperlessPermissionServiceClient
	stats    paperlessv1.PaperlessStatisticsServiceClient
	maxBytes int
}

// Option tunes a Client.
type Option func(*Client)

// WithMaxMessageBytes overrides DefaultMaxMessageBytes for both directions.
// Values <= 0 keep the default.
func WithMaxMessageBytes(n int) Option {
	return func(c *Client) {
		if n > 0 {
			c.maxBytes = n
		}
	}
}

// New builds a client from a connected (SPIFFE-mTLS) gRPC connection to the
// paperless service.
func New(conn grpc.ClientConnInterface, opts ...Option) *Client {
	c := &Client{
		docs:     paperlessv1.NewPaperlessDocumentServiceClient(conn),
		cats:     paperlessv1.NewPaperlessCategoryServiceClient(conn),
		perms:    paperlessv1.NewPaperlessPermissionServiceClient(conn),
		stats:    paperlessv1.NewPaperlessStatisticsServiceClient(conn),
		maxBytes: DefaultMaxMessageBytes,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// callOpts are the per-call message-size bounds.
func (c *Client) callOpts() []grpc.CallOption {
	return []grpc.CallOption{grpc.MaxCallRecvMsgSize(c.maxBytes), grpc.MaxCallSendMsgSize(c.maxBytes)}
}

// mapErr turns a gRPC status into the package sentinels, keeping the status in
// the chain. Other codes are returned unchanged.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	var sentinel error
	switch status.Code(err) {
	case codes.NotFound:
		sentinel = ErrNotFound
	case codes.PermissionDenied:
		sentinel = ErrForbidden
	case codes.Unavailable, codes.DeadlineExceeded:
		sentinel = ErrUnavailable
	default:
		return err
	}
	return fmt.Errorf("%w: %w", sentinel, err)
}

// ---- documents

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

// CreateInput describes a new document. Content is the raw bytes; the service
// stores them in object storage, checksums them, and enqueues async text
// extraction (which feeds full-text search). CategoryID may be empty (unfiled).
type CreateInput struct {
	CategoryID  string
	Name        string
	Description string
	FileName    string
	MimeType    string
	Tags        map[string]string
	Content     []byte
}

// CreateDocument uploads a document and returns its metadata. The caller
// becomes the document's owner.
func (c *Client) CreateDocument(ctx context.Context, tenantID string, in CreateInput) (Document, error) {
	pb, err := c.docs.Create(ctx, &paperlessv1.CreateDocumentRequest{
		TenantId: tenantID, Name: in.Name, Description: in.Description,
		CategoryId: in.CategoryID, Tags: in.Tags, FileName: in.FileName,
		MimeType: in.MimeType, Content: in.Content, Size: int64(len(in.Content)),
		Source: paperlessv1.DocumentSource_DOCUMENT_SOURCE_UPLOAD,
	}, c.callOpts()...)
	if err != nil {
		return Document{}, mapErr(err)
	}
	return toDocument(pb), nil
}

// GetDocument returns one document's metadata by id.
func (c *Client) GetDocument(ctx context.Context, tenantID, id string) (Document, error) {
	pb, err := c.docs.Get(ctx, &paperlessv1.GetDocumentRequest{TenantId: tenantID, Id: id}, c.callOpts()...)
	if err != nil {
		return Document{}, mapErr(err)
	}
	return toDocument(pb), nil
}

// DeleteDocument deletes a document. A soft delete (hard=false) marks it
// deleted and keeps the bytes; a hard delete also removes the stored object.
func (c *Client) DeleteDocument(ctx context.Context, tenantID, id string, hard bool) error {
	_, err := c.docs.Delete(ctx, &paperlessv1.DeleteDocumentRequest{TenantId: tenantID, Id: id, Hard: hard}, c.callOpts()...)
	return mapErr(err)
}

// DownloadDocument returns a document's bytes plus its file name and MIME type.
func (c *Client) DownloadDocument(ctx context.Context, tenantID, id string) (content []byte, fileName, mimeType string, err error) {
	resp, err := c.docs.Download(ctx, &paperlessv1.DownloadDocumentRequest{TenantId: tenantID, Id: id}, c.callOpts()...)
	if err != nil {
		return nil, "", "", mapErr(err)
	}
	return resp.GetContent(), resp.GetFileName(), resp.GetMimeType(), nil
}

// DownloadURL returns a short-lived presigned URL for a document's bytes.
func (c *Client) DownloadURL(ctx context.Context, tenantID, id string) (string, error) {
	resp, err := c.docs.GetDownloadUrl(ctx, &paperlessv1.GetDownloadUrlRequest{TenantId: tenantID, Id: id}, c.callOpts()...)
	if err != nil {
		return "", mapErr(err)
	}
	return resp.GetUrl(), nil
}

// ListDocuments lists a tenant's documents, optionally filtered by category and
// status ("active"/"archived"/"deleted"; empty filters match all). Paperless
// requires tenant-wide read for listing, which service callers do not hold by
// default; use Search for permission-filtered discovery.
func (c *Client) ListDocuments(ctx context.Context, tenantID, categoryID, status string) ([]Document, error) {
	resp, err := c.docs.List(ctx, &paperlessv1.ListDocumentsRequest{
		TenantId: tenantID, CategoryId: categoryID, Status: docStatusEnum(status),
	}, c.callOpts()...)
	if err != nil {
		return nil, mapErr(err)
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
	CategoryID   string
	CategoryPath string
	MimeType     string
	Snippet      string
	Rank         float64
}

// Search runs a permission-filtered full-text search over the tenant's
// documents (a service caller sees the documents it owns). limit <= 0 uses the
// server default.
func (c *Client) Search(ctx context.Context, tenantID, query string, limit int) ([]SearchHit, error) {
	resp, err := c.docs.Search(ctx, &paperlessv1.SearchDocumentsRequest{
		TenantId: tenantID, Query: query, Limit: int64(max(limit, 0)),
	}, c.callOpts()...)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]SearchHit, 0, len(resp.GetHits()))
	for _, h := range resp.GetHits() {
		out = append(out, SearchHit{
			ID: h.GetId(), Name: h.GetName(), CategoryID: h.GetCategoryId(), CategoryPath: h.GetCategoryPath(),
			MimeType: h.GetMimeType(), Snippet: h.GetSnippet(), Rank: h.GetRank(),
		})
	}
	return out, nil
}

// ---- categories

// EnsureCategory resolves a nested category path (e.g. ["Assets", "AT-000123"])
// to the id of its last segment, creating any missing segment. It is idempotent
// and safe for concurrent callers: when a create loses a race (AlreadyExists, or
// InvalidArgument from servers that report duplicates as validation failures)
// the tree is re-read and the existing node is used. Segment names are trimmed
// (paperless stores them trimmed); among same-named siblings the lowest id wins,
// so concurrent callers converge on one node.
func (c *Client) EnsureCategory(ctx context.Context, tenantID string, path []string) (string, error) {
	if len(path) == 0 {
		return "", errors.New("paperless: empty category path")
	}
	names := make([]string, len(path))
	for i, p := range path {
		names[i] = strings.TrimSpace(p)
		if names[i] == "" {
			return "", fmt.Errorf("paperless: empty category path segment %d", i)
		}
	}
	all, err := c.listCategories(ctx, tenantID)
	if err != nil {
		return "", err
	}
	parent := ""
	for _, name := range names {
		if id, ok := findChild(all, parent, name); ok {
			parent = id
			continue
		}
		created, cerr := c.cats.Create(ctx, &paperlessv1.CreateCategoryRequest{
			TenantId: tenantID, ParentId: parent, Name: name,
		}, c.callOpts()...)
		if cerr != nil {
			if code := status.Code(cerr); code != codes.AlreadyExists && code != codes.InvalidArgument {
				return "", mapErr(cerr)
			}
			// Lost a race: someone else created it meanwhile; re-read.
			if all, err = c.listCategories(ctx, tenantID); err != nil {
				return "", err
			}
			id, ok := findChild(all, parent, name)
			if !ok {
				return "", mapErr(cerr)
			}
			parent = id
			continue
		}
		if parent != "" {
			// Children are unique per parent in paperless; ours is the node.
			parent = created.GetId()
			continue
		}
		// A root: converge with a concurrent creator on databases without the
		// root-name unique index by taking the lowest id among same-named roots.
		if all, err = c.listCategories(ctx, tenantID); err != nil {
			return "", err
		}
		parent = created.GetId()
		if id, ok := findChild(all, "", name); ok {
			parent = id
		}
	}
	return parent, nil
}

// Category is a folder in the tenant's tree.
type Category struct {
	ID       string
	ParentID string
	Name     string
	Path     string
	Depth    int64
}

// ListCategories returns every category in the tenant (flat).
func (c *Client) ListCategories(ctx context.Context, tenantID string) ([]Category, error) {
	all, err := c.listCategories(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]Category, 0, len(all))
	for _, pb := range all {
		out = append(out, Category{ID: pb.GetId(), ParentID: pb.GetParentId(), Name: pb.GetName(), Path: pb.GetPath(), Depth: pb.GetDepth()})
	}
	return out, nil
}

func (c *Client) listCategories(ctx context.Context, tenantID string) ([]*paperlessv1.Category, error) {
	resp, err := c.cats.List(ctx, &paperlessv1.ListCategoriesRequest{TenantId: tenantID}, c.callOpts()...)
	if err != nil {
		return nil, mapErr(err)
	}
	return resp.GetCategories(), nil
}

// findChild returns the lowest id among parent's children named name.
func findChild(all []*paperlessv1.Category, parent, name string) (string, bool) {
	var ids []string
	for _, c := range all {
		if c.GetParentId() == parent && c.GetName() == name {
			ids = append(ids, c.GetId())
		}
	}
	if len(ids) == 0 {
		return "", false
	}
	sort.Strings(ids)
	return ids[0], true
}

// ---- permissions

// GrantInput binds a subject to a resource with a relation. Types are the
// lowercase domain strings ("document"/"category", "user"/"role"/"tenant",
// "owner"/"editor"/"viewer"/"sharer").
type GrantInput struct {
	ResourceType string
	ResourceID   string
	SubjectType  string
	SubjectID    string
	Relation     string
	ExpiresAt    *time.Time
}

// GrantAccess creates a permission tuple. The caller must hold share on the
// resource.
func (c *Client) GrantAccess(ctx context.Context, tenantID string, in GrantInput) error {
	req := &paperlessv1.GrantAccessRequest{
		TenantId: tenantID, ResourceType: resourceEnum(in.ResourceType), ResourceId: in.ResourceID,
		SubjectType: subjectEnum(in.SubjectType), SubjectId: in.SubjectID, Relation: relationEnum(in.Relation),
	}
	if in.ExpiresAt != nil {
		req.ExpiresAt = in.ExpiresAt.Unix()
	}
	_, err := c.perms.GrantAccess(ctx, req, c.callOpts()...)
	return mapErr(err)
}

// CheckAccess reports whether the calling subject (derived from the mTLS caller
// identity) may perform action (read/write/delete/share/download) on a resource.
func (c *Client) CheckAccess(ctx context.Context, tenantID, resourceType, resourceID, action string) (bool, error) {
	resp, err := c.perms.CheckAccess(ctx, &paperlessv1.CheckAccessRequest{
		TenantId: tenantID, ResourceType: resourceEnum(resourceType), ResourceId: resourceID,
		Permission: permissionEnum(action),
	}, c.callOpts()...)
	if err != nil {
		return false, mapErr(err)
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
	}, c.callOpts()...)
	if err != nil {
		return EffectivePermissions{}, mapErr(err)
	}
	p := resp.GetPermissions()
	return EffectivePermissions{
		Read: p.GetRead(), Write: p.GetWrite(), Delete: p.GetDelete(),
		Share: p.GetShare(), Download: p.GetDownload(),
	}, nil
}

// ---- statistics

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
	pb, err := c.stats.GetStatistics(ctx, &paperlessv1.GetStatisticsRequest{TenantId: tenantID}, c.callOpts()...)
	if err != nil {
		return Statistics{}, mapErr(err)
	}
	return Statistics{
		DocumentsByStatus: pb.GetDocumentsByStatus(), DocumentsBySource: pb.GetDocumentsBySource(),
		DocumentsByMime: pb.GetDocumentsByMime(), StorageBytes: pb.GetStorageBytes(),
		StorageByCategory: pb.GetStorageByCategory(), CategoriesTotal: pb.GetCategoriesTotal(),
		DocumentsTotal: pb.GetDocumentsTotal(), Backlog: pb.GetBacklog(),
	}, nil
}

// ---- mappers

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
	case paperlessv1.ProcessingStatus_PROCESSING_STATUS_RETRYING:
		return "retrying"
	}
	return ""
}

// Package documents is the paperless document service (US1): create/upload,
// read, list, update, move, delete, and download of documents. It owns the
// object-store round-trip (bytes never touch the metadata store), records the
// creator as owner, enqueues an extraction job, and emits a processing event.
//
// content_text and extracted_metadata are secrets of the read path: they are
// returned only when a caller explicitly asks for them (Get with
// includeContent) and are ALWAYS redacted from List projections.
package documents

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/blob"
	"github.com/go-freya/freya/services/paperless/internal/events"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// ErrNotFound is returned when a document does not exist or the caller may not
// see it (store/authz not-found are masked into this single error).
var ErrNotFound = errors.New("documents: not found")

// Document is the stored metadata + content model (re-exported for callers that
// need the row alongside a download stream).
type Document = store.Document

// Service manages documents.
type Service struct {
	st         repo.Store
	az         *authz.Authorizer
	blob       blob.Store
	pub        events.Publisher
	presignTTL time.Duration
	now        func() time.Time
}

// New builds the service.
func New(st repo.Store, az *authz.Authorizer, bs blob.Store, pub events.Publisher, presignTTL time.Duration) *Service {
	return &Service{st: st, az: az, blob: bs, pub: pub, presignTTL: presignTTL, now: time.Now}
}

// SetClock injects the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// CreateInput is an upload request. Reader streams the bytes; Size/MimeType are
// forwarded to the object store.
type CreateInput struct {
	Name        string
	Description string
	CategoryID  string
	Tags        map[string]string
	FileName    string
	MimeType    string
	Reader      io.Reader
	Size        int64
	Source      string
}

// UpdateInput changes a document's metadata. Empty Name keeps the stored value;
// a non-empty CategoryID re-parents the document (and recomputes its path).
type UpdateInput struct {
	Name        string
	Description string
	CategoryID  string
	Tags        map[string]string
}

// View is the JSON projection of a document. content_text and
// extracted_metadata appear only when explicitly included (never in List).
type View struct {
	ID                string            `json:"id"`
	TenantID          string            `json:"tenant_id"`
	CategoryID        string            `json:"category_id,omitempty"`
	CategoryPath      string            `json:"category_path,omitempty"`
	Name              string            `json:"name"`
	Description       string            `json:"description,omitempty"`
	FileName          string            `json:"file_name"`
	FileSize          int64             `json:"file_size"`
	MimeType          string            `json:"mime_type"`
	Checksum          string            `json:"checksum"`
	Status            string            `json:"status"`
	Source            string            `json:"source"`
	Tags              map[string]string `json:"tags,omitempty"`
	ProcessingStatus  string            `json:"processing_status"`
	CreatedBy         string            `json:"created_by"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	ContentText       string            `json:"content_text,omitempty"`
	ExtractedMetadata map[string]string `json:"extracted_metadata,omitempty"`
}

// BatchResult is one entry of a batch operation's per-id outcome.
type BatchResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// view projects a stored document. content_text/extracted_metadata are included
// only when includeContent is true.
func view(d store.Document, includeContent bool) View {
	v := View{
		ID: d.ID, TenantID: d.TenantID, CategoryPath: d.CategoryPath, Name: d.Name,
		Description: d.Description, FileName: d.FileName, FileSize: d.FileSize, MimeType: d.MimeType,
		Checksum: d.Checksum, Status: d.Status, Source: d.Source, Tags: d.Tags,
		ProcessingStatus: d.ProcessingStatus, CreatedBy: d.CreatedBy,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
	if d.CategoryID != nil {
		v.CategoryID = *d.CategoryID
	}
	if includeContent {
		v.ContentText = d.ContentText
		v.ExtractedMetadata = d.ExtractedMetadata
	}
	return v
}

func objectKey(tenantID, id string) string {
	return "tenants/" + tenantID + "/documents/" + id
}

// categoryPath resolves a category's materialized path within the tenant.
func (s *Service) categoryPath(ctx context.Context, tenantID, categoryID string) (string, error) {
	c, err := s.st.GetCategory(ctx, tenantID, categoryID)
	if err != nil {
		return "", mapNF(err)
	}
	return c.Path, nil
}

// Create uploads bytes, stores metadata, records the creator as owner, enqueues
// an extraction job, and emits a processing event.
func (s *Service) Create(ctx context.Context, subj authz.Subjects, in CreateInput) (View, error) {
	if in.CategoryID != "" {
		if err := s.az.Check(ctx, subj, authz.Category, in.CategoryID, authz.Write); err != nil {
			return View{}, mapAZ(err)
		}
	}
	id := store.NewID()
	key := objectKey(subj.TenantID, id)
	checksum, err := s.blob.Put(ctx, key, in.Reader, in.Size, in.MimeType)
	if err != nil {
		return View{}, err
	}

	d := store.Document{
		ID: id, TenantID: subj.TenantID, Name: in.Name, Description: in.Description,
		ObjectKey: key, FileName: in.FileName, FileSize: in.Size, MimeType: in.MimeType,
		Checksum: checksum, Status: store.DocActive, Source: sourceOrDefault(in.Source),
		Tags: in.Tags, ProcessingStatus: store.ProcPending,
		CreatedBy: subj.ActorID(), UpdatedBy: subj.ActorID(),
	}
	if in.CategoryID != "" {
		path, perr := s.categoryPath(ctx, subj.TenantID, in.CategoryID)
		if perr != nil {
			return View{}, perr
		}
		cid := in.CategoryID
		d.CategoryID = &cid
		d.CategoryPath = path
	}
	if err := s.st.InsertDocument(ctx, d); err != nil {
		return View{}, err
	}
	if err := s.az.GrantOwner(ctx, subj.TenantID, authz.Document, id, subj.UserID); err != nil {
		return View{}, err
	}
	if err := s.st.InsertJob(ctx, store.ProcessingJob{
		ID: store.NewID(), TenantID: subj.TenantID, DocumentID: id, Status: store.ProcPending,
	}); err != nil {
		return View{}, err
	}
	s.pub.Publish(ctx, subj.TenantID, events.Processing, events.DocumentPayload(id, store.ProcPending, in.Name))

	out, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	return view(out, false), nil
}

// Get returns a document's metadata; content_text/extracted_metadata are
// included only when includeContent is set.
func (s *Service) Get(ctx context.Context, subj authz.Subjects, id string, includeContent bool) (View, error) {
	if err := s.az.Check(ctx, subj, authz.Document, id, authz.Read); err != nil {
		return View{}, mapAZ(err)
	}
	d, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	return view(d, includeContent), nil
}

// List returns the tenant's documents matching f. content_text and
// extracted_metadata are ALWAYS redacted from list projections.
func (s *Service) List(ctx context.Context, subj authz.Subjects, f repo.DocFilter) ([]View, error) {
	if err := s.az.Check(ctx, subj, authz.Document, "", authz.Read); err != nil {
		return nil, mapAZ(err)
	}
	rows, err := s.st.ListDocuments(ctx, subj.TenantID, f)
	if err != nil {
		return nil, err
	}
	out := make([]View, 0, len(rows))
	for _, d := range rows {
		out = append(out, view(d, false))
	}
	return out, nil
}

// Update changes metadata (name/description/category/tags). A non-empty
// CategoryID re-parents the document and recomputes its category_path.
func (s *Service) Update(ctx context.Context, subj authz.Subjects, id string, in UpdateInput) (View, error) {
	if err := s.az.Check(ctx, subj, authz.Document, id, authz.Write); err != nil {
		return View{}, mapAZ(err)
	}
	d, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	if in.Name != "" {
		d.Name = in.Name
	}
	d.Description = in.Description
	if in.Tags != nil {
		d.Tags = in.Tags
	}
	if in.CategoryID != "" {
		path, perr := s.categoryPath(ctx, subj.TenantID, in.CategoryID)
		if perr != nil {
			return View{}, perr
		}
		cid := in.CategoryID
		d.CategoryID = &cid
		d.CategoryPath = path
	}
	d.UpdatedBy = subj.ActorID()
	if err := s.st.UpdateDocument(ctx, d); err != nil {
		return View{}, mapNF(err)
	}
	out, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	return view(out, false), nil
}

// Move re-parents a document into categoryID (recomputing its category_path).
func (s *Service) Move(ctx context.Context, subj authz.Subjects, id, categoryID string) (View, error) {
	if err := s.az.Check(ctx, subj, authz.Document, id, authz.Write); err != nil {
		return View{}, mapAZ(err)
	}
	d, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	if categoryID == "" {
		d.CategoryID = nil
		d.CategoryPath = ""
	} else {
		path, perr := s.categoryPath(ctx, subj.TenantID, categoryID)
		if perr != nil {
			return View{}, perr
		}
		cid := categoryID
		d.CategoryID = &cid
		d.CategoryPath = path
	}
	d.UpdatedBy = subj.ActorID()
	if err := s.st.UpdateDocument(ctx, d); err != nil {
		return View{}, mapNF(err)
	}
	out, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return View{}, mapNF(err)
	}
	return view(out, false), nil
}

// Delete removes a document. Soft delete marks it deleted (hidden from List);
// hard delete removes the row and its object bytes.
func (s *Service) Delete(ctx context.Context, subj authz.Subjects, id string, hard bool) error {
	if err := s.az.Check(ctx, subj, authz.Document, id, authz.Delete); err != nil {
		return mapAZ(err)
	}
	d, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return mapNF(err)
	}
	if hard {
		if err := s.st.DeleteDocument(ctx, subj.TenantID, id); err != nil {
			return mapNF(err)
		}
		return s.blob.Delete(ctx, d.ObjectKey)
	}
	d.Status = store.DocDeleted
	d.UpdatedBy = subj.ActorID()
	return mapNF(s.st.UpdateDocument(ctx, d))
}

// BatchDelete deletes each id, collecting a per-id outcome.
func (s *Service) BatchDelete(ctx context.Context, subj authz.Subjects, ids []string, hard bool) ([]BatchResult, error) {
	out := make([]BatchResult, 0, len(ids))
	for _, id := range ids {
		r := BatchResult{ID: id}
		if err := s.Delete(ctx, subj, id, hard); err != nil {
			r.Error = err.Error()
		} else {
			r.OK = true
		}
		out = append(out, r)
	}
	return out, nil
}

// Download opens the document's object bytes for reading (caller must Close),
// returning the metadata row alongside.
func (s *Service) Download(ctx context.Context, subj authz.Subjects, id string) (io.ReadCloser, Document, error) {
	if err := s.az.Check(ctx, subj, authz.Document, id, authz.Read); err != nil {
		return nil, Document{}, mapAZ(err)
	}
	d, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return nil, Document{}, mapNF(err)
	}
	rc, err := s.blob.Get(ctx, d.ObjectKey)
	if err != nil {
		return nil, Document{}, err
	}
	return rc, d, nil
}

// DownloadURL returns a presigned GET URL for the document's object bytes.
func (s *Service) DownloadURL(ctx context.Context, subj authz.Subjects, id string) (string, error) {
	if err := s.az.Check(ctx, subj, authz.Document, id, authz.Download); err != nil {
		return "", mapAZ(err)
	}
	d, err := s.st.GetDocument(ctx, subj.TenantID, id)
	if err != nil {
		return "", mapNF(err)
	}
	return s.blob.PresignGet(ctx, d.ObjectKey, s.presignTTL)
}

func sourceOrDefault(src string) string {
	if src == "" {
		return store.SourceUpload
	}
	return src
}

func mapAZ(err error) error {
	if errors.Is(err, authz.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func mapNF(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

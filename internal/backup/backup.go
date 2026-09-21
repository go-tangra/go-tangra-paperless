// Package backup exports and imports a tenant's paperless data (categories,
// document metadata, and permission tuples) for backup or tenant migration.
// Document BYTES are never included — a document travels as its object_key only,
// and an optional link placeholder can carry a signed-URL reference later; the
// extracted content_text is always excluded. Because an object key is bound to
// its document, import preserves entity ids so keys and permission tuples remain
// valid in the target tenant. Duplicate handling is per-name: skip or overwrite.
package backup

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-freya/freya/services/paperless/internal/authz"
	"github.com/go-freya/freya/services/paperless/internal/repo"
	"github.com/go-freya/freya/services/paperless/internal/store"
)

// SchemaVersion is the export format version.
const SchemaVersion = 1

// Import modes.
const (
	ModeSkip      = "skip"
	ModeOverwrite = "overwrite"
)

// ErrBadSchema is returned when importing an unsupported schema version.
var ErrBadSchema = errors.New("backup: unsupported schema version")

// CategoryExport is a category (folder) in a backup.
type CategoryExport struct {
	ID          string  `json:"id"`
	ParentID    *string `json:"parent_id,omitempty"`
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	Description string  `json:"description,omitempty"`
	SortOrder   int     `json:"sort_order"`
}

// DocumentExport is a document's metadata in a backup. content_text and extracted
// metadata are deliberately absent; the bytes are referenced by ObjectKey only.
// Link is a nil placeholder for a future signed-URL reference (never inline bytes)
// and is present only when the export requested links.
type DocumentExport struct {
	ID          string            `json:"id"`
	CategoryID  *string           `json:"category_id,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	ObjectKey   string            `json:"object_key"`
	FileName    string            `json:"file_name"`
	FileSize    int64             `json:"file_size"`
	MimeType    string            `json:"mime_type"`
	Checksum    string            `json:"checksum"`
	Status      string            `json:"status"`
	Source      string            `json:"source"`
	Tags        map[string]string `json:"tags,omitempty"`
	Link        *string           `json:"link,omitempty"`
}

// PermissionExport is one permission tuple in a backup.
type PermissionExport struct {
	ID           string     `json:"id"`
	ResourceType string     `json:"resource_type"`
	ResourceID   string     `json:"resource_id"`
	SubjectType  string     `json:"subject_type"`
	SubjectID    string     `json:"subject_id"`
	Relation     string     `json:"relation"`
	GrantedBy    string     `json:"granted_by,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}

// Backup is the export document.
type Backup struct {
	SchemaVersion int                `json:"schema_version"`
	ExportedAt    time.Time          `json:"exported_at"`
	IncludeLinks  bool               `json:"include_links"`
	Categories    []CategoryExport   `json:"categories"`
	Documents     []DocumentExport   `json:"documents"`
	Permissions   []PermissionExport `json:"permissions"`
}

// ImportResult reports what an import did.
type ImportResult struct {
	CategoriesImported  int `json:"categories_imported"`
	CategoriesSkipped   int `json:"categories_skipped"`
	DocumentsImported   int `json:"documents_imported"`
	DocumentsSkipped    int `json:"documents_skipped"`
	PermissionsImported int `json:"permissions_imported"`
	PermissionsSkipped  int `json:"permissions_skipped"`
}

// Service exports and imports tenant data.
type Service struct {
	st  repo.Store
	now func() time.Time
}

// New builds the service.
func New(st repo.Store) *Service { return &Service{st: st, now: time.Now} }

// SetClock injects the clock (tests).
func (s *Service) SetClock(now func() time.Time) { s.now = now }

// Export builds a backup of the caller's tenant. When includeLinks is true each
// document carries a nil link placeholder; document bytes are never inlined.
func (s *Service) Export(ctx context.Context, subj authz.Subjects, includeLinks bool) (Backup, error) {
	b := Backup{SchemaVersion: SchemaVersion, ExportedAt: s.now(), IncludeLinks: includeLinks}

	cats, err := s.st.ListCategories(ctx, subj.TenantID)
	if err != nil {
		return b, err
	}
	for _, c := range cats {
		b.Categories = append(b.Categories, CategoryExport{
			ID: c.ID, ParentID: c.ParentID, Name: c.Name, Path: c.Path,
			Description: c.Description, SortOrder: c.SortOrder,
		})
	}

	docs, err := s.st.ListDocuments(ctx, subj.TenantID, repo.DocFilter{})
	if err != nil {
		return b, err
	}
	for _, d := range docs {
		de := DocumentExport{
			ID: d.ID, CategoryID: d.CategoryID, Name: d.Name, Description: d.Description,
			ObjectKey: d.ObjectKey, FileName: d.FileName, FileSize: d.FileSize,
			MimeType: d.MimeType, Checksum: d.Checksum, Status: d.Status, Source: d.Source, Tags: d.Tags,
		}
		// content_text and ExtractedMetadata are intentionally omitted.
		if includeLinks {
			de.Link = nil // placeholder for a future signed-URL reference; bytes are never inlined
		}
		b.Documents = append(b.Documents, de)

		perms, err := s.st.ListPermissionsByResource(ctx, subj.TenantID, store.ResourceDocument, d.ID)
		if err != nil {
			return b, err
		}
		for _, p := range perms {
			b.Permissions = append(b.Permissions, toPermExport(p))
		}
	}

	for _, c := range cats {
		perms, err := s.st.ListPermissionsByResource(ctx, subj.TenantID, store.ResourceCategory, c.ID)
		if err != nil {
			return b, err
		}
		for _, p := range perms {
			b.Permissions = append(b.Permissions, toPermExport(p))
		}
	}

	return b, nil
}

// Import recreates the backup's categories, documents, and permissions in the
// caller's tenant. mode is skip (default) or overwrite for name collisions.
// Entity ids are preserved so object keys and tuples remain valid.
func (s *Service) Import(ctx context.Context, subj authz.Subjects, b Backup, mode string) (ImportResult, error) {
	var res ImportResult
	if b.SchemaVersion != SchemaVersion {
		return res, fmt.Errorf("%w: %d", ErrBadSchema, b.SchemaVersion)
	}
	if mode != ModeOverwrite {
		mode = ModeSkip
	}

	err := s.st.Atomic(ctx, subj.TenantID, func(tx repo.Store) error {
		// Categories, deduped by path.
		existingCats, err := tx.ListCategories(ctx, subj.TenantID)
		if err != nil {
			return err
		}
		catByPath := map[string]store.Category{}
		for _, c := range existingCats {
			catByPath[c.Path] = c
		}
		for _, ce := range b.Categories {
			if old, ok := catByPath[ce.Path]; ok {
				if mode == ModeSkip {
					res.CategoriesSkipped++
					continue
				}
				if err := tx.DeleteCategory(ctx, subj.TenantID, old.ID); err != nil {
					return err
				}
			}
			c := store.Category{
				ID: ce.ID, TenantID: subj.TenantID, ParentID: ce.ParentID, Name: ce.Name,
				Path: ce.Path, Description: ce.Description, SortOrder: ce.SortOrder, CreatedBy: subj.ActorID(),
			}
			if err := tx.InsertCategory(ctx, c); err != nil {
				return err
			}
			res.CategoriesImported++
		}

		// Documents, deduped by (category, name). Category path is recovered from
		// the backup's own categories so imported docs keep their materialized path.
		pathByCatID := map[string]string{}
		for _, ce := range b.Categories {
			pathByCatID[ce.ID] = ce.Path
		}
		existingDocs, err := tx.ListDocuments(ctx, subj.TenantID, repo.DocFilter{})
		if err != nil {
			return err
		}
		docByKey := map[string]store.Document{}
		for _, d := range existingDocs {
			docByKey[docKey(d.CategoryID, d.Name)] = d
		}
		for _, de := range b.Documents {
			if old, ok := docByKey[docKey(de.CategoryID, de.Name)]; ok {
				if mode == ModeSkip {
					res.DocumentsSkipped++
					continue
				}
				if err := tx.DeleteDocument(ctx, subj.TenantID, old.ID); err != nil {
					return err
				}
			}
			d := store.Document{
				ID: de.ID, TenantID: subj.TenantID, CategoryID: de.CategoryID, Name: de.Name,
				Description: de.Description, ObjectKey: de.ObjectKey, FileName: de.FileName,
				FileSize: de.FileSize, MimeType: de.MimeType, Checksum: de.Checksum,
				Status: defaultStr(de.Status, store.DocActive), Source: de.Source, Tags: de.Tags,
				CreatedBy: subj.ActorID(), UpdatedBy: subj.ActorID(),
			}
			if de.CategoryID != nil {
				d.CategoryPath = pathByCatID[*de.CategoryID]
			}
			if err := tx.InsertDocument(ctx, d); err != nil {
				return err
			}
			res.DocumentsImported++
		}

		// Permissions, deduped by (resource, subject, relation).
		for _, pe := range b.Permissions {
			existing, err := tx.ListPermissionsByResource(ctx, subj.TenantID, pe.ResourceType, pe.ResourceID)
			if err != nil {
				return err
			}
			if old, ok := findTuple(existing, pe); ok {
				if mode == ModeSkip {
					res.PermissionsSkipped++
					continue
				}
				if err := tx.DeletePermission(ctx, subj.TenantID, old.ID); err != nil {
					return err
				}
			}
			p := store.PermissionTuple{
				ID: pe.ID, TenantID: subj.TenantID, ResourceType: pe.ResourceType, ResourceID: pe.ResourceID,
				SubjectType: pe.SubjectType, SubjectID: pe.SubjectID, Relation: pe.Relation,
				GrantedBy: defaultStr(pe.GrantedBy, subj.ActorID()), ExpiresAt: pe.ExpiresAt,
			}
			if err := tx.InsertPermission(ctx, p); err != nil {
				return err
			}
			res.PermissionsImported++
		}

		return nil
	})
	return res, err
}

func toPermExport(p store.PermissionTuple) PermissionExport {
	return PermissionExport{
		ID: p.ID, ResourceType: p.ResourceType, ResourceID: p.ResourceID,
		SubjectType: p.SubjectType, SubjectID: p.SubjectID, Relation: p.Relation,
		GrantedBy: p.GrantedBy, ExpiresAt: p.ExpiresAt,
	}
}

func findTuple(tuples []store.PermissionTuple, pe PermissionExport) (store.PermissionTuple, bool) {
	for _, t := range tuples {
		if t.SubjectType == pe.SubjectType && t.SubjectID == pe.SubjectID && t.Relation == pe.Relation {
			return t, true
		}
	}
	return store.PermissionTuple{}, false
}

func docKey(categoryID *string, name string) string {
	cid := ""
	if categoryID != nil {
		cid = *categoryID
	}
	return cid + "\x00" + name
}

func defaultStr(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

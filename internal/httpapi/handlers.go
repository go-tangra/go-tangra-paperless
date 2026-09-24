package httpapi

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/backup"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/categories"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/documents"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/permissions"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/repo"
)

// Register mounts the paperless HTTP routes.
func (s *Server) Register(d Deps) {
	p := "/api/paperless/v1"
	maxUpload := d.MaxUpload
	if maxUpload <= 0 {
		maxUpload = 100 << 20
	}

	// ---- Documents
	s.MustHandle("POST", p+"/documents", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		if err := r.ParseMultipartForm(maxUpload); err != nil {
			WriteError(w, http.StatusBadRequest, "malformed_body")
			return
		}
		file, header, ferr := r.FormFile("file")
		if ferr != nil {
			WriteError(w, http.StatusBadRequest, "file_required")
			return
		}
		defer func() { _ = file.Close() }()
		var tags map[string]string
		if t := r.FormValue("tags"); t != "" {
			_ = json.Unmarshal([]byte(t), &tags)
		}
		name := r.FormValue("name")
		if name == "" {
			name = header.Filename
		}
		mime := header.Header.Get("Content-Type")
		if !mimeAllowed(d.AllowedMime, mime) {
			WriteError(w, http.StatusUnsupportedMediaType, "unsupported_media_type")
			return
		}
		if header.Size > maxUpload {
			WriteError(w, http.StatusRequestEntityTooLarge, "payload_too_large")
			return
		}
		v, err := d.Documents.Create(r.Context(), subj, documents.CreateInput{
			Name: name, Description: r.FormValue("description"), CategoryID: r.FormValue("category_id"),
			Tags: tags, FileName: header.Filename, MimeType: mime,
			Reader: io.LimitReader(file, maxUpload), Size: header.Size,
		})
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusCreated, v)
	})
	s.MustHandle("GET", p+"/documents", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		q := r.URL.Query()
		items, err := d.Documents.List(r.Context(), subj, repo.DocFilter{
			CategoryID: q.Get("category_id"), Status: q.Get("status"), MimeType: q.Get("mime_type"),
			Source: q.Get("source"), ProcessingStatus: q.Get("processing_status"), Tag: q.Get("tag"), CreatedBy: q.Get("created_by"),
		})
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	s.MustHandle("GET", p+"/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		v, err := d.Documents.Get(r.Context(), subj, r.PathValue("id"), r.URL.Query().Get("include_content") == "true")
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	s.MustHandle("PUT", p+"/documents/{id}", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in documents.UpdateInput
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		v, err := d.Documents.Update(r.Context(), subj, r.PathValue("id"), in)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	s.MustHandle("POST", p+"/documents/{id}/remove", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		if err := d.Documents.Delete(r.Context(), subj, r.PathValue("id"), r.URL.Query().Get("hard") == "true"); err != nil {
			failSvc(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.MustHandle("POST", p+"/documents/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			CategoryID string `json:"category_id"`
		}
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		v, err := d.Documents.Move(r.Context(), subj, r.PathValue("id"), in.CategoryID)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	s.MustHandle("GET", p+"/documents/{id}/download", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		rc, doc, err := d.Documents.Download(r.Context(), subj, r.PathValue("id"))
		if err != nil {
			failSvc(w, err)
			return
		}
		defer func() { _ = rc.Close() }()
		w.Header().Set("Content-Type", def(doc.MimeType, "application/octet-stream"))
		w.Header().Set("Content-Disposition", "attachment; filename=\""+doc.FileName+"\"")
		_, _ = io.Copy(w, rc)
	})
	s.MustHandle("GET", p+"/documents/{id}/download-url", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		url, err := d.Documents.DownloadURL(r.Context(), subj, r.PathValue("id"))
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"url": url})
	})
	s.MustHandle("POST", p+"/documents/search", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		hits, err := d.Search.Search(r.Context(), subj, in.Query, in.Limit)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": hits})
	})
	s.MustHandle("POST", p+"/documents/batch-delete", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			IDs  []string `json:"ids"`
			Hard bool     `json:"hard"`
		}
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		res, err := d.Documents.BatchDelete(r.Context(), subj, in.IDs, in.Hard)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"results": res})
	})

	// ---- Categories
	s.MustHandle("POST", p+"/categories", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in categories.Input
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		v, err := d.Categories.Create(r.Context(), subj, in)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusCreated, v)
	})
	s.MustHandle("GET", p+"/categories", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		items, err := d.Categories.List(r.Context(), subj)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	s.MustHandle("GET", p+"/categories/tree", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		tree, err := d.Categories.GetTree(r.Context(), subj)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"tree": tree})
	})
	s.MustHandle("GET", p+"/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		v, err := d.Categories.Get(r.Context(), subj, r.PathValue("id"))
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	s.MustHandle("PUT", p+"/categories/{id}", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in categories.Input
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		v, err := d.Categories.Update(r.Context(), subj, r.PathValue("id"), in)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	s.MustHandle("POST", p+"/categories/{id}/remove", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		if err := d.Categories.Delete(r.Context(), subj, r.PathValue("id"), r.URL.Query().Get("cascade") == "true"); err != nil {
			failSvc(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	s.MustHandle("POST", p+"/categories/{id}/move", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			ParentID string `json:"parent_id"`
		}
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		v, err := d.Categories.Move(r.Context(), subj, r.PathValue("id"), in.ParentID)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})

	// ---- Permissions
	s.MustHandle("POST", p+"/permissions/grant", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			ResourceType string     `json:"resource_type"`
			ResourceID   string     `json:"resource_id"`
			SubjectType  string     `json:"subject_type"`
			SubjectID    string     `json:"subject_id"`
			Relation     string     `json:"relation"`
			ExpiresAt    *time.Time `json:"expires_at"`
		}
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		v, err := d.Permissions.Grant(r.Context(), subj, permissions.GrantInput{
			ResourceType: in.ResourceType, ResourceID: in.ResourceID, SubjectType: in.SubjectType,
			SubjectID: in.SubjectID, Relation: in.Relation, ExpiresAt: in.ExpiresAt,
		})
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	s.MustHandle("POST", p+"/permissions/revoke", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			ResourceType string `json:"resource_type"`
			ResourceID   string `json:"resource_id"`
			ID           string `json:"id"`
		}
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		if err := d.Permissions.Revoke(r.Context(), subj, in.ResourceType, in.ResourceID, in.ID); err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"revoked": true})
	})
	s.MustHandle("GET", p+"/permissions", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		q := r.URL.Query()
		items, err := d.Permissions.ListForResource(r.Context(), subj, q.Get("resource_type"), q.Get("resource_id"))
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	s.MustHandle("POST", p+"/permissions/check", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			ResourceType string `json:"resource_type"`
			ResourceID   string `json:"resource_id"`
			Permission   string `json:"permission"`
		}
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		ok, err := d.Permissions.Check(r.Context(), subj, in.ResourceType, in.ResourceID, in.Permission)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"allowed": ok})
	})
	s.MustHandle("GET", p+"/permissions/effective", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		q := r.URL.Query()
		perms, grants, err := d.Permissions.Effective(r.Context(), subj, q.Get("resource_type"), q.Get("resource_id"))
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, map[string]any{"permissions": perms, "grants": grants})
	})

	// ---- Statistics
	s.MustHandle("GET", p+"/statistics", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		v, err := d.Stats.SystemWide(r.Context(), subj)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})
	s.MustHandle("GET", p+"/statistics/tenant", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		v, err := d.Stats.Tenant(r.Context(), subj)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, v)
	})

	// ---- Backup
	s.MustHandle("POST", p+"/backup/export", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			IncludeLinks bool `json:"include_links"`
		}
		if r.ContentLength != 0 {
			if err := DecodeJSON(r, &in, 0); err != nil {
				Fail(w, r, nil, err)
				return
			}
		}
		b, err := d.Backup.Export(r.Context(), subj, in.IncludeLinks)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, b)
	})
	s.MustHandle("POST", p+"/backup/import", func(w http.ResponseWriter, r *http.Request) {
		subj, err := subjects(r)
		if err != nil {
			failSvc(w, err)
			return
		}
		var in struct {
			Mode   string        `json:"mode"`
			Backup backup.Backup `json:"backup"`
		}
		if err := DecodeJSON(r, &in, 0); err != nil {
			Fail(w, r, nil, err)
			return
		}
		res, err := d.Backup.Import(r.Context(), subj, in.Backup, in.Mode)
		if err != nil {
			failSvc(w, err)
			return
		}
		WriteJSON(w, http.StatusOK, res)
	})

	// ---- Realtime SSE stream (optional: only when a hub is wired).
	if d.Hub != nil {
		s.RegisterStream(d.Hub)
	}
}

func def(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

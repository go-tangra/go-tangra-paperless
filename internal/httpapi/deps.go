package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-tangra/go-tangra-paperless/v4/internal/authz"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/backup"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/categories"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/documents"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/permissions"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/search"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/stats"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/store"
	"github.com/go-tangra/go-tangra-paperless/v4/internal/stream"
)

// Deps wire the paperless HTTP handlers.
type Deps struct {
	Documents   *documents.Service
	Categories  *categories.Service
	Permissions *permissions.Service
	Search      *search.Service
	Stats       *stats.Service
	Backup      *backup.Service
	Hub         *stream.Hub // optional: enables GET /stream (SSE) when set
	MaxUpload   int64
	AllowedMime []string // optional upload MIME allow-list; empty = allow all
}

// mimeAllowed reports whether mime is permitted by the allow-list. An empty
// allow-list permits everything. The match is on the bare media type; any
// parameters (e.g. "; charset=…") and case are ignored.
func mimeAllowed(allow []string, mime string) bool {
	if len(allow) == 0 {
		return true
	}
	base := mime
	if i := strings.IndexByte(base, ';'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimSpace(strings.ToLower(base))
	for _, a := range allow {
		if strings.ToLower(strings.TrimSpace(a)) == base {
			return true
		}
	}
	return false
}

// subjects derives the authz subject from the verified platform identity.
func subjects(r *http.Request) (authz.Subjects, error) {
	id, err := Caller(r)
	if err != nil {
		return authz.Subjects{}, err
	}
	return authz.Subjects{TenantID: id.TenantID, UserID: id.UserID, Roles: id.Roles, ActorKind: "user"}, nil
}

// failSvc maps a service/domain error to an HTTP response.
func failSvc(w http.ResponseWriter, err error) {
	var cve *categories.ValidationError
	switch {
	case errors.As(err, &cve):
		WriteError(w, http.StatusUnprocessableEntity, "validation_failed")
	case errors.Is(err, authz.ErrForbidden):
		WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, authz.ErrNotFound), errors.Is(err, store.ErrNotFound),
		errors.Is(err, documents.ErrNotFound), errors.Is(err, categories.ErrNotFound), errors.Is(err, permissions.ErrNotFound):
		WriteError(w, http.StatusNotFound, "not_found")
	case errors.Is(err, categories.ErrNotEmpty), errors.Is(err, categories.ErrCycle), errors.Is(err, store.ErrConflict):
		WriteError(w, http.StatusConflict, "conflict")
	case errors.Is(err, backup.ErrBadSchema):
		WriteError(w, http.StatusUnprocessableEntity, "validation_failed")
	case errors.Is(err, ErrUnauthenticated):
		WriteError(w, http.StatusUnauthorized, "unauthenticated")
	default:
		WriteError(w, http.StatusInternalServerError, "internal")
	}
}

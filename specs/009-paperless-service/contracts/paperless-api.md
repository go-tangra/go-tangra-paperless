# Contracts: Paperless Service (Phase 1)

Two external surfaces + two internal interfaces + the event contract. HTTP is
gateway-proxied under `/api/paperless` (browser, platform token); gRPC is module-to-module
(SPIFFE mTLS, not gateway-proxied). Every non-public HTTP operation declares an
`x-freya-permission`; mutating operations require the `X-CSRF-Token` header.

## HTTP (OpenAPI `api/openapi/paperless.yaml`, prefix `/api/paperless/v1`)

**Documents** (`documents:read` / `documents:write` / `documents:delete`)

| Method | Path | Permission | Purpose |
|--------|------|-----------|---------|
| POST | /documents | documents:write | Upload (multipart): store bytes, create metadata, enqueue extraction → 201 |
| GET | /documents | documents:read | List (filters: category, status, mime, source, tags, created_by, time, page) |
| GET | /documents/{id} | documents:read | Get metadata (content_text only on explicit include) |
| PUT | /documents/{id} | documents:write | Update metadata (name/description/category/tags) |
| POST | /documents/{id}/remove | documents:delete | Soft delete (?hard=true removes the object) |
| POST | /documents/{id}/move | documents:write | Move to another category |
| GET | /documents/{id}/download | documents:read | Stream the bytes |
| GET | /documents/{id}/download-url | documents:read | Short-lived presigned URL |
| POST | /documents/search | search:read | Full-text search (permission-filtered) |
| POST | /documents/batch-delete | documents:delete | Delete many by id (per-id result) |

**Categories** (`categories:read` / `categories:manage`)

| Method | Path | Permission |
|--------|------|-----------|
| POST | /categories | categories:manage |
| GET | /categories | categories:read |
| GET | /categories/{id} | categories:read |
| PUT | /categories/{id} | categories:manage |
| POST | /categories/{id}/remove | categories:manage (?cascade=true) |
| POST | /categories/{id}/move | categories:manage |
| GET | /categories/tree | categories:read |

**Permissions** (`permissions:manage` for writes, `documents:read`/`categories:read` for checks)

| Method | Path | Permission |
|--------|------|-----------|
| POST | /permissions/grant | permissions:manage |
| POST | /permissions/revoke | permissions:manage |
| GET | /permissions | permissions:manage (list for a resource or subject) |
| POST | /permissions/check | documents:read \| categories:read |
| GET | /permissions/accessible | documents:read (resources the caller may read) |
| GET | /permissions/effective | documents:read \| categories:read |

**Statistics / Backup**

| Method | Path | Permission |
|--------|------|-----------|
| GET | /statistics | stats:read (system-wide, admin) |
| GET | /statistics/tenant | stats:read |
| POST | /backup/export | backup:manage |
| POST | /backup/import | backup:manage |

Redaction: no response ever contains object-store credentials or (in list) extracted
content; `content_text`/`extracted_metadata` appear only on an explicit single-document
read or a search snippet.

## gRPC (`api/proto/paperless/v1/*.proto`, module-to-module)

- `PaperlessDocumentService`: Create, Get, List, Update, Delete, Move, Download,
  GetDownloadUrl, Search, BatchDelete.
- `PaperlessCategoryService`: Create, Get, List, Update, Delete, Move, GetTree.
- `PaperlessPermissionService`: GrantAccess, RevokeAccess, ListPermissions, CheckAccess,
  ListAccessibleResources, GetEffectivePermissions.
- `PaperlessStatisticsService`: GetStatistics.

Enums: `DocumentStatus{active,archived,deleted}`, `DocumentSource{upload,email}`,
`ProcessingStatus{pending,processing,completed,failed}`, `ResourceType{category,document}`,
`Relation{owner,editor,viewer,sharer}`, `SubjectType{user,role,tenant}`,
`Permission{read,write,delete,share,download}`. The caller/tenant come from the verified
SPIFFE identity + the request; responses redact `content_text`/credentials.

## Internal interface: Blob store (`internal/blob`)

```
Put(ctx, key string, r io.Reader, size int64, contentType string) (checksum string, err error)
Get(ctx, key string) (io.ReadCloser, error)
PresignGet(ctx, key string, ttl time.Duration) (url string, err error)
Delete(ctx, key string) error
```

Backed by the MinIO SDK over sealed credentials; a fake backs tests. `Put` streams while
computing SHA-256; never logs bytes or credentials.

## Internal interface: Extractor (`internal/extract`)

```
Extract(ctx, r io.Reader, contentType string) (text string, meta map[string]string, err error)   // Tika
Convert(ctx, r io.Reader, contentType string) (pdf io.ReadCloser, err error)                      // Gotenberg (optional)
Capabilities() (tikaURL, gotenbergURL string, convert bool)
```

Responses are size-bounded and treated as untrusted; a timeout bounds each call; when the
endpoints are unset the pipeline no-ops to `completed` with empty text (documents remain
downloadable).

## Event contract (platform event bus)

Publishes to `platform:events:<tenant>` (the shared Valkey stream): event types
`document.processing`, `document.completed`, `document.failed` with payload
`{document_id, processing_status, name}` (no content, no credentials). The gateway SSE hub
relays these to the browser for live status; the module ignores events it did not
originate.

---
title: Reference
summary: Document and category fields, states, access levels, permissions, roles, events, API routes, errors and limits.
kind: reference
order: 60
tags: [paperless, reference, fields, permissions, roles, api, limits]
---
## Document fields

| Field | Meaning |
|---|---|
| `id` | document ID |
| `name` | defaults to the file name; the portal allows 255 characters |
| `description` | free text |
| `category_id`, `category_path` | the category and its path when the document was filed |
| `file_name`, `file_size`, `mime_type` | the uploaded file |
| `checksum` | SHA-256 of the bytes |
| `status` | `active`, `archived` or `deleted` |
| `source` | `upload` (browser) or what the calling service set, for example `email` |
| `tags` | key/value pairs (the portal allows 32) |
| `processing_status` | see [processing](#processing-states) |
| `created_by`, `created_at`, `updated_at` | who and when |
| `content_text`, `extracted_metadata` | the extracted text and file metadata; returned only by `GET /documents/{id}?include_content=true`, never in lists |

## Document states

| Status | Meaning |
|---|---|
| `active` | normal |
| `archived` | accepted as a filter and in backups; the portal does not set it |
| `deleted` | soft-deleted: hidden from the list (unless filtered for) and from search |

## Processing states

| State | Meaning |
|---|---|
| `pending` | queued, or waiting to retry |
| `processing` | text extraction running |
| `completed` | text extracted and indexed |
| `failed` | extraction failed after the retries (3 by default) |

The events `document.processing`, `document.completed` and
`document.failed` are published live for each change.

## Category fields

| Field | Meaning |
|---|---|
| `name` | unique under its parent; top-level names unique in the tenant |
| `parent_id` | empty for a top-level category |
| `path` | for example `/Finance/2026` |
| `depth` | 0 for a top-level category |
| `subcategory_count` | number of direct subcategories |

## Access levels

| Level | Read | Download link | Change | Delete | Share |
|---|:-:|:-:|:-:|:-:|:-:|
| `viewer` | yes | yes | | | |
| `sharer` | yes | | | | yes |
| `editor` | yes | yes | yes | yes | |
| `owner` | yes | yes | yes | yes | yes |

Subjects: a user, a role, or everyone in the tenant. Grants may expire.
Grants on a category reach every document and category beneath it. Tenant
`owner` and `admin` role holders always have `owner` access. A `sharer` may
grant and revoke `viewer` and `sharer` only.

## Permissions and roles

| Permission | Allows |
|---|---|
| `documents:read` | list, read and download documents; read access checks; live events |
| `documents:write` | upload, edit and move documents |
| `documents:delete` | delete documents |
| `categories:read` | browse categories |
| `categories:manage` | create, change, move and delete categories |
| `permissions:manage` | list, grant and revoke access |
| `search:read` | full-text search |
| `stats:read` | statistics |
| `backup:manage` | export and import the tenant's documents and grants |

Each permission works only on the documents and categories your access level
allows.

| Module role | Permissions |
|---|---|
| Paperless administrator | all nine |
| Paperless editor | `documents:read`, `documents:write`, `categories:read`, `search:read` |
| Paperless viewer | `documents:read`, `categories:read`, `search:read` |

The platform's built-in roles also receive paperless permissions: `owner` and
`admin` all; `operator` all but `backup:manage`; `member`
`documents:read`, `documents:write`, `categories:read`, `search:read` and
`stats:read`; `auditor` the same without `documents:write`.

## Backup

`POST /backup/export` returns the tenant's categories, document metadata
(never the extracted text, never the file bytes) and grants.
`POST /backup/import` recreates them with their IDs, `mode` `skip` or
`overwrite` for records that already exist. There is no portal page for
backup. The import request body is currently limited to 64 KiB.

## API

The browser API is served by the gateway under `/api/paperless/v1`. Every
change needs the `X-CSRF-Token` header.

| Route | Permission |
|---|---|
| `GET /documents` (`page`, `page_size` 1–200, `sort`, `order`, `category_id`, `status`, `mime_type`, `source`, `processing_status`, `tag`, `created_by`) | `documents:read` |
| `POST /documents` (multipart: `file`, `name`, `description`, `category_id`, `tags` as JSON) | `documents:write` |
| `GET /documents/{id}`, `GET /documents/{id}/download`, `GET /documents/{id}/download-url` | `documents:read` |
| `PUT /documents/{id}`, `POST /documents/{id}/move` | `documents:write` |
| `POST /documents/{id}/remove` (`?hard=true` deletes for good), `POST /documents/batch-delete` | `documents:delete` |
| `POST /documents/search` (`{"query": "...", "limit": 25}`) | `search:read` |
| `GET /categories`, `GET /categories/tree`, `GET /categories/{id}` | `categories:read` |
| `POST /categories`, `PUT /categories/{id}`, `POST /categories/{id}/move`, `POST /categories/{id}/remove` | `categories:manage` |
| `GET /permissions`, `POST /permissions/grant`, `POST /permissions/revoke` | `permissions:manage` |
| `POST /permissions/check`, `GET /permissions/effective` | `documents:read` |
| `GET /statistics/tenant` | `stats:read` |
| `POST /backup/export`, `POST /backup/import` | `backup:manage` |
| `GET /stream` (server-sent events) | `documents:read` |

Search returns at most 200 hits per request (50 when `limit` is missing or above 200).

## Errors

| Reason | Status | Typical cause |
|---|---|---|
| `validation_failed` | 422 | a bad field or list parameter |
| `forbidden` | 403 | missing permission or access level |
| `not_found` | 404 | the document or category does not exist, or you may not see it |
| `conflict` | 409 | duplicate category name, deleting a category with subcategories, moving a category under itself |
| `unsupported_media_type` | 415 | the file type is not in the operator's allow-list |
| `payload_too_large` | 413 | the file is over the upload limit |
| `malformed_body` | 400 | a broken upload, or a request body over the limit |
| `file_required` | 400 | the upload has no `file` part |

# Data Model: Paperless Service (Phase 1)

All tables carry `tenant_id` and are governed by per-tenant row-level security (RLS); the
application role is `NOBYPASSRLS`. Trusted worker/event paths run under a scoped system
transaction (`app.system`) that still names a tenant. Document **bytes** are never stored
here — only in object storage, referenced by `object_key`.

## Entities

### Document (`paperless_documents`)

| Field | Type | Notes |
|-------|------|-------|
| id | uuid PK | |
| tenant_id | uuid NOT NULL | RLS key |
| category_id | uuid NULL | FK → paperless_categories; null = root |
| category_path | text NOT NULL DEFAULT '' | materialized path of the category at write time |
| name | text NOT NULL | 1..255 |
| description | text NOT NULL DEFAULT '' | |
| object_key | text NOT NULL | S3 object key of the bytes |
| file_name | text NOT NULL | original filename |
| file_size | bigint NOT NULL | bytes |
| mime_type | text NOT NULL | detected/declared content type |
| checksum | text NOT NULL | SHA-256 hex of the bytes |
| status | text NOT NULL DEFAULT 'active' | active \| archived \| deleted |
| source | text NOT NULL DEFAULT 'upload' | upload \| email |
| tags | jsonb NOT NULL DEFAULT '{}' | string→string |
| content_text | text NOT NULL DEFAULT '' | extracted full text — REDACTED from list/log/audit |
| extracted_metadata | jsonb NOT NULL DEFAULT '{}' | REDACTED |
| processing_status | text NOT NULL DEFAULT 'pending' | pending \| processing \| completed \| failed |
| search_tsv | tsvector | GIN-indexed; content+name+description+tags |
| created_by | text NOT NULL DEFAULT '' | |
| updated_by | text NOT NULL DEFAULT '' | |
| created_at | timestamptz NOT NULL DEFAULT now() | |
| updated_at | timestamptz NOT NULL DEFAULT now() | |

Indexes: `(tenant_id, category_id)`, `(tenant_id, status)`, `(tenant_id, processing_status)`,
GIN on `search_tsv`, GIN on `tags`. Unique: `(tenant_id, object_key)`.

**State transitions** — `status`: active → archived → active; active → deleted (soft);
hard delete removes the row and the object. `processing_status`: pending → processing →
completed | failed; failed → pending (reprocess).

### Category (`paperless_categories`)

| Field | Type | Notes |
|-------|------|-------|
| id | uuid PK | |
| tenant_id | uuid NOT NULL | RLS key |
| parent_id | uuid NULL | FK → self; null = root |
| name | text NOT NULL | 1..255, unique per (tenant, parent) |
| path | text NOT NULL | materialized, e.g. /finance/2026 |
| description | text NOT NULL DEFAULT '' | |
| depth | int NOT NULL DEFAULT 0 | |
| sort_order | int NOT NULL DEFAULT 0 | |
| document_count | int NOT NULL DEFAULT 0 | denormalized |
| subcategory_count | int NOT NULL DEFAULT 0 | denormalized |
| created_by | text NOT NULL DEFAULT '' | |
| created_at / updated_at | timestamptz | |

Indexes: `(tenant_id, parent_id)`, `(tenant_id, path text_pattern_ops)` for subtree/prefix.
Unique: `(tenant_id, parent_id, name)`. Move recomputes `path`/`depth` for the subtree.

### Permission Tuple (`paperless_permissions`)

| Field | Type | Notes |
|-------|------|-------|
| id | uuid PK | |
| tenant_id | uuid NOT NULL | RLS key |
| resource_type | text NOT NULL | document \| category |
| resource_id | text NOT NULL | |
| subject_type | text NOT NULL | user \| role \| tenant |
| subject_id | text NOT NULL DEFAULT '' | empty for tenant-wide |
| relation | text NOT NULL | owner \| editor \| viewer \| sharer |
| granted_by | text | |
| granted_at | timestamptz NOT NULL DEFAULT now() | |
| expires_at | timestamptz NULL | null = never |

Index: `(tenant_id, resource_type, resource_id)`, `(tenant_id, subject_type, subject_id)`.
**Effective relation** on a document = strongest of its own tuples ∪ tuples on its category
and every ancestor category (matched by `path` prefix) ∪ tenant-wide tuples, ignoring rows
where `expires_at <= now()`.

### Processing Job (`paperless_jobs`)

| Field | Type | Notes |
|-------|------|-------|
| id | uuid PK | |
| tenant_id | uuid NOT NULL | RLS key |
| document_id | uuid NOT NULL | FK → paperless_documents |
| status | text NOT NULL DEFAULT 'pending' | pending \| processing \| completed \| failed \| retrying |
| retry_count | int NOT NULL DEFAULT 0 | |
| max_retries | int NOT NULL DEFAULT 3 | |
| lease_until | timestamptz NULL | single-winner claim |
| next_retry_at | timestamptz NULL | backoff |
| result | jsonb NULL | non-secret summary |
| started_at / completed_at | timestamptz NULL | |
| created_at / updated_at | timestamptz | |

Index: `(status, next_retry_at) WHERE status IN ('pending','retrying')` for the claim.
Claim uses `FOR UPDATE SKIP LOCKED` (system scope).

### Audit Event (`paperless_audit_events`)

Append-only hypertable (time-keyed, no PK), same shape as the deployer's: `ts`,
`tenant_id`, `event_type`, `actor_kind`, `actor_id`, `subject_kind`, `subject_id`,
`outcome`, `reason`, `correlation_id`, `details` (jsonb, secret/content keys guarded out).

## Relationships

- Document *belongs to* zero-or-one Category (null = root); Category *has many* Documents
  and *has many* child Categories (tree).
- Document *has one* current Processing Job lifecycle; jobs are per-document.
- Permission Tuples attach to Documents or Categories; grants on a Category inherit to its
  subtree via `path` prefix.
- Blob (object storage) is referenced by `Document.object_key`; 1:1 with an active
  Document.

## Redaction & sealing

- `content_text` and `extracted_metadata` are **never** serialized into list responses,
  logs, or audit `details`; they are returned only by the explicit document-read and
  search paths to permitted callers.
- Object-store credentials live in config sealed by the `sealed` envelope package (KEK);
  never persisted in these tables and never returned.

## Validation rules (from FRs)

- name 1..255; description ≤ 1024; tags bounded count/length; upload size ≤ configured max;
  mime type against an allow/deny policy.
- category name unique per (tenant, parent); move must not create a cycle (a category
  cannot be moved under its own descendant).
- permission relation/subject_type/resource_type from closed enums; expiry in the future.
- pagination limit bounded; search query length bounded.

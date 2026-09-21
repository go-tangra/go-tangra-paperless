# Paperless service — operations

The **paperless** service is a tenant-scoped **document management** module. It
stores documents (metadata in TimescaleDB, bytes in S3-compatible object
storage), organizes them into a category hierarchy, extracts their text
asynchronously (Apache Tika / Gotenberg), full-text-searches them, and governs
access with Zanzibar-style fine-grained permissions. It registers with the
application gateway (browser API under `/api/paperless`) and exposes a
service-to-service gRPC API (`paperless.v1`, not gateway-proxied).

## Running

```
paperlesssvc -config deploy/container.yaml     # run (applies migrations)
paperlesssvc bootstrap -config <cfg>           # apply migrations and exit
```

In the containerized platform stack it comes up with one command; see
`deploy/stack/README.md`. The service:

- enrolls for its SVID (`spiffe://<td>/svc/paperless`) over lcm's enrollment gRPC,
- migrates its TimescaleDB schema (per-tenant row-level security),
- serves the browser API (via the gateway) and `paperless.v1` gRPC on `:9955`,
  with a health/readiness admin endpoint on `:9790`,
- registers its routes, permissions, CASL abilities and nav with the gateway,
- seeds its API permissions into auth and grants them to the built-in roles,
- runs the async extraction worker pool.

## Configuration

`container.yaml` sections: `db`, `valkey`, `kek` (envelope key), `object_store`
(S3-compatible endpoint, bucket, sealed credentials, presign TTL), `extract`
(`tika_url`, `gotenberg_url`, timeout, max bytes), `uploads` (max size, allowed
MIME allow-list), `jobs` (worker pool: workers, interval, lease, job timeout,
retry policy, backoff, cleanup), `events`, `gateway`, and `enroll` (SVID
enrollment). `limits_paperless` bounds backup and search-query size.

## Documents

A document is metadata + a blob. On **Create**, the bytes are streamed to object
storage under `tenants/<tenant>/documents/<id>` while a SHA-256 checksum is
computed, the metadata row is written (status `active`, processing `pending`),
the creator is granted `owner`, and a processing job is enqueued. Reads redact
`content_text` and `extracted_metadata` unless explicitly requested. **Delete**
is soft by default (status `deleted`); a hard delete also removes the object.
**Download** streams the bytes; **GetDownloadUrl** returns a short-lived
presigned URL. **Move** reassigns the category and recomputes `category_path`.

## Categories

Folders form a hierarchy with a materialized `path` (e.g. `/finance/2026`),
`depth`, and cached `document_count`/`subcategory_count`. **Move** reparents a
subtree and recomputes every descendant's path/depth (cycle-guarded). **Delete**
requires the category be empty unless cascade is requested. **GetTree** returns
the nested tree. Unique per tenant by `(parent_id, name)`.

## Permissions

Zanzibar-style tuples bind a subject (`user` | `role` | `tenant`) to a resource
(`document` | `category`) with a relation: `owner` (read/write/delete/share),
`editor` (read/write/delete), `viewer` (read/download), `sharer` (read/share).
Grants support expiry. Access is **inherited** down the category tree: a grant on
a category (or an ancestor category, matched by materialized-path prefix) confers
that access to descendant categories and documents; a tenant-wide grant applies
to every resource in the tenant. `CheckAccess`, `ListAccessibleResources`, and
`GetEffectivePermissions` answer authorization queries. Granting requires the
`share` permission on the resource.

## Extraction pipeline

On Create the document is `pending`. A distributed worker pool claims due jobs
with a single-winner lease (`FOR UPDATE SKIP LOCKED`) so no job runs twice across
instances. Processing: mark `processing` (published live), fetch the bytes, call
Tika to extract `content_text` + metadata (optionally Gotenberg to convert for
preview), which feeds the Postgres full-text index, then mark `completed`; on
failure retry with exponential backoff up to `max_retries`, else `failed`. Every
transition is published as `document.processing` / `document.completed` /
`document.failed` on the platform event bus (`platform:events:<tenant>` Valkey
stream) for the gateway SSE hub, so the UI updates without polling. A cleanup
worker deletes old finished jobs.

## Full-text search

`POST /api/paperless/v1/documents/search` runs a tenant-scoped Postgres
full-text query over a generated `tsvector` (name, description, extracted
content) using `websearch_to_tsquery`, ranks with `ts_rank`, returns
`ts_headline` snippets, and is filtered to only the documents the caller may
read (via the permission model). Supports category/status/MIME filters and
pagination.

## Statistics

Per-tenant counts by status, source and MIME type; total and per-category
storage usage (bytes); category counts; processing backlog (pending/processing/
failed); recent activity. Administrators additionally get a per-tenant breakdown
(system-wide).

## Security notes

- **Object-store credentials** are sealed with envelope encryption (the `sealed`
  package + KEK) and never returned in any read, list, or export.
- **Extracted content** (`content_text`, `extracted_metadata`) is redacted from
  list/read responses, logs, audit detail and backups unless explicitly
  requested; presigned URLs and file keys — never document bytes — are the only
  blob references that ever leave the service.
- **Tenant isolation** is enforced by per-tenant PostgreSQL row-level security;
  trusted worker paths run with a system scope.
- **Audit** is append-only and tamper-evident, recording actor (SVID or platform
  user), tenant, outcome and reason for every operation; access refusals are
  audited.

## Backup

`POST /api/paperless/v1/backup/export` exports the tenant's categories (with
their hierarchy), document metadata (never `content_text`) and permission
tuples, versioned by schema; document bytes are referenced by file key
(optionally as presigned URLs, never inlined). `POST
/api/paperless/v1/backup/import` recreates them (mode `skip` or `overwrite`),
preserving entity ids so blob references and grants re-link unchanged.

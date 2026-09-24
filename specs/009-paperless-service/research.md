# Research: Paperless Service (Phase 0)

Decisions resolving the Technical Context, with rationale and alternatives. Each entry
follows Decision / Rationale / Alternatives.

## 1. Object storage for document bytes (S3-compatible)

**Decision**: Store document bytes in S3-compatible object storage (MinIO/RustFS),
keyed by a per-document object key `tenants/<tenant>/documents/<doc-id>`, using the MinIO
Go SDK (`github.com/minio/minio-go/v7`) behind an internal `blob` interface; compute a
SHA-256 checksum over the bytes on upload; issue short-lived presigned GET URLs for
downloads. Store credentials are sealed with the platform `sealed` package + KEK.

**Rationale**: The reference (go-tangra-paperless) uses S3/MinIO; keeping bytes out of the
database is the correct scaling choice and matches the platform's "database holds
metadata, not blobs" posture. The MinIO SDK is the de-facto S3 client, supports presigned
URLs and streaming, and works against MinIO/RustFS/AWS. The `blob` interface keeps the SDK
at the edge and lets a `memstore`-style fake back tests.

**Alternatives**: (a) hand-rolled SigV4 `net/http` client — fewer deps but re-implements
presigning/multipart, more risk; kept as a fallback if the SDK's transitive deps are
undesirable. (b) Postgres large objects — rejected (heavy on the DB, not how tangra works,
loses presigned-URL offload).

## 2. Text extraction & conversion (Tika + Gotenberg, async)

**Decision**: Extraction is an asynchronous pipeline, not inline. On upload the document
is `pending` and an extraction job is enqueued. A worker calls Apache **Tika**
(`PUT /tika` for text, `PUT /meta` for metadata) over `net/http`, optionally calls
**Gotenberg** to convert office formats to PDF for preview, stores the extracted text +
metadata, updates the full-text index, and marks the document `completed`; failures retry
with exponential backoff then mark `failed`. Both services are configurable HTTP
endpoints; when absent, documents stay stored and reprocessable.

**Rationale**: Matches the reference pipeline; keeping extraction async keeps upload fast
and isolates a slow/hostile extractor from the request path. Tika/Gotenberg are the same
tools tangra uses and are language-agnostic HTTP services (no CGo/Java in-process).

**Alternatives**: in-process Go extractors (e.g. `ledongthuc/pdf`) — limited format
coverage vs Tika; rejected for parity and breadth. Synchronous extraction — rejected
(couples upload latency to the extractor).

## 3. Category hierarchy: materialized paths

**Decision**: Categories store a materialized `path` (e.g. `/finance/2026`), a `depth`,
and denormalized `document_count`/`subcategory_count`. Subtree queries and permission
inheritance use `path LIKE '<prefix>/%'`. Moving a category recomputes the paths/depths of
the subtree in one transaction.

**Rationale**: The reference uses materialized-path queries; they make subtree reads and
inheritance a single indexed scan, which is exactly what permission inheritance and the
tree view need. Denormalized counts avoid N+1 aggregation on the tree view.

**Alternatives**: recursive CTEs / adjacency list — simpler writes but heavier reads and
no cheap prefix filter for inheritance; `ltree` — Postgres-specific and adds an extension
dependency; rejected in favor of a plain indexed text prefix.

## 4. Zanzibar permissions with inheritance and expiry

**Decision**: Reuse the platform's Zanzibar-style tuple model (as in warden/lcm): a tuple
binds a subject (user|role|tenant) to a resource (document|category) with a relation
(owner|editor|viewer|sharer), optionally with `expires_at`. Effective permission on a
document = the strongest relation among (its own tuples) ∪ (tuples on its category and all
ancestor categories) ∪ (tenant-wide tuples), excluding expired tuples. `CheckAccess`,
`ListAccessibleResources`, and `GetEffectivePermissions` derive from this. Resource
existence is masked from subjects without read.

**Rationale**: Matches the reference permission model and the platform's existing authz
convention; inheritance via the category's materialized path is a cheap prefix query.

**Alternatives**: OpenFGA/SpiceDB service — the platform already standardizes on the
in-module tuple model (warden/lcm); adding an external authz service is out of scope and
heavier. RBAC-only — insufficient for per-document sharing.

## 5. Full-text search (Postgres tsvector) filtered by permission

**Decision**: Maintain a `tsvector` column over extracted content + name + description +
tags, GIN-indexed, updated by the extraction pipeline. `SearchDocuments` runs a
tenant-scoped `websearch_to_tsquery` ranked (`ts_rank`) query, returns `ts_headline`
snippets, and filters results to documents the caller may read (via the permission
engine's accessible-id set or a per-row check), paginated.

**Rationale**: The database already holds the extracted text under RLS; Postgres FTS gives
ranking + snippets without a separate search engine, and permission filtering stays in one
place. Matches the reference's DB-backed search.

**Alternatives**: Elasticsearch/OpenSearch — powerful but a heavy new dependency and a
second copy of tenant content to secure; rejected for v1. `LIKE`/trigram — no ranking/
snippets; rejected.

## 6. Extraction worker pool (distributed lease claim)

**Decision**: A pool of workers claims due extraction jobs with `FOR UPDATE SKIP LOCKED`
and a lease, exactly as the deployer/lcm schedulers do, so a job runs on exactly one
worker across instances. Jobs carry a retry count against a max, exponential backoff, and
a per-job timeout; a cleanup worker prunes old terminal jobs. Status transitions publish
to `platform:events:<tenant>` for the gateway SSE hub.

**Rationale**: Reuses a proven platform pattern; the SQL lease gives single-winner
execution without a separate queue system.

**Alternatives**: an external queue (NATS/Redis streams as the primary queue) — the
platform's lease-in-Postgres pattern is already the convention and avoids a new broker.

## 7. Uploads & presigned downloads

**Decision**: Uploads come through the gateway as multipart to the module HTTP handler,
which streams bytes to the object store while hashing (SHA-256) and enforcing a configured
max size and an allow/deny content-type policy. Downloads are either a direct streamed
`GET` or a short-lived presigned URL scoped to the single object, read-only, for the
caller's tenant.

**Rationale**: Streaming avoids buffering large files in memory; presigned URLs offload
bandwidth to the object store while remaining short-lived and single-resource (SR-006).

**Alternatives**: presigned PUT for direct-to-store upload — skips the module for bytes
but complicates checksum/authz/quotas at create time; deferred.

## 8. UI: Module-Federation remote (Vue 3 + Vuetify/Materio)

**Decision**: Ship `services/paperless/ui` as a Vue 3 + Vite + Vuetify Module-Federation
remote, mirroring `services/deployer/ui`: exposes `./routes` and `./nav`, registered with
the gateway; screens for documents+upload, a category tree, a permissions editor, search,
and a statistics dashboard; live processing status via the shared SSE hub.

**Rationale**: Matches every other module's UI shape; reuses the shell, auth, CSRF, and
SSE conventions already established.

**Alternatives**: server-rendered UI — inconsistent with the platform shell; rejected.

## STRIDE Threat Model

The feature touches authentication, transport, parsing (search queries + extraction
responses), and secrets (object-store credentials) plus confidential document content, so
a STRIDE model is required (Constitution Threat-Model gate).

- **Spoofing**: a caller impersonating a service or user. *Mitigation*: SPIFFE mTLS for
  service-to-service; platform-token verification for browser calls; no anonymous routes
  except explicitly `x-freya-public`.
- **Tampering**: altering documents, permissions, or audit. *Mitigation*: authz before
  every mutation; append-only tamper-evident audit; checksums on stored bytes; RLS.
- **Repudiation**: denying an action. *Mitigation*: every state change audited with
  actor/tenant/outcome/reason and correlation id.
- **Information Disclosure**: cross-tenant leakage; credential or extracted-content
  leakage; a search returning unreadable documents; an over-broad presigned link.
  *Mitigation*: per-tenant RLS on all metadata/content; sealed credentials never emitted;
  extracted content redacted from logs/audit and returned only via permitted read/search;
  search filtered by effective permission and tenant; presigned links short-lived,
  single-resource, read-only.
- **Denial of Service**: oversized/malicious uploads, a hostile extraction response, a
  slow extractor, a pathological search. *Mitigation*: bounded upload size + content-type
  policy; per-extraction timeout + bounded retries + worker caps; extraction responses
  treated as untrusted and size-bounded; search queries validated and paginated.
- **Elevation of Privilege**: reading a resource through a stale/expired/revoked grant, or
  via inheritance that should not apply. *Mitigation*: expiry enforced on every check;
  revocation immediate; inheritance computed from the live category path; existence of
  unreadable resources masked.

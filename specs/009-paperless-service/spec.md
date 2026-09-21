# Feature Specification: Paperless Service

**Feature Branch**: `009-paperless-service`

**Created**: 2026-09-20

**Status**: Draft

**Input**: User description: "Create a new service under services/ named `paperless`, replicating the functionality of go-tangra-paperless as a Freya platform module: a tenant-scoped document management service that stores, organizes, extracts text from, and full-text-searches documents, with S3-compatible blob storage, a category hierarchy, and Zanzibar-style fine-grained permissions."

## Overview

The **paperless** service is a tenant-scoped **document management** module. It stores
documents — their bytes in S3-compatible object storage and their metadata and extracted
text in the platform database — organizes them into a hierarchy of categories (folders),
extracts their text so they can be searched, and controls who can see or change each
document or folder through fine-grained, inheritable permissions.

A person uploads a file; the service stores the bytes, records the metadata, and (in the
background) extracts the text and any embedded metadata so the document becomes
searchable and previewable. People browse a category tree, move documents between
folders, search across the full text of everything they are allowed to read, download
originals, and share individual documents or whole folders with other people, roles, or
the whole tenant — read-only, editor, or owner — optionally with an expiry.

It is a platform module: it registers with the application gateway, authenticates callers
with the platform's service identities and end-user tokens, isolates every tenant's data,
seals the object-store credentials at rest, redacts extracted text from logs and audit,
and ships a browser UI as a composed remote. Document bytes live only in object storage;
the database holds metadata and the extracted, searchable text.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Upload, store, and download a document (Priority: P1)

A person uploads a file. The service stores the bytes durably, records the document's
metadata (name, size, type, checksum), and lets the person list, open, and download it
again — including via a short-lived shareable link — with the uploader becoming its owner.

**Why this priority**: Storing and retrieving a document is the irreducible core of the
service; nothing else matters without it. It is the MVP.

**Independent Test**: Upload a small file, confirm it appears in the document list with
the correct name, size, type, and a computed checksum; download it and confirm the bytes
match; obtain a time-limited download link and confirm it retrieves the same bytes — all
without categories, search, or sharing.

**Acceptance Scenarios**:

1. **Given** a signed-in person with write permission, **When** they upload a file,
   **Then** a document is created with its bytes stored in object storage, a SHA-256
   checksum computed over the bytes, its size and content type recorded, and the person
   recorded as its owner.
2. **Given** an existing document, **When** the person requests its bytes or a
   short-lived download link, **Then** the exact stored bytes are returned (directly or
   via the link) and the link expires after a bounded time.
3. **Given** an existing document, **When** the person updates its metadata (name,
   description, tags) or soft-deletes it, **Then** the change is recorded, a soft-deleted
   document is hidden from normal listings, and a hard delete also removes the stored
   bytes.
4. **Given** several documents, **When** the person lists documents with filters (status,
   type, source, tags, time range), **Then** only matching documents the person may read
   are returned, paginated.

---

### User Story 2 - Automatic text extraction and full-text search (Priority: P1)

After a document is uploaded, the service extracts its text and embedded metadata in the
background so the person can find the document later by searching its contents, and can
watch the extraction progress from pending to completed without refreshing.

**Why this priority**: Turning an opaque blob into searchable content is the defining
value of a document-management service over plain file storage; it is why the module
exists.

**Independent Test**: Upload a text-bearing document, observe its processing status move
from pending through processing to completed, then search for a word contained only in
the document body and confirm the document is returned with a matching snippet — while a
document containing a different word is not.

**Acceptance Scenarios**:

1. **Given** a newly uploaded document, **When** it is created, **Then** its processing
   status is "pending" and a background extraction job is enqueued.
2. **Given** the extraction pipeline runs, **When** the text is extracted, **Then** the
   document's searchable content and extracted metadata are stored, its processing status
   becomes "completed", and each status transition is reflected in the UI in real time.
3. **Given** documents with extracted content, **When** the person searches for a term,
   **Then** only documents the person may read whose content, name, description, or tags
   match are returned, ranked, each with a snippet, filtered by any category/status/type
   filters and paginated.
4. **Given** a document that cannot be extracted, **When** the pipeline fails after its
   retries, **Then** the document's processing status becomes "failed" and the document
   remains downloadable and manageable.

---

### User Story 3 - Organize documents in a category hierarchy (Priority: P2)

A person groups documents into a tree of categories (folders), browses the tree, and
moves documents and whole subtrees between folders, with the service keeping each folder's
path and counts correct.

**Why this priority**: Organization is essential to managing more than a handful of
documents, but the service is usable (upload/search) before folders exist.

**Independent Test**: Create a parent category and a child category, upload a document
into the child, fetch the category tree and confirm the nesting, paths, and document
counts, then move the document to the parent and confirm its category path updates.

**Acceptance Scenarios**:

1. **Given** a signed-in person, **When** they create a category (optionally under a
   parent), **Then** it is stored with a materialized path, a depth, and initial counts,
   unique among its siblings by name within the tenant.
2. **Given** a category tree, **When** the person requests the tree, **Then** the nested
   categories are returned with their paths, depths, and document and subcategory counts.
3. **Given** a category with a document, **When** the person moves the document to
   another category, **Then** the document's category and materialized path are updated
   and both categories' document counts are corrected.
4. **Given** a category, **When** the person moves it under a different parent, **Then**
   the category and every descendant have their materialized paths and depths recomputed.
5. **Given** a non-empty category, **When** the person deletes it without a cascade flag,
   **Then** the delete is refused; with an explicit cascade it removes the subtree.

---

### User Story 4 - Fine-grained, inheritable sharing (Priority: P2)

A person shares a document or a whole folder with another person, a role, or the entire
tenant, choosing whether they may read, edit, or own it, optionally until an expiry date;
grants on a folder flow down to everything inside it.

**Why this priority**: Controlled sharing is what makes the store multi-user and safe;
without it every document is either private or fully open. It builds on storage and
folders.

**Independent Test**: As an owner, grant another subject viewer access to a folder,
confirm that subject can read a document inside the folder but not modify it, revoke the
grant, and confirm access is gone; add an expiring grant and confirm it lapses.

**Acceptance Scenarios**:

1. **Given** an owner of a document or category, **When** they grant a subject (user,
   role, or tenant) a relation (owner, editor, viewer, or sharer), **Then** the grant is
   stored and the subject immediately has the corresponding permissions.
2. **Given** a viewer grant on a category, **When** a subject reads a document inside that
   category or any descendant, **Then** access is allowed by inheritance even without a
   direct grant on the document.
3. **Given** any subject and any resource, **When** access is checked for an action
   (read, write, delete, share, download), **Then** the service answers from the
   subject's strongest effective relation across direct and inherited grants.
4. **Given** an expiring grant, **When** its expiry passes, **Then** the grant no longer
   confers access; **and** when a grant is revoked, access is withdrawn immediately.
5. **Given** a subject, **When** they list the resources accessible to them, **Then** only
   resources they may read are returned.

---

### User Story 5 - Statistics and backup (Priority: P3)

An administrator views how many documents exist by status, source, and type, how much
storage each tenant uses, and the processing backlog, and can export a tenant's documents,
folders, and permissions for backup or migration and re-import them elsewhere.

**Why this priority**: Operability and portability; valuable but not required to store,
find, or share a document.

**Independent Test**: View the statistics dashboard and confirm document counts by status
and type and total storage usage; export a tenant's metadata and permissions and re-import
them into an empty tenant, confirming categories, document metadata, and grants are
recreated.

**Acceptance Scenarios**:

1. **Given** documents and categories, **When** the person views statistics, **Then** they
   see document counts by status, source, and type, total and per-category storage usage,
   category counts, and the processing backlog, with a per-tenant breakdown for
   administrators.
2. **Given** a tenant's data, **When** it is exported, **Then** the export contains
   category hierarchy, document metadata, and permission tuples, versioned by schema, with
   document bytes referenced (not inlined) and included as links only when explicitly
   requested.
3. **Given** an export, **When** it is imported into another tenant, **Then** categories,
   document metadata, and permissions are recreated with duplicates skipped or overwritten
   per the chosen mode.

---

### Edge Cases

- **Extraction service unavailable**: if the text-extraction or conversion service cannot
  be reached, the document remains stored and downloadable, its processing status stays
  pending/failed after retries, and it can be reprocessed later.
- **Concurrent workers**: two service instances must never run the same extraction job
  twice; a job is claimed by exactly one worker.
- **Large or unsupported files**: an upload above the configured size limit, or of an
  unsupported type, is rejected with a clear reason; extraction of an unsupported type
  fails cleanly and leaves the document usable.
- **Duplicate content**: two documents with identical bytes are stored as distinct
  documents (deduplication is not assumed) but share the same checksum.
- **Deleting a non-empty folder**: refused without an explicit cascade; a cascade removes
  the subtree and the contained documents' bytes.
- **Permission inheritance vs. revocation**: revoking a grant on a parent must remove
  inherited access to descendants unless a descendant has its own grant; an expired grant
  must never confer access.
- **Extracted-text exposure**: extracted content and embedded metadata must never appear
  in logs or audit records and must be returned only through the explicit document-read /
  search paths to permitted callers.
- **Credential exposure**: the object-store credentials must never appear in API
  responses, logs, audit records, backups, or error messages.
- **Cross-tenant isolation**: a person in one tenant can never read, search, download, or
  share another tenant's documents, folders, or permissions, and search never returns
  another tenant's content.
- **Orphaned blobs**: a failed upload or interrupted delete must not leave the metadata
  and the stored bytes permanently inconsistent (bytes without metadata are reclaimable;
  metadata always references retrievable bytes once the document is active).

## Requirements *(mandatory)*

### Functional Requirements

**Documents**

- **FR-001**: People MUST be able to upload a file, creating a document whose bytes are
  stored in object storage and whose metadata (name, description, file name, size,
  content type, a SHA-256 checksum of the bytes, source, tags, owner, timestamps) is
  recorded; the document MUST start in processing status "pending".
- **FR-002**: People MUST be able to read a document's metadata, list documents with
  filters (category, status, content type, source, tags, created-by, time range,
  pagination), update a document's metadata (name, description, category, tags), soft-delete
  a document (hidden from normal listings) and hard-delete it (also removing its stored
  bytes).
- **FR-003**: People MUST be able to download a document's exact stored bytes and to
  obtain a short-lived, expiring download link for it.
- **FR-004**: People MUST be able to move a document to another category, updating its
  materialized category path, and to delete many documents at once, receiving a per-document
  result.
- **FR-005**: A document MUST carry a status (active, archived, deleted) and a processing
  status (pending, processing, completed, failed); extracted content and extracted metadata
  MUST NOT be returned in listings or logs and MUST be returned only through the explicit
  read and search paths to permitted callers.

**Categories (hierarchy)**

- **FR-006**: People MUST be able to create, read, update, and delete categories, each with
  an optional parent, a name unique among its siblings within the tenant, a description, a
  materialized path, a depth, and a sort order.
- **FR-007**: The system MUST maintain each category's document count and subcategory count
  and MUST recompute the materialized paths and depths of a category and all its descendants
  when it is moved.
- **FR-008**: People MUST be able to retrieve a category as a nested tree for the tenant and
  to move a category under a different parent; deleting a non-empty category MUST be refused
  unless an explicit cascade is requested.

**Permissions (fine-grained, inheritable)**

- **FR-009**: People MUST be able to grant and revoke access on a document or category to a
  subject (user, role, or tenant) with a relation of owner (read, write, delete, share),
  editor (read, write, delete), viewer (read, download), or sharer (read, share); the creator
  of a resource MUST become its owner.
- **FR-010**: The system MUST support permission expiry, tenant-wide grants, and inheritance:
  a grant on a category MUST confer the corresponding access to every document and category
  beneath it, and an effective-permission check MUST combine direct and inherited grants into
  the subject's strongest relation.
- **FR-011**: People MUST be able to check whether a subject has a given permission (read,
  write, delete, share, download) on a resource, list the permissions on a resource or for a
  subject, list the resources accessible to a subject, and get a subject's effective
  permissions on a resource with the contributing grants.
- **FR-012**: Every document and category read, write, delete, download, and share operation
  MUST be authorized against the effective permissions before it acts, and no operation MUST
  reveal the existence of a resource the caller may not read.

**Content extraction & search**

- **FR-013**: On upload the system MUST enqueue a background extraction job; a pool of
  workers MUST claim and run jobs such that a job is executed by exactly one worker even
  across multiple service instances.
- **FR-014**: The extraction pipeline MUST mark a document "processing", extract its text and
  embedded metadata via the external extraction service (and optionally convert its format for
  preview), store the searchable content, update the full-text index, and mark the document
  "completed"; on failure it MUST retry with exponential backoff up to a configured maximum
  and then mark the document "failed", leaving the document downloadable.
- **FR-015**: The system MUST publish each processing-status transition (pending → processing
  → completed/failed) to the platform event bus so the browser UI reflects it in real time.
- **FR-016**: People MUST be able to run a tenant-scoped full-text search over documents'
  extracted content, name, description, and tags, returning ranked matches with snippets,
  restricted to documents the caller may read, filtered by optional category/status/type
  filters, and paginated.

**Statistics, backup, audit**

- **FR-017**: The system MUST provide per-tenant and system-wide statistics: document counts
  by status, by source, and by content type; total and per-category storage usage; category
  counts; the processing backlog (pending/processing/failed counts); recent activity; and a
  per-tenant breakdown for administrators.
- **FR-018**: The system MUST support per-tenant export and import of category hierarchy,
  document metadata, and permission tuples, versioned by schema, with duplicate handling (skip
  or overwrite); document bytes MUST be referenced by key and included as links only when
  explicitly requested, never inlined.
- **FR-019**: The system MUST write every state-changing operation to an append-only,
  tamper-evident audit trail carrying the actor identity, tenant, outcome, and reason, with
  extracted content and credentials redacted from audit detail.

**Access, tenancy, platform**

- **FR-020**: Access MUST be governed by fine-grained, tenant-scoped permissions and by
  route-level API permissions (documents read/write/delete, categories read/manage,
  permissions manage, search read, statistics read, backup manage); service-to-service callers
  MUST use verified service identities and browser callers a valid platform token.
- **FR-021**: All data MUST be isolated per tenant such that no operation — including search —
  can read or affect another tenant's documents, folders, or permissions.
- **FR-022**: The system MUST register itself with the application gateway (its routes, API
  permissions, and UI abilities), MUST expose module-to-module operations for other services,
  and MUST expose a browser UI, composed by the platform shell, offering a documents list with
  upload and a detail/preview drawer, a category-tree navigator, a permissions editor, a
  full-text search view, and a statistics dashboard.
- **FR-023**: The browser UI MUST reflect a document's processing progress and completion or
  failure in real time without a manual refresh.

### Security Requirements *(mandatory — Constitution: Development Workflow)*

- **Trust boundaries crossed**:
  - Browser → gateway → paperless (end-user requests carrying a platform token).
  - paperless ↔ auth (service-to-service, mutually authenticated service identities).
  - paperless → object storage (outbound, using sealed store credentials).
  - paperless → external extraction/conversion services (outbound, document bytes leave the
    service transiently for text extraction).
  - paperless → platform event bus (outbound processing events).
- **Data classification**: object-store credentials (secret); document bytes and extracted
  content (confidential, tenant-owned — may contain personal or sensitive information);
  document/category/permission metadata (internal, tenant-scoped); audit records (internal,
  integrity-protected).
- **Authentication/Authorization**: service-to-service calls MUST use verified service
  identities; browser calls MUST carry a valid platform token verified by the platform; every
  operation MUST pass a tenant-scoped, fine-grained permission check enforced before the handler
  runs; the existence of unreadable resources MUST be masked.
- **Threat scenarios**: credential exfiltration via API/log/backup/error leakage; a tenant
  reaching another tenant's documents or search results; a subject reading a document through a
  stale, expired, or revoked grant; an oversized or malicious upload exhausting storage or the
  extraction workers; a hostile extraction response; extracted sensitive text leaking into logs
  or audit; a presigned link outliving its intended lifetime or scope.
- **SR-001**: The system MUST reject any request that lacks a valid service identity or
  platform token, and MUST enforce a tenant-scoped, fine-grained permission check on every
  operation before acting, masking the existence of resources the caller may not read.
- **SR-002**: The system MUST seal object-store credentials with envelope encryption at rest
  and MUST never emit credentials, extracted content, or embedded metadata in logs, audit
  records, error messages, or backups (extracted content is returned only through the explicit
  read/search paths to permitted callers).
- **SR-003**: The system MUST scope every read, write, and search to the caller's tenant so no
  operation can observe or affect another tenant's data, and full-text search MUST never return
  another tenant's content.
- **SR-004**: The system MUST validate all inputs — upload size and content type, search
  queries, category and document identifiers, permission tuples, and pagination — against an
  explicit schema and bound their size, and MUST treat extraction-service responses as untrusted
  input.
- **SR-005**: The system MUST bound each extraction attempt with a timeout and each job with a
  maximum retry count, and MUST cap the size of uploads and of extracted content so a large or
  hostile document cannot exhaust storage or workers.
- **SR-006**: Presigned download links MUST be short-lived, single-resource, and grant no more
  than read access to the one document's bytes for the requesting tenant.

### Key Entities *(include if feature involves data)*

- **Document**: a tenant-scoped record of a stored file — id, category (id and materialized
  path), name, description, object-store key, file name, size, content type, SHA-256 checksum,
  status, source, tags, owner and editor, timestamps, extracted content and extracted metadata
  (redacted), and processing status. Relates to a Category, to Permission Tuples, and to a
  Processing Job; its bytes live as a Blob in object storage.
- **Category**: a tenant-scoped folder — id, optional parent, name, materialized path, depth,
  sort order, document count, subcategory count, description, owner, timestamps. Forms a tree;
  grants on it inherit to descendants.
- **Permission Tuple**: a grant binding a subject (user, role, or tenant) to a resource
  (document or category) with a relation (owner, editor, viewer, sharer), optionally expiring;
  the unit of the fine-grained, inheritable access model.
- **Processing Job**: a unit of extraction work for one document — status, retry count against
  a maximum, timestamps, and result; claimed by exactly one worker; drives the pending →
  processing → completed/failed lifecycle.
- **Blob**: the document's bytes in S3-compatible object storage, keyed by the document's
  object-store key, checksummed; the only place bytes are stored, never in the database.
- **Audit Record**: an append-only, tamper-evident entry for every state-changing operation,
  with actor, tenant, operation, outcome, and reason; extracted content and credentials are
  redacted.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A person can upload a document and download the identical bytes back (directly or
  via a short-lived link) in under 1 minute, with the size, type, and checksum recorded.
- **SC-002**: After upload, a text-bearing document becomes findable by a word in its body,
  automatically and with no human action, in at least 99% of successfully extracted documents,
  and its processing status is visible as it progresses.
- **SC-003**: A full-text search returns only documents the searcher is permitted to read and
  never another tenant's content — verified by permission and cross-tenant tests.
- **SC-004**: A grant on a folder gives the grantee access to every document within it, a
  revocation or expiry removes that access immediately, and an effective-permission check
  reflects the subject's strongest relation for 100% of checked cases.
- **SC-005**: No object-store credential and no extracted document content ever appears in any
  log line, audit record, error message, or backup — verified by inspection and automated tests.
- **SC-006**: No person can read, search, download, move, or share another tenant's documents,
  folders, or permissions — verified by cross-tenant tests.
- **SC-007**: The same extraction job is never executed twice when multiple service instances
  run concurrently — verified under a concurrency test.
- **SC-008**: A person can determine why a document failed to process from its status without
  access to service internals, and the document remains downloadable and manageable.
- **SC-009**: The browser UI reflects a document's processing progress and its completion or
  failure in real time without a manual refresh.

## Assumptions

- The platform provides S3-compatible object storage (MinIO/RustFS-compatible) reachable by the
  service over sealed credentials; document bytes are stored there and never in the database.
- External text-extraction and format-conversion services (Apache Tika and Gotenberg, or
  equivalents) are reachable over the network for content extraction; when they are unavailable,
  documents remain stored and are reprocessed later.
- The platform database is TimescaleDB with per-tenant row-level security for all metadata,
  extracted content, and permission tuples; full-text search is served from the database's
  full-text index over extracted content.
- The auth service is the source of identity, tenants, and roles; the application gateway
  forwards browser traffic with a verifiable platform token and enforces the module's route
  allow-list; the shared platform event bus carries processing-status events consumed by the
  gateway SSE hub.
- The framework, mutual TLS, service identity, audit, and observability are provided by the
  Freya framework, as for the other platform modules (lcm, warden, notification, deployer).

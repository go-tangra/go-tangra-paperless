# Tasks: Paperless Service

**Input**: Design documents from `/specs/009-paperless-service/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: MANDATORY (Constitution IV). Every user story lists test tasks before its
implementation tasks; features touching auth, transport, parsing, crypto, or secrets also
include negative-security and fuzz tests.

**Organization**: grouped by user story so each is independently implementable and testable.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: parallelizable (different files, no incomplete dependency)
- **[Story]**: US1..US5 (setup/foundational/polish carry no story label)

## Path Conventions

New Go module `services/paperless/` (see plan.md Project Structure); UI under
`services/paperless/ui/`; specs contracts under `specs/009-paperless-service/`.

---

## Phase 1: Setup (Shared Infrastructure)

- [X] T001 Create the module skeleton `services/paperless/` with the package tree from plan.md (cmd/paperlesssvc, api/openapi, api/proto/paperless/v1, internal/{app,config,store,repo,memstore,sealed,authz,blob,documents,categories,permissions,search,jobs,extract,events,stats,backup,audit,httpapi,grpcapi}, pkg/{paperlessmanifest,paperlessclient}, ui, deploy).
- [X] T002 Add `services/paperless/go.mod` (module `github.com/go-freya/freya/services/paperless`, Go 1.26) with replaces for `../..`, `../auth`, `../gateway`; add to the workspace/build like other services.
- [X] T003 [P] Add `services/paperless/buf.yaml` + `buf.gen.yaml` and `api/proto/paperless/v1/*.proto` stubs; wire proto codegen into the Makefile (mirror `services/deployer`).
- [X] T004 [P] Add `services/paperless/Dockerfile` (build UI remote, embed with `-tags ui`, build `paperlesssvc`) and `services/paperless/Makefile` mirroring the deployer.
- [X] T005 [P] Scaffold `services/paperless/ui/` (Vue 3 + Vite + Vuetify Module-Federation remote named `paperless`) from `services/deployer/ui` (package.json, vite.config, main.ts, api/client, remote/{routes,nav}, stores, views).

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: platform wiring every story depends on. No user story starts until this is done.

- [X] T006 Typed, validated config in `internal/config/config.go` (server/admin addrs, db, valkey user, kek, object-store{endpoint,bucket,region,access_key,secret_key sealed}, extract{tika_url,gotenberg_url,timeout}, jobs{worker_count,max_retries,retry_delay,backoff_multiplier,job_timeout,cleanup_days}, uploads{max_size_bytes,allowed_mime}, events{enabled}) + `config_test.go` asserting secure defaults and startup rejection of missing kek/db/identity/object-store.
- [X] T007 Store migrations in `internal/store/migrations/` — `0001_schema.sql` (paperless_documents incl. search_tsv + tags jsonb, paperless_categories with path/depth/counts, paperless_permissions, paperless_jobs; indexes + GIN per data-model.md), `0002_hypertables.sql` (paperless_audit_events), `0003_rls.sql` (per-tenant RLS on every paperless_* table + app-role grants), `0004_fts.sql` (tsvector trigger/GIN, materialized-path index). Each with `-- +goose Up/Down`.
- [X] T008 Store models + repos in `internal/store/{models.go,repos.go}` and the store interface in `internal/repo/repo.go` (documents, categories incl. subtree-by-path + move, permissions incl. inherited lookup by path prefix, jobs incl. ClaimDueJobs `FOR UPDATE SKIP LOCKED`, audit, stats aggregations, tenant ids).
- [X] T009 [P] In-memory `internal/memstore/memstore.go` implementing `repo.Store` (with error injection) for tests.
- [X] T010 [P] `internal/sealed/` envelope-seal/open + redaction helpers (reuse the platform `sealed` pattern) + `sealed_test.go` (100% — round-trip, AD binding, redaction never leaks).
- [X] T011 [P] `internal/authz/` Zanzibar tuples for `document|category` × `owner|editor|viewer|sharer`, tenant-wide grants, expiry, category→document inheritance by materialized path, `Grant/Revoke/Check/Effective/ListAccessible/GetEffective` + `authz_test.go` (100%, incl. inheritance, expiry, cross-tenant denial).
- [X] T012 [P] `internal/audit/` writer adapter to the framework append-only audit (event types per data-model.md) + redaction of credential + extracted-content fields.
- [X] T013 Blob store `internal/blob/` — S3/MinIO client (`Put` streaming+SHA-256, `Get`, `PresignGet`, `Delete`) over sealed credentials + a fake for tests + `blob_test.go` (never logs bytes/creds; presign is short-lived).
- [X] T014 [P] Extractor clients `internal/extract/` — Tika text/metadata + optional Gotenberg convert over `net/http`, size-bounded + timeout, no-op when endpoints unset + `extract_test.go` against a fake HTTP server (untrusted-response handling).
- [X] T015 Event-bus plumbing in `internal/events/` — a publisher for `document.processing|completed|failed` on `platform:events:<tenant>` (reuse the `stream` package pattern); wire the Valkey client in config.
- [X] T016 App build/wire/run in `internal/app/{app.go}` — `freya.New`, identity, store/KEK, authz, blob client, extractor, gateway registration via `pkg/paperlessmanifest`, HTTP mux (OpenAPI-validated) + gRPC servers, worker pool start/stop, admin health/readiness. Refuses to start insecure (Constitution I).
- [X] T017 `cmd/paperlesssvc/main.go` + `bootstrap` subcommand (config load, migrate, run) mirroring `deployersvc`.
- [X] T018 [P] `pkg/paperlessmanifest/manifest.go` — derive gateway routes/permissions from `api/openapi/paperless.yaml` (Routes with Public flag) + Grants/Abilities/Nav, like `pkg/deployermanifest`; `SeedPermissions` grants the API permissions to the built-in roles.
- [X] T019 `api/openapi/paperless.yaml` skeleton (info, components, CSRF/id/cursor params, error shapes) so the OpenAPI-validated mux and manifest have a document to load; contract test `tests/contract/openapi_test.go` (parses, every mounted route declared).
- [X] T020 Provider-free foundational check: `internal/repo/repodb` implementing the store interface over TimescaleDB + integration test (testcontainers) for schema/RLS/materialized-path/tsvector.

**Checkpoint**: framework, store+RLS+FTS, sealing, authz+inheritance, blob, extractor, event plumbing, and app wiring exist; the service starts, registers with the gateway, and serves an empty API.

---

## Phase 3: User Story 1 — Upload, store, and download a document (Priority: P1) 🎯 MVP

**Goal**: upload a file (bytes to object storage, metadata recorded, owner set), list/read
it, download the bytes and a short-lived link.

**Independent test**: quickstart Scenario 1.

### Tests (write first, must fail)
- [X] T021 [P] [US1] Contract test `tests/contract/documents_test.go` — upload/get/list/update/remove/download/download-url shapes; content_text never appears in list responses; owner recorded.
- [X] T022 [P] [US1] Unit test `internal/documents/documents_test.go` — Create stores bytes via a fake blob, computes checksum, records size/mime/owner, status active + processing pending; List/Update/soft+hard Delete; hard delete removes the object.
- [X] T023 [P] [US1] Unit test `internal/blob` round-trip against a fake/MinIO (put→get bytes match; presign returns a short-lived URL; delete removes).
- [X] T024 [P] [US1] Security test — object-store credentials + extracted content never in responses/logs/audit on the create/read path (SR-002, SC-005).

### Implementation
- [X] T025 [US1] `internal/documents/documents.go` — Document service: Create (stream to blob, checksum, metadata, owner-grant, enqueue extraction job), Get, List (filters + pagination, redacts content), Update (metadata), Delete (soft + hard removes blob), Download, GetDownloadUrl (presign), BatchDelete.
- [X] T026 [US1] HTTP handlers `internal/httpapi/documents.go` (multipart upload → 201, list/get/update/remove/download/download-url/batch-delete) + register routes; add to `api/openapi/paperless.yaml` with `x-freya-permission` + upload body limits.
- [X] T027 [P] [US1] gRPC `internal/grpcapi/` PaperlessDocumentService (Create/Get/List/Update/Delete/Download/GetDownloadUrl/BatchDelete) + register.
- [X] T028 [P] [US1] UI: `ui/src/views/documents/` list + upload + detail drawer (metadata, tags, download) + `stores/documents.ts` + api client.

**Checkpoint**: US1 independently demoable — upload/download MVP.

---

## Phase 4: User Story 2 — Automatic text extraction and full-text search (Priority: P1)

**Goal**: background extraction (Tika/Gotenberg) makes documents searchable; live status;
permission-filtered full-text search.

**Independent test**: quickstart Scenario 2.

### Tests (write first, must fail)
- [X] T029 [P] [US2] Unit test `internal/jobs/worker_test.go` — a worker claims a pending extraction job exactly once (concurrency), runs it, marks completed; a failing extractor retries then marks failed; document stays downloadable.
- [X] T030 [P] [US2] Unit test `internal/search/search_test.go` — full-text over content/name/description/tags returns ranked matches with snippets; non-matching excluded; results filtered to readable docs; tenant-scoped.
- [X] T031 [P] [US2] Fuzz test `internal/search/query_fuzz_test.go` + `internal/extract` untrusted-response test — malformed/oversized search queries and extraction responses never panic (SR-004).
- [X] T032 [P] [US2] Contract test `tests/contract/search_test.go` — search request/response shape; cross-tenant search returns nothing (SR-003).

### Implementation
- [X] T033 [US2] `internal/jobs/jobs.go` + `internal/jobs/scheduler.go` — extraction-job store ops + worker pool (poll pending/retryable, `ClaimDueJobs` single-winner lease, per-job timeout, exponential backoff, cleanup worker).
- [X] T034 [US2] `internal/jobs/process.go` — process one document: mark processing → Extract (Tika) text+metadata → optional Convert (Gotenberg) → store content_text/extracted_metadata + update tsvector → mark completed; on failure retry then failed; publish `document.processing|completed|failed`.
- [X] T035 [US2] `internal/search/search.go` — tenant-scoped `websearch_to_tsquery` + `ts_rank` + `ts_headline` snippets, filtered by the caller's accessible-document set (from authz), paginated.
- [X] T036 [US2] HTTP handler `internal/httpapi/search.go` (POST /documents/search) + OpenAPI entry; gRPC PaperlessDocumentService.Search; SSE relay of processing events in `internal/httpapi/stream.go`.
- [X] T037 [P] [US2] UI: `ui/src/views/search/` full-text search view with snippets + live processing-status badge on documents via `ctx.live` (`stores/live.ts`).

**Checkpoint**: US1 + US2 — documents are stored, extracted, and searchable with live status.

---

## Phase 5: User Story 3 — Organize documents in a category hierarchy (Priority: P2)

**Goal**: category tree with materialized paths; create/move/tree; document move updates paths/counts.

**Independent test**: quickstart Scenario 3.

### Tests (write first, must fail)
- [X] T038 [P] [US3] Contract test `tests/contract/categories_test.go` — categories CRUD, tree, move; non-empty delete refused without cascade.
- [X] T039 [P] [US3] Unit test `internal/categories/categories_test.go` — create sets path/depth/counts, unique per (tenant,parent); move recomputes subtree paths/depths and rejects a cycle; delete refuses non-empty without cascade; counts stay correct on document move.

### Implementation
- [X] T040 [US3] `internal/categories/categories.go` — Category service: CRUD, GetTree, Move (recompute subtree materialized paths + depths in one tx, cycle-guard), maintain document_count/subcategory_count.
- [X] T041 [US3] Wire document Move/Create into category counts + category_path (in `internal/documents`), and subtree listing by path.
- [X] T042 [US3] HTTP handlers `internal/httpapi/categories.go` (+ tree) + OpenAPI entries; gRPC PaperlessCategoryService (Create/Get/List/Update/Delete/Move/GetTree) + register.
- [X] T043 [P] [US3] UI: `ui/src/views/categories/` category-tree navigator + move + `stores/categories.ts`; documents list filters by selected category.

**Checkpoint**: US1–US3 — documents organized in a browsable, movable tree.

---

## Phase 6: User Story 4 — Fine-grained, inheritable sharing (Priority: P2)

**Goal**: grant/revoke owner/editor/viewer/sharer to user/role/tenant with expiry;
category grants inherit to descendants; effective-permission checks gate every operation.

**Independent test**: quickstart Scenario 4.

### Tests (write first, must fail)
- [X] T044 [P] [US4] Contract test `tests/contract/permissions_test.go` — grant/revoke/list/check/accessible/effective shapes; existence of unreadable resources masked.
- [X] T045 [P] [US4] Unit test `internal/permissions/permissions_test.go` — grant sets a tuple; creator becomes owner; check returns the action set per relation; category grant inherits to descendant document; tenant-wide grant applies to all; expiry lapses; revoke withdraws immediately.
- [X] T046 [P] [US4] Security test `internal/permissions/inheritance_security_test.go` — a revoked/expired grant never confers access; a descendant with its own grant survives a parent revoke; cross-tenant tuples never match (SR-001/003).

### Implementation
- [X] T047 [US4] `internal/permissions/permissions.go` — Permission service: GrantAccess, RevokeAccess, ListPermissions (by resource or subject), CheckAccess, ListAccessibleResources, GetEffectivePermissions (strongest relation across direct + inherited + tenant-wide, expiry-aware).
- [X] T048 [US4] Enforce effective permissions in every documents/categories operation (read/write/delete/download/share) before it acts + mask non-readable existence (wire authz into US1/US3 handlers).
- [X] T049 [US4] HTTP handlers `internal/httpapi/permissions.go` + OpenAPI entries; gRPC PaperlessPermissionService (Grant/Revoke/List/Check/ListAccessible/GetEffective) + register.
- [X] T050 [P] [US4] UI: `ui/src/views/permissions/` permissions editor (grant/revoke owner/editor/viewer/sharer to user/role/tenant with expiry) on the document/category drawer + `stores/permissions.ts`.

**Checkpoint**: US1–US4 — documents are shareable with inheritable, expiring permissions.

---

## Phase 7: User Story 5 — Statistics and backup (Priority: P3)

**Goal**: per-tenant + system statistics and per-tenant export/import.

**Independent test**: quickstart Scenario 5.

### Tests (write first, must fail)
- [X] T051 [P] [US5] Contract test `tests/contract/stats_backup_test.go` — /statistics + /statistics/tenant shapes; export omits credentials + inlined bytes; import skip/overwrite.
- [X] T052 [P] [US5] Unit test `internal/stats/stats_test.go` — documents by status/source/mime, storage usage, category counts, processing backlog, per-tenant breakdown for admin.
- [X] T053 [P] [US5] Unit test `internal/backup/backup_test.go` — export/import round-trip of categories + document metadata + permissions; ids preserved; duplicate skip vs overwrite; schema version.

### Implementation
- [X] T054 [US5] `internal/stats/stats.go` + HTTP `internal/httpapi/statistics.go` + gRPC PaperlessStatisticsService + OpenAPI entries.
- [X] T055 [US5] `internal/backup/backup.go` + HTTP `internal/httpapi/backup.go` (export include-links flag, import mode) + OpenAPI entries; bytes referenced by object_key, links only on request.
- [X] T056 [P] [US5] UI: `ui/src/views/dashboard/` statistics widgets (documents by status/mime, storage usage, processing backlog) + export/import controls.

**Checkpoint**: US1–US5 — full document-management feature set.

---

## Phase 8: Platform integration & polish

- [X] T057 Stack wiring in `deploy/stack/`: add a `paperless` DB + `paperless_app` role to `init-db.sql`; a `paperless` Valkey user; a MinIO/RustFS object-store service (+ bucket init) and optional Tika + Gotenberg sidecars; a `paperless` compose service (enrolls, mounts policy + kek), a `paperless-token` mint init, and `configs/paperless.yaml`.
- [X] T058 Gateway allow-list: add `spiffe://example.org/svc/paperless=/api/paperless;paperless` to the gateway-bootstrap; confirm the service registers (`registered:true`) and the **Paperless** menu renders.
- [X] T059 `deploy/policy.yaml` (service-to-service policy: gateway-forwards; module callers of PaperlessDocument/Category/Permission/Statistics) + `deploy/kek.dev` fixture.
- [X] T060 [P] `services/paperless/deploy/README.md` (setup, object store, extraction, security notes) + update `deploy/stack/README.md` to list paperless.
- [X] T061 [P] `pkg/paperlessclient/` optional Go client for module-to-module use + doc.
- [X] T062 Coverage gate: `go -C services/paperless test ./...` ≥80% overall (with the integration harness), 100% on sealed/authz; `govulncheck` clean.
- [X] T063 Stack smoke test: full freya-stack brought up (all 14 services healthy, all bootstraps exited 0); paperless **registered:true** with a renewing gateway lease; gateway edge routes `GET /api/paperless/v1/documents`→401 (auth-required, i.e. registered+allow-listed+proxied, not 404) and `/console/signin`→200; bucket self-provisioned in RustFS. Quickstart Scenario 1 (browser upload/download) is the one manual step — it needs the operator sign-in (password+TOTP), which the assistant is prohibited from performing; accept-invite URL surfaced for the operator.
- [X] T064 [P] Fuzz + negative tests for the upload path (oversized/disallowed mime), the search-query parser, and the extraction-response parser across the edge (Constitution IV).

---

## Dependencies & Execution Order

- Phase 1 (Setup) → Phase 2 (Foundational) block everything.
- US1 (P1) is the MVP; US2 (P1) depends on US1 (documents exist to extract/search).
- US3 (P2) and US4 (P2) build on US1 (documents) and each other loosely (permissions gate
  documents/categories); US4's enforcement (T048) wires into US1/US3 handlers.
- US5 (P3) depends on US1–US4 data being present.
- Phase 8 integrates and hardens after the stories.

## Parallel Opportunities

- Setup T003/T004/T005 in parallel.
- Foundational T009/T010/T011/T012/T014/T018 in parallel (distinct files).
- Within each story, the `[P]` test tasks run together, then implementation; the UI `[P]`
  task runs alongside the backend once the service exists.

## Implementation Strategy

MVP first: Setup + Foundational + US1 (upload/download) → demoable. Then US2 (extraction +
search) for the core value, US3 (categories) and US4 (permissions) for organization and
sharing, US5 (stats/backup) for operability, then Phase 8 to integrate into the stack and
meet the coverage gate.

## Summary

- **Total tasks**: 64 across 8 phases.
- **Per story**: US1 = 8 (T021–T028), US2 = 9 (T029–T037), US3 = 6 (T038–T043),
  US4 = 7 (T044–T050), US5 = 6 (T051–T056). Setup = 5, Foundational = 15, Integration/
  polish = 8.
- **MVP scope**: Setup + Foundational + US1.
- Every user story has test tasks (contract/unit/security) before implementation, per
  Constitution IV; auth/parsing/secrets paths carry negative + fuzz tests.

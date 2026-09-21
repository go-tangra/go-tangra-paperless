# Implementation Plan: Paperless Service

**Branch**: `009-paperless-service` | **Date**: 2026-09-20 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/009-paperless-service/spec.md`

## Summary

Build a new Freya platform module, **`services/paperless`**, that stores documents
(bytes in S3-compatible object storage, metadata and extracted text in TimescaleDB),
organizes them into a category tree, extracts their text in the background so they are
full-text searchable, and governs access with Zanzibar-style, inheritable per-resource
permissions. It reuses the exact platform conventions proven in `services/lcm`,
`services/warden`, `services/notification`, and `services/deployer`: the Freya framework
(SPIFFE mTLS, identity, audit, observability), gateway registration + Module-Federation
UI, an OpenAPI-declared HTTP surface proxied by the gateway, gRPC for module-to-module
calls, envelope-sealed secrets, and TimescaleDB with per-tenant RLS. The distinctive new
machinery is: an **object-store client** (S3/MinIO) with SHA-256 checksums and presigned
URLs over sealed credentials; a **category hierarchy** with materialized paths; a
**Zanzibar permission engine** with category→document inheritance and expiry; an **async
extraction pipeline** (a distributed worker pool with SQL lease claim like the
deployer/lcm schedulers) that calls **Tika** (text) and optionally **Gotenberg**
(conversion) and maintains a Postgres full-text index; a **full-text search** filtered by
effective permissions; and processing-status events on the shared
`platform:events:<tenant>` Valkey stream (the bus built for lcm) for live UI updates.

## Technical Context

**Language/Version**: Go 1.26 (matches every other Freya service).

**Primary Dependencies**: the Freya framework (`github.com/go-freya/freya`) for
transport/mTLS/identity/audit/config/observability; `services/auth/pkg/authclient`
(platform-token verification); `services/gateway/pkg/gatewayclient` (route/permission
registration + renew lease); the shared `stream` package pattern (Valkey Streams) and
`github.com/valkey-io/valkey-go` (event bus + rate/lease); an S3-compatible object-store
client (the MinIO Go SDK `github.com/minio/minio-go/v7`, or a thin SigV4 `net/http`
client — justified in research.md); `net/http` clients for Tika and Gotenberg. The Go
standard library + `golang.org/x/crypto` only for crypto; the platform `sealed`
envelope-encryption package for store credentials. Each new dependency is justified in
research.md (Constitution VI). UI: Vue 3 + Vite + Vuetify (Materio) Module-Federation
remote, mirroring `services/deployer/ui`.

**Storage**: TimescaleDB (Postgres) with per-tenant row-level security for all metadata,
extracted content, permission tuples, and processing jobs; the same `internal/store` +
`internal/repo` + goose-migration pattern as the deployer/lcm; a `memstore` fake for
tests. Full-text search over a Postgres `tsvector` index of extracted content. Document
**bytes** live only in S3-compatible object storage, keyed by the document's object key;
store credentials sealed via the `sealed` package + KEK.

**Testing**: Go `testing` with a `testrt` test runtime + `memstore` fake; contract tests
against the OpenAPI document (every declared route mounted) and the gRPC protos;
integration tests (testcontainers: TimescaleDB + MinIO + Valkey) for the store/RLS, the
object-store client, and the extraction pipeline; negative-security and fuzz tests for
the search-query and extraction-response parsers, the permission-inheritance evaluator,
and the credential/extracted-content redaction path; ≥80% coverage overall, 100% on the
sealing/authz packages.

**Target Platform**: Linux server container in the `deploy/stack` compose, behind the
gateway, with a MinIO/RustFS object store and (optionally) Tika + Gotenberg sidecars; UI
composed by the gateway shell.

**Project Type**: Web service (Go backend + gRPC + OpenAPI HTTP) with a Module-Federation
frontend — the established Freya module shape.

**Performance Goals**: upload + metadata create returns in well under a second for
typical files (bytes streamed to the object store); a text-bearing document becomes
searchable within seconds of upload once the pipeline runs; a tenant-scoped full-text
search returns ranked results in under ~1s for typical corpora; the worker pool sustains
the configured concurrency without double-executing an extraction job across instances.

**Constraints**: document bytes never stored in the database (only in the object store);
store credentials sealed at rest and never emitted; extracted content never in logs or
audit and returned only through the explicit read/search paths; every request
authenticated + fine-grained tenant-authz before the handler; per-extraction timeout,
bounded upload size, and bounded retries; presigned links short-lived and single-resource;
RLS enforces tenant isolation (a scoped system subject is used only for the worker/event
paths and still carries a tenant).

**Scale/Scope**: thousands of documents and a deep category tree per tenant; a Zanzibar
permission model with inheritance; ~5 UI screens (documents+upload, category tree,
permissions editor, search, statistics); a single new service module plus one gateway
allow-list entry, one compose service, and the object-store (+ optional Tika/Gotenberg)
dependencies.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Evaluated against `.specify/memory/constitution.md`.

- [x] **I. Secure by Default**: object-store credentials sealed by default and never
      returned; the service refuses to start without a KEK/DB/identity/object-store
      config; presigned links are short-lived read-only. PASS.
- [x] **II. Zero Trust**: every HTTP route carries an `x-freya-permission` (or explicit
      `x-freya-public`) enforced by the gateway + module authz middleware before the
      handler, and every operation additionally passes a fine-grained per-resource
      permission check; gRPC is SPIFFE-mTLS peer-verified; the worker/event paths act as a
      scoped system subject, never an unauthenticated bypass. PASS.
- [x] **III. Boundary Validation**: OpenAPI + proto schemas validate every inbound field;
      upload size/content-type, search queries, permission tuples, and pagination are
      validated and bounded; extraction-service responses are treated as untrusted; the
      framework edge enforces body/rate limits. PASS.
- [x] **IV. Test-First (NON-NEGOTIABLE)**: tasks.md orders contract/unit/security tests
      before implementation; negative + fuzz tests planned for the search/extraction
      parsers and the permission-inheritance evaluator, plus credential/extracted-content
      redaction tests; coverage targets achievable. PASS.
- [x] **V. Observability**: every state change → append-only audit with actor/tenant/
      outcome/reason; redaction covers credentials and extracted content; correlation IDs
      propagate; health/readiness on the framework's separate admin port. PASS.
- [x] **VI. Supply Chain**: stdlib preferred; crypto only from stdlib/`x/crypto` (sealing
      reuses the platform `sealed` package); the object-store SDK and Tika/Gotenberg
      clients justified in research.md; `go.sum` pinned, `govulncheck` in CI. PASS.
- [x] **VII. Simplicity**: typed, validated config at startup (worker count, retry policy,
      timeouts, upload/content caps, object-store + extraction endpoints); no
      reflection/global mutable state. PASS.
- [x] **Threat Model**: the feature touches auth, transport, parsing (search/extraction),
      and secrets (store credentials) and confidential content, so a STRIDE threat model
      is included in research.md. PASS.

No violations — Complexity Tracking is empty.

## Project Structure

### Documentation (this feature)

```text
specs/009-paperless-service/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (OpenAPI + proto + object-store/extraction ifaces + events)
└── tasks.md             # Phase 2 output (/speckit-tasks)
```

### Source Code (repository root)

New module `services/paperless/`, modelled on `services/deployer/`:

```text
services/paperless/
├── cmd/paperlesssvc/           # main + bootstrap subcommand (migrations)
├── api/
│   ├── openapi/paperless.yaml  # HTTP surface (x-freya-permission per route); gateway proxies /api/paperless
│   └── proto/paperless/v1/     # gRPC (Document/Category/Permission/Statistics — module-to-module)
├── internal/
│   ├── app/                    # Build/Wire/Run (freya.New, gateway register, worker + event start)
│   ├── config/                 # typed, validated config (jobs, events, object-store, tika/gotenberg, valkey, db, kek)
│   ├── store/                  # TimescaleDB repos + goose migrations (RLS, tsvector, materialized paths)
│   ├── repo/                   # store interface (memstore fake in tests)
│   ├── memstore/               # in-memory store for tests
│   ├── sealed/                 # envelope-sealing helpers (reuse platform pattern)
│   ├── authz/                  # Zanzibar tuples: document|category × owner|editor|viewer|sharer, inheritance, expiry
│   ├── blob/                   # S3/MinIO object-store client (put/get/presign/delete, SHA-256) over sealed creds
│   ├── documents/              # document service (Create/Get/List/Update/Delete/Move/Download/Url/Search/BatchDelete)
│   ├── categories/             # category service (CRUD, Move, GetTree; materialized-path maintenance)
│   ├── permissions/            # permission service (Grant/Revoke/List/Check/ListAccessible/GetEffective)
│   ├── search/                 # full-text search (tsvector) filtered by effective permissions
│   ├── jobs/                   # extraction-job service + worker pool scheduler (lease claim, retry)
│   ├── extract/                # Tika + Gotenberg clients (text/metadata extraction, conversion)
│   ├── events/                 # processing-status publisher on the platform event bus
│   ├── stats/                  # statistics aggregation
│   ├── backup/                 # export/import (metadata + categories + permissions)
│   ├── audit/                  # audit writer adapter
│   ├── httpapi/                # HTTP handlers + OpenAPI-validated mux + SSE relay
│   └── grpcapi/                # gRPC servers (module-to-module) + registration
├── pkg/
│   ├── paperlessmanifest/      # routes/permissions from OpenAPI for gateway registration
│   └── paperlessclient/        # optional Go client for module-to-module use
├── ui/                         # Vue MF remote (documents+upload, category tree, permissions, search, statistics)
├── deploy/                     # policy.yaml, container config
├── Dockerfile
├── go.mod                      # module github.com/go-freya/freya/services/paperless (+ replaces)
└── Makefile
```

Platform wiring (outside the module):

- `deploy/stack/compose.yaml`: a `paperless` service (enrolls for its SVID, mounts its
  policy + kek), plus a MinIO/RustFS object store and (optionally) Tika + Gotenberg
  sidecars; `paperless-token` init to mint the enrollment token; a `paperless` Valkey
  user; a `paperless` DB + `paperless_app` role in `init-db.sql`; a gateway-bootstrap
  allow-list entry `spiffe://example.org/svc/paperless=/api/paperless;paperless`.
- `deploy/stack/configs/paperless.yaml`: identity (provided + enroll block), server/admin
  ports, db, valkey, kek, object-store endpoint/bucket, extraction endpoints, jobs/events
  sections.

## Complexity Tracking

No Constitution violations to justify — this section is intentionally empty.

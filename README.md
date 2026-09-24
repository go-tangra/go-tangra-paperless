# go-tangra-paperless

Tenant document management service for the
[go-tangra v4 platform](https://github.com/go-tangra/go-tangra).

It stores **documents** (metadata in TimescaleDB, bytes in S3-compatible object
storage such as RustFS or MinIO), organises them into a **category hierarchy**,
extracts their text asynchronously through **Apache Tika** (optionally converting
with **Gotenberg**), and makes them **full-text searchable**. Access is
Zanzibar-style (owner / editor / viewer / sharer, to users, roles or the tenant,
with expiry), inherited down the category tree. Object-store credentials are
sealed at rest (AES-256-GCM envelope) and never returned; extracted content
never reaches logs or the audit trail. Processing progress is published live on
the platform event bus, so the UI updates without polling.

Operations: [`deploy/README.md`](deploy/README.md).
Design history: `specs/009-paperless-service`.

## Place in the platform

```
go-tangra/go-tangra          platform module + @go-tangra/ui kit
        |
go-tangra-auth  <---->  go-tangra-portal (gateway)  <---->  go-tangra-lcm
                                  |
                        go-tangra-paperless  ---->  object storage (S3), Tika, Gotenberg
```

- Built on `github.com/go-tangra/go-tangra/v4` (mTLS transports, identity,
  service policy, audit, observability).
- Verifies platform tokens, resolves people and checks permissions through the
  auth SDK (`github.com/go-tangra/go-tangra-auth/sdk/v4`).
- Registers with the gateway through the portal SDK
  (`github.com/go-tangra/go-tangra-portal/sdk/v4`), which fronts the browser API
  (`/api/paperless`) and the federated UI remote.
- Enrolls for its SVID with lcm over the network
  (`github.com/go-tangra/go-tangra-lcm/sdk/v4`), as the platform stack does.

The repository holds one Go module, `github.com/go-tangra/go-tangra-paperless/v4`.
Other services call it through `pkg/paperlessclient` and the `paperless.v1` protos
(not proxied by the gateway).

## Layout

| Path | What |
|------|------|
| `api/openapi/paperless.yaml` | browser API contract (served under `/api/paperless`) |
| `api/proto/paperless/v1/` | document, category, permission and statistics gRPC services |
| `internal/config` | configuration + validation (secure defaults, named opt-outs) |
| `internal/store`, `internal/repo` | TimescaleDB schema (RLS), repositories; `internal/memstore` is the in-memory test double |
| `internal/blob` | S3-compatible object storage (minio-go), presigned downloads |
| `internal/extract` | Tika text extraction and Gotenberg conversion clients |
| `internal/jobs` | distributed extraction worker pool (single-winner lease, retry with backoff) |
| `internal/sealed` | envelope encryption of object-store credentials (KEK -> DEK) |
| `internal/authz`, `internal/permissions` | Zanzibar grants and category-tree inheritance |
| `internal/documents`, `internal/categories`, `internal/search` | documents, folders, full-text search |
| `internal/events`, `internal/stream` | processing events on the platform bus, live stream |
| `internal/backup`, `internal/stats`, `internal/audit` | backup export/import, statistics, audit |
| `internal/httpapi`, `internal/grpcapi` | browser and service APIs |
| `internal/app`, `cmd/paperlesssvc` | wiring and the service binary (serve, `bootstrap`, `version`) |
| `pkg/paperlessmanifest` | gateway manifest and built-in role grants |
| `pkg/paperlessclient` | Go client other services use |
| `deploy` | service policy and operations notes |
| `ui/` | Vue 3 + FlyonUI federated remote on `@go-tangra/ui` |

## Build and test

You need Go 1.26, Node 22, Docker (for the integration suite and the image), and a
GitHub token with `read:packages` to install `@go-tangra/ui` from GitHub Packages.

```bash
go build ./... && go vet ./... && go test -race ./...
buf lint
make test-integration                     # -tags integration, TimescaleDB via testcontainers (needs Docker)
make lint cover vuln

cd ui
export NODE_AUTH_TOKEN=$(gh auth token)   # ui/.npmrc only references this variable
npm ci && npm run lint && npm run test:unit && npm run build
```

The unit coverage gate requires at least 80 % overall and 100 % for the
authorization and sealing packages. Generated code, SQL bindings and wiring are
covered by the integration suite instead. The Playwright specs in `ui/tests/e2e`
need a running platform and operator credentials; they skip otherwise.

## Run

The service runs in the go-tangra platform stack (`deploy/stack` in
[go-tangra](https://github.com/go-tangra/go-tangra)), next to TimescaleDB, Valkey,
RustFS, Tika and Gotenberg. The stack mounts its configuration at
`/app/deploy/container.yaml` and the development key-encryption key at
`/app/deploy/kek.dev`. `deploy/kek.dev` in this repository is a development key
only; it is excluded from the image.

```bash
paperlesssvc bootstrap -config deploy/container.yaml    # apply migrations and exit
paperlesssvc -config deploy/container.yaml              # serve (applies migrations)
```

See `specs/009-paperless-service/quickstart.md` for the end-to-end walkthrough.

## Container image

The image is `ghcr.io/go-tangra/go-tangra-paperless`, built by
`.github/workflows/ci.yaml`. It carries `paperlesssvc` with the embedded UI remote.

```bash
docker buildx build --secret id=npm_token,env=NODE_AUTH_TOKEN \
  --build-arg APP_VERSION=4.0.0 -t go-tangra-paperless:dev .
docker run --rm go-tangra-paperless:dev version
```

The image runs `paperlesssvc -config deploy/container.yaml` as user `app`
(uid 10001). It contains no configuration and no key material: deployments mount
their own `deploy/container.yaml` and key-encryption key. Tika, Gotenberg and the
object store are separate services the configuration points at
(`extract.tika_url`, `extract.gotenberg_url`, `object_store.endpoint`); the
bucket is created on start when missing.

## API permissions

`documents:read/write/delete`, `categories:read/manage`, `search:read`,
`permissions:manage`, `backup:manage`, `stats:read`. The gateway enforces the
per-route permission from the manifest; the module then enforces the Zanzibar
grant on the document or category. Built-in role grants are seeded by the module
(`pkg/paperlessmanifest.Grants`).

## Versioning

- Releases are tagged `vX.Y.Z`. CI publishes the image as `X.Y.Z`, `X.Y`, `X`
  and `sha-<short>`. There is no `latest` tag.
- v4.0.0 rebuilds the service on the go-tangra v4 platform. The v3 line stays on
  the `v3` branch and its `v3.x` tags.

---
title: Operate Paperless
summary: Configuration keys and defaults, Tika and the object store, upload size limits, the service policy for the gateway and the Assets module, and fixes for common problems.
kind: reference
order: 90
tags: [paperless, operations, configuration, policy, troubleshooting]
audience: operator
---
Paperless runs as `paperlesssvc` (image
`ghcr.io/go-tangra/go-tangra-paperless`, user `app`, uid 10001).

```sh
paperlesssvc -config deploy/container.yaml             # serve; applies migrations
paperlesssvc bootstrap -config deploy/container.yaml   # apply migrations and exit
paperlesssvc version
```

At start it enrolls for its SVID `spiffe://<trust domain>/svc/paperless`
(when `enroll.enabled`), migrates its TimescaleDB schema, creates the object
store bucket if it is missing, serves the browser API and the `paperless.v1`
gRPC services, starts the extraction workers, registers with the gateway,
and registers its permissions, roles and built-in grants with auth (at start
and every five minutes).

## Dependencies

| Service | Needed for | Required |
|---|---|:-:|
| TimescaleDB | metadata, grants, jobs, the full-text index | yes |
| Valkey (platform event bus) | live processing events, user `paperless` | yes |
| S3-compatible object store (RustFS, MinIO) | document bytes | yes |
| auth | token verification, registration | yes |
| gateway | browser traffic, registration | yes |
| Apache Tika | text extraction | no |

Without `extract.tika_url` every document is marked `completed` with no
extracted text: it stays searchable by name and description only.

## Configuration

The file is YAML. Unknown keys are refused, so a typo stops the service at
start. Besides the framework keys (`service_name`, `trust_domain`, `env`,
`identity`, `authz`, `server`, `admin`, `discovery`, `limits`) paperless
reads:

| Key | Default | Notes |
|---|---|---|
| `db.dsn` | — | required; in production `sslmode=verify-full` (or `verify-ca`) |
| `db.migrate_dsn` | `db.dsn` | owner connection used for migrations |
| `db.max_conns` | 16 | |
| `valkey.addresses` | — | required |
| `valkey.username`, `valkey.password`, `valkey.ca_file` | — | Valkey user `paperless` over TLS |
| `valkey.allow_plaintext` | false | development only; refused in production |
| `kek.source` | `file` | `file` (with `kek.path`) or `env` (with `kek.env`); a 32-byte key (raw or base64) must load, or the service does not start |
| `object_store.endpoint`, `object_store.bucket` | — | required |
| `object_store.region` | `us-east-1` | |
| `object_store.use_ssl` | false | |
| `object_store.access_key`, `object_store.secret_key` | — | |
| `object_store.presign_ttl_seconds` | 300 | 30–3600; lifetime of download links |
| `extract.tika_url` | empty | Tika server, for example `http://tika:9998` |
| `extract.gotenberg_url` | empty | accepted, but not used by this release |
| `extract.timeout_seconds` | 60 | 1–3600, one Tika call |
| `extract.max_bytes` | 32 MiB | how much of a document is sent to Tika, and of its answer is read |
| `uploads.max_size_bytes` | 100 MiB | 1 KiB–5 GiB; largest browser upload |
| `uploads.allowed_mime` | empty | allow-list of upload types (`application/pdf`, ...); empty allows all |
| `jobs.workers` | 5 | 1–64 extraction workers |
| `jobs.interval_seconds` | 5 | 1–60, how often workers look for jobs |
| `jobs.lease_seconds` | 300 | interval–3600, how long a claimed job stays claimed |
| `jobs.retry_delay_seconds` | 60 | first retry delay |
| `jobs.backoff_multiplier` | 2.0 | growth of the delay per retry |
| `jobs.job_timeout_seconds` | 300 | 10–3600, one extraction |
| `jobs.cleanup_days` | 30 | 1–3650, retention of finished jobs |
| `jobs.max_retries` | 3 | 0–20; checked at start, but jobs are currently created with 3 retries |
| `events.enabled` | true | accepted; events are currently always published |
| `gateway.service` | `gateway` | mesh name of the gateway; the only peer allowed to fetch the help pages |
| `gateway.issuer` | — | required, an `https://` origin: the platform token issuer |
| `enroll.*` | — | `enabled`, `enroll_url`, `lcm_grpc`, `tenant_id`, `token_file`, `state_file`, `insecure` |
| `limits_paperless.backup_max_bytes` | 32 MiB | 4–128 MiB; range-checked at start |
| `limits_paperless.search_max_len` | 512 | 16–4096; range-checked at start |

At start the log lists accepted insecure settings (`valkey.allow_plaintext`,
`object_store.use_ssl=false`).

### Upload size

Three limits apply to a browser upload; the smallest wins:

- `uploads.max_size_bytes` (default 100 MiB);
- the framework's `limits.max_request_bytes`, which paperless defaults to
  33 MiB (at most 1 GiB). It also bounds each `paperless.v1` gRPC message,
  which carries a whole document;
- the gateway route limit of 100 MiB.

So by default a browser upload is limited to about 32 MiB. To accept larger
files, raise `limits.max_request_bytes` together with
`uploads.max_size_bytes`. The gateway's add-module bundle sets both to
64 MiB.

## Service policy

Paperless's policy must let the gateway in; the usual rule also covers the
help pages (`GET /docs/v1/bundle`). The Assets module needs its own rule to
store its attachments:

```yaml
rules:
  - id: gateway-forwards
    from: ["spiffe://<trust domain>/svc/gateway"]
    to: ["paperless"]
    operations: ["*"]
    effect: allow
  - id: asset-documents
    from: ["spiffe://<trust domain>/svc/asset"]
    to: ["paperless"]
    operations: ["/paperless.v1.PaperlessDocumentService/Create",
                 "/paperless.v1.PaperlessDocumentService/Get",
                 "/paperless.v1.PaperlessDocumentService/Download",
                 "/paperless.v1.PaperlessDocumentService/Delete",
                 "/paperless.v1.PaperlessDocumentService/Search",
                 "/paperless.v1.PaperlessDocumentService/List",
                 "/paperless.v1.PaperlessCategoryService/Create",
                 "/paperless.v1.PaperlessCategoryService/Get",
                 "/paperless.v1.PaperlessCategoryService/List",
                 "/paperless.v1.PaperlessCategoryService/GetTree"]
    effect: allow
```

A calling service becomes owner of the categories and documents it creates,
may create top-level categories and read the category tree, and has no other
access. The Assets module files its documents under `/Assets`.

## Troubleshooting

**The service refuses to start with `config: ...`.** The message names the
key and its allowed range, for example `db.dsn must use sslmode=verify-full
(or verify-ca) in production` or `gateway.issuer must be an https origin`.

**It refuses to start with `kek: ...`.** The key file or variable is missing
or is not 32 bytes.

**It refuses to start with `paperless: help docs: ...`.** The help pages
built into the binary are invalid. CI catches this with `tangra-docs lint`;
rebuild from a commit that passed CI.

**Documents stay `pending`.** The workers cannot claim jobs (look for
`paperless: claim failed`) or retry after errors (`paperless: extraction
retry` with the reason). Check Tika and the object store.

**Documents end `failed`.** Extraction failed after the retries: Tika was
unreachable, timed out (`extract.timeout_seconds`) or could not read the
file. The document stays stored and downloadable.

**Search finds nothing inside documents.** `extract.tika_url` is empty, so
no text is extracted.

**Uploads fail with `malformed_body` or `payload_too_large`.** The file is
over one of the [upload limits](#upload-size).

**The Assets module cannot store documents.** Paperless's policy lacks the
`asset-documents` rule, or the asset configuration has no
`discovery.static.paperless`.

**Roles are missing in auth.** Look for `auth registration failed; retrying`
in the log. Paperless retries until auth accepts, then registers every five
minutes.

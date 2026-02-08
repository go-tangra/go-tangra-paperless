# go-tangra-paperless

Document management service with S3 storage, full-text search, hierarchical categories, and Zanzibar-style fine-grained permissions.

## Features

- **Document Management** — Upload, download, search, move, batch delete with presigned URLs
- **Category Hierarchy** — Parent-child folder organization with materialized path queries
- **Zanzibar Permissions** — Fine-grained access control with Owner/Editor/Viewer/Sharer relations
- **Content Extraction** — Automatic text extraction via Apache Tika and document conversion via Gotenberg
- **Full-text Search** — Search across extracted document content
- **S3 Storage** — RustFS/MinIO-compatible object storage with SHA-256 checksums
- **Multi-Tenant** — Complete tenant isolation across all resources
- **Statistics** — Document counts by status, source, MIME type, and storage usage

## gRPC Services

| Service | Endpoints | Purpose |
|---------|-----------|---------|
| PaperlessDocumentService | Create, Get, List, Update, Delete, Move, Download, Search, BatchDelete | Document lifecycle |
| PaperlessCategoryService | Create, Get, List, Update, Delete, Move, GetTree | Category hierarchy |
| PaperlessPermissionService | Grant, Revoke, List, Check, ListAccessible, GetEffective | Access control |
| PaperlessStatisticsService | GetStatistics | System metrics |

**Port:** 9500 (gRPC) with REST endpoints via gRPC-Gateway

## Permission Model

| Relation | Permissions |
|----------|------------|
| **Owner** | Read, Write, Delete, Share |
| **Editor** | Read, Write |
| **Viewer** | Read |
| **Sharer** | Read, Share |

Permissions can be granted to users, roles, or entire tenants. Supports expiring permissions and inherited access from parent categories.

## Document Processing Pipeline

```
Upload → Store in S3 → PENDING
  → Tika (text extraction) → PROCESSING
  → Gotenberg (format conversion) → COMPLETED
  → Full-text index updated
```

Supported: PDF, DOC, DOCX, and other formats supported by Apache Tika.

## Configuration

```yaml
server:
  grpc:
    addr: "0.0.0.0:9500"
data:
  database:
    driver: postgres
    source: "postgresql://..."
  storage:
    endpoint: "rustfs:9000"
    bucket: "paperless"
    access_key: "minioadmin"
    secret_key: "minioadmin"
```

## Build

```bash
make build-server       # Build binary
make generate           # Generate Ent + Wire
make docker             # Build Docker image
make docker-buildx      # Multi-platform (amd64/arm64)
make test               # Run tests
make ent                # Regenerate Ent schemas
```

## Docker

```bash
docker run -p 9500:9500 ghcr.io/go-tangra/go-tangra-paperless:latest
```

Runs as non-root user `paperless` (UID 1000). Requires PostgreSQL, S3 storage, and optionally Tika + Gotenberg for content extraction.

## Dependencies

- **Framework**: Kratos v2
- **ORM**: Ent (PostgreSQL, MySQL)
- **Storage**: MinIO SDK (S3-compatible)
- **Cache**: Redis
- **Protobuf**: Buf

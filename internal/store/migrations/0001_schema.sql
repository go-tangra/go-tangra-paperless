-- +goose Up
CREATE TABLE paperless_categories (
  id                 uuid PRIMARY KEY,
  tenant_id          uuid NOT NULL,
  parent_id          uuid REFERENCES paperless_categories(id) ON DELETE CASCADE,
  name               text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
  path               text NOT NULL DEFAULT '',
  description        text NOT NULL DEFAULT '',
  depth              int NOT NULL DEFAULT 0,
  sort_order         int NOT NULL DEFAULT 0,
  document_count     int NOT NULL DEFAULT 0,
  subcategory_count  int NOT NULL DEFAULT 0,
  created_by         text NOT NULL DEFAULT '',
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, parent_id, name)
);
CREATE INDEX categories_tenant_parent ON paperless_categories (tenant_id, parent_id);
CREATE INDEX categories_path ON paperless_categories (tenant_id, path text_pattern_ops);

CREATE TABLE paperless_documents (
  id                  uuid PRIMARY KEY,
  tenant_id           uuid NOT NULL,
  category_id         uuid REFERENCES paperless_categories(id) ON DELETE SET NULL,
  category_path       text NOT NULL DEFAULT '',
  name                text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
  description         text NOT NULL DEFAULT '',
  object_key          text NOT NULL,
  file_name           text NOT NULL DEFAULT '',
  file_size           bigint NOT NULL DEFAULT 0,
  mime_type           text NOT NULL DEFAULT '',
  checksum            text NOT NULL DEFAULT '',
  status              text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived','deleted')),
  source              text NOT NULL DEFAULT 'upload' CHECK (source IN ('upload','email')),
  tags                jsonb NOT NULL DEFAULT '{}'::jsonb,
  content_text        text NOT NULL DEFAULT '',
  extracted_metadata  jsonb NOT NULL DEFAULT '{}'::jsonb,
  processing_status   text NOT NULL DEFAULT 'pending' CHECK (processing_status IN ('pending','processing','completed','failed')),
  search_tsv          tsvector GENERATED ALWAYS AS (
                        to_tsvector('english',
                          coalesce(name,'') || ' ' || coalesce(description,'') || ' ' || coalesce(content_text,''))
                      ) STORED,
  created_by          text NOT NULL DEFAULT '',
  updated_by          text NOT NULL DEFAULT '',
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, object_key)
);
CREATE INDEX documents_tenant_category ON paperless_documents (tenant_id, category_id);
CREATE INDEX documents_tenant_status ON paperless_documents (tenant_id, status);
CREATE INDEX documents_tenant_processing ON paperless_documents (tenant_id, processing_status);
CREATE INDEX documents_search_tsv ON paperless_documents USING gin (search_tsv);
CREATE INDEX documents_tags ON paperless_documents USING gin (tags);

CREATE TABLE paperless_permissions (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL,
  resource_type  text NOT NULL CHECK (resource_type IN ('document','category')),
  resource_id    text NOT NULL,
  subject_type   text NOT NULL CHECK (subject_type IN ('user','role','tenant')),
  subject_id     text NOT NULL DEFAULT '',
  relation       text NOT NULL CHECK (relation IN ('owner','editor','viewer','sharer')),
  granted_by     text,
  granted_at     timestamptz NOT NULL DEFAULT now(),
  expires_at     timestamptz
);
CREATE INDEX permissions_resource ON paperless_permissions (tenant_id, resource_type, resource_id);
CREATE INDEX permissions_subject ON paperless_permissions (tenant_id, subject_type, subject_id);

CREATE TABLE paperless_jobs (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL,
  document_id    uuid NOT NULL REFERENCES paperless_documents(id) ON DELETE CASCADE,
  status         text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','processing','completed','failed','retrying')),
  retry_count    int NOT NULL DEFAULT 0,
  max_retries    int NOT NULL DEFAULT 3,
  lease_until    timestamptz,
  next_retry_at  timestamptz,
  result         jsonb,
  started_at     timestamptz,
  completed_at   timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_due ON paperless_jobs (status, next_retry_at) WHERE status IN ('pending','retrying');
CREATE INDEX jobs_document ON paperless_jobs (tenant_id, document_id);

CREATE TABLE paperless_audit_events (
  ts             timestamptz NOT NULL DEFAULT now(),
  tenant_id      uuid NOT NULL,
  event_type     text NOT NULL,
  actor_kind     text NOT NULL,
  actor_id       text NOT NULL DEFAULT '',
  subject_kind   text NOT NULL DEFAULT '',
  subject_id     text NOT NULL DEFAULT '',
  outcome        text NOT NULL,
  reason         text NOT NULL DEFAULT '',
  correlation_id text NOT NULL DEFAULT '',
  details        jsonb
);

-- +goose Down
DROP TABLE IF EXISTS paperless_audit_events;
DROP TABLE IF EXISTS paperless_jobs;
DROP TABLE IF EXISTS paperless_permissions;
DROP TABLE IF EXISTS paperless_documents;
DROP TABLE IF EXISTS paperless_categories;

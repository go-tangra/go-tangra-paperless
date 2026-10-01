-- +goose Up
-- Server-side list sorting (go-tangra specs/032-server-side-tables): the
-- documents table pages per tenant ordered by
--   created_at DESC NULLS LAST, id DESC   (default)
--   lower(name) ASC NULLS LAST, id ASC    (name)
-- Each index matches its ORDER BY exactly (including the NULLS placement the
-- list contract emits), so the default page is an index range scan.
CREATE INDEX IF NOT EXISTS documents_tenant_created_id
  ON paperless_documents (tenant_id, created_at DESC NULLS LAST, id DESC);
CREATE INDEX IF NOT EXISTS documents_tenant_lower_name_id
  ON paperless_documents (tenant_id, lower(name), id);

-- +goose Down
DROP INDEX IF EXISTS documents_tenant_lower_name_id;
DROP INDEX IF EXISTS documents_tenant_created_id;

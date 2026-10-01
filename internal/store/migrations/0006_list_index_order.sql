-- +goose Up
-- Index-backed sorting (go-tangra 032 perf): the documents sort fields are NOT
-- NULL, so the list contract now orders by "created_at DESC, id DESC" (no
-- NULLS LAST) and "lower(name) DESC, id DESC". A plain (tenant_id, created_at,
-- id) btree serves the newest-first default by a backward scan and oldest
-- first by a forward one; the 0005 index's DESC NULLS LAST no longer matches.
-- documents_tenant_lower_name_id already has this shape and is kept.
DROP INDEX IF EXISTS documents_tenant_created_id;
CREATE INDEX documents_tenant_created_id ON paperless_documents (tenant_id, created_at, id);

-- +goose Down
DROP INDEX IF EXISTS documents_tenant_created_id;
CREATE INDEX documents_tenant_created_id
  ON paperless_documents (tenant_id, created_at DESC NULLS LAST, id DESC);

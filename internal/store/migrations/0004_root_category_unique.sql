-- +goose Up
-- UNIQUE (tenant_id, parent_id, name) does not constrain ROOT categories:
-- parent_id is NULL there and Postgres treats NULLs as distinct, so two
-- concurrent creates of the same root name (e.g. a module's EnsureCategory for
-- "/Assets") could both succeed. This partial index closes that gap. A tenant
-- that already holds duplicate root names keeps working: the index is created
-- only when no duplicates exist (otherwise a NOTICE is raised; an operator
-- merges the duplicates and creates the index by hand), and the paperlessclient
-- SDK converges on the lowest id among same-named siblings either way.
-- +goose StatementBegin
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM paperless_categories WHERE parent_id IS NULL
    GROUP BY tenant_id, name HAVING count(*) > 1
  ) THEN
    RAISE NOTICE 'paperless: duplicate root category names exist; categories_root_name_unique not created';
  ELSE
    CREATE UNIQUE INDEX IF NOT EXISTS categories_root_name_unique
      ON paperless_categories (tenant_id, name) WHERE parent_id IS NULL;
  END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP INDEX IF EXISTS categories_root_name_unique;

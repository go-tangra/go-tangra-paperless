-- +goose Up
-- The audit stream is append-only and time-keyed with no primary key, so it
-- becomes a hypertable for time-based retention.
SELECT create_hypertable('paperless_audit_events', 'ts', if_not_exists => TRUE, migrate_data => TRUE);

-- +goose Down
SELECT 1;

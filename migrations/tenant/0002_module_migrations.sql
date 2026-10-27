-- +goose Up
-- Jejak migrasi per modul di tenant DB (PRD-000 FR-07); versi goose per modul ada di goose_module_<slug_snake>.
CREATE TABLE module_migrations (
    id               uuid PRIMARY KEY,
    tenant_id        uuid NOT NULL,
    module_key       text NOT NULL,
    module_version   text NOT NULL,
    migration_key    text NOT NULL,
    checksum         text NOT NULL,
    applied_at       timestamptz NOT NULL DEFAULT now(),
    execution_status text NOT NULL CHECK (execution_status IN ('succeeded', 'failed')),
    operation_id     uuid NULL
);

CREATE INDEX module_migrations_tenant_key_idx ON module_migrations (tenant_id, module_key);

-- +goose Down
DROP TABLE module_migrations;

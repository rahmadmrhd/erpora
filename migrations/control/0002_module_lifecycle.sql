-- +goose Up
-- Tabel lifecycle modul (PRD-000 FR-19, FR-21): status per tenant, catatan operasi, dan event append-only.

CREATE TABLE tenant_state (
    tenant_id             uuid PRIMARY KEY,
    module_state_revision bigint NOT NULL DEFAULT 0 CHECK (module_state_revision >= 0),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE module_operations (
    id           uuid PRIMARY KEY,
    tenant_id    uuid NOT NULL,
    module_key   text NOT NULL,
    operation    text NOT NULL CHECK (operation IN ('install', 'enable', 'disable', 'upgrade', 'uninstall', 'reconcile')),
    from_version text NULL,
    to_version   text NULL,
    status       text NOT NULL CHECK (status IN ('running', 'succeeded', 'failed')),
    steps        jsonb NOT NULL DEFAULT '[]',
    error        jsonb NULL,
    actor        text NOT NULL,
    trace_id     text NULL,
    started_at   timestamptz NOT NULL DEFAULT now(),
    finished_at  timestamptz NULL
);

CREATE INDEX module_operations_tenant_key_started_idx ON module_operations (tenant_id, module_key, started_at DESC);
CREATE INDEX module_operations_running_idx ON module_operations (tenant_id) WHERE status = 'running';

CREATE TABLE module_installations (
    id                uuid PRIMARY KEY,
    tenant_id         uuid NOT NULL,
    module_key        text NOT NULL,
    installed_version text NULL,
    status            text NOT NULL CHECK (status IN ('verified', 'installed', 'enabled', 'disabled', 'upgrade_pending', 'failed', 'uninstalled')),
    previous_status   text NULL,
    last_operation_id uuid NULL REFERENCES module_operations (id),
    settings          jsonb NOT NULL DEFAULT '{}',
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, module_key)
);

CREATE TABLE module_lifecycle_events (
    id           uuid PRIMARY KEY,
    tenant_id    uuid NOT NULL,
    module_key   text NOT NULL,
    event        text NOT NULL,
    actor        text NOT NULL,
    operation_id uuid NULL,
    details      jsonb NOT NULL DEFAULT '{}',
    occurred_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX module_lifecycle_events_tenant_key_idx ON module_lifecycle_events (tenant_id, module_key, occurred_at DESC);

-- Event lifecycle bersifat append-only (transaction-data-standards): UPDATE/DELETE/TRUNCATE ditolak.
-- +goose StatementBegin
CREATE FUNCTION module_lifecycle_events_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'module_lifecycle_events is append-only (% ditolak)', TG_OP;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER module_lifecycle_events_no_change
    BEFORE UPDATE OR DELETE ON module_lifecycle_events
    FOR EACH ROW EXECUTE FUNCTION module_lifecycle_events_append_only();

CREATE TRIGGER module_lifecycle_events_no_truncate
    BEFORE TRUNCATE ON module_lifecycle_events
    FOR EACH STATEMENT EXECUTE FUNCTION module_lifecycle_events_append_only();

-- +goose Down
DROP TRIGGER module_lifecycle_events_no_truncate ON module_lifecycle_events;
DROP TRIGGER module_lifecycle_events_no_change ON module_lifecycle_events;
DROP FUNCTION module_lifecycle_events_append_only();
DROP TABLE module_lifecycle_events;
DROP TABLE module_installations;
DROP TABLE module_operations;
DROP TABLE tenant_state;

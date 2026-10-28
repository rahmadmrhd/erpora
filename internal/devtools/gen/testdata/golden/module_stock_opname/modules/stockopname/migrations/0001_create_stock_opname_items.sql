-- +goose Up
-- Tanpa tenant_id: isolasi tenant ditangani lapisan database (ADR-004).
-- version: optimistic concurrency (api-guidelines §4.3/§7.1); updated_at: waktu perubahan terakhir.
CREATE TABLE stock_opname_items (
    id         uuid PRIMARY KEY,
    title      text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    version    bigint NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX stock_opname_items_created_at_idx ON stock_opname_items (created_at);

-- +goose Down
DROP TABLE stock_opname_items;

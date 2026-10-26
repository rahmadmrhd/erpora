-- +goose Up
-- version: optimistic concurrency (api-guidelines §4.3/§7.1); baris lama mulai dari 1.
-- updated_at: baris lama diisi created_at agar konsisten (bukan waktu migrasi).
ALTER TABLE example_notes
    ADD COLUMN version    bigint      NOT NULL DEFAULT 1 CHECK (version >= 1),
    ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();

UPDATE example_notes SET updated_at = created_at;

-- +goose Down
ALTER TABLE example_notes
    DROP COLUMN updated_at,
    DROP COLUMN version;

-- +goose Up
-- Timpaan Desk per tenant (revisi M3a, design.md section 6, Q40 butir 12): menyembunyikan atau mengurutkan menu
-- Desk pada tingkat situs (scope 'site') atau pengguna (scope 'user'; dibaca setelah identitas PRD-001 ada).
-- Revisi M3a hanya MEMBACA tabel ini (jalur tulis datang di M3b dengan permission platform.desks.update).
-- Kolom tenant_id NOT NULL (FR-21); skema hanya lewat migrasi goose.
CREATE TABLE desk_overrides (
    tenant_id  uuid        NOT NULL,
    desk_id    text        NOT NULL CHECK (desk_id ~ '^[a-z0-9:-]{1,64}$'),
    scope      text        NOT NULL CHECK (scope IN ('site', 'user')),
    subject    text        NOT NULL DEFAULT '' CHECK (length(subject) <= 128),
    -- config berbentuk {"hidden": [id...], "order": [id...]}; skema divalidasi aplikasi (desks.ParseConfig).
    config     jsonb       NOT NULL DEFAULT '{}',
    revision   bigint      NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_by text        NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, desk_id, scope, subject)
);

-- +goose Down
DROP TABLE desk_overrides;

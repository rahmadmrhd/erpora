-- +goose Up
-- Override terjemahan per tenant (PRD-000 FR-25, Q34): pemilik instalasi mengganti teks katalog bawaan tanpa
-- build ulang. Kolom tenant_id NOT NULL (FR-21); skema hanya lewat migrasi goose.
CREATE TABLE i18n_overrides (
    tenant_id  uuid        NOT NULL,
    namespace  text        NOT NULL CHECK (namespace ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$' AND char_length(namespace) <= 50),
    locale     text        NOT NULL CHECK (locale ~ '^[a-z]{2,3}(-[A-Z]{2})?$'),
    key        text        NOT NULL CHECK (key ~ '^[A-Za-z0-9_.-]+$' AND char_length(key) <= 200),
    message    text        NOT NULL CHECK (char_length(message) BETWEEN 1 AND 2000),
    updated_by text        NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, namespace, locale, key)
);

-- Revisi per (tenant, namespace, locale): naik pada setiap perubahan override; basis ETag dan invalidasi cache.
CREATE TABLE i18n_revisions (
    tenant_id  uuid        NOT NULL,
    namespace  text        NOT NULL,
    locale     text        NOT NULL,
    revision   bigint      NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, namespace, locale)
);

-- +goose Down
DROP TABLE i18n_revisions;
DROP TABLE i18n_overrides;

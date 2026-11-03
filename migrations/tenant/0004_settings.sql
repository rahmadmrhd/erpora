-- +goose Up
-- Subset Settings untuk C1 (PRD-001 section 4 tabel tenant settings, Q20): hanya penyimpanan nilai per tenant; skema
-- field, validasi, audit, cache revisi, dan UI adalah C3 (FR-120). C1 memakainya untuk satu kunci platform,
-- platform.public_url (tautan email reset sandi dan pemeriksaan Origin cookie refresh).
-- Tenant DB terisolasi per database (ADR-0004; Q18): tanpa tenant_id. module_key kosong untuk scope platform.
-- Nilai terenkripsi (encrypted = true) disimpan sebagai string JSON berisi base64 hasil crypto.Sealer (AAD
-- settings|<scope>|<module_key>|<key>), bukan plaintext.
CREATE TABLE settings (
    scope      text        NOT NULL CHECK (scope IN ('platform', 'module')),
    module_key text        NOT NULL DEFAULT '' CHECK (length(module_key) <= 64),
    key        text        NOT NULL CHECK (key ~ '^[a-z][a-z0-9_.-]{0,127}$'),
    value      jsonb       NOT NULL,
    encrypted  boolean     NOT NULL DEFAULT false,
    updated_by text        NOT NULL DEFAULT '' CHECK (length(updated_by) <= 128),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT settings_scope_key UNIQUE (scope, module_key, key),
    -- scope platform tidak punya modul; scope module wajib punya module_key.
    CONSTRAINT settings_module_key_scope CHECK ((scope = 'platform' AND module_key = '') OR (scope = 'module' AND module_key ~ '^[a-z][a-z0-9_.-]{0,63}$')),
    -- nilai terenkripsi selalu string JSON (base64 segel).
    CONSTRAINT settings_encrypted_string CHECK (NOT encrypted OR jsonb_typeof(value) = 'string')
);

-- +goose Down
DROP TABLE settings;

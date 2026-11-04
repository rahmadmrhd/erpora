-- +goose Up
-- Subset FR-113 untuk C1 (keputusan K-C1.3, ADR-0013): tabel roles dan user_roles beserta benih role sistem
-- administrator (dibutuhkan MFA wajib Administrator dan core:create-admin). Matriks permission, scope, dan field
-- access ditambahkan C2. Tenant DB terisolasi per database (ADR-0004; PRD-001 section 4 tidak memuat tenant_id pada
-- tabel ini), sehingga benih di migrasi tidak membutuhkan tenant_id yang belum ada saat goose berjalan.
-- user_roles.user_id merujuk users di control DB tanpa FK (database terpisah pada mode multi-tenant).
CREATE TABLE roles (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    key           text        NOT NULL CHECK (key ~ '^[a-z][a-z0-9_.-]{0,63}$'),
    label         text        NOT NULL CHECK (length(btrim(label)) BETWEEN 1 AND 120),
    description   text        NOT NULL DEFAULT '' CHECK (length(description) <= 1000),
    kind          text        NOT NULL CHECK (kind IN ('system', 'module_default', 'custom')),
    source_module text        NULL,
    customized    boolean     NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT roles_key_key UNIQUE (key)
);

CREATE TABLE user_roles (
    user_id    uuid        NOT NULL,
    role_id    uuid        NOT NULL REFERENCES roles (id),
    granted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);
CREATE INDEX user_roles_role_idx ON user_roles (role_id);

INSERT INTO roles (key, label, description, kind)
VALUES ('administrator', 'Administrator', 'Akses penuh ke seluruh sistem, termasuk modul baru.', 'system');

-- +goose Down
DROP TABLE user_roles;
DROP TABLE roles;

-- +goose Up
-- Skema otorisasi PRD-001 C2 (FR-110 sampai FR-113, FR-118; Q35 D-C2.8 dan D-C2.9): matriks permission per role,
-- akses grup field per role, penugasan dimensi scope per pengguna, dan revisi otorisasi (authz_state). Tabel roles dan
-- user_roles sudah ada sejak 0003_roles_min (subset C1, K-C1.3).
-- Tenant DB terisolasi per database (ADR-0004; PRD-001 Q18): tanpa kolom tenant_id. user_scope_assignments.user_id
-- merujuk users di control DB tanpa FK (database terpisah pada mode multi-tenant), seperti user_roles.user_id.
-- Seluruh bentuk yang tidak sah ditolak oleh CHECK sehingga SQL langsung pun tidak dapat menyimpan scope kosong atau
-- rusak (gagal tertutup, NFR-106).

-- Baris matriks. Satu role boleh punya beberapa baris untuk resource yang sama (union per baris, FR-111), maka tanpa
-- UNIQUE pada (role_id, resource). scope adalah larik token tidak kosong: all, owner, assigned:<dimensi>,
-- custom:<key>; `all` dinyatakan eksplisit, larik kosong tidak sah.
CREATE TABLE role_permissions (
    id       uuid  PRIMARY KEY DEFAULT gen_random_uuid(),
    role_id  uuid  NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    resource text  NOT NULL CHECK (resource ~ '^[a-z][a-z0-9_.-]{0,127}$'),
    actions  text[] NOT NULL,
    scope    jsonb NOT NULL,
    CONSTRAINT role_permissions_actions_valid CHECK (
        cardinality(actions) >= 1
        AND array_position(actions, NULL) IS NULL
        AND array_position(actions, '') IS NULL
        -- digabung tanpa pemisah: setiap karakter setiap elemen harus [a-z_] (pemisah akan dapat dipalsukan elemen)
        AND array_to_string(actions, '') ~ '^[a-z_]+$'
    ),
    CONSTRAINT role_permissions_scope_nonempty_array CHECK (
        CASE WHEN jsonb_typeof(scope) = 'array' THEN jsonb_array_length(scope) >= 1 ELSE false END
    ),
    CONSTRAINT role_permissions_scope_tokens CHECK (
        NOT jsonb_path_exists(scope, '$[*] ? (@.type() != "string" || !(@ like_regex "^(all|owner|assigned:[a-z][a-z0-9_.-]{0,127}|custom:[a-z][a-z0-9_.-]{0,127})$"))')
    )
);
CREATE INDEX role_permissions_role_idx ON role_permissions (role_id);

CREATE TABLE role_field_access (
    role_id     uuid NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    resource    text NOT NULL CHECK (resource ~ '^[a-z][a-z0-9_.-]{0,127}$'),
    field_group text NOT NULL CHECK (field_group ~ '^[a-z][a-z0-9_.-]{0,127}$'),
    access      text NOT NULL CHECK (access IN ('none', 'read', 'write')),
    PRIMARY KEY (role_id, resource, field_group)
);

-- Penugasan nilai dimensi scope (mis. inventory.warehouse) per pengguna. Pengguna tanpa baris untuk sebuah dimensi
-- yang disyaratkan scope `assigned:<dimensi>` tidak melihat apa pun (FALSE, bukan klausa yang dilewati); baris dengan
-- nilai kosong tidak sah (penugasan dihapus, bukan dikosongkan). include_descendants (FR-118, D-C2.8) hanya bermakna
-- untuk dimensi core.org_unit: true = unit beserta turunannya.
CREATE TABLE user_scope_assignments (
    user_id             uuid    NOT NULL,
    dimension           text    NOT NULL CHECK (dimension ~ '^[a-z][a-z0-9_.-]{0,127}$'),
    "values"            text[]  NOT NULL,
    include_descendants boolean NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, dimension),
    CONSTRAINT user_scope_assignments_values_valid CHECK (
        cardinality("values") >= 1
        AND array_position("values", NULL) IS NULL
        AND array_position("values", '') IS NULL
    )
);

-- Revisi otorisasi: satu baris (id = 1), naik dalam transaksi yang sama dengan setiap perubahan role, matriks, field
-- access, user_roles, penugasan scope, dan sinkronisasi default_roles; kunci cache hak efektif (FR-116).
CREATE TABLE authz_state (
    id                  smallint PRIMARY KEY CHECK (id = 1),
    permission_revision bigint   NOT NULL CHECK (permission_revision >= 1)
);
INSERT INTO authz_state (id, permission_revision) VALUES (1, 1);

-- Role sistem kedua (FR-113): hak dasar semua pengguna login; berlaku implisit tanpa baris user_roles, matriksnya
-- dapat diubah admin.
INSERT INTO roles (key, label, description, kind)
VALUES ('authenticated', 'Authenticated', 'Hak dasar semua pengguna yang sudah login.', 'system');

-- +goose Down
DROP TABLE authz_state;
DROP TABLE user_scope_assignments;
DROP TABLE role_field_access;
DROP TABLE role_permissions;
DELETE FROM user_roles WHERE role_id IN (SELECT id FROM roles WHERE key = 'authenticated');
DELETE FROM roles WHERE key = 'authenticated';

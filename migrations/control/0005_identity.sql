-- +goose Up
-- Skema identitas PRD-001 C1 (FR-101, FR-102, FR-106, FR-107, FR-108; ADR-0013): pengguna, keanggotaan tenant,
-- sesi, refresh token, kunci penandatangan JWT, faktor MFA, kode pemulihan, reset sandi, dan catatan percobaan login.
-- Rahasia saat diam: secret TOTP dan kunci privat JWT disimpan sebagai bytea hasil crypto.Sealer (AES-256-GCM,
-- ERP_SECRET_KEY); token (refresh, reset) dan kode pemulihan hanya disimpan sebagai hash 32 byte (SHA-256/HMAC-SHA-256).
-- Semua cap waktu adalah timestamptz (disimpan UTC, ISO 8601 di batas API; transaction-data-standards).
-- users bersifat global (identitas lintas tenant, PRD-001 section 4); isolasi tenant lewat tenant_memberships, dan
-- tabel peristiwa (sesi, refresh token, reset, percobaan login) membawa tenant_id NOT NULL (FR-21).
-- signing_keys bersifat global platform (JWKS tunggal; klaim tid menyatakan tenant), tanpa tenant_id.

CREATE TABLE users (
    id                uuid        PRIMARY KEY,
    -- email dinormalisasi aplikasi (trim + huruf kecil); CHECK menolak bentuk tak ternormalisasi lewat SQL langsung,
    -- sehingga UNIQUE (email) berarti unik tanpa memandang huruf/spasi.
    email             text        NOT NULL CHECK (email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 254 AND position('@' IN email) > 1),
    name              text        NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 200),
    status            text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'locked')),
    -- format PHC argon2id; NULL untuk pengguna SSO-only.
    password_hash     text        NULL CHECK (password_hash IS NULL OR length(password_hash) BETWEEN 1 AND 512),
    email_verified_at timestamptz NULL,
    last_login_at     timestamptz NULL,
    locked_at         timestamptz NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_email_key UNIQUE (email)
);

CREATE TABLE tenant_memberships (
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL REFERENCES users (id),
    status     text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tenant_id, user_id)
);
CREATE INDEX tenant_memberships_user_idx ON tenant_memberships (user_id);

CREATE TABLE user_preferences (
    user_id    uuid        PRIMARY KEY REFERENCES users (id),
    locale     text        NOT NULL DEFAULT '' CHECK (length(locale) <= 35),
    timezone   text        NOT NULL DEFAULT '' CHECK (length(timezone) <= 64),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE signing_keys (
    -- kid: karakter aman untuk header JWT dan URL; menolak path, kutip, dan spasi (Review Focus 3).
    kid                text        PRIMARY KEY CHECK (kid ~ '^[A-Za-z0-9_-]{8,64}$'),
    status             text        NOT NULL CHECK (status IN ('pending', 'active', 'retiring', 'retired')),
    public_key         bytea       NOT NULL CHECK (octet_length(public_key) = 32),
    private_key_sealed bytea       NOT NULL CHECK (octet_length(private_key_sealed) BETWEEN 1 AND 4096),
    created_at         timestamptz NOT NULL DEFAULT now(),
    activated_at       timestamptz NULL,
    retiring_at        timestamptz NULL,
    retired_at         timestamptz NULL,
    -- cap waktu konsisten dengan status; kunci pending yang dibuang boleh langsung retired tanpa activated_at.
    CONSTRAINT signing_keys_status_timestamps CHECK (
        (status = 'pending'  AND activated_at IS NULL     AND retiring_at IS NULL     AND retired_at IS NULL) OR
        (status = 'active'   AND activated_at IS NOT NULL AND retired_at IS NULL) OR
        (status = 'retiring' AND activated_at IS NOT NULL AND retiring_at IS NOT NULL AND retired_at IS NULL) OR
        (status = 'retired'  AND retired_at IS NOT NULL)
    )
);
CREATE INDEX signing_keys_status_idx ON signing_keys (status);

CREATE TABLE auth_sessions (
    id                  uuid        PRIMARY KEY, -- = sid pada JWT
    tenant_id           uuid        NOT NULL,
    user_id             uuid        NOT NULL,
    client_kind         text        NOT NULL CHECK (client_kind IN ('web', 'mobile', 'desktop')),
    auth_method         text        NOT NULL DEFAULT 'password' CHECK (auth_method IN ('password', 'oidc', 'pat', 'client_credentials')),
    -- amr (RFC 8176) sesi: ["pwd"] setelah sandi, ["pwd","otp"] setelah MFA.
    amr                 jsonb       NOT NULL DEFAULT '["pwd"]' CHECK (jsonb_typeof(amr) = 'array' AND jsonb_array_length(amr) BETWEEN 1 AND 8),
    user_agent          text        NOT NULL DEFAULT '' CHECK (length(user_agent) <= 512),
    ip                  text        NOT NULL DEFAULT '' CHECK (length(ip) <= 45),
    created_at          timestamptz NOT NULL DEFAULT now(),
    last_seen_at        timestamptz NOT NULL DEFAULT now(),
    absolute_expires_at timestamptz NOT NULL,
    revoked_at          timestamptz NULL,
    revoke_reason       text        NULL CHECK (revoke_reason IS NULL OR length(revoke_reason) <= 64),
    -- sesi hanya milik anggota tenant itu.
    FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_memberships (tenant_id, user_id),
    CONSTRAINT auth_sessions_id_tenant_key UNIQUE (id, tenant_id)
);
CREATE INDEX auth_sessions_user_idx ON auth_sessions (user_id);
CREATE INDEX auth_sessions_tenant_user_active_idx ON auth_sessions (tenant_id, user_id) WHERE revoked_at IS NULL;
CREATE INDEX auth_sessions_revoked_idx ON auth_sessions (tenant_id, revoked_at) WHERE revoked_at IS NOT NULL;

CREATE TABLE refresh_tokens (
    id         uuid        PRIMARY KEY,
    tenant_id  uuid        NOT NULL,
    session_id uuid        NOT NULL,
    family_id  uuid        NOT NULL,
    parent_id  uuid        NULL,
    token_hash bytea       NOT NULL CHECK (octet_length(token_hash) = 32), -- SHA-256 token opaque
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at    timestamptz NULL,     -- diset sekali, atomik: UPDATE ... WHERE used_at IS NULL
    expires_at timestamptz NOT NULL,
    CONSTRAINT refresh_tokens_token_hash_key UNIQUE (token_hash),
    FOREIGN KEY (session_id, tenant_id) REFERENCES auth_sessions (id, tenant_id),
    -- anak harus satu family dan satu sesi dengan induknya (MATCH SIMPLE: akar tanpa parent_id tidak diperiksa).
    CONSTRAINT refresh_tokens_id_family_key UNIQUE (id, family_id),
    CONSTRAINT refresh_tokens_id_session_key UNIQUE (id, session_id),
    FOREIGN KEY (parent_id, family_id) REFERENCES refresh_tokens (id, family_id),
    FOREIGN KEY (parent_id, session_id) REFERENCES refresh_tokens (id, session_id)
);
CREATE INDEX refresh_tokens_session_idx ON refresh_tokens (session_id);
CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id);

CREATE TABLE user_mfa_factors (
    id             uuid        PRIMARY KEY,
    user_id        uuid        NOT NULL REFERENCES users (id),
    type           text        NOT NULL DEFAULT 'totp' CHECK (type IN ('totp')),
    secret_sealed  bytea       NOT NULL CHECK (octet_length(secret_sealed) BETWEEN 1 AND 1024),
    confirmed_at   timestamptz NULL,
    last_used_step bigint      NOT NULL DEFAULT 0 CHECK (last_used_step >= 0), -- anti-replay: hanya step > last_used_step
    created_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_mfa_factors_user_type_key UNIQUE (user_id, type)
);

CREATE TABLE user_recovery_codes (
    id         uuid        PRIMARY KEY,
    user_id    uuid        NOT NULL REFERENCES users (id),
    code_hash  bytea       NOT NULL CHECK (octet_length(code_hash) = 32),
    used_at    timestamptz NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT user_recovery_codes_user_hash_key UNIQUE (user_id, code_hash)
);

CREATE TABLE password_resets (
    id         uuid        PRIMARY KEY,
    tenant_id  uuid        NOT NULL,
    user_id    uuid        NOT NULL,
    token_hash bytea       NOT NULL CHECK (octet_length(token_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    used_at    timestamptz NULL,
    CONSTRAINT password_resets_token_hash_key UNIQUE (token_hash),
    FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_memberships (tenant_id, user_id)
);
CREATE INDEX password_resets_user_idx ON password_resets (user_id);

-- Catatan percobaan autentikasi untuk jeda per pasangan (akun, IP) dan ambang lintas IP (ADR-0013 section 5, Q16).
CREATE TABLE login_attempts (
    id           bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tenant_id    uuid        NOT NULL,
    user_id      uuid        NOT NULL,
    ip           text        NOT NULL CHECK (length(ip) BETWEEN 1 AND 45),
    kind         text        NOT NULL CHECK (kind IN ('password', 'mfa')),
    success      boolean     NOT NULL,
    attempted_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, user_id) REFERENCES tenant_memberships (tenant_id, user_id)
);
CREATE INDEX login_attempts_pair_idx ON login_attempts (tenant_id, user_id, ip, attempted_at DESC);
CREATE INDEX login_attempts_user_time_idx ON login_attempts (tenant_id, user_id, attempted_at);
CREATE INDEX login_attempts_time_idx ON login_attempts (tenant_id, attempted_at);

-- +goose Down
DROP TABLE login_attempts;
DROP TABLE password_resets;
DROP TABLE user_recovery_codes;
DROP TABLE user_mfa_factors;
DROP TABLE refresh_tokens;
DROP TABLE auth_sessions;
DROP TABLE signing_keys;
DROP TABLE user_preferences;
DROP TABLE tenant_memberships;
DROP TABLE users;

# PRD-001 — Core: Identity, Authorization, Settings & Audit

| Atribut        | Nilai                                                                 |
| -------------- | --------------------------------------------------------------------- |
| Status         | **Accepted** (21 Oktober 2026), semua pertanyaan Q1–Q9 diputuskan     |
| Tanggal        | 21 Oktober 2026                                                       |
| Bergantung pada | PRD-000 (Accepted): Module Registry, SDK, seam tenancy, observability, CLI |
| Fase roadmap   | Blueprint §14 fase **02 — Core platform** (tanpa tenancy) · Blueprint §14.1 urutan 1 "Core + Settings" |
| Sumber acuan   | Dokumen awal Blueprint & arsitektur §4 & §6 (sudah dihapus; lihat `docs/architecture/README.md`), keputusan diskusi 21 Okt 2026 |

---

## 1. Latar belakang

PRD-000 menghasilkan platform yang dapat memuat, mengaktifkan, dan menonaktifkan modul, tetapi masih memakai **stub** untuk autentikasi dan otorisasi (PRD-000 FR-05). PRD-001 mengganti stub tersebut dengan Core yang sebenarnya:

1. **Identity**: siapa pengguna ini (login lokal, SSO eksternal, token API).
2. **Authorization**: apa yang boleh ia lakukan, pada data mana, dan field mana.
3. **Settings**: konfigurasi platform dan modul berbasis schema.
4. **Audit**: siapa melakukan apa, kapan, dan terhadap apa.

Prinsip arsitektur tetap berlaku (PRD-000 §7): **backend adalah otoritas**. Permission di frontend hanya untuk UX.

## 2. Tujuan & non-goals

### 2.1. Goals

1. **G1 — Autentikasi aman untuk web, mobile/desktop, dan integrasi**: JWT access token + refresh token berotasi, SSO OIDC (Google, Microsoft, dan IdP perusahaan), personal access token, dan OAuth2 client credentials.
2. **G2 — RBAC yang kuat tetapi mudah dipahami admin**: matriks per role, tiga lapis (aksi, record, field), aturan union tanpa deny, dan penjelas akses.
3. **G3 — Aksi dan scope yang dapat diperluas modul**: aksi standar ERP + aksi kustom, serta scope data generik (milik sendiri, penugasan gudang/cabang, scope kustom modul).
4. **G4 — Settings berbasis schema**: modul mendeklarasikan settings, platform menyimpan, memvalidasi, dan merender UI-nya.
5. **G5 — Audit trail menyeluruh** untuk autentikasi, perubahan hak akses, settings, lifecycle modul, dan (opt-in) perubahan data bisnis.
6. **G6 — Modul bisnis tidak menulis logika auth sendiri**: semua pemeriksaan lewat SDK.

### 2.2. Non-goals

- Multi-tenancy penuh (tetap seam PRD-000 FR-21; tabel memakai `tenant_id`).
- **ERP sebagai penyedia identitas (OIDC provider/SSO) untuk aplikasi lain**: ditunda (keputusan 21 Okt).
- Policy engine berbahasa kebijakan (Cedar/OPA/ABAC penuh). Kontrak SDK (§6.6) memungkinkan penggantian mesin kelak.
- Aturan "deny" eksplisit.
- Struktur organisasi **lengkap** (legal entity, mata uang, akun per company, dsb.). Yang masuk hanya hierarki unit organisasi minimal untuk scope (FR-118).
- Workflow approval multi-level → PRD tersendiri. Aksi `submit`/`approve` di sini hanya *izin*, bukan alur kerja.
- Authorization Code + PKCE untuk aplikasi mobile/desktop → saat aplikasi tersebut mulai dibuat (Q3).
- LDAP / Active Directory langsung → ditunda; gunakan IdP OIDC sebagai jembatan (Q8).
- Mapping grup IdP → role ERP otomatis → ditunda (Q7).
- Admin memindahkan field antar-grup atau membuat grup field baru → ditunda (Q2).
- Template & antrean email lengkap → PRD Notifications. Yang masuk hanya mailer minimal (FR-108).

## 3. Penempatan: hibrida platform + modul `core`

| Lokasi | Isi | Alasan |
| --- | --- | --- |
| **Platform** (`internal/platform/identity`, `authorization`, `settings`, `audit`) | Penerbitan & verifikasi token, middleware auth, **mesin evaluasi** permission/scope/field, store settings, audit writer, **pemilik tabel** | Harus selalu tersedia, termasuk untuk route platform, dan tidak boleh hilang saat modul bermasalah |
| **SDK** (`sdk/authz`, `sdk/identity`, `sdk/settings`, `sdk/audit`) | Interface yang dipakai modul, diimplementasikan platform (dependency inversion, PRD-000 D5/D9) | Modul tidak boleh mengimpor platform |
| **Modul `modules/core`** (key `erp.core`, slug `core`, **`required: true`**) | Use case & API administrasi: user, role, matriks permission, scope assignment, identity provider, OAuth client, PAT, settings UI, audit viewer + halaman frontend-nya | Fitur administrasi berkembang seperti modul biasa (rilis, migrasi UI, command), tetapi tidak bisa di-disable |

- Field manifest baru **`required`**: modul `required` di-install & di-enable otomatis oleh `erpora migrate up`, serta tidak bisa di-`disable`/`uninstall` (perluasan PRD-000 FR-19, ditolak dengan `module_required`).
- Modul `core` mengakses data identity/authz **hanya** lewat antarmuka admin di SDK (`sdk/identity.Admin`, `sdk/authz.Admin`). Ia tidak memiliki tabel identity sendiri.

## 4. Model data (ringkas)

Mengikuti Blueprint §3.1: identitas global di **control DB**, hak akses bisnis per tenant di **tenant DB**. Pada mode single-tenant keduanya database fisik yang sama.

| DB | Tabel | Isi |
| --- | --- | --- |
| control | `users` | `id`, `email` (unik, ternormalisasi), `name`, `status` (`active`/`disabled`/`locked`), `password_hash` (nullable untuk user SSO-only), `email_verified_at`, `last_login_at` |
| control | `user_identities` | Tautan ke IdP eksternal: `provider_id`, `subject`, `email`, `linked_at` |
| control | `identity_providers` | Konfigurasi OIDC: issuer, client id, secret (terenkripsi), scopes, mapping klaim, kebijakan provisioning |
| control | `auth_sessions` | Sesi per perangkat/klien: `id` (= `sid`), `user_id`, `client_kind` (`web`/`mobile`/`desktop`), `user_agent`, `ip`, `created_at`, `last_seen_at`, `revoked_at`, `revoke_reason` |
| control | `refresh_tokens` | Hash token, `session_id`, `family_id`, `parent_id`, `used_at`, `expires_at` (rotasi & deteksi reuse) |
| control | `personal_access_tokens` | Hash, prefix tampilan, `user_id`, `name`, `permissions[]` (subset), `expires_at`, `last_used_at`, `revoked_at` |
| control | `oauth_clients` | Client integrasi: `client_id`, hash secret, `service_user_id`, `allowed_grants`, `allowed_scopes`, `status` |
| control | `signing_keys` | Kunci penandatangan JWT (privat terenkripsi), `kid`, `status` (`active`/`retiring`/`retired`) |
| control | `tenant_memberships` | `user_id`, `tenant_id`, `status` (sesuai Blueprint; satu baris tenant default saat ini) |
| control | `user_mfa_factors` | `user_id`, `type` (`totp`), secret terenkripsi, `confirmed_at`, `last_used_step` (anti-replay) |
| control | `user_recovery_codes` | `user_id`, hash kode, `used_at` |
| control | `user_preferences` | `user_id`, `locale`, `timezone` |
| tenant | `org_units` | `id`, `parent_id`, `code`, `name`, `kind` (`company`/`branch`/`department`/kustom), `path` (materialized path untuk query hierarki), `active` |
| tenant | `roles` | `key`, `label`, `description`, `kind` (`system`/`module_default`/`custom`), `source_module`, `customized` |
| tenant | `role_permissions` | Baris matriks: `role_id`, `resource`, `actions[]`, `scope` (jsonb, §6.4) |
| tenant | `role_field_access` | `role_id`, `resource`, `field_group`, `access` (`none`/`read`/`write`) |
| tenant | `user_roles` | `user_id`, `role_id` |
| tenant | `user_scope_assignments` | `user_id`, `dimension` (mis. `inventory.warehouse`), `values[]` |
| tenant | `settings` | `scope` (`platform`/`module`), `module_key`, `key`, `value` (jsonb), `encrypted` |
| tenant | `audit_log` | Append-only (§8) |
| tenant | `authz_state` | `permission_revision` (naik setiap perubahan role/assignment) |

## 5. Kebutuhan fungsional — Identity & API access

### FR-101 — Akun & login lokal

- Password di-hash dengan **argon2id** (parameter dapat dikonfigurasi; default mengikuti rekomendasi OWASP terkini). Kebijakan password: panjang minimum 12, cek terhadap daftar password umum, tanpa aturan komposisi yang memaksa.
- **Rate limit & lockout**: per akun dan per IP (mis. 5 gagal → jeda bertingkat; 20 gagal → `locked` sampai di-unlock admin atau via reset). Pesan error login tidak membedakan "email tidak ada" dan "password salah".
- **Reset password** lewat tautan email sekali pakai (berlaku 30 menit, hash disimpan), dikirim memakai mailer FR-108.
- **Admin pertama** dibuat lewat command modul: `erpora core:create-admin --email ...` (PRD-000 FR-22.4). Tidak ada endpoint setup publik.
- Endpoint: `POST /api/core/auth/login`, `POST /api/core/auth/logout`, `POST /api/core/auth/password/forgot`, `POST /api/core/auth/password/reset`, `POST /api/core/auth/password/change`. Semua masuk allowlist route publik PRD-000 kecuali `change` dan `logout`.

**AC:** brute-force simulasi terkena lockout; timing respons login untuk email ada/tidak ada tidak berbeda signifikan; password lama tidak berlaku setelah reset, dan semua sesi lain dicabut.

### FR-107 — MFA (TOTP)

- TOTP (RFC 6238, 30 detik, 6 digit) dengan aplikasi authenticator. Pendaftaran lewat QR code, wajib dikonfirmasi dengan satu kode sebelum aktif.
- **Wajib untuk user yang memiliki role Administrator**: setelah login password, user tersebut diarahkan ke pendaftaran MFA dan tidak bisa mengakses apa pun sebelum selesai. Opsional untuk user lain; admin dapat mewajibkannya per role lewat settings.
- 10 **recovery code** sekali pakai (hash disimpan), ditampilkan sekali saat pendaftaran dan dapat dibuat ulang.
- Anti-replay: kode yang sama tidak bisa dipakai dua kali dalam jendela waktunya (`last_used_step`).
- Rate limit verifikasi kode mengikuti FR-101 (lockout).
- User SSO tidak diminta TOTP ERP; MFA mengikuti kebijakan IdP. Klaim `amr` dari IdP dicatat di sesi.
- Admin dapat **me-reset MFA** user lain (tercatat di audit). Administrator terakhir hanya bisa di-reset lewat `erpora core:reset-mfa` di server.
- Klaim `amr` access token berisi `pwd`+`otp` setelah MFA. Endpoint sensitif (mis. mengubah matriks role, membuat OAuth client) dapat mensyaratkan MFA lewat opsi `RequireMFA` di `Registrar.Handle`.

**AC:** Administrator tanpa MFA tidak bisa memanggil API apa pun selain pendaftaran MFA dan logout; kode replay ditolak; recovery code hanya berlaku sekali; reset MFA oleh admin tercatat di audit.

### FR-108 — Mailer minimal

- `sdk/mail.Send(ctx, mail.Message{To, Subject, Text, HTML})` dengan dua driver: **`smtp`** (STARTTLS/TLS, autentikasi) dan **`log`** (menulis email ke log; default di `ERP_ENV=development`).
- Konfigurasi SMTP lewat settings platform (password bertipe `secret`).
- Pengiriman sinkron dengan timeout dan retry singkat. Antrean, template engine, dan pelacakan pengiriman ikut PRD Notifications.
- Template fase ini hanya untuk reset password dan undangan user, dalam bahasa sesuai preferensi user (FR-120).

**AC:** reset password terkirim lewat SMTP (diuji dengan container MailHog/Mailpit); mode `log` tidak membuka koneksi jaringan; password SMTP tidak muncul di log.

### FR-102 — Token: JWT access + refresh berotasi

| Token | Bentuk | Umur default | Penyimpanan di klien |
| --- | --- | --- | --- |
| Access token | JWT ditandatangani **EdDSA (Ed25519)** | 10 menit | **Memori saja** (web); secure storage OS (mobile/desktop) |
| Refresh token | String acak opaque (bukan JWT), hash disimpan | 14 hari, sliding; absolut 90 hari | **Cookie `HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh`** (web); secure storage OS (mobile/desktop) |

- Klaim access token: `iss`, `sub` (user id), `sid` (session id), `tid` (tenant id), `aud`, `exp`, `iat`, `jti`, `perm_rev`, `amr` (metode login), `scope` (untuk PAT/OAuth client). **Daftar permission tidak dimasukkan ke JWT** (ukuran dan risiko basi), dan dievaluasi di server dari cache (§6.7).
- **Rotasi refresh token** setiap dipakai. Pemakaian ulang token lama (*reuse*) → seluruh *family* dan sesinya dicabut, dan dicatat sebagai event audit keamanan.
- **Pencabutan instan**: middleware memeriksa `sid` terhadap **daftar sesi tercabut di memori**, bukan query database per request. Daftar ini hanya berisi sesi tercabut yang access token-nya belum kedaluwarsa (≤ umur access token), sehingga tetap kecil. Saat horizontal scaling, perubahan disebarkan ke semua instance lewat `StateNotifier` (PRD-000 FR-19.7, `LISTEN/NOTIFY`), dan instance baru memuat daftar awal saat startup. User `disabled`, ganti password, dan "logout semua perangkat" berlaku segera, bukan menunggu token kedaluwarsa.
- **Rotasi kunci**: `erpora core:rotate-keys` membuat kunci baru (`active`). Kunci lama berstatus `retiring` sampai seluruh token bertanda tangan kunci itu kedaluwarsa. Kunci publik diterbitkan di `GET /.well-known/jwks.json`. Kunci privat dienkripsi dengan `ERP_SECRET_KEY`.
- **CSRF**: API memakai header `Authorization: Bearer` sehingga kebal CSRF. Satu-satunya endpoint berbasis cookie (`refresh`) dilindungi `SameSite=Strict` + pemeriksaan header `Origin`.
- **CORS**: default same-origin saja. Origin klien eksternal (mis. app desktop berbasis web) diatur lewat allowlist di settings.
- **Offline PWA**: snapshot bootstrap tetap dapat dibaca. Operasi outbox dikirim ulang setelah refresh token berhasil. Token kedaluwarsa tidak pernah dianggap otorisasi (`docs/architecture/offline-strategy.md` §5).

**AC:** token dengan tanda tangan salah, `kid` tak dikenal, `exp` lewat, atau `sid` tercabut ditolak 401; reuse refresh token mencabut sesi; rotasi kunci tanpa downtime (token lama tetap valid sampai kedaluwarsa); access token tidak pernah ditulis ke `localStorage`/`sessionStorage` (diuji E2E dengan inspeksi storage).

### FR-103 — SSO via OIDC (Google, Microsoft, IdP perusahaan)

- Satu implementasi **OIDC generik** (Authorization Code + PKCE + `state` + `nonce`) dengan *preset* untuk **Google** dan **Microsoft Entra ID**. IdP perusahaan (Keycloak, Okta, Authentik, ADFS-OIDC, dsb.) dikonfigurasi lewat issuer discovery (`/.well-known/openid-configuration`).
- Konfigurasi per provider: nama tampilan, issuer, client id/secret, scopes, **domain email yang diizinkan**, mapping klaim → nama/email, dan **kebijakan provisioning**:
  - `link_only`: hanya user yang sudah ada (dicocokkan lewat email terverifikasi) yang bisa login.
  - `auto_create`: user baru dibuat otomatis dengan role default yang ditentukan (JIT provisioning).
- Penautan akun berdasarkan email **hanya** bila IdP menyatakan `email_verified=true`.
- Opsi per provider: **wajibkan SSO** untuk domain tertentu (password lokal dinonaktifkan untuk user domain tersebut).
- Tombol login muncul di halaman login sesuai provider yang aktif.
- Role user SSO diatur manual di ERP. Mapping grup IdP → role ditunda (Q7).

**AC:** login Google & Keycloak (container di test) berhasil end-to-end; `state`/`nonce` salah ditolak; email tidak terverifikasi tidak ditautkan; domain di luar allowlist ditolak; user `link_only` yang belum ada ditolak dengan pesan jelas.

### FR-104 — Personal Access Token (PAT)

- Dibuat user dari halaman profil: nama, masa berlaku (maks. dapat diatur admin, default 90 hari), dan **subset permission** (tidak bisa melebihi hak user; hak efektif = irisan hak user saat ini dan subset token).
- Format `erp_pat_<random>`, ditampilkan **sekali**, disimpan sebagai hash. Prefix memudahkan secret scanner (mis. GitHub) mendeteksi kebocoran.
- Dipakai sebagai `Authorization: Bearer erp_pat_...`. Platform menukarnya secara internal menjadi konteks principal (bukan JWT).
- Admin dapat melihat dan mencabut PAT semua user; user dapat mencabut miliknya.

**AC:** PAT dengan subset `inventory.item.read` ditolak saat `update`; PAT milik user yang di-disable langsung ditolak; `last_used_at` diperbarui.

### FR-105 — OAuth2 client credentials (integrasi sistem)

- Endpoint token `POST /oauth/token` (RFC 6749 §4.4). Client diautentikasi dengan `client_secret_basic` (fase ini); `private_key_jwt` dicadangkan.
- Setiap OAuth client terikat ke **service user** (user bertipe `service`, tanpa login interaktif). Hak aksesnya diatur dengan role biasa, sehingga admin memakai matriks yang sama.
- Access token client credentials: JWT yang sama (FR-102) dengan `sub` = service user dan `scope` = permission yang diminta ∩ diizinkan. Tanpa refresh token.
- Rate limit per client dapat diatur.
- Authorization Code + PKCE untuk aplikasi mobile/desktop pihak pertama ditunda sampai aplikasi tersebut dibuat (Q3). Tabel `auth_sessions.client_kind` dan FR-102 sudah mendukung klien `mobile`/`desktop`.

**AC:** client yang dicabut/di-disable ditolak; scope melebihi izin dipangkas; token client tidak dapat dipakai di endpoint `refresh`.

### FR-106 — Principal & konteks request

- Setelah autentikasi, middleware membangun **`Principal`** (`user_id`, `kind` = `human`/`service`, `session_id`, `auth_method` = `password`/`oidc`/`pat`/`client_credentials`, `token_scope`) dan menaruhnya di context. Modul membacanya lewat `sdk/identity.FromContext(ctx)`.
- Middleware stub PRD-000 (authN, permission check) diganti tanpa mengubah modul (PRD-000 FR-05).
- `actor` di audit dan lifecycle modul memakai `Principal` (menggantikan `api:dev`).
- `/api/bootstrap` kini mensyaratkan login, dan mengembalikan `user`, ringkasan permission (§6.8), serta `perm_rev`.

## 6. Kebutuhan fungsional — Authorization

### 6.1. Konsep

```
Resource  : sesuatu yang dilindungi, dideklarasikan modul      (inventory.item)
Action    : apa yang dilakukan terhadap resource               (read, create, submit, approve…)
Permission: <resource>.<action>                                (inventory.item.update)
Role      : kumpulan baris matriks (permission + scope) + akses grup field
Scope     : batas DATA tempat permission berlaku              (milik sendiri, gudang WH-01, …)
Field group: kelompok field bernama dengan akses none/read/write
```

**Aturan evaluasi tunggal:** hak efektif user = **union** dari semua baris semua role-nya. **Tidak ada deny.**

### FR-110 — Deklarasi resource, aksi, dan field group di manifest

Perluasan `contracts/module.schema.json` (aditif; `schema_version` tetap `1`):

```jsonc
"resources": [{
  "key": "inventory.item",
  "label": "Barang",
  "actions": ["read", "create", "update", "delete", "import", "export", "print"],
  "custom_actions": [{ "key": "adjust_price", "label": "Ubah Harga Massal" }],
  "owner_field": "created_by",                       // dipakai scope "owner"; default created_by
  "scope_fields": { "inventory.warehouse": "warehouse_id" },   // field → dimensi scope
  "field_groups": [
    { "key": "general",  "label": "Umum",            "fields": ["name", "sku", "uom"], "default": true },
    { "key": "pricing",  "label": "Informasi Harga", "fields": ["cost_price", "sale_price"] },
    { "key": "profit",   "label": "Profitabilitas",  "fields": ["margin", "markup"] }
  ],
  "audit": { "data_changes": true }                  // FR-131
}],
"scope_dimensions": [
  { "key": "inventory.warehouse", "label": "Gudang", "source": "inventory.warehouse" }
],
"default_roles": [
  { "key": "inventory.manager", "label": "Manajer Gudang", "permissions": [ /* baris matriks */ ] }
],
"settings": { /* FR-120 */ }
```

**Aksi standar** (disarankan dipakai sebelum membuat aksi kustom, agar konsisten antarmodul):

| Aksi | Makna | Aksi | Makna |
| --- | --- | --- | --- |
| `read` | Melihat record & daftar | `submit` | Mengunci draft menjadi dokumen sah |
| `create` | Membuat record | `cancel` | Membatalkan dokumen yang sudah submit |
| `update` | Mengubah record | `amend` | Membuat revisi dari dokumen yang dibatalkan |
| `delete` | Menghapus (umumnya hanya draft) | `approve` | Menyetujui dokumen |
| `print` | Mencetak / PDF | `import` | Impor data massal |
| `export` | Ekspor data | `share` | Membagikan record ke user lain |
| `report` | Mengakses laporan resource | `email` | Mengirim record via email |

- Aksi kustom berpola `[a-z_]+`, tidak boleh bentrok dengan aksi standar, dan wajib punya label.
- Registry memvalidasi: setiap `Registrar.Handle(..., permission)` harus merujuk `<resource>.<action>` yang dideklarasikan (menggantikan cek `capabilities` PRD-000; `capabilities` tetap diterima sebagai turunan otomatis untuk kompatibilitas).
- Field yang tidak tercantum di grup mana pun masuk grup `default` resource tersebut. Setiap resource wajib punya tepat satu grup `default`.

**AC:** manifest dengan aksi kustom bentrok, `scope_fields` merujuk dimensi tak terdaftar, atau resource tanpa grup default ditolak dengan pesan yang jelas.

### FR-111 — Scope data (record-level)

Setiap baris matriks memiliki scope. Admin memilih dari tipe berikut:

| Tipe scope | Arti untuk admin | Evaluasi |
| --- | --- | --- |
| `all` | Semua data | Tanpa filter |
| `owner` | **Hanya yang dibuat sendiri** (onlyCreator) | `owner_field = principal.user_id` |
| `assigned:<dimensi>` | **Sesuai penugasan user**, mis. "gudang yang ditugaskan" | `scope_fields[dimensi] IN user_scope_assignments.values` |
| `custom:<key>` | Scope khusus yang disediakan modul, mis. "dokumen yang ditugaskan ke saya", "departemen saya" | `ScopeProvider` dari modul (GORM scope + pemeriksaan record) |

- Dalam satu baris, beberapa kondisi digabung dengan **AND** ("milik sendiri **dan** di gudang saya"). Antarbaris/antarrole digabung dengan **OR** (union).
- Modul mendaftarkan scope kustom lewat `Registrar.Scope(sdk.ScopeProvider{Key, Label, Filter, Match})`. `Filter` menghasilkan GORM scope untuk query daftar; `Match` memeriksa satu record.
- **Penugasan dimensi ke user** (`user_scope_assignments`) dikelola admin di halaman user. Nilai yang bisa dipilih diambil dari `source` dimensi (lookup ke modul pemilik lewat `contract/`).
- Untuk aksi `create`, scope diperiksa terhadap **data yang akan dibuat** (mis. staf gudang WH-01 tidak bisa membuat barang di WH-02).

**AC:** tabel uji kombinasi: owner saja; assigned saja; owner AND assigned; dua role dengan scope berbeda (union); scope kustom dari modul `example`; `create` di luar scope ditolak; user tanpa assignment untuk dimensi yang disyaratkan tidak melihat data apa pun (gagal tertutup).

### FR-112 — Akses grup field (field-level)

- Per role dan per resource, setiap grup field diberi akses `none` / `read` / `write`. Grup `default` mengikuti aksi resource (punya `read` → minimal read; punya `update` → write).
- **Saat membaca**: field dalam grup `none` **dihapus dari respons** oleh helper SDK (`authz.MaskFields`). Field tidak sekadar dikosongkan, agar tidak membocorkan keberadaan nilai.
- **Saat menulis**: perubahan pada field yang tidak `write` **ditolak 403** `field_forbidden` dengan daftar field yang ditolak. Tidak diabaikan diam-diam, karena integrasi API harus tahu bahwa perubahannya tidak tersimpan.
- Query daftar, ekspor, impor, print, dan laporan **wajib** memakai helper yang sama, termasuk sorting dan filter: filter berdasarkan field `none` ditolak agar nilai tidak bisa ditebak lewat filter.

**AC:** user tanpa akses grup `profit` tidak menerima `margin` di read, list, export, dan print; filter `?margin_gt=` ditolak; update `cost_price` dengan akses `read` → 403 dengan daftar field.

### FR-118 — Unit organisasi minimal (dimensi scope bawaan)

- Hierarki **unit organisasi** di modul `core`: pohon bebas kedalaman dengan `kind` standar `company`, `branch`, `department`, ditambah jenis kustom yang dapat dibuat admin (mis. "Divisi", "Wilayah").
- Didaftarkan sebagai dimensi scope bawaan **`core.org_unit`** dengan dua mode penugasan:
  - **Unit itu saja**: user ditugaskan ke Cabang Bandung → hanya data Cabang Bandung.
  - **Unit beserta turunannya**: user ditugaskan ke Wilayah Jawa Barat → data semua cabang di bawahnya. Query memakai `path` (materialized path) agar cepat.
- Resource modul memetakan field-nya ke dimensi ini lewat `scope_fields` (mis. `"core.org_unit": "branch_id"`).
- Modul lain merujuk unit organisasi lewat `modules/core/contract` (D1), bukan foreign key lintas modul (Blueprint §3.3).
- UI: pohon unit organisasi (tambah, pindah, nonaktifkan) dan penugasan unit di halaman user. Unit yang masih dirujuk data tidak bisa dihapus, hanya dinonaktifkan.

**AC:** user yang ditugaskan ke node induk dengan mode "beserta turunan" melihat data semua cabang di bawahnya; memindahkan cabang ke induk lain langsung mengubah hasil scope; unit nonaktif tetap terlihat di data historis.

### FR-113 — Role: sistem, bawaan modul, dan kustom

| Jenis | Contoh | Bisa diubah admin? |
| --- | --- | --- |
| `system` | **Administrator** (semua izin, termasuk modul baru), **Authenticated** (hak dasar semua user login) | Administrator tidak bisa diubah. User Administrator terakhir tidak bisa dicabut/di-disable |
| `module_default` | `inventory.manager`, `inventory.staff` dari `default_roles` | Ya. Setelah diubah ditandai `customized` |
| `custom` | "Kasir Cabang Bandung" | Ya, sepenuhnya; bisa dibuat dengan menduplikasi role lain |

- Saat modul di-install/upgrade: role bawaan baru ditambahkan, dan role bawaan yang **belum** `customized` diperbarui mengikuti manifest. Role `customized` **tidak disentuh**; admin diberi notifikasi "ada izin baru dari modul X yang belum diberikan".
- Saat modul di-disable, baris matriks resource-nya tetap tersimpan tetapi tidak berlaku dan tampil abu-abu di UI.

### FR-114 — UI administrasi hak akses (modul `core`)

1. **Matriks role**: baris = resource (dikelompokkan per modul), kolom = aksi yang dideklarasikan resource tersebut, plus kolom **Scope** (pemilih tipe scope) dan panel **Grup field** yang dapat dibuka untuk melihat daftar field di dalamnya.
2. **Halaman user**: role yang dimiliki, penugasan dimensi (gudang, cabang, …), PAT, sesi aktif (dengan tombol cabut).
3. **Hak efektif**: matriks gabungan semua role user, dengan keterangan role mana yang memberi setiap izin.
4. **Penjelas akses**: "Kenapa *Budi* tidak bisa *update* *Barang BRG-001*?" → jawaban per lapis (aksi / scope / field) dan saran role yang akan memberi izin.
5. **Perbandingan role**: dua role berdampingan, perbedaan disorot.
6. Semua perubahan tercatat di audit (FR-130) dan menaikkan `permission_revision`.

**AC (E2E):** admin membuat role baru dari duplikat, mengubah scope ke "milik sendiri", menyembunyikan grup "Profitabilitas", meng-assign ke user; user tersebut langsung (tanpa login ulang) melihat perubahan setelah `perm_rev` berubah.

### FR-115 — SDK otorisasi untuk modul (`sdk/authz`)

```go
// Lapis aksi — umumnya otomatis oleh Registrar.Handle(method, path, permission, h)
authz.Require(ctx, "inventory.item.update") error

// Lapis record
authz.Filter(ctx, "inventory.item", "read") func(*gorm.DB) *gorm.DB   // untuk query daftar
authz.Check(ctx, "inventory.item.update", record) error                // untuk satu record

// Lapis field
authz.Fields(ctx, "inventory.item") FieldAccess                        // grup → none/read/write
authz.MaskFields(ctx, "inventory.item", dto) any                       // untuk respons
authz.CheckWrite(ctx, "inventory.item", changedFields []string) error  // untuk update
```

- Middleware `Registrar.Handle` memeriksa lapis aksi **tanpa** data record. Aturan bersyarat (owner/scope) membuat lolos middleware hanya berarti "punya izin di sebagian data"; handler **wajib** memanggil `Filter`/`Check`.
- Generator `erpora make:handler` dan `make:repository` (PRD-000 FR-22) menghasilkan kode yang sudah memanggil `Filter`, `Check`, `MaskFields`, dan `CheckWrite`.
- Arch/lint test: handler di `presentation/http` yang memanggil repository tanpa melalui helper authz diberi peringatan oleh analyzer kustom (`go vet` analyzer) sebagai pengaman tambahan.

### FR-119 — Pencegahan IDOR (akses objek tanpa otorisasi)

IDOR dicegah di **setiap titik masuk** yang menerima ID objek, bukan hanya di endpoint HTTP.

**1. Endpoint HTTP**
- Setiap handler yang menerima ID (path, query, body, termasuk ID di baris dokumen dan referensi seperti `warehouseId`) **wajib** memuat record lewat `authz.Check` / query ber-`authz.Filter`. Memuat record dengan `db.First(&x, id)` tanpa filter otorisasi dilarang.
- ID **referensi** di body (mis. `warehouseId` saat membuat penyesuaian stok) juga diperiksa: user harus boleh `read` record yang dirujuk **dan** referensi itu harus berada dalam scope-nya. Kalau tidak, respons `422 validation_failed` dengan `code: not_found` pada field tersebut, sehingga keberadaan record tidak terungkap.
- Record di luar scope → `404 not_found` (api-guidelines §8).

**2. Kontrak antarmodul (mencegah *confused deputy*)**
- Context yang diteruskan ke `contract/` **selalu** membawa `Principal` dan tenant pemanggil asli (FR-106). Modul pemanggil **tidak boleh** mengganti, membuang, atau membuat principal.
- **Modul penyedia kontrak menegakkan otorisasi di implementasi kontraknya** dengan `authz.Check`/`authz.Filter` terhadap principal dari context. Modul penyedia tidak mempercayai bahwa pemanggil sudah memeriksa.
- Parameter kontrak **tidak boleh** berisi `tenantId`, `userId`, atau penanda hak akses dari pemanggil. Identitas hanya berasal dari context.
- Operasi yang memang harus berjalan dengan hak sistem (mis. posting jurnal otomatis dari Inventory ke Finance) memakai **`authz.AsSystem(ctx, reason string)`** secara eksplisit:
  - hanya boleh dipanggil dari kode modul (bukan dari input pengguna), dengan `reason` yang jelas;
  - setiap pemakaian tercatat di audit (`actor` = principal asli, `metadata.system_reason`);
  - kontrak yang menerima konteks sistem mendeklarasikannya di dokumentasi kontrak (`// erp:system-call-allowed`), dan arch test memastikan `AsSystem` hanya muncul di paket `internal/application` modul.
- Event asinkron (outbox) membawa principal pemicu dan tenant. Konsumen memeriksa ulang otorisasi atau memakai `AsSystem` dengan alasan yang tercatat.

**3. Penegakan otomatis**
- **Analyzer `go vet` kustom (FR-115)** diperluas: memberi error bila handler/implementasi kontrak memanggil method repository yang menerima ID tanpa melewati `authz.Check`/`authz.Filter`, dan bila `AsSystem` dipanggil di luar `internal/application`.
- **Generator** (`erpora make:handler`, `make:repository`, `make:contract`) menghasilkan kode yang sudah memanggil helper otorisasi, beserta **test IDOR** default.
- **Test IDOR wajib** untuk setiap endpoint dan setiap metode kontrak yang menerima ID, dengan dua user fixture (A dan B) di scope berbeda:
  - B membaca/mengubah/menghapus/menjalankan aksi pada record A → 404;
  - B memakai ID record A sebagai referensi di body → 422 `not_found`;
  - B memanggil kontrak (lewat modul lain) dengan ID record A → error `not_found`;
  - list/ekspor/print B tidak pernah memuat record A, termasuk lewat filter dan `expand`.
- Security test (PRD-000 FR-14) gagal bila ada endpoint di `Registry.Routes()` yang menerima parameter ID tanpa test IDOR terdaftar.

**AC:** modul `example` dan `exampleaddon` lulus semua skenario test IDOR di atas, termasuk akses lewat kontrak `example/contract` dari `exampleaddon`; analyzer menolak fixture handler yang memuat record tanpa authz; `AsSystem` tanpa `reason` atau di luar `internal/application` ditolak.

### FR-116 — Performa & caching

- Hak efektif per `(user, perm_rev)` dikompilasi menjadi struktur siap evaluasi dan di-cache di memori (LRU, invalidasi saat `permission_revision` naik).
- Target: `Require` ≤ 20 µs p99 (cache hit); `Filter` menambah klausa `WHERE` yang dapat memakai indeks. Generator migrasi menambahkan indeks untuk `owner_field` dan `scope_fields`.

### FR-117 — Ringkasan permission untuk frontend

`/api/bootstrap` mengembalikan per resource: aksi yang dimiliki (dengan penanda `conditional` bila bergantung scope), akses per grup field, dan `perm_rev`. Frontend menyembunyikan tombol/kolom/field sesuai ringkasan ini. Hanya UX; backend tetap memeriksa.

## 7. Kebutuhan fungsional — Settings

### FR-120 — Settings berbasis schema

- Modul mendeklarasikan settings di manifest (`settings.fields[]`): `key`, `label`, `type` (`string`/`number`/`boolean`/`enum`/`json`/`secret`/`reference`), `default`, `validation`, `group`, `description`, `restart_required`.
- Platform mendeklarasikan settings-nya sendiri dengan format yang sama (mis. kebijakan password, umur token, allowlist CORS, provider SSO).
- Tingkatan: **platform** dan **modul**, per tenant.
- **Preferensi user** (tabel `user_preferences`): **bahasa** (`id`/`en`, default dari settings platform) dan **zona waktu** (IANA, mis. `Asia/Jakarta`). Diatur di halaman profil, dikembalikan di `/api/bootstrap`, dan dipakai frontend untuk format tanggal serta mailer (FR-108). Preferensi lain menyusul.
- Tipe `secret` dienkripsi (AES-256-GCM, kunci dari `ERP_SECRET_KEY`), tidak pernah dikembalikan ke API (hanya status "terisi/belum"), dan disamarkan di audit.
- SDK: `settings.Get[T](ctx, key)` bertipe, dengan cache + `settings_revision`. Perubahan memicu callback opsional `OnSettingsChanged` di modul.
- Izin: `core.settings.read`, `core.settings.update` per modul (resource `core.settings` dengan scope `custom:module`).
- **UI**: formulir dibangkitkan otomatis dari schema, dikelompokkan per modul & grup, dengan validasi yang sama di frontend dan backend (schema dari `contracts/`).

**AC:** nilai tidak valid ditolak di backend walau UI dilewati; secret tidak muncul di respons API, log, maupun audit; perubahan langsung terbaca oleh `settings.Get` tanpa restart (kecuali `restart_required`, yang ditandai di UI).

## 8. Kebutuhan fungsional — Audit

### FR-130 — Audit log platform

- Tabel `audit_log` **append-only**: aplikasi hanya punya izin `INSERT`/`SELECT` (dipaksa dengan trigger yang menolak `UPDATE`/`DELETE`).
- Kolom: `id`, `tenant_id`, `occurred_at`, `actor_user_id`, `actor_kind`, `auth_method`, `session_id`, `ip`, `user_agent`, `module_key`, `resource`, `record_id`, `action`, `outcome` (`success`/`denied`/`failed`), `changes` (jsonb), `metadata`, `trace_id`.
- Event yang **wajib** tercatat otomatis:
  - Autentikasi: login berhasil/gagal, logout, lockout, reset password, SSO link, refresh token reuse.
  - Token: pembuatan/pencabutan PAT, OAuth client, rotasi kunci.
  - Hak akses: perubahan role, matriks, field group, assignment, role user.
  - Settings: perubahan nilai (secret disamarkan).
  - Lifecycle modul: dicerminkan dari `module_lifecycle_events` (PRD-000).
  - **Penolakan akses** (403) pada aksi selain `read` (dibatasi laju agar tidak membanjiri).
- SDK `audit.Record(ctx, audit.Event{...})` untuk event bisnis modul (mis. "Dokumen disubmit").
- Retensi diatur lewat settings (default 365 hari); pembersihan oleh command terjadwal `erpora core:audit-prune` dan dicatat sebagai event audit.

### FR-131 — Audit perubahan data (opt-in per resource)

- Resource dengan `audit.data_changes: true` dicatat otomatis oleh plugin GORM: before/after **hanya untuk field yang berubah**.
- Nilai field dari grup yang ditandai `sensitive` di manifest disamarkan di `changes`.
- Viewer audit menerapkan field-level access yang sama (FR-112): user yang tidak boleh melihat `margin` juga tidak melihat perubahan `margin` di riwayat.

### FR-132 — Viewer audit

- Halaman audit dengan filter (waktu, actor, modul, resource, record, action, outcome), detail diff, dan tautan ke trace OTel (`trace_id`).
- Tab **Riwayat** di halaman record mana pun (komponen frontend reusable).
- Ekspor butuh izin `core.audit.export`.

**AC:** `UPDATE audit_log` langsung di DB ditolak trigger; semua event wajib muncul di skenario E2E; diff menyamarkan field sensitif.

## 9. Kebutuhan non-fungsional

| ID | Kebutuhan |
| --- | --- |
| NFR-101 | Target keamanan **OWASP ASVS 5.0.0 Level 2** untuk autentikasi (V6), sesi (V7), otorisasi (V8), token (V9), dan OAuth/OIDC (V10); ditambah kontrol L3 terpilih untuk aksi kritis keuangan (mis. re-autentikasi saat approve pembayaran) |
| NFR-102 | `Require` ≤ 20 µs p99 (cache hit); validasi JWT ≤ 100 µs p99 |
| NFR-103 | Tidak ada rahasia (password, token, secret settings, refresh token) yang tercatat di log, trace, atau audit. Diuji dengan pemindai log di integration test |
| NFR-104 | Variabel baru `ERP_SECRET_KEY` (wajib, 32 byte base64). Startup gagal bila tidak ada/terlalu pendek. Prosedur rotasinya didokumentasikan di `deployment.md` |
| NFR-105 | Semua endpoint auth & token terkena rate limit (per IP dan per principal) dengan respons `429` + `Retry-After` |
| NFR-106 | **Gagal tertutup**: error saat evaluasi permission/scope (mis. cache rusak, scope provider panic) → akses ditolak, bukan diizinkan |

## 10. Milestone

| Milestone | Cakupan | Exit gate |
| --- | --- | --- |
| **C1 — Identity lokal** | FR-101, FR-102, FR-106, FR-107, FR-108, modul `core` (`required`), `erpora core:create-admin` | Login/logout/refresh/reset end-to-end; MFA wajib Administrator berjalan; stub authN PRD-000 diganti; ASVS checklist sesi lolos |
| **C2 — Authorization** | FR-110–FR-119 | Semua tabel uji scope & field lulus (termasuk hierarki unit organisasi); matriks & penjelas akses di UI; modul `example` memakai resource, scope kustom, dan field group |
| **C3 — Settings & Audit** | FR-120 (termasuk preferensi user), FR-130–FR-132 | Settings modul `example` diatur dari UI; semua event wajib tercatat; audit append-only terbukti |
| **C4 — Integrasi** | FR-103, FR-104, FR-105 | SSO Google + Keycloak (test container) berjalan; PAT & client credentials dipakai oleh skrip integrasi di CI |

## 11. Keputusan

| # | Pertanyaan | Keputusan |
| --- | --- | --- |
| Q1 | Level field? | ✅ **Grup field bernama** didefinisikan modul, daftar field terlihat di UI (FR-110, FR-112) |
| Q2 | Admin memindahkan field / membuat grup? | ✅ Tidak di fase ini; dievaluasi setelah ada umpan balik pengguna |
| Q3 | Authorization Code + PKCE (mobile/desktop)? | ✅ Ditunda sampai aplikasinya dibuat; fondasi token sudah mendukung |
| Q4 | Pengiriman email? | ✅ Mailer minimal SMTP + mode `log` (FR-108) |
| Q5 | Preferensi user? | ✅ Bahasa & zona waktu (FR-120) |
| Q6 | Struktur organisasi? | ✅ Unit organisasi minimal sebagai dimensi scope `core.org_unit` (FR-118) |
| Q7 | Mapping grup IdP → role? | ✅ Ditunda; role diatur manual |
| Q8 | LDAP / AD langsung? | ✅ Ditunda; gunakan IdP OIDC sebagai jembatan |
| Q9 | MFA? | ✅ TOTP, wajib untuk Administrator, opsional untuk lainnya; user SSO mengikuti IdP (FR-107) |
| Q10 | Metode autentikasi? | ✅ JWT access + refresh berotasi **untuk semua klien termasuk web** (FR-102), SSO OIDC (FR-103), PAT (FR-104), OAuth2 client credentials (FR-105). Alternatif session cookie untuk web (pola ERPNext/Odoo) dipertimbangkan dan **ditolak** dengan alasan kemudahan horizontal scaling: verifikasi token cukup dengan kunci publik + daftar pencabutan in-memory per instance, tanpa store sesi bersama di jalur request. Dicatat di ADR-013 |
| Q11 | Model otorisasi? | ✅ Matriks per role, union tanpa deny, tiga lapis aksi/record/field, aksi standar + kustom, scope generik (FR-110–FR-117) |
| Q13 | Dependensi antarmodul & IDOR? | ✅ Dependensi **eksplisit dan ketat** (manifest + `go.mod` + `package.json`, akses hanya lewat `contract/`/`index.ts`); IDOR dicegah di endpoint **dan** di kontrak antarmodul — modul penyedia kontrak menegakkan otorisasi terhadap principal asli, hak sistem hanya lewat `authz.AsSystem` yang teraudit (FR-119) |
| Q12 | Penempatan Core? | ✅ Hibrida: mesin di platform, administrasi di modul `core` yang `required` (§3) |

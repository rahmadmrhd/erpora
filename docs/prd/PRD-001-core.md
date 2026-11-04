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
| control | `signing_keys` | Kunci penandatangan JWT (privat terenkripsi), `kid`, `status` (`pending`/`active`/`retiring`/`retired`; `pending` = sudah dipublikasikan di JWKS tetapi belum dipakai menandatangani, perluasan aditif ADR-0013 §3 per 3 Nov 2026) |
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
| tenant | `settings` | `scope` (`platform`/`module`), `module_key` (kosong untuk `platform`), `key`, `value` (jsonb), `encrypted`, `updated_by`, `created_at`, `updated_at`; UNIQUE (`scope`, `module_key`, `key`). **Subset sudah dibangun di C1** (migrasi tenant `0004_settings`, Q24) untuk kunci platform `platform.public_url`; C3 memperluasnya (FR-120) |
| tenant | `audit_log` | Append-only (§8) |
| tenant | `authz_state` | `permission_revision` (naik setiap perubahan role/assignment) |

## 5. Kebutuhan fungsional — Identity & API access

### FR-101 — Akun & login lokal

- Password di-hash dengan **argon2id** (parameter dapat dikonfigurasi; default mengikuti rekomendasi OWASP terkini). Password dinormalisasi ke **Unicode NFKC** sebelum di-hash, diverifikasi, dan diperiksa kebijakan (keputusan pemilik produk 3 Nov 2026, Q17). Kebijakan password: panjang minimum 12 karakter (rune pasca-NFKC), maksimum 1024 byte (diperiksa pada masukan mentah sebelum normalisasi), cek terhadap daftar password umum (pencarian pada bentuk NFKC dan huruf kecil), tanpa aturan komposisi yang memaksa.
- **Rate limit & lockout** (keputusan pemilik produk 3 Nov 2026, Q16): jeda bertingkat **per pasangan (akun, IP)** (5 gagal → jeda 30 detik berlipat dua, maksimum 15 menit; 20 gagal → pasangan diblokir 1 jam; hanya kegagalan dalam **24 jam terakhir** yang dihitung, Q19; sukses mereset hitungan pasangan); status `locked` pada **akun** hanya bila terkumpul ≥ 50 gagal lintas ≥ 5 IP berbeda dalam 1 jam, sampai di-unlock lewat reset password, command `erpora core:unlock --email ...`, atau admin (angka dan mekanisme di ADR-0013 §5). Status `locked` hanya menolak **login baru**; sesi yang sudah ada **tidak** dicabut (Q30 butir 4): pencabutan sesi khusus status `disabled`, reset password, dan aksi admin eksplisit. Penyerang dari satu IP tidak dapat mengunci pemilik akun yang login dari IP lain. Pesan error login tidak membedakan "email tidak ada" dan "password salah".
- **Reset password** lewat tautan email sekali pakai (berlaku 30 menit, hash disimpan), dikirim memakai mailer FR-108. Akun `disabled` **tidak menerima email reset** (jawaban `forgot` tetap seragam, lantai waktu sama), dan tautan milik akun `disabled` ditolak seperti token tidak sah (Q30 butir 5); akun `active` dan `locked` dilayani.
- **Retensi `login_attempts`** (memuat user_id dan IP: data pribadi, UU PDP; Q30 butir 3): proses `serve` memangkas baris lebih tua dari **30 hari** tiap jam lewat pekerja latar; 30 hari aman terhadap jendela lockout 24 jam dan 1 jam. Detail di ADR-0013 §5.
- **Unlock akun**: `erpora core:unlock --email ...` (hak sistem, tercatat di audit; ditambahkan 3 Nov 2026, Q16, di luar teks asli FR-101). Operasi admin tanpa Principal hanya diterima untuk hak sistem berawalan `cli:core:` (command CLI `core:*`); alasan lain (hook lifecycle, seed) ditolak `ErrForbidden` dan dicatat `identity.admin_denied` (Q30 butir 1).
- **Admin pertama** dibuat lewat command modul: `erpora core:create-admin --email ...` (PRD-000 FR-22.4). Tidak ada endpoint setup publik. Pembuatan akun, pemberian role, dan aktivasi bukan satu transaksi (control dan tenant DB dapat berbeda), jadi urutannya: akun dibuat **nonaktif**, role diberikan, status `active` **terakhir**; kegagalan di tengah menyisakan akun nonaktif tanpa role. **`CreateAdmin` dapat dilanjutkan (idempoten, Q28):** email yang sudah ada berstatus `disabled`, tanpa role apa pun, `last_login_at` kosong, dan tanpa sesi (termasuk yang tercabut) dilanjutkan: sandi diatur ulang dari masukan baru, role `administrator` diberikan, akun diaktifkan. Selain itu (aktif, berrole, pernah login, atau bersesi) tetap `ErrEmailTaken`, sehingga tidak ada jalur pengambilalihan akun. Seluruh alur diserialkan per email (kunci advisory transaksi PostgreSQL pada email ternormalisasi, batas tunggu 30 detik): dua `CreateAdmin` serentak untuk email yang sama tidak bisa saling melanjutkan; yang kedua menunggu lalu melihat akun aktif atau berrole dan ditolak `ErrEmailTaken`.
- Endpoint: `POST /api/core/auth/login`, `POST /api/core/auth/refresh`, `POST /api/core/auth/logout`, `POST /api/core/auth/password/forgot`, `POST /api/core/auth/password/reset`, `POST /api/core/auth/password/change`. `login`, `refresh`, `forgot`, dan `reset` masuk allowlist route publik PRD-000; `logout` dan `change` terautentikasi saja (`x-erp-authenticated`, Q29 butir 1). Semua respons auth `Cache-Control: no-store` (ADR-0013 §1), dan refresh token hanya lewat cookie. **Konvensi galat (Q31):** login gagal 401 `invalid_credentials`; token/sesi kedaluwarsa 401 `token_expired`; akun terkunci 403 `account_locked`; masukan salah pada request terautentikasi atau berbody (sandi saat ini salah, token reset tidak sah atau kedaluwarsa) 422 `validation_failed` dengan `details[].code` `wrong_current_password` atau `invalid_token`; pelanggaran kebijakan sandi 422 `password_policy_violation`. `Idempotency-Key` diterima tetapi tidak disimpan (Q31).

**AC:** brute-force simulasi dari satu IP terkena jeda dan pemblokiran pasangan (akun, IP) tanpa mengunci akun bagi IP lain; kegagalan lintas ≥ 5 IP melewati ambang menjadikan akun `locked` dan `core:unlock` serta reset password membukanya; timing respons login untuk email ada/tidak ada tidak berbeda signifikan; password lama tidak berlaku setelah reset, dan semua sesi lain dicabut.

### FR-107 — MFA (TOTP)

- TOTP (RFC 6238, 30 detik, 6 digit) dengan aplikasi authenticator. Pendaftaran lewat QR code, wajib dikonfirmasi dengan satu kode sebelum aktif.
- **Wajib untuk user yang memiliki role Administrator**: setelah login password, user tersebut diarahkan ke pendaftaran MFA dan tidak bisa mengakses apa pun sebelum selesai (kecuali `GET /api/bootstrap`, Q29 butir 2). Opsional untuk user lain; admin dapat mewajibkannya per role lewat settings. **User mana pun yang memiliki faktor TOTP aktif tidak boleh lolos hanya dengan password:** sesi yang belum terverifikasi MFA hanya dapat memanggil `mfa/verify`, `mfa/recovery`, dan `logout`; route lain dijawab 403 `mfa_required` (ADR-0013 §4), kecuali `GET /api/bootstrap` (Q29 butir 2).
- 10 **recovery code** sekali pakai (hash disimpan), ditampilkan sekali saat pendaftaran dan dapat dibuat ulang.
- Endpoint (semuanya terautentikasi saja, `x-erp-authenticated`, Q29 butir 1; daftar route yang boleh dipanggil sesi menunggu MFA milik platform, ADR-0013 §4): `POST /api/core/auth/mfa/enroll`, `mfa/confirm`, `mfa/verify`, `mfa/recovery`, dan `mfa/recovery-codes` (membuat ulang 10 recovery code; opsi `RequireMFA`, sehingga sesi harus sudah memuat `otp`). Kode TOTP atau recovery yang salah dijawab 422 `validation_failed` dengan `details[].code` `invalid_code` (Q31). Respons `confirm`, `verify`, dan `recovery` menulis ulang cookie refresh (Q27).
- Anti-replay: kode hanya diterima bila langkah waktunya **lebih besar** dari `last_used_step` (`step > last_used_step`, pembaruan bersyarat atomik); kode yang sama, kode langkah sebelumnya, dan pemakaian serentak ditolak.
- Rate limit verifikasi kode mengikuti FR-101 (jeda per pasangan (akun, IP) dan ambang `locked` lintas IP; Q16).
- User SSO tidak diminta TOTP ERP; MFA mengikuti kebijakan IdP. Klaim `amr` dari IdP dicatat di sesi.
- Admin dapat **me-reset MFA** user lain (tercatat di audit). Administrator terakhir hanya bisa di-reset lewat `erpora core:reset-mfa` di server.
- **Naik ke `otp` merotasi refresh token (keputusan pemilik produk, 3 Nov 2026, Q27):** saat sesi naik dari `amr=["pwd"]` ke `["pwd","otp"]` (konfirmasi pendaftaran MFA, `mfa/verify`, maupun `mfa/recovery`), refresh token pra-MFA **mati** dan token refresh **baru** untuk sesi yang sama diterbitkan; cookie refresh ditulis ulang oleh handler. Pemegang cookie pra-MFA yang dicuri tidak dapat memperoleh access token `otp` (verifikasi yang gagal tidak menghasilkan token apa pun). Pemakaian token pra-MFA yang sudah mati ditolak seragam `401` **tanpa** mencabut sesi (bukan serangan *reuse*: sesi ber-`otp` milik pemilik sah tetap hidup). Rotasi atomik: tepat satu dari beberapa upgrade serentak yang menang; yang kalah tetap mendapat access token `otp` tanpa refresh token baru. Respons `mfa/confirm` kini memuat **access token `otp` baru** di samping recovery code (refresh token baru hanya lewat cookie), sehingga klien **tidak memanggil `refresh` lagi** setelah konfirmasi bila respons memuat token; respons `mfa/verify` dan `mfa/recovery` juga menulis ulang cookie.
- Klaim `amr` access token berisi `pwd`+`otp` setelah MFA. Endpoint sensitif (mis. mengubah matriks role, membuat OAuth client) dapat mensyaratkan MFA lewat opsi `RequireMFA` di `Registrar.Handle`.

**AC (tambahan Q27):** refresh token pra-MFA ditolak (`401`, sesi tidak tercabut) setelah sesi naik ke `otp`; cookie baru bekerja; upgrade serentak menghasilkan tepat satu pemenang; verifikasi gagal oleh pemegang cookie pra-MFA tidak pernah menghasilkan token `otp`.

**AC:** Administrator tanpa MFA tidak bisa memanggil API apa pun selain pendaftaran MFA dan logout; pengguna dengan TOTP aktif yang baru login dengan password tidak bisa memanggil API apa pun selain verifikasi MFA, recovery code, dan logout; kode replay ditolak; recovery code hanya berlaku sekali; reset MFA oleh admin tercatat di audit.

### FR-108 — Mailer minimal

- `sdk/mail.Send(ctx, mail.Message{To, Subject, Text, HTML})` dengan dua driver: **`smtp`** (STARTTLS/TLS, autentikasi) dan **`log`** (tidak mengirim apa pun; hanya mencatat penerima tersamar dan subjek, isi surat penuh hanya di `ERP_ENV=development`; default di `ERP_ENV=development`). **`serve` gagal start di production bila `ERP_MAIL_DRIVER` tidak diisi** (fail-closed; `log` sah hanya bila dipilih eksplisit; `config.RequireMailDriver`, keputusan pemilik produk 3 Nov 2026, Q21); `migrate`, `inspect`, `doctor`, `modules`, dan `i18n` tidak membutuhkan konfigurasi surat.
- Konfigurasi SMTP di milestone C1 lewat **env sementara** `ERP_MAIL_DRIVER` dan `ERP_SMTP_*` (sandi lewat `ERP_SMTP_PASSWORD`, disamarkan di log dan `inspect config`) karena Settings (FR-120) baru ada di C3; setelah C3 dipindah ke settings platform (password bertipe `secret`) tanpa mengubah `sdk/mail` (keputusan K-C1.6, 2 Nov 2026, Q15).
- **Driver mail mengirim sinkron** dengan timeout dan retry singkat (hanya atas kegagalan sementara, dibatasi). **Layanan reset password mengirim email di latar terbatas** (goroutine dengan batas serentak; penuh = dibuang) agar waktu respons `forgot` seragam untuk akun ada dan tidak ada; konsekuensinya email dapat hilang bila proses mati sebelum terkirim, dan pengguna cukup mencoba lagi (diubah 3 Nov 2026, Q23). Antrean, template engine, dan pelacakan pengiriman ikut PRD Notifications.
- **URL publik aplikasi** untuk tautan di email berasal dari **settings platform** (`settings.Service.PublicURL`), bukan env dan tidak pernah dari header permintaan; bila belum dikonfigurasi, `forgot` tetap dijawab seragam dan tidak ada email terkirim (peringatan dicatat). Tautan reset berbentuk `<url-publik>/reset-password#token=<token>` (token di fragmen agar tidak masuk log akses atau Referer) (Q20, Q22).
- Template fase ini hanya untuk reset password dan undangan user, dalam bahasa sesuai preferensi user (FR-120).

**AC:** reset password terkirim lewat SMTP (diuji dengan server SMTP tiruan dalam proses dan, opsional, container MailHog/Mailpit); mode `log` tidak membuka koneksi jaringan dan tidak menulis isi surat atau tautan di luar development; password SMTP tidak muncul di log atau `inspect config`; `serve` di production tanpa `ERP_MAIL_DRIVER` gagal start; waktu respons `forgot` tidak membedakan akun ada dan tidak ada.

### FR-102 — Token: JWT access + refresh berotasi

| Token | Bentuk | Umur default | Penyimpanan di klien |
| --- | --- | --- | --- |
| Access token | JWT ditandatangani **EdDSA (Ed25519)** | 10 menit | **Memori saja** (web); secure storage OS (mobile/desktop) |
| Refresh token | String acak opaque (bukan JWT), hash disimpan | 14 hari, sliding; absolut 90 hari | **Cookie `HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh`** (web); secure storage OS (mobile/desktop) |

- Klaim access token: `iss`, `sub` (user id), `sid` (session id), `tid` (tenant id), `aud`, `exp`, `iat`, `jti`, `perm_rev`, `amr` (metode login), `scope` (untuk PAT/OAuth client). **Daftar permission tidak dimasukkan ke JWT** (ukuran dan risiko basi), dan dievaluasi di server dari cache (§6.7).
- **Rotasi refresh token** setiap dipakai. Pemakaian ulang token lama (*reuse*) → seluruh *family* dan sesinya dicabut, dan dicatat sebagai event audit keamanan. **Pengecualian (Q27, 3 Nov 2026):** kenaikan sesi ke `otp` mengganti refresh token belum terpakai milik sesi dengan satu token baru (family sama) secara atomik; token pra-MFA yang diganti menjadi "tidak dikenal" (ditolak seragam, **tanpa** mencabut sesi), sedangkan token leluhur yang sudah terpakai tetap memicu deteksi *reuse* (FR-107).
- **Kontrak `sdk/identity`:** `RefreshInput.Origin` (header `Origin`; ditolak `ErrOriginNotAllowed` bila bukan origin URL publik, ADR-0013 §1); `LoginResult.RefreshToken`/`RefreshExpiresAt` bertag `json:"-"` (hanya lewat cookie); `ConfirmMFA` mengembalikan `ConfirmMFAResult{Codes, Session LoginResult}` (aditif; Session kosong bila penaikan sesi gagal setelah faktor aktif); `VerifyMFA` mengembalikan `LoginResult` dengan `RefreshToken` baru. Galat domain yang dipetakan handler ke kode galat api-guidelines ada di `sdk/identity/errors.go`: `ErrInvalidCredentials`, `ErrWrongCurrentPassword`, `ErrInvalidRefresh`, `ErrOriginNotAllowed`, `ErrInvalidResetToken`, `ErrInvalidMFACode`, `ErrMFARequired`, `ErrMFAAlreadyEnrolled`, `ErrMFANotEnrolled`, `ErrAccountLocked`, `ErrForbidden`, `ErrEmailTaken`, `ErrNotFound`, `ErrInvalidInput`, `ErrRateLimited`, serta tipe `*RateLimitedError{RetryAfter}` (`AsRateLimited`) dan `*PasswordPolicyError{Codes}`. `identity.Admin` memiliki `SetPublicURL(ctx, rawURL)` (mengisi `platform.public_url`, FR-108/Q24) di samping `CreateAdmin`, `RotateSigningKeys`, `ResetMFA`, `UnlockAccount`, `LogoutAllDevices`.
- **Pencabutan instan**: middleware memeriksa `sid` terhadap **daftar sesi tercabut di memori**, bukan query database per request. Daftar ini hanya berisi sesi tercabut yang access token-nya belum kedaluwarsa (≤ umur access token + leeway verifier 30 detik), sehingga tetap kecil. Saat horizontal scaling, perubahan disebarkan ke semua instance lewat `StateNotifier` (PRD-000 FR-19.7, `LISTEN/NOTIFY`), dan instance baru memuat daftar awal saat startup. User `disabled`, ganti password, dan "logout semua perangkat" berlaku segera, bukan menunggu token kedaluwarsa.
- **Rotasi kunci** (tanpa downtime): `erpora core:rotate-keys` hanya membuat kunci baru berstatus `pending`, mempublikasikannya di JWKS, dan melaporkan waktu aktivasinya. Setelah **minimal `max-age` JWKS (300 detik)** pekerja latar di proses `serve` mempromosikannya menjadi `active` (memicu notifikasi `identity.keys_changed`; bila `serve` tak berjalan, promosi terjadi saat `serve` start); kunci lama menjadi `retiring` sampai seluruh token bertanda tangan kunci itu kedaluwarsa (umur access token + leeway), lalu `retired` (ADR-0013 §3). Kunci publik diterbitkan di `GET /.well-known/jwks.json`. Kunci privat dienkripsi dengan `ERP_SECRET_KEY`.
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
- `/api/bootstrap` kini mensyaratkan login (cukup Principal terautentikasi: `x-erp-authenticated`, dan dikecualikan dari `mfa_required`; Q29), dan mengembalikan `user`, ringkasan permission (§6.8), serta `permRev` (nama JSON camelCase; di klaim JWT `perm_rev`). Sampai C2, `permissions` tetap ada dan `permRev` bernilai 0.

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

**Catatan (PRD-000 Q41, 1 Nov 2026, disetujui pemilik produk; perubahan aditif):** keanggotaan field pada grup juga dapat dinyatakan di metadata resource (spec `docs/specs/resource-metadata`, field `fieldGroup`). Bila sebuah resource memiliki metadata, daftar `fields` pada grup di manifest menjadi **opsional** dan, bila ada, `module:check` memeriksa kesamaannya dengan metadata; definisi grup (kunci, label, klasifikasi, grup `default`) tetap di manifest.

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
- **Subset C1 (keputusan pemilik produk 3 Nov 2026, Q24):** tabel `settings` dan paket `internal/platform/settings` (+ `settings/store`) dibangun **lebih awal, di C1**, dalam bentuk minimal: penyimpanan nilai per tenant (`store.Get`/`Upsert`, nilai `encrypted` disegel `crypto.Sealer` dengan AAD `settings|scope|module|key`) dan **satu API tertutup** untuk kunci platform `platform.public_url` (`settings.Service.PublicURL`, `SetPublicURL`, `OriginOf`). Tanpa registri schema, validasi per tipe, audit, cache revisi, `Get[T]`, atau UI: itu tetap pekerjaan C3 dan memperluas paket yang sama secara aditif.
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
| NFR-104 | Variabel baru `ERP_SECRET_KEY` (32 byte base64), **wajib hanya untuk `serve` dan command yang menyegel/membuka data (`core:*`)**: startup/command itu gagal bila tidak ada atau tidak valid. `migrate`, `inspect`, `doctor`, `modules`, dan `i18n` tidak membutuhkannya (format tetap divalidasi bila terisi). Rotasi lewat `ERP_SECRET_KEY_PREVIOUS` (kunci lama hanya untuk membuka) dan command `core:reseal` (kelak); format segel membawa id kunci dan header ikut AAD (ADR-0013 §3; revisi 3 Nov 2026, Q16). Prosedurnya didokumentasikan di `deployment.md` |
| NFR-105 | Semua endpoint auth & token terkena rate limit (per IP dan per principal) dengan respons `429` + `Retry-After`; IP klien ditentukan menurut `ERP_TRUSTED_PROXIES` (Q26) |
| NFR-106 | **Gagal tertutup**: error saat evaluasi permission/scope (mis. cache rusak, scope provider panic) → akses ditolak, bukan diizinkan |

## 10. Milestone

| Milestone | Cakupan | Exit gate |
| --- | --- | --- |
| **C1 — Identity lokal** (implementasi selesai 4 Nov 2026; verifikasi browser end-to-end lolos; menunggu putaran perbaikan review akhir dan merge PR) | FR-101, FR-102, FR-106, FR-107, FR-108, modul `core` (`required`), `erpora core:create-admin`, `core:unlock` | Login/logout/refresh/reset end-to-end; MFA wajib Administrator berjalan; stub authN PRD-000 diganti; ASVS checklist sesi lolos |
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
| Q10 | Metode autentikasi? | ✅ JWT access + refresh berotasi **untuk semua klien termasuk web** (FR-102), SSO OIDC (FR-103), PAT (FR-104), OAuth2 client credentials (FR-105). Alternatif session cookie untuk web (pola ERPNext/Odoo) dipertimbangkan dan **ditolak** dengan alasan kemudahan horizontal scaling: verifikasi token cukup dengan kunci publik + daftar pencabutan in-memory per instance, tanpa store sesi bersama di jalur request. Dicatat di ADR-0013 (Accepted 2 Nov 2026, mekanisme rinci) |
| Q11 | Model otorisasi? | ✅ Matriks per role, union tanpa deny, tiga lapis aksi/record/field, aksi standar + kustom, scope generik (FR-110–FR-117) |
| Q13 | Dependensi antarmodul & IDOR? | ✅ Dependensi **eksplisit dan ketat** (manifest + `go.mod` + `package.json`, akses hanya lewat `contract/`/`index.ts`); IDOR dicegah di endpoint **dan** di kontrak antarmodul — modul penyedia kontrak menegakkan otorisasi terhadap principal asli, hak sistem hanya lewat `authz.AsSystem` yang teraudit (FR-119) |
| Q12 | Penempatan Core? | ✅ Hibrida: mesin di platform, administrasi di modul `core` yang `required` (§3) |
| Q14 | Urutan pengerjaan milestone? | ✅ **Disetujui pemilik produk (2 Nov 2026):** mengikuti §10, C1 Identity lokal dahulu lalu C2 Authorization; C3 dan C4 setelahnya. Rencana: `docs/superpowers/plans/2026-11-02-c-overview.md` dan `2026-11-02-c1-identity-lokal.md`. T4 spec `resource-metadata` menunggu C2 |
| Q15 | Keputusan C1 K-C1.1 sampai K-C1.9? | ✅ **Disetujui seluruhnya sesuai rekomendasi oleh pemilik produk (2 Nov 2026); rincian dan alasan di ADR-0013:** (K-C1.1) ADR-0013 disahkan (Accepted); (K-C1.2) dependensi baru hanya `x/crypto/argon2`, `x/time/rate`, `x/term`, JWT EdDSA dan TOTP ditulis sendiri dengan stdlib; (K-C1.3) C1 membangun subset FR-113 (tabel `roles`, `user_roles`, role sistem `administrator`), matriks/scope/field tetap C2; (K-C1.4) linter kontrak diperluas aditif dengan `x-erp-credential-response: true` untuk respons login/refresh (modul `core`/`platform`); (K-C1.5) `Invocation.In io.Reader`, tanpa flag sandi; (K-C1.6) SMTP lewat env sementara `ERP_MAIL_DRIVER`/`ERP_SMTP_*` sampai Settings (C3); (K-C1.7) `internal/platform/identity/store` masuk allowlist GORM D3b (PRD-000 Q42); (K-C1.8) `/.well-known/jwks.json` didaftarkan platform di luar `/api/{slug}`; (K-C1.9) `ERP_AUTHZ_STUB` dan `ERP_MODULES_ADMIN_API` dipertahankan sampai C2. Metode MFA wajib Administrator (Q9) kini bergantung pada role `administrator` minimal dari K-C1.3 |
| Q16 | Keputusan setelah review independen Wave 0 (lockout, `ERP_SECRET_KEY`, penanda kredensial, SHA-1 TOTP, risiko DoS lockout)? | ✅ **Disetujui pemilik produk (3 Nov 2026); rincian di ADR-0013 §3, §5, bagian 7a:** (1) **Lockout** per pasangan (akun, IP): 5 gagal jeda bertingkat (30 detik berlipat dua, maks 15 menit), 20 gagal blokir pasangan 1 jam (keduanya atas kegagalan dalam jendela 24 jam terakhir, Q19), akun `locked` hanya bila ≥ 50 gagal lintas ≥ 5 IP dalam 1 jam; unlock lewat reset password atau command baru `core:unlock --email` (di luar teks asli FR-101; FR-101, FR-107, dan AC diubah). (2) **`ERP_SECRET_KEY`** wajib hanya untuk `serve` dan `core:*`; `migrate`/`inspect`/`doctor`/`modules`/`i18n` tidak butuh (format divalidasi bila terisi); rotasi via `ERP_SECRET_KEY_PREVIOUS` (hanya membuka) dan `core:reseal` kelak; segel membawa id kunci, header ikut AAD; NFR-104 direvisi. (3) **Penanda kredensial linter** berupa allowlist (modul, operationId, properti): `accessToken` pada login/refresh, `secret`/`otpauthUri` pada enroll MFA, dan `codes` (recovery code, tampil sekali sesuai FR-107) pada confirm MFA dan regenerate recovery codes (modul `core`/`platform`); properti `refresh*` di body selalu ERROR; penanda tingkat operasi saja tidak cukup. (4) **Pengecualian SHA-1** khusus HOTP/TOTP (RFC 4226/6238) dari larangan security-guidelines §3.4 (hanya paket TOTP, penekanan gosec G401/G505 beralasan tertulis) dan **penerimaan risiko residual DoS lockout** (botnet dapat meng-`lock` akun; mitigasi: jeda per (akun, IP), ambang lintas IP, unlock via reset sandi, `core:unlock`, admin), disetujui pemilik produk 3 Nov 2026 atas pertanyaan langsung (ADR-0013 §5, §6). (5) **Akses `Service`/`Admin`:** `identity.Admin` dan `identity.Service` hanya dipasang pada context modul `core` dan alasan sistem `cli:core:*`; aturan arch `IDENT` diperluas: modul selain `modules/core` dilarang memanggil/mereferensikan `identity.AdminFrom` dan `identity.ServiceFrom` (disetujui pemilik produk 3 Nov 2026); aturan statis ini mempersulit dan mendeteksi, bukan menjamin. **Pengerasan (disetujui pemilik produk 3 Nov 2026):** `unsafe`, `//go:linkname`, dan setiap ekspresi bertipe `unsafe.Pointer` dilarang di kode non-test modul (menutup pemalsuan lewat refleksi tanpa `import "unsafe"`); hak core dikunci ke direktori persis `<root>/modules/core` (aturan `MODPATH`); pemeriksaan mencakup varian tag `devtools` dan target matriks rilis lain tanpa cgo; Kotak tertutup di `sdk/identity` hanya **mempersulit pemalsuan murni-refleksi** dan menyerahkan kontrol ke aturan `IDENT`, yang menandai setiap ekspresi bertipe `unsafe.Pointer` (termasuk yang didapat lewat `reflect.NewAt`/`Value.UnsafePointer` tanpa `import "unsafe"`). Risiko residual yang diterima: `unsafe.Pointer` yang diperoleh lewat cgo atau assembly (berkas bantu `_cgo_*` tidak dipindai) dan dependensi pihak ketiga yang memanggil API tanpa dipindai arch test; modul pihak ketiga dianggap kode yang di-review sebelum dipasang (ADR-0013 7a butir 6). (6) **Allowlist kredensial** ditambah `codes` (recovery code, tampil sekali sesuai FR-107) pada operasi confirm MFA dan regenerate recovery codes (modul `core`). (7) **Tiga keputusan kecil C1.1** (`KeyIDOf` = byte pertama SHA-256(material) dengan penolakan tabrakan 1/256; `doctor` melaporkan kunci kosong sebagai FAIL; `config.Load` mengembalikan Config parsial bersama galat kunci dan hanya `doctor` yang memakainya): **disetujui pemilik produk 3 Nov 2026 (tidak ada yang dipilih untuk diubah)**; mudah dibatalkan bila pemilik produk meninjaunya kembali (ADR-0013 §9). **Diterapkan atas temuan review independen (sesuai spek, bukan keputusan pemilik produk):** status kunci `pending` (AC FR-102 "rotasi tanpa downtime"), aturan arsitektur `IDENT` (FR-119 butir 2), dan aturan MFA bagi pengguna non-admin berfaktor aktif (FR-107) (ADR-0013 §8). (8) **Petunjuk pembuatan kunci (disetujui pemilik produk 3 Nov 2026, ADR-0013 §3):** pesan galat `ERP_SECRET_KEY` menyebut `erpora secret:generate` (hanya build pengembang, tidak dipindahkan ke binary release), `openssl rand -base64 32`, dan PowerShell yang jalan di Windows PowerShell 5.1 dan 7+ (`RandomNumberGenerator.Create().GetBytes($b)`; `GetBytes(32)` statis dihindari karena baru ada di .NET 6; bukan `Get-Random`, bukan CSPRNG), dengan rujukan ke `deployment.md`. (9) **Satu `go.mod` per modul (`MODNEST`, disetujui pemilik produk 3 Nov 2026):** `go.mod` bersarang di bawah `modules/<x>/` dilarang dan `replace` lokal hanya `erpora/sdk => ../../sdk` serta `erpora/modules/<x> => ../<x>`; modul tetap dikelola lewat `go.work` + manifest (ADR-0013 7a butir 7). |
| Q17 | Normalisasi Unicode password (ASVS 5.0) dan tindak lanjut review C1.4? | ✅ **Disetujui pemilik produk (3 Nov 2026); rincian di ADR-0013 §5 dan 7a butir 7:** (1) **Password dinormalisasi ke NFKC** (`golang.org/x/text/unicode/norm`, sudah dependensi langsung) sebelum `Hash`, `Verify`, dan `CheckPolicy`, agar password yang sama dari IME/perangkat berbeda (mis. "é" komposisi U+00E9 vs "e" + U+0301, ligatur, angka lebar penuh) tetap cocok; alasan: ASVS 5.0 menganjurkan normalisasi Unicode dan konsistensi lintas input method. Batas 1024 byte diperiksa pada **masukan mentah sebelum normalisasi** (cegah DoS/alokasi besar); panjang minimum 12 dihitung dalam rune pasca-NFKC; daftar password umum dicari pada NFKC + huruf kecil; NUL/karakter kontrol tidak diubah. Belum ada data hash lama, jadi tanpa kompatibilitas mundur. FR-101 diubah. (2) **Default argon2** tetap OWASP minimum m=19456 KiB, t=2, p=1 (disetujui; konfigurasi lewat `ERP_ARGON2_*` dengan batas bawah OWASP dan batas atas wajar). (3) **Pembatas konkurensi hash** (semaphore; 19 MiB per `Verify`) belum ada di C1.4 dan menjadi pekerjaan C1.7/C1.10 di rencana C1; C1.7 memakai `password.NewDummy(cfg.Argon2)` dan memperlakukan `ErrInvalidHash` pada akun nyata seperti password salah. Pertanyaan terbuka normalisasi Unicode pada C1.4: **ditutup**. |
| Q18 | Tabel tenant `roles` dan `user_roles` (C1.2) memakai kolom `tenant_id`? | ✅ **Disetujui pemilik produk (3 Nov 2026): tanpa `tenant_id`.** Tabel di database tenant terisolasi per database (ADR-0004), dan §4 tidak mencantumkan kolom itu; dengan begitu seed role sistem `administrator` tetap berada di migrasi SQL tenant (tenant id belum ada saat goose berjalan). Tabel control (sesi, refresh, reset, percobaan login) tetap membawa `tenant_id`. Jika kelak tabel tenant dipindah ke model multi-tenant dalam satu database, `tenant_id` ditambahkan lewat expand → migrate → contract dan seed dipindah ke langkah aplikasi. Keputusan kecil implementer C1.2 yang dibiarkan (bukan keputusan pemilik produk): kegagalan login untuk email tak dikenal tidak tercatat di `login_attempts` (`user_id` NOT NULL); C1.7 menyeragamkan waktu respons jalur akun-dikenal dan tak-dikenal. |
| Q19 | Jendela waktu hitungan gagal per pasangan (akun, IP) pada lockout (Q16)? | ✅ **Disetujui pemilik produk (3 Nov 2026); rincian di ADR-0013 §5:** hitungan kegagalan per pasangan (akun, IP) untuk jeda bertingkat (gagal ke-5 dan seterusnya) dan blokir 20 gagal **hanya memperhitungkan kegagalan dalam 24 jam terakhir** (kegagalan lebih tua diabaikan); sukses tetap mereset hitungan pasangan. Alasan: salah ketik lama tidak boleh menumpuk, sedangkan jendela 1 jam terlalu longgar (4 tebakan per jam tidak pernah kena jeda, dan tiap blokir 1 jam diikuti jatah baru 20 tebakan); dengan 24 jam, setelah blokir 1 jam hitungan masih 20 dalam jendela sehingga satu kegagalan lagi memblokir 1 jam lagi. FR-101 dan Q16 butir 1 diubah. Jendela 1 jam untuk ambang `locked` akun lintas >= 5 IP tidak berubah. Implementasi: `LockoutConfig.PairWindow` (default 24 jam) di `guard`, `Store.PairFailureStateSince`; retensi `login_attempts` harus >= 24 jam. `UnlockUser` kini selalu menghapus catatan percobaan akun (membuka juga blokir pasangan pada akun active). Tidak ada risiko baru: risiko residual DoS lockout (botnet) tetap seperti Q16 butir 4 (dokumen ini tidak memiliki tabel risiko terpisah). |
| Q20 | Dari mana URL publik untuk tautan email (reset sandi) berasal? | ✅ **Disetujui pemilik produk (3 Nov 2026):** `ERP_PUBLIC_URL` **ditolak** (tidak ada variabel env itu). Sumber URL publik adalah **settings platform** (`settings.Service.PublicURL(ctx)`, `ErrNotConfigured` bila belum diisi); layanan reset memakai antarmuka `PublicURLSource`, membacanya tiap permintaan dan memvalidasinya (http(s) absolut tanpa userinfo, query, fragmen). Alasan: URL publik adalah konfigurasi aplikasi yang diubah operator tanpa restart dan tidak boleh diambil dari header permintaan (host header injection). Bila belum dikonfigurasi atau tidak sah, `forgot` tetap dijawab seragam (tanpa membocorkan), peringatan terstruktur dicatat, dan tidak ada email terkirim. |
| Q21 | `serve` di production dengan driver surat `log`? | ✅ **Disetujui pemilik produk (3 Nov 2026):** **Fail-closed:** `serve` gagal start di production bila `ERP_MAIL_DRIVER` tidak diisi (`config.RequireMailDriver`); `log` sah hanya bila dipilih eksplisit (`ERP_MAIL_DRIVER=log`), `smtp` sah bila lengkap. Di development `log` tetap sah. `migrate`, `inspect`, `doctor`, `modules`, dan `i18n` tetap berjalan tanpa konfigurasi surat. Alasan: surat (mis. tautan reset) yang diam-diam tidak terkirim di production adalah kegagalan yang tidak terlihat. Pemanggilan di `serve` dilakukan perakitan C1.10/C1.12. |
| Q22 | Bentuk tautan reset sandi? | ✅ **Disetujui pemilik produk (3 Nov 2026):** `<url-publik>/reset-password#token=<token>`: token di **fragmen**, bukan query, agar tidak masuk log akses/proksi atau header Referer; halaman frontend membacanya dari `location.hash` lalu menghapusnya dari URL. Hanya hash token (SHA-256) yang disimpan; berlaku 30 menit; sekali pakai. |
| Q23 | Pengiriman email reset sinkron atau asinkron (FR-108)? | ✅ **Disetujui pemilik produk (3 Nov 2026):** **Layanan reset mengirim di latar terbatas** (goroutine dengan batas serentak; penuh = dibuang, dicatat) dan jawaban `forgot` menunggu lantai waktu 100 ms, sehingga waktu respons seragam untuk akun ada dan tidak ada (SMTP yang berdetik-detik tidak dapat disamarkan lantai waktu). **Driver mail tetap sinkron** dengan timeout dan retry singkat. FR-108 diubah sesuai ini. Konsekuensi diterima: email dapat hilang bila proses mati sebelum terkirim, dan pengguna mencoba lagi. |
| Q24 | Di mana URL publik aplikasi disimpan (tautan email reset sandi dan pemeriksaan `Origin` cookie refresh), dan apakah tabel `settings` dibangun sekarang? | ✅ **Disetujui pemilik produk (3 Nov 2026):** URL publik **tidak** memakai env `ERP_PUBLIC_URL` (menegaskan Q20), melainkan disimpan di **tabel settings platform yang dibangun sekarang dalam bentuk minimal, mendahului C3** (FR-120). Wujud: migrasi tenant `0004_settings` (tabel `settings` sesuai §4, tanpa `tenant_id` karena database tenant terisolasi, Q18), paket `internal/platform/settings/store` (GORM; masuk allowlist D3b, PRD-000 §4.2) dan `internal/platform/settings` (`Service`: `PublicURL(ctx)` dengan cache dalam memori dan `ErrNotConfigured` bila belum diisi, `SetPublicURL(ctx, url, updatedBy)`, `PublicURLOrDevDefault`, `OriginOf`). Aturan nilai: http(s) absolut dengan host, tanpa userinfo, query, atau fragmen, **https wajib di production**, dinormalisasi (host huruf kecil, port bawaan dan slash akhir dibuang), panjang maksimum 512; nilai tersimpan divalidasi ulang saat dibaca (gagal tertutup). URL tidak pernah diturunkan dari header permintaan. Di development, bila belum diisi, pemanggil boleh meminta default eksplisit yang diturunkan dari `ERP_HTTP_ADDR`; di production `serve` gagal start bila belum diisi (penegakan oleh perakit, C1.10/C1.12). Pengisian awal lewat command modul `core` `core:settings:set` (C1.12), yang memanggil `SetPublicURL`. Alasan: tautan email dan pemeriksaan `Origin` membutuhkan URL tepercaya yang dapat diubah operator tanpa restart; Settings yang proper adalah C3 dan tidak ada env sementara untuk nilai ini. Cakupan C1 sengaja minimal (tanpa schema, tipe, audit, cache revisi, UI); C3 memperluas paket yang sama secara aditif dan mengubah FR-120/§4 hanya dengan menambah. Keputusan kecil implementer (bukan keputusan pemilik produk): tanpa kolom `id` (identitas baris cukup UNIQUE (`scope`, `module_key`, `key`)), TTL cache 30 detik (`Invalidate` tersedia untuk notifikasi lintas instance), path pada URL publik diizinkan (aplikasi di sub-path), IDN harus punycode. |
| Q25 | Apakah JWKS (`GET /.well-known/jwks.json`) boleh tanpa amplop `{"data": ...}` (api-guidelines §4.2)? | ✅ **Disetujui pemilik produk (3 Nov 2026):** **ya, sebagai satu-satunya pengecualian amplop.** Pengecualian berlaku hanya untuk kontrak **platform** dan dokumen standar `/.well-known/*` (saat ini hanya `/.well-known/jwks.json`), ditandai `x-erp-raw-response: true` pada operasinya; kontrak modul dan path platform lain tetap wajib beramplop. Linter `erp-success-envelope`, `erp-path-module-prefix`, dan `erp-path-kebab-case` menerima penanda/awalan itu hanya pada kontrak platform untuk path `/.well-known/`; `apicontract.Parse` menjaga daftar izin yang ketat (tepat `/.well-known/jwks.json`), sehingga menambah dokumen `/.well-known/*` lain butuh persetujuan pemilik produk dan perubahan daftar itu. Alasan: RFC 7517 mewajibkan bentuk `{"keys": [...]}` agar pustaka JWT standar (verifikasi token oleh layanan lain) dapat memakai endpoint ini langsung; membungkusnya dalam `data` memutus interoperabilitas. Diterapkan di `contracts/openapi/platform.yaml`, `api-guidelines.md` §4.2, PRD-000 (FR-23, §4.2 kontrak), dan `docs/module-sdk/contracts.md`; pernyataan "selalu dibungkus `data`" diubah menjadi memuat pengecualian ini. |
| Q26 | Dari mana IP klien berasal bila aplikasi berjalan di belakang reverse proxy (batas laju per IP, lockout per (akun, IP), audit)? | ✅ **Disetujui pemilik produk (3 Nov 2026):** env baru **`ERP_TRUSTED_PROXIES`** (daftar IP/CIDR dipisah koma; bawaan **kosong**). Tanpa isi, IP klien = alamat peer TCP dan header `X-Forwarded-For` **diabaikan** (klien tidak dapat memalsukannya). Bila peer TCP ada dalam daftar, `X-Forwarded-For` dibaca dari **kanan ke kiri** dan hop pertama yang **bukan** proxy tepercaya dipakai sebagai IP klien (hop di kirinya adalah klaim klien dan diabaikan). Header cacat (hop kosong, bukan IP, berport, berzona IPv6), lebih dari 4096 byte, atau semua hop tepercaya: memakai peer; hanya 32 hop terkanan dinilai. `X-Real-IP` tidak dipakai. Validasi saat start (exit 2): maksimal 64 entri, tanpa prefix `/0`, tanpa hostname. Alasan: di belakang proxy tanpa konfigurasi ini semua klien tampak berasal dari IP proxy sehingga batas laju per IP dan lockout per pasangan (akun, IP) (Q16, Q19) tidak efektif atau saling mengunci, sedangkan memercayai `X-Forwarded-For` tanpa daftar memungkinkan pemalsuan IP untuk menghindari batas. Operator wajib mengisinya bila ada reverse proxy (`docs/operations/deployment.md`). |
| Q27 | Apakah kenaikan sesi ke `otp` merotasi refresh token (cookie refresh pra-MFA yang dicuri tidak boleh memberi token ber-`otp`)? | ✅ **Disetujui pemilik produk (3 Nov 2026):** **ya.** Pada konfirmasi pendaftaran MFA, `mfa/verify`, dan `mfa/recovery`, refresh token pra-MFA mati dan refresh token baru (AMR `[pwd, otp]`) diterbitkan untuk sesi yang sama; handler menulis ulang cookie. Alasan: pemegang cookie pra-MFA yang dicuri tidak boleh dapat menukarnya menjadi access token `otp`. Implementasi: `session.Service.Upgrade` atomik (compare-and-set pada `auth_sessions.amr`, hapus token belum terpakai milik sesi, sisipkan token baru sebagai akar family yang sama; tanpa perubahan skema); token pra-MFA menjadi "tidak dikenal" sehingga ditolak seragam tanpa mencabut sesi, sedangkan leluhur yang sudah terpakai tetap memicu deteksi *reuse*. `ConfirmMFA` kini mengembalikan `ConfirmMFAResult` (recovery code + `Session` berisi access token otp dan refresh token baru); klien tidak memanggil `refresh` lagi setelah konfirmasi bila respons memuat token. Mengubah ADR-0013 §1 dan §4, FR-102, FR-107. Dicatat bersama kontrak `sdk/identity` (daftar galat `errors.go`, `Admin.SetPublicURL`, `RefreshInput.Origin`; FR-102). |
| Q28 | Apakah `CreateAdmin` boleh dilanjutkan (idempoten) bila terputus di tengah (akun dibuat tetapi role/aktivasi gagal)? | ✅ **Disetujui pemilik produk (4 Nov 2026; diusulkan 3 Nov 2026, isi usulan tidak berubah):** akun dibuat nonaktif, role diberikan, `active` terakhir; email yang sudah ada dengan status `disabled`, tanpa role, belum pernah login (`last_login_at` kosong), dan tanpa sesi sama sekali dilanjutkan (sandi diatur ulang, role diberikan, diaktifkan); keadaan lain tetap `ErrEmailTaken` (tidak ada jalur pengambilalihan). Alasan: kompensasi lama (menonaktifkan akun tanpa role) mengunci email dan memaksa pembersihan manual. Nama akun yang sudah ada tidak diubah saat dilanjutkan. Mengubah FR-101. |
| Q29 | Bagaimana route yang cukup dengan login (tanpa permission tertentu) dinyatakan di kontrak dan runtime, dan apakah `/api/bootstrap` tunduk pada aturan `mfa_required`? | ✅ **Disetujui pemilik produk (3 Nov 2026), dua butir:** **(1) Penanda `x-erp-authenticated: true`.** Operasi kontrak yang hanya mewajibkan Principal terautentikasi, tanpa permission tertentu, ditandai `x-erp-authenticated: true` (sejajar `x-erp-permission` dan `x-erp-public`; ketiganya saling meniadakan). Kontrak: tanpa `x-erp-permission` dan tanpa 403 permission, tetapi **401 wajib didokumentasikan** (rule `erp-operation-permission` dan `erp-error-codes-documented` diperluas; api-guidelines §13). Runtime: opsi route SDK `module.Authenticated()` dipakai pada `Registrar.Handle(method, path, "", h, module.Authenticated())` (permission kosong yang dinyatakan eksplisit; kombinasi lain ditolak `invalid_route_option`; tanpa perubahan antarmuka `Registrar`); registry menyimpan `RouteInfo.Authenticated`; `Decide` tetap mensyaratkan Principal (401), token tanpa scope terbatas, dan aturan MFA ADR-0013 §4 (daftar route auth core tetap milik platform), hanya pemeriksaan permission yang dilewati, sehingga route ini tetap berjalan walau `ERP_AUTHZ_STUB=deny` sampai C2. Dipakai untuk `GET /api/bootstrap` dan route auth modul `core` yang dikelola pengguna sendiri (logout, ganti sandi, MFA). Setiap route berpenanda ini wajib tercantum di allowlist keamanan `tests/security/authenticated_routes.txt` (seperti `public_routes.txt`; route platform langsung di router: `httpserver.AuthenticatedRoutes`) dan test kesesuaian kontrak memeriksa route non-publik tanpa permission terhadap penanda. **(2) Bootstrap dikecualikan dari `mfa_required`.** `GET /api/bootstrap` hanya melewati `Authenticate` (bukan `TypedAuthorizer`), sehingga sesi yang belum memenuhi aturan MFA (faktor aktif belum terverifikasi, atau administrator belum punya faktor) tetap dapat memuatnya. Alasan: UI membutuhkan bootstrap (data pengguna, katalog i18n, daftar modul) untuk menampilkan layar enroll dan verify MFA; isinya hanya data pengguna sendiri dan metadata modul, sedangkan data bisnis dan route modul tetap diblokir `mfa_required`. Yang dikecualikan hanya aturan MFA, bukan scope token (diperjelas hasil review 4 Nov 2026, sesuai butir 1): handler bootstrap menolak Principal ber-scope terbatas (`TokenScope` tidak kosong) dengan 403 `permission_denied`, dan kontrak mendokumentasikan 403 itu hanya untuk token ber-scope terbatas (bukan `mfa_required`). Alasan butir 1: linter dan test kesesuaian hanya mengenal permission atau publik, padahal bootstrap tidak lagi publik (FR-106) dan route logout/ganti sandi/MFA tidak boleh bergantung pada permission yang belum ada (C2). Diterapkan di `contracts/openapi/platform.yaml`, `contracts/spectral-functions/operations.ts`, `internal/platform/apicontract`, `sdk/module`, `internal/platform/modules/{registry,runtime}`, ADR-0013 §4, api-guidelines §13, dan `docs/module-sdk/contracts.md`. |
| Q30 | Keputusan atas butir review putaran perbaikan C1 yang menunggu pemilik produk? | ✅ **Disetujui pemilik produk (4 Nov 2026, "sesuai rekomendasi"), lima butir:** **(1) `Admin.authorize` hanya menerima hak sistem berawalan `cli:core:`** (command CLI `core:*`, sesuai keputusan "Akses Service/Admin" ADR-0013 §7a butir 5); alasan lain (hook lifecycle, seed `cli:db:seed`, modul lain) ditolak `ErrForbidden` + log `identity.admin_denied`. Alasan: Admin adalah operasi paling berhak; menyempitkan hak sistem ke command core mencegah jalur hak sistem lain mengangkat dirinya jadi administrator. Konsekuensi: seed atau hook core tidak boleh memanggil `Admin`. **(2) Pelacak email tak dikenal di memori per instance (`guard/lockout.go` `unknownPairs`) diterima sebagai risiko residual selama deployment satu instance;** kode tidak diubah, ditinjau ulang saat penskalaan horizontal / `StateNotifier`. Alasan: email tak dikenal tidak punya baris `users` untuk dikunci di DB; tanpa pelacak jalur itu menjadi oracle waktu, dan dengan satu instance pelacak memori memadai. **(3) Retensi `login_attempts` 30 hari:** pekerja latar di `serve` menghapus baris `attempted_at` lebih tua dari 30 hari tiap jam; berhenti bersih saat shutdown. Alasan: baris memuat user_id dan IP (data pribadi, UU PDP) sehingga tidak boleh disimpan selamanya; 30 hari jauh di atas jendela pasangan 24 jam (Q19) dan jendela `locked` 1 jam (Q16). **(4) Status `locked` hanya menolak login baru dan TIDAK mencabut sesi yang ada** (sesi tetap valid dan dapat di-refresh; bootstrap tetap dilayani); pencabutan sesi khusus `disabled`, keanggotaan tenant nonaktif, reset password, dan aksi admin eksplisit. Alasan: penyerang lintas IP dapat memicu `locked` (risiko DoS lockout, Q16), dan mencabut sesi pemilik sah akan memperbesar dampaknya. **(5) Reset password ditolak untuk akun `disabled`:** `forgot` tidak mengirim email (jawaban seragam, lantai waktu sama, tanpa oracle) dan `Complete` menolak tautan milik akun `disabled` seperti token tidak sah (tanpa memakai tautan; penjaga atomik di `store.ResetPassword`). Alasan: akun nonaktif tidak boleh dihidupkan kembali jalur kredensialnya lewat tautan lama. Diterapkan di ADR-0013 §5 dan §7b, FR-101, `docs/operations/deployment.md` §3.4. |
| Q31 | Konvensi kode galat dan `Idempotency-Key` endpoint auth modul `core` (hasil implementasi C1.11)? | ✅ **Disetujui pemilik produk (4 Nov 2026), tiga butir:** **(1) Masukan salah pada endpoint auth berautentikasi atau berbody dijawab 422 `validation_failed`** dengan `details[].code` spesifik: `wrong_current_password` (ganti sandi), `invalid_code` (kode TOTP atau recovery salah), `invalid_token` (token reset tidak dikenal, kedaluwarsa, terpakai, atau milik akun nonaktif); **login gagal tetap 401 `invalid_credentials`** dan token/sesi kedaluwarsa tetap 401 `token_expired` (tidak memakai 419). Alasan: request sudah terautentikasi atau token reset adalah data body, dan 401 akan memicu interceptor refresh/logout di klien; sejalan dengan `ValidationException` Laravel (419 di Laravel berarti CSRF mismatch, dan API ini tanpa token CSRF). **(2) `account_locked` = 403** (bukan 423). **(3) `Idempotency-Key` pada POST auth modul `core` diterima tetapi tidak disimpan** (preseden PRD-000 Q29 butir 19 untuk API admin modul, dan deskripsi parameter di `contracts/openapi/common.yaml`). Alasan: me-replay respons berisi access token, recovery code, atau secret MFA tidak aman, dan operasi auth sudah sekali pakai lewat rotasi refresh token dan anti-replay. Diterapkan di `modules/core`, `modules/core/openapi.yaml` (deskripsi parameter), api-guidelines §7.2 dan §8, ADR-0013 §7c, FR-101 dan FR-107. |
| Q32 | Keputusan atas butir implementasi C1.12 (command CLI `core:*`) yang menyimpang dari atau tidak diatur spek? | ✅ **Disetujui pemilik produk (4 Nov 2026, "sesuai rekomendasi"), empat butir:** **(1) Nama command modul boleh memuat satu segmen grup `<grup>:<aksi>`:** pola registry diperluas secara aditif dari `^[a-z][a-z0-9-]*$` (ADR-0006) menjadi `^[a-z][a-z0-9-]*(:[a-z][a-z0-9-]*)?$` (`settings:set` sah; `a:b:c`, `x:`, `:x`, huruf besar tetap ditolak). Alasan: Q24 sudah menamai `erpora core:settings:set`, sedangkan ADR-0006 melarang titik dua; perluasan aditif menjaga semua nama lama tetap sah dan tidak mengubah Q24, deployment.md, maupun CI. Diterapkan di `internal/platform/modules/registry/reserved.go` dan ADR-0006 butir Registrar. **(2) Pencabutan sesi oleh command CLI (`core:reset-mfa`) sampai ke proses `serve` yang sedang berjalan lewat muat ulang daftar tercabut berkala (paling lama 1 menit), tanpa notifikasi lintas proses;** diterima sebagai risiko residual sampai ada notifier LISTEN/NOTIFY (`StateNotifier`). Alasan: jendela ≤ 1 menit kecil dibanding risiko yang ditanggung, dan notifier lintas proses adalah pekerjaan terpisah. Didokumentasikan di `docs/operations/deployment.md` §3.4a. **(3) `core:create-admin` tanpa `--name` memakai nama "Administrator".** **(4) Command `core:*` memastikan kunci penandatangan aktif pertama ada (`EnsureActive`, sama seperti `serve`)** sebelum menjalankan aksinya. Alasan: `core:rotate-keys` pada instalasi baru tanpa langkah ini hanya menyisakan kunci `pending`, sehingga tidak ada kunci yang dapat menandatangani sampai promosi. |
| Q33 | Di mana layar autentikasi web (login, MFA, lupa/reset sandi, ubah sandi) ditaruh, dan komponen apa yang dipakai (C1.14)? | ✅ **Disetujui pemilik produk (4 Nov 2026):** **(1) Semua layar auth milik modul `core`** (`modules/core/web`, paket `@erpora/module-core`), sejalan dengan API `/api/core/auth/*` milik modul yang sama; platform hanya menyediakan mesin (backend `internal/platform/identity`, `sdk/identity`) dan klien token frontend (`packages/platform-web/src/auth`: access token di memori, refresh single-flight saat 401, deteksi `mfa_required`) yang dibutuhkan semua modul. **(2) Layar login, MFA (verify/enroll), lupa sandi, dan reset dipasang shell sebagai gerbang sebelum bootstrap** (router aplikasi baru dibangun setelah `/api/bootstrap`, yang wajib login sejak C1.10), tanpa kerangka shell, di jalur tingkat atas `/login`, `/forgot-password`, `/reset-password` (sesuai tautan email `<url-publik>/reset-password#token=…`, ADR-0013 K-C1.6); slug `login`, `logout`, `forgot-password`, dan `reset-password` dicadangkan di registry. Pengecualian dari aturan "rute modul berawalan slug" (PRD-000 Q40 butir 13) hanya untuk gerbang ini, karena dirender sebelum router modul ada. **(3) Halaman "Ubah sandi" adalah halaman modul biasa di `/core/change-password`;** modul core mendapat desk minimal (Dashboard + Ubah sandi; menu memakai `"access": "authenticated"` tanpa kapabilitas, PRD-000 Q43) yang kelak diisi halaman Pengguna/Role/Settings (C2/C3). **(4) QR code pendaftaran MFA dirender dengan pustaka `uqr`** (MIT, tanpa dependensi, matriks dirender sebagai `<svg>` React tanpa `dangerouslySetInnerHTML`), dimuat lazy di layar enroll; **pengecualian token desain (disetujui pemilik produk 4 Nov 2026):** modul QR memakai warna literal hitam di atas putih, bukan token tema, agar kontras pemindai (ISO/IEC 18004) terjamin di tema gelap maupun terang. **(5) Kode TOTP/recovery memakai komponen shadcn baru `input-otp`** (dipasang lewat `task shadcn:sync`; disetujui sebagai komponen baru). Komponen lain hanya yang sudah terpasang (Card, Input, Label, Button, LoadingButton, Alert); tata letak meniru halaman login ERPNext. Alasan: pembagian per lapisan (mesin di platform karena middleware dan klien API dipakai semua modul, aturan D1/D2) dan per fitur yang terlihat pengguna (UI + API auth di core) membuat kepemilikan jelas. Diterapkan di rencana C1.13/C1.14. |

# PRD-001 C1 Identity Lokal: Rencana Implementasi

> **Untuk agentic worker:** WAJIB memakai `superpowers:subagent-driven-development` (disarankan) atau `superpowers:executing-plans`. Langkah memakai kotak centang (`- [ ]`). Alur sprint dan pemilihan model subagent mengikuti `~/.claude/CLAUDE.md` (Sonnet implementer, review akhir Opus independen, maksimal 3 subagent bersamaan, saya yang commit).

**Tujuan:** mengganti stub autentikasi PRD-000 dengan identitas sungguhan: akun dan login lokal, JWT EdDSA + refresh berotasi, MFA TOTP, mailer minimal, Principal di context, modul `core` (`required`) beserta command `core:create-admin`, `core:rotate-keys`, `core:reset-mfa`.

**Arsitektur:** mesin (hash, token, sesi, revokasi, rate limit, MFA, middleware, store) di `internal/platform/identity` (+ `identity/store` untuk GORM); antarmuka yang dipakai modul di `sdk/identity` dan `sdk/mail` (dependency inversion D5/D9); modul `modules/core` hanya berisi API, use case admin, i18n, dan frontend; tabel identitas dimiliki platform (control DB). `Authorizer` diperluas ke keputusan bertipe agar 401, 403, dan `mfa_required` terbedakan tanpa mengubah modul.

**Tech Stack:** Go 1.27 stdlib (`crypto/ed25519`, `crypto/hmac`, `crypto/aes`, `net/smtp`, `crypto/tls`), `golang.org/x/crypto/argon2`, `golang.org/x/time/rate`, `golang.org/x/term`; goose (migrasi control dan tenant), GORM di `identity/store` saja; React/TS (`modules/core/web`, `packages/platform-web`).

**Spec:** `docs/prd/PRD-001-core.md` (FR-101, FR-102, FR-106, FR-107, FR-108, §3, §4, NFR-101 s.d. 106); `docs/superpowers/plans/2026-11-02-c-overview.md` (keputusan K-C1.1 sampai K-C1.9, **disetujui pemilik produk 2 Nov 2026**; keputusan lanjutan 3 Nov 2026 di PRD-001 Q16 dan ADR-0013 §3, §5, §6, bagian 7a); `docs/security/security-guidelines.md` §3.4 sampai §3.6, §3.10, §3.12; `docs/architecture/api-guidelines.md` §8, §12.

## Global Constraints

- Password: **argon2id**, parameter dari konfigurasi (default OWASP terkini), panjang minimum **12**, cek terhadap daftar sandi umum, tanpa aturan komposisi.
- Access token: JWT **EdDSA (Ed25519)**, umur **10 menit**, klaim `iss sub sid tid aud exp iat jti perm_rev amr scope`; daftar permission **tidak** masuk JWT. Kunci penandatangan berstatus `pending`/`active`/`retiring`/`retired`: kunci baru dipublikasikan di JWKS (`pending`, minimal 300 detik) sebelum dipakai menandatangani; `retiring` menjadi `retired` hanya setelah umur access token + leeway 30 detik; keyring dimuat ulang lewat `StateNotifier` kanal `identity.keys_changed` dan sekali (dibatasi laju) saat `kid` tak dikenal. Refresh token: string acak opaque, hash disimpan, **14 hari sliding, absolut 90 hari**, rotasi tiap pakai, reuse mencabut seluruh *family*.
- Cookie refresh: `HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh`, ditambah pemeriksaan header `Origin`. Access token hanya di memori klien (tidak pernah `localStorage`/`sessionStorage`).
- Revokasi: middleware memeriksa `sid` terhadap daftar sesi tercabut **di memori**, bukan query per request; disebarkan lewat `StateNotifier`; instance baru memuat daftar awal saat startup. Entri disimpan selama umur access token + leeway 30 detik (10 menit 30 detik), tidak boleh dibuang lebih awal.
- TOTP RFC 6238: 30 detik, 6 digit, anti-replay `step > last_used_step` (update atomik bersyarat); MFA ditegakkan untuk **semua** pengguna berfaktor aktif: sesi belum terverifikasi hanya boleh `mfa/verify`, `mfa/recovery`, `logout`; admin tanpa faktor hanya `mfa/enroll`, `mfa/confirm`, `logout`; route lain 403 `mfa_required`, kecuali `GET /api/bootstrap` (PRD-001 Q29 butir 2) (ADR-0013 §4); **10** recovery code sekali pakai (hash); wajib untuk role Administrator.
- Reset password: tautan sekali pakai **30 menit**, hash disimpan; reset mencabut semua sesi lain.
- Lockout (Q16, 3 Nov 2026): jeda bertingkat **per pasangan (akun, IP)** (5 gagal: 30 detik berlipat dua, maksimum 15 menit; 20 gagal pada satu pasangan: pasangan diblokir 1 jam; hanya kegagalan dalam **24 jam terakhir** yang dihitung, keputusan pemilik produk 3 Nov 2026, Q19); `locked` pada **akun** hanya bila >= 50 gagal lintas >= 5 IP berbeda dalam 1 jam; unlock lewat reset sandi, command `core:unlock --email`, atau admin; pesan galat login **tidak** membedakan "email tidak ada" dan "password salah"; waktu respons seragam.
- Rahasia: `ERP_SECRET_KEY` 32 byte base64, **wajib hanya untuk `serve` dan command yang menyegel/membuka data (`core:*`)**, yang gagal bila tidak ada/tidak valid; `migrate`, `inspect`, `doctor`, `modules`, `i18n` tidak membutuhkannya (format tetap divalidasi bila terisi; Q16). Rotasi lewat `ERP_SECRET_KEY_PREVIOUS` (kunci lama hanya untuk membuka) dan command `core:reseal` (kelak, bukan C1); format segel membawa **id kunci** dan header (termasuk id kunci) ikut AAD; tanpa rahasia di URL atau log; `Cache-Control: no-store` pada respons auth.
- Batas laju per IP **dan** per principal (atau per identitas sasaran pada endpoint publik: email pada `login`/`forgot`, family pada `refresh`) untuk semua endpoint auth/token, dengan urutan middleware: header, `tenantContext`, batas IP, `Authenticate`, batas principal, permission (ADR-0013 §4).
- Penanda linter kredensial: allowlist **(modul, operationId, properti)** dengan penanda `x-erp-credential-response: true` **pada properti**; hanya `accessToken` (login/refresh), `secret`/`otpauthUri` (enroll MFA), dan `codes` (confirm MFA dan regenerate recovery codes); properti `refresh*` di body selalu ERROR; penanda tingkat operasi saja tidak cukup.
- Endpoint: `POST /api/core/auth/{login,logout,refresh,password/forgot,password/reset,password/change}` (semua publik kecuali `change` dan `logout`; `refresh` publik berbasis cookie), `GET /.well-known/jwks.json`; 401 `unauthenticated`/`token_expired`, 403 `permission_denied`/`mfa_required`, 429 `rate_limited` + `Retry-After`.
- Arsitektur: modul tidak mengimpor platform (D9); kode non-test modul tidak memanggil `identity.WithPrincipal/WithService/WithAdmin` dan `mail.WithSender`, tidak mengimpor `identitytest` atau `testing`, dan **selain `modules/core` tidak memanggil `identity.AdminFrom`/`identity.ServiceFrom`** (`Service`/`Admin` hanya dipasang pada context modul `core` dan alasan sistem `cli:core:*`; keputusan pemilik produk 3 Nov 2026) (aturan `IDENT`; `LoadTyped` memeriksa dengan dan tanpa tag `devtools`); GORM hanya di `identity/store` (setelah allowlist D3b diperbarui); tanpa `AutoMigrate`; tidak ada breaking change API; teks UI lewat i18n; `dangerouslySetInnerHTML` dilarang.
- TDD; satu commit per unit kerja; pesan konvensional; tanpa push atau PR.

## Review Focus

Lima kelas masukan/kegagalan yang tidak dicakup tes tiap task dan paling mungkin menggigit pengguna, dan tes yang memastikannya (ditambahkan ke task pemilik, dijalankan ulang di C1.15):

1. **Waktu respons login** untuk email tak ada vs sandi salah vs akun terkunci tidak boleh berbeda signifikan (tes statistik pada hash tiruan, bukan sekadar pesan seragam): C1.4 dan C1.7.
2. **Refresh paralel**: dua permintaan `refresh` serentak dengan token sama harus menghasilkan tepat satu rotasi sukses dan pencabutan family untuk yang kalah (bukan dua token valid, bukan deadlock): C1.6.
3. **JWT**: `alg: none`/HS256 dengan kunci publik sebagai rahasia, `kid` tak dikenal atau berisi path/SQL, `aud`/`iss` salah, `exp` lewat tepi leeway, token dari kunci `retired`: ditolak 401 tanpa membedakan sebab ke klien: C1.5.
4. **Revokasi lintas instance**: sesi tercabut harus ditolak instance lain segera setelah notifikasi (dan setelah restart instance lewat daftar awal); notifikasi hilang tidak boleh membuat sesi tercabut valid selamanya (batas atas umur access token): C1.6.
5. **Penyalahgunaan alur sekali pakai**: tautan reset dipakai dua kali serentak, kode TOTP/recovery code dipakai ulang serentak, user dengan banyak permintaan `forgot` (enumerasi akun lewat waktu/pesan, banjir email): C1.8 dan C1.9. Juga penguncian akun korban oleh penyerang (DoS lockout): mitigasi jeda per (akun, IP), ambang `locked` lintas >= 5 IP, unlock via reset email, `core:unlock`, atau admin; risiko residual botnet diterima pemilik produk 3 Nov 2026 (ADR-0013 §5).

## Struktur berkas

| Berkas/paket | Tanggung jawab |
| --- | --- |
| `sdk/identity/` | `Principal`, `WithPrincipal`/`FromContext`, antarmuka `Service` (dipakai modul `core`), `identitytest` |
| `sdk/mail/` | `Message`, `Sender`, `WithSender`/`From(ctx)`, `Send` |
| `internal/platform/crypto/` | AES-256-GCM `Seal/Open` berkunci `ERP_SECRET_KEY` |
| `internal/platform/identity/` | password, kebijakan, JWT dan kunci, sesi/refresh/revokasi, rate limit/lockout, TOTP, service, middleware `Authenticate` |
| `internal/platform/identity/store/` | GORM store (control DB) |
| `internal/platform/mail/` | driver `smtp` dan `log` |
| `migrations/control/0005_identity.sql`, `migrations/tenant/0003_roles_min.sql` | skema |
| `modules/core/` | manifest (`required: true`), handler `/api/core/auth/*`, i18n `id`/`en`, `web/` |

## Peta ketergantungan dan gelombang

| Gelombang | Task | Catatan |
| --- | --- | --- |
| 0 | C1.0 dokumen/ADR, C1.1 konfigurasi dan crypto, C1.3 `sdk/identity` dan `sdk/mail` | independen; maksimal 3 agent |
| 1 | C1.2 migrasi dan store, C1.4 password, C1.5 kunci dan JWT | bergantung C1.1 dan C1.3 |
| 2 | C1.6 sesi dan revokasi, C1.7 rate limit dan lockout, C1.8 mailer dan reset, C1.9 MFA | bergantung gelombang 1 |
| 3 | C1.10 middleware dan Authorizer, C1.11 modul core, C1.12 command CLI | bergantung gelombang 2 |
| 4 | C1.13 pensiun `api:dev` dan bootstrap, C1.14 frontend | bergantung C1.10 dan C1.11 |
| 5 | C1.15 keamanan, gerbang, review akhir, verifikasi browser | seluruh C1 |

Berkas yang dimiliki satu task tidak disentuh task lain dalam gelombang yang sama (hindari build pecah akibat edit bersamaan).

---

### Task C1.0: ADR-0013 dan catatan keputusan (dokumen)

**Kelas:** Sonnet (draf), pemilik produk menyetujui.

**Files:** Modify `docs/adr/0013-autentikasi-otorisasi-audit.md` (Proposed menjadi Accepted), `docs/prd/PRD-001-core.md` (tabel keputusan Q14 dan seterusnya), `docs/security/security-guidelines.md` (§2:48 pengecualian kredensial terbatas, §7), `docs/prd/PRD-000-inisialisasi-project.md` §4.2 (D3b) dan Q-number baru, `CLAUDE.md` dan `AGENTS.md` (baris D3b, identik), `internal/archcheck/rules.go` dan `rules_test.go` (allowlist `internal/platform/identity/store`).

**Interfaces:** Produces: keputusan K-C1.1 sampai K-C1.9 tertulis; `archcheck` mengizinkan GORM di `identity/store`.

- [x] **Step 1: Test.** tes allowlist D3b yang sudah ada (`internal/archcheck/rules_test.go` dan `tests/architecture`) gagal sampai `rules.go`, `CLAUDE.md`, `AGENTS.md`, dan PRD-000 §4.2 memuat daftar identik; tambah kasus untuk `internal/platform/identity/store`.
- [x] **Step 2: Jalankan** `go test ./internal/archcheck/... ./tests/architecture/...`, harapkan gagal.
- [x] **Step 3: Tulis ADR-0013** (mekanisme token, rotasi kunci, revokasi, urutan middleware dan kontrak 401/403/`mfa_required`, rate limit, penyimpanan klien, ancaman DoS lockout) dan catat K-C1.1 sampai K-C1.9 lengkap dengan tanggal absolut dan pemilik produk sebagai penyetuju, serta keputusan lanjutan 3 Nov 2026 (PRD-001 Q16); perbarui allowlist di empat tempat sekaligus.
- [x] **Step 4: Jalankan** tes yang sama, hijau; `task lint arch`.
- [x] **Step 5: Commit** `docs(core): ADR-0013 diterima dan keputusan C1; allowlist D3b identity/store`.

### Task C1.1: Konfigurasi `ERP_SECRET_KEY` dan paket crypto

**Kelas:** Sonnet.

**Files:** Create `internal/platform/crypto/{seal.go,seal_test.go}`; Modify `internal/platform/config/{config.go,redact.go}`, `.env.example`, `docs/operations/deployment.md` bila perlu.

**Interfaces:** Produces `crypto.NewSealer(active Key, previous ...Key) (*Sealer, error)` dan `crypto.NewSealerFromMaterial(active []byte, previous ...[]byte) (*Sealer, error)` (kunci aktif menyegel dan membuka; `previous` hanya untuk membuka); `crypto.KeyIDOf(material) byte` (byte pertama SHA-256(material); tabrakan id 1/256 ditolak di `config` dan `NewSealer`); `(*Sealer).Seal(plaintext, aad []byte) ([]byte, error)` dan `(*Sealer).Open(sealed, aad []byte) ([]byte, error)` dengan format segel `[versi 1][id kunci 1][nonce 12][ct+tag]`, header ikut AAD, AAD pemanggil wajib tidak kosong (`ErrEmptyAAD`), galat terpisah `ErrUnknownVersion`/`ErrUnknownKeyID`/`ErrOpen`; `config.Load` mengembalikan Config parsial bersama galat kunci (hanya `doctor` yang boleh memakainya; pemanggil lain wajib berhenti); command `secret:generate`; `doctor` FAIL bila kunci kosong (KeyIDOf, doctor FAIL, dan Config parsial adalah keputusan implementer yang disetujui pemilik produk 3 Nov 2026, ADR-0013 §9); `config.Config.SecretKey` dan `SecretKeyPrevious` (diurai dari `ERP_SECRET_KEY` dan `ERP_SECRET_KEY_PREVIOUS`, 32 byte base64). **Format divalidasi bila terisi pada semua command; kewajiban ada hanya untuk `serve` dan `core:*`** (`migrate`, `inspect`, `doctor`, `modules`, `i18n` tidak butuh; Q16, ADR-0013 §3).

- [x] **Step 1: Test** `TestSealOpenRoundTrip`, `TestOpenRejectsTamperedCiphertext`, `TestOpenRejectsWrongAAD`, `TestSealUsesFreshNonce` (dua Seal atas masukan sama berbeda), `TestConfigSecretKeyInvalidFailsStartup` (untuk `serve`), `TestConfigSecretKeyOptionalForMigrateInspectDoctor` (format tetap divalidasi bila terisi), `TestOpenWithPreviousKeyByKeyID`, `TestSealedHeaderIncludedInAAD`, `TestRedactHidesSecretKey` (nilai tidak muncul di `inspect config`).
- [x] **Step 2:** jalankan, harapkan gagal.
- [x] **Step 3: Implementasi** AES-256-GCM, nonce acak dari `crypto/rand` diawali pada ciphertext, versi format satu byte di depan; parse dan validasi kunci di `config`; samarkan di `redact.go`.
- [x] **Step 4:** tes hijau. **Step 5: Commit** `feat(platform): ERP_SECRET_KEY dan crypto.Sealer AES-256-GCM`.

### Task C1.2: Migrasi dan store identitas

**Kelas:** Sonnet. **Bergantung:** C1.0 (allowlist), C1.1.

**Files:** Create `migrations/control/0005_identity.sql`, `migrations/tenant/0003_roles_min.sql`, `internal/platform/identity/store/{models.go,store.go,store_test.go,store_integration_test.go}`; Modify `internal/app/assemble.go` (wiring `identitystore.New(database.ControlDB{DB: db})`).

**Interfaces:** Produces tabel control `users`, `tenant_memberships`, `user_preferences`, `signing_keys`, `auth_sessions`, `refresh_tokens`, `user_mfa_factors`, `user_recovery_codes`, `password_resets`, `login_attempts`; tabel tenant `roles`, `user_roles` (seed role sistem `administrator`); `store.Store` dengan metode per entitas (nama metode ditetapkan task ini dan didokumentasikan di `doc.go`, dipakai C1.4 sampai C1.9).

**Tambahan 3 Nov 2026 (keputusan pemilik produk, PRD-001 Q24):** tabel tenant `settings` (migrasi tenant `0004_settings`, subset FR-120) beserta paket `internal/platform/settings/store` (GORM, masuk allowlist D3b) dan `internal/platform/settings` (`Service.PublicURL`/`SetPublicURL`/`OriginOf`/`PublicURLOrDevDefault`) dibangun di C1 agar URL publik untuk tautan email dan pemeriksaan `Origin` tidak memakai env; Settings penuh tetap C3.

- [x] **Step 1: Test integrasi** (`pgtest`, tag `integration`): migrasi naik dan turun bersih; `email` unik ternormalisasi (huruf kecil, trim); `refresh_tokens.parent_id` dan `family_id` konsisten; `used_at` tidak dapat di-set dua kali (kondisi `WHERE used_at IS NULL` mengembalikan 0 baris); semua query menyaring `tenant_id` (pola `desk_overrides`); tidak ada `AutoMigrate`.
- [x] **Step 2:** jalankan (VPS lewat tunnel), harapkan gagal.
- [x] **Step 3: Implementasi** migrasi dengan CHECK pada kolom enum (`signing_keys.status` memuat `pending`, `active`, `retiring`, `retired`; `users.status` memuat `locked`), `login_attempts` per (user_id, ip, waktu) untuk jeda per pasangan dan ambang lintas IP (Q16), dan indeks `(user_id)`, `(session_id)`, `(family_id)`, `(token_hash)`; rahasia (secret TOTP, kunci privat) disimpan sebagai `bytea` hasil `Sealer`; hash token sebagai `bytea` SHA-256. Struct `row` dengan `TableName()` mengikuti `desks/store`.
- [x] **Step 4:** hijau; `task test:integration` untuk paket. **Step 5: Commit** `feat(identity): migrasi skema identitas dan store`.

### Task C1.3: `sdk/identity`, `sdk/mail`, dan helper uji

**Kelas:** Sonnet.

**Files:** Create `sdk/identity/{identity.go,identity_test.go}`, `sdk/identity/identitytest/identitytest.go`, `sdk/mail/{mail.go,mail_test.go}`.

**Interfaces:**
- `identity.Kind` (`KindHuman`, `KindService`); `identity.Principal{UserID string; Kind Kind; SessionID string; AuthMethod string; TokenScope []string; AMR []string}` dengan method `MFAVerified() bool` (true bila `AMR` memuat `otp`); `identity.WithPrincipal(ctx, Principal) context.Context`; `identity.FromContext(ctx) (Principal, bool)`.
- `identity.Service` (dipakai modul `core`; **selesai**): `Login(ctx, LoginInput) (LoginResult, error)`, `Refresh(ctx, RefreshInput) (LoginResult, error)`, `Logout(ctx) error` (pengguna dan sesi diturunkan dari Principal di context lewat `RequirePrincipal`, tanpa parameter sesi, sehingga tak ada jalur IDOR), `ForgotPassword(ctx, ForgotInput) error`, `ResetPassword(ctx, ResetInput) error`, `ChangePassword(ctx, ChangeInput) error`, serta metode MFA (`BeginMFAEnrollment`, `ConfirmMFA`, `VerifyMFA`, `RegenerateRecoveryCodes`, semuanya dari Principal). `LoginResult.MFAState` (`none`/`enroll_required`/`verify_required`) menggantikan flag boolean. `identity.Admin` (`CreateAdmin`, `RotateSigningKeys`, `ResetMFA`, `UnlockAccount`, `LogoutAllDevices`) dengan `WithAdmin/AdminFrom` dipakai command CLI (C1.12) dan layar admin; implementasi menegakkan otorisasi sendiri (principal berhak atau hak sistem teraudit). Tipe input/hasil berada di paket yang sama (tanpa tipe platform); `WithPrincipal`, `WithService`, `WithAdmin`, `mail.WithSender` hanya dipanggil platform (aturan arch `IDENT`, PRD-000 §4.2).
- `mail.Message{To []string; Subject, Text, HTML string}`; `mail.Sender` (`Send(ctx, Message) error`); `mail.WithSender/From(ctx)`; `mail.Send(ctx, Message) error` memakai sender di context.

- [x] **Step 1: Test** `TestFromContextEmpty`, `TestWithPrincipalRoundTrip`, `TestPrincipalIsValueCopy`, `TestMailSendWithoutSenderFailsClosed`; arsitektur: `sdk/identity` dan `sdk/mail` hanya mengimpor stdlib (D9).
- [x] **Step 2:** gagal. **Step 3:** implementasi (tanpa dependensi baru; `identitytest.WithPrincipal(t, ctx, ...)` untuk uji modul). **Step 4:** hijau; `task arch`. **Step 5: Commit** `feat(sdk): identity.Principal, identity.Service, dan mail.Sender`.

### Task C1.4: Password dan kebijakan sandi

**Kelas:** Sonnet. **Bergantung:** C1.1, C1.3.

**Files:** Create `internal/platform/identity/password/{hash.go,policy.go,commonpasswords.txt,*_test.go}`; Modify `internal/platform/config/config.go` (parameter argon2 `ERP_ARGON2_*` dengan default OWASP).

**Interfaces:** `password.Hasher{Params}` dengan `Hash(plain string) (phc string, err error)`; `Verify(plain, phc string) (ok bool, needsRehash bool, err error)`; `password.Dummy() *Hasher` untuk hash tiruan pada email tak dikenal; `password.CheckPolicy(plain string) []Violation` (panjang minimal 12, tidak di daftar sandi umum, maksimum 1024 byte). Sandi dinormalisasi NFKC sebelum `Hash`/`Verify`/`CheckPolicy` (keputusan pemilik produk 3 Nov 2026, Q17; batas 1024 byte pada masukan mentah sebelum normalisasi). Diimplementasikan: `Dummy()`, `NewDummy(Params)`, `(*Hasher).DummyHash()`, `Params.ValidateConfig()`.

- [x] **Step 1: Test** format PHC (`$argon2id$v=19$m=..,t=..,p=..$salt$hash`); `Verify` benar/salah dan konstan-waktu (`subtle.ConstantTimeCompare`); `needsRehash` saat parameter naik; kebijakan menolak `"password1234"` (daftar), `"short"`, 1025 byte; **Review Focus 1:** `TestVerifyUnknownUserTimingMatchesKnownUser` (rasio median waktu ≤ 1,2 pada 200 iterasi dengan parameter uji kecil).
- [x] **Step 2:** gagal. **Step 3:** implementasi dengan `golang.org/x/crypto/argon2`; daftar sandi umum di-embed (`//go:embed`) sebagai himpunan dalam memori; sandi tidak pernah masuk log atau pesan galat. **Step 4:** hijau; `govulncheck` untuk dependensi. **Step 5: Commit** `feat(identity): argon2id, kebijakan sandi, dan hash tiruan`.

### Task C1.5: Kunci penandatangan, JWT EdDSA, JWKS

**Kelas:** Sonnet. **Bergantung:** C1.1, C1.2.

**Files:** Create `internal/platform/identity/token/{keys.go,jwt.go,jwks.go,*_test.go}`.

**Interfaces:** `token.Keyring` (`Active() Key`, `ByKID(kid string) (Key, bool)`, `Rotate(ctx) (Key, error)`, status `pending/active/retiring/retired`; muat ulang lewat `StateNotifier` `identity.keys_changed` dan sekali (dibatasi laju) saat `kid` tak dikenal); `token.Issuer.Issue(Claims) (string, error)`; `token.Verifier.Verify(raw string) (Claims, error)` (hanya `alg=EdDSA`, `kid` wajib dan dicari di keyring, validasi `iss/aud/exp/iat` dengan leeway 30 detik); `token.JWKSHandler(Keyring) http.Handler` (`Cache-Control: public, max-age=300`, hanya kunci `pending`, `active`, dan `retiring`).

- [x] **Step 1: Test** vektor Ed25519 RFC 8032; Review Focus 3: `TestVerifyRejectsNoneAlg`, `TestVerifyRejectsHS256WithPublicKeyAsSecret`, `TestVerifyRejectsUnknownKID`, `TestVerifyRejectsKIDWithPathOrSQL`, `TestVerifyRejectsWrongIssuerAudience`, `TestVerifyExpiryEdgeWithLeeway`, `TestVerifyRejectsRetiredKey`; `TestRotateKeepsOldTokensValidWhileRetiring` (rotasi tanpa downtime); `TestPendingKeyPublishedInJWKSButNotUsedToSign`, `TestRetiringBecomesRetiredOnlyAfterTokenTTLPlusLeeway`, `TestUnknownKIDReloadsOnceRateLimited`, `TestPromoterActivatesPendingAfterJWKSMaxAge` (pekerja latar `serve` mempromosikan `pending` menjadi `active` setelah >= 300 detik dan memicu `identity.keys_changed`; saat start bila sudah lewat); `TestPrivateKeyStoredSealed` (kolom privat tidak berisi bytes kunci mentah); JWKS tidak membocorkan kunci privat dan tidak memuat `retired`.
- [x] **Step 2:** gagal. **Step 3:** implementasi JWT kompak (header `{"alg":"EdDSA","kid":...}`), parser ketat (tiga segmen base64url tanpa padding, batas ukuran 4 KB, tolak header ekstra yang tidak dikenal), kunci privat disegel `Sealer`, cache keyring di memori dengan muat ulang saat `Rotate`. **Step 4:** hijau. **Step 5: Commit** `feat(identity): kunci Ed25519, JWT EdDSA, dan JWKS`.

### Task C1.6: Sesi, refresh berotasi, dan revokasi

**Kelas:** Sonnet. **Bergantung:** C1.2, C1.5.

**Files:** Create `internal/platform/identity/session/{service.go,revocation.go,*_test.go,*_integration_test.go}`; Modify `internal/app/assemble.go` (muat daftar awal dan langganan `StateNotifier`).

**Interfaces:** `session.Service.Start(ctx, StartInput) (Session, RefreshToken, error)`; `Rotate(ctx, rawRefresh string, origin string) (Session, RefreshToken, error)` (deteksi reuse mencabut family dan sesi, mencatat event keamanan); `Revoke(ctx, sessionID, reason string) error`; `RevokeAllForUser(ctx, userID string) error`; `session.RevocationList` (`IsRevoked(sid string) bool`, `Add(sid string, until time.Time)`, `Load(ctx)`), dengan penyebaran lewat `StateNotifier` kanal `identity.session_revoked`.

- [x] **Step 1: Test integrasi** `TestRotateIssuesNewTokenAndMarksParentUsed`; `TestReuseOfUsedTokenRevokesFamilyAndSession`; Review Focus 2: `TestConcurrentRefreshSameTokenExactlyOneSucceeds` (20 goroutine, tepat satu sukses, family tercabut, tanpa deadlock); `TestAbsoluteLifetime90DaysEnforced`; `TestSlidingExpiry14Days`; Review Focus 4: `TestRevocationVisibleToOtherInstanceViaNotifier`, `TestRevocationListLoadedOnStartup`, `TestRevocationEntriesRetainedForAccessTokenTTLPlusLeeway` (entri tidak dibuang sebelum 10 menit 30 detik); `TestRefreshRejectsMissingOrCrossOriginOrigin`; `TestDisabledUserSessionsRevokedImmediately`.
- [x] **Step 2:** gagal. **Step 3:** implementasi rotasi dalam satu transaksi dengan `UPDATE ... SET used_at=now() WHERE id=$1 AND used_at IS NULL RETURNING` (pemenang tunggal), token acak 32 byte `crypto/rand` base64url dengan hash SHA-256 disimpan; daftar tercabut `sync.RWMutex` + map berumur. **Step 4:** hijau (VPS). **Step 5: Commit** `feat(identity): sesi, refresh berotasi, dan revokasi lintas instance`.

### Task C1.7: Rate limit, lockout, dan 429

**Kelas:** Sonnet. **Bergantung:** C1.2, C1.4.

**Files:** Create `internal/platform/identity/guard/{limiter.go,lockout.go,*_test.go}`; Modify `sdk/httpx` (tambah `TooManyRequests(w, r, retryAfter)` dengan header `Retry-After` dan kode `rate_limited`), katalog i18n platform bila kunci galat belum ada.

**Interfaces:** `guard.IPLimiter.Allow(ip string) (ok bool, retryAfter time.Duration)` (`x/time/rate`, per IP) dan `guard.PrincipalLimiter.Allow(key string) (ok bool, retryAfter time.Duration)` (per principal atau per identitas sasaran: email pada `login`/`forgot`, family pada `refresh`; dipasang di urutan middleware ADR-0013 §4); `guard.Lockout.RegisterFailure(ctx, userID, ip string) (State, error)`; `guard.Lockout.RegisterSuccess(ctx, userID, ip string) error`; `State{PairBlocked bool; AccountLocked bool; RetryAfter time.Duration}` menurut Q16: **per pasangan (akun, IP)** 5 gagal memicu jeda 30 detik berlipat dua (maksimum 15 menit), 20 gagal memblokir pasangan 1 jam, keduanya hanya atas kegagalan dalam **24 jam terakhir** (`LockoutConfig.PairWindow`, `Store.PairFailureStateSince`; keputusan pemilik produk 3 Nov 2026, Q19); **`AccountLocked`** hanya bila >= 50 gagal lintas >= 5 IP berbeda dalam 1 jam; `Lockout.Unlock(ctx, userID)` dipakai reset sandi, `core:unlock`, dan admin (`identity.Admin.UnlockAccount`).

**Tindak lanjut C1.4 (keputusan pemilik produk 3 Nov 2026, PRD-001 Q17):** (a) email tak dikenal diverifikasi lewat `password.NewDummy(cfg.Argon2)` (BUKAN `password.Dummy()`, supaya biaya identik dengan Hasher sungguhan bila `ERP_ARGON2_*` non-default) dengan `d.Verify(plain, d.DummyHash())`, hasilnya diabaikan; (b) `ErrInvalidHash` dari `Hasher.Verify` pada akun nyata diperlakukan seperti sandi salah (dicatat sebagai event keamanan, respons seragam); (c) **pembatas hash serentak**: semaphore berkapasitas terbatas (mis. dari `ERP_ARGON2_MAX_CONCURRENT`, default berdasar RAM/CPU) membungkus setiap `Hash`/`Verify`/dummy agar login serentak tidak menghabiskan RAM (19 MiB per `Verify`); saat penuh, permintaan menunggu dengan batas waktu lalu 429 `rate_limited` + `Retry-After` (tidak boleh membedakan email ada/tidak). Dikerjakan di sini, bukan di C1.4.

- [x] **Step 1: Test** (tambahan tindak lanjut C1.4: `TestUnknownEmailUsesDummyWithConfiguredParams`, `TestInvalidHashOnRealAccountTreatedAsWrongPassword`, `TestConcurrentVerifyBoundedBySemaphore` yang membuktikan jumlah `Verify` bersamaan tidak melebihi kapasitas) jeda bertingkat per pasangan (akun, IP) tepat pada gagal ke-5 dan seterusnya; pemblokiran pasangan pada gagal ke-20 (1 jam) tanpa mengunci akun bagi IP lain; kegagalan > 24 jam tidak dihitung, <= 24 jam dihitung, blokir 20 gagal tidak memberi jatah baru penuh setelah 1 jam (hitungan masih 20 dalam jendela), `Unlock` membersihkan blokir pasangan pada akun active; `AccountLocked` hanya pada >= 50 gagal lintas >= 5 IP dalam 1 jam dan tidak terbuka sendiri; `PrincipalLimiter` menjawab 429 per principal dan per sasaran; sukses mereset hitungan; penghitung disimpan di DB (`login_attempts`) agar berlaku lintas instance; kegagalan untuk email tak dikenal tidak tercatat di `login_attempts` (`user_id` NOT NULL, keputusan implementer C1.2, PRD-001 Q18), sehingga jalur akun-dikenal punya satu tulis DB tambahan: seragamkan waktu respons kedua jalur (tes `TestLoginTimingUnknownEmailMatchesKnownEmailFailure`, mis. tulis tiruan atau penundaan seragam); IP limiter menjawab 429 + `Retry-After` bilangan bulat; Review Focus 1: respons terkunci tidak lebih cepat dari respons sandi salah (waktu diseragamkan); **DoS lockout:** satu IP penyerang tak dapat mengunci akun korban maupun mengganggu login sah dari IP lain (uji: IP A gagal 25 kali, user dari IP B tetap bisa login tanpa jeda); risiko residual botnet (>= 50 gagal lintas >= 5 IP) diterima pemilik produk (Q16); unlock lewat reset sandi, `core:unlock`, atau admin.
- [x] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau. **Step 5: Commit** `feat(identity): rate limit per IP, lockout bertingkat, dan 429 Retry-After`.

### Task C1.8: Mailer dan reset password

**Kelas:** Sonnet. **Bergantung:** C1.2, C1.3, C1.4, C1.7.

**Files:** Create `internal/platform/mail/{smtp.go,log.go,*_test.go}`, `internal/platform/identity/reset/{service.go,templates/{id,en}/*.tmpl,*_test.go}`; Modify `internal/platform/config/config.go` (`ERP_MAIL_DRIVER`, `ERP_SMTP_*`, `ERP_SMTP_PASSWORD` tersamarkan), `internal/app/assemble.go`.

**Keputusan pemilik produk 3 Nov 2026 (PRD-001 Q20 sampai Q23):** (a) tidak ada `ERP_PUBLIC_URL`: `reset.Config.PublicURL` bertipe `PublicURLSource` (`PublicURL(ctx) (string, error)`, dipenuhi `settings.Service`); belum dikonfigurasi/galat/tidak sah = `Request` tetap seragam, peringatan dicatat, tidak ada email; (b) `config.Config.RequireMailDriver()` dipanggil `serve` (production gagal start bila `ERP_MAIL_DRIVER` tidak diisi; `log` sah bila eksplisit); (c) tautan `<url-publik>/reset-password#token=<token>`; (d) layanan reset mengirim di latar terbatas, driver mail tetap sinkron. API sebenarnya: `mail.NewSMTP(config.SMTPConfig, ...SMTPOption)` dan `mail.NewLog(logger, ...LogOption)` (paket tunggal `internal/platform/mail`); `reset.WithClientIP(ctx, ip)` memasok IP untuk batas laju per IP.

**Catatan Q24 (3 Nov 2026):** `settings.Service` (paket `internal/platform/settings`) sudah tersedia dari C1.2 dan memenuhi `PublicURLSource`; `serve` di production gagal start bila `PublicURL` mengembalikan `ErrNotConfigured` (penegakan oleh perakit, C1.10/C1.12).

**Interfaces:** `mail/smtp.New(cfg) mail.Sender` (STARTTLS atau TLS, autentikasi, timeout, retry singkat); `mail/log.New(log) mail.Sender` (default di development, tanpa koneksi jaringan, isi tautan tidak ditulis ke log produksi); `reset.Service.Request(ctx, email, locale string) error` (selalu sukses secara eksternal, tanpa membedakan akun ada/tidak); `reset.Service.Complete(ctx, rawToken, newPassword string) error` (sekali pakai, 30 menit, mencabut semua sesi, membuka `locked`).

- [x] **Step 1: Test** driver `log` tidak membuka soket; SMTP diuji terhadap server uji lokal (container Mailpit di VPS atau fake `net.Listener`) termasuk STARTTLS wajib bila dikonfigurasi; sandi SMTP tidak muncul di log atau `inspect config`; `Request` untuk email tak ada dan ada: respons dan waktu setara (Review Focus 5); token disimpan sebagai hash; `Complete` dua kali serentak: tepat satu sukses; token kedaluwarsa ditolak; sandi lama tidak berlaku dan sesi lain dicabut; batas permintaan `forgot` per email dan per IP (banjir email dicegah, 429); templat `id` dan `en` sesuai preferensi (`user_preferences.locale`).
- [x] **Step 2:** gagal. **Step 3:** implementasi (`net/smtp` + `crypto/tls`, `html/template` untuk HTML). **Step 4:** hijau. **Step 5: Commit** `feat(identity): mailer smtp/log dan reset password`.

### Task C1.9: MFA TOTP dan recovery code

**Kelas:** Sonnet. **Bergantung:** C1.2, C1.4, C1.7.

**Files:** Create `internal/platform/identity/mfa/{totp.go,recovery.go,service.go,*_test.go}`.

**Interfaces:** `mfa.TOTP.Code(secret []byte, t time.Time) string`, `Verify(secret []byte, code string, t time.Time, lastStep int64) (step int64, ok bool)` (jendela ±1 langkah); `mfa.Service.BeginEnrollment(ctx, userID) (EnrollmentInfo{OTPAuthURI string, Secret string}, error)`; `ConfirmEnrollment(ctx, userID, code string) (recoveryCodes []string, error)`; `Verify(ctx, userID, code string) error`; `UseRecoveryCode(ctx, userID, code string) error`; `Reset(ctx, actor, userID)` (tercatat); `Required(ctx, userID) (bool, error)` (true bila role `administrator`).

**Catatan Q27 (3 Nov 2026, disetujui pemilik produk):** konfirmasi pendaftaran dan verifikasi MFA kini menaikkan sesi ke `otp` **dengan merotasi refresh token** lewat `session.Service.Upgrade` (C1.6/C1.10b; atomik, tanpa perubahan skema); `mfa.Service` tidak berubah (rotasi dilakukan `identity.Service` setelah verifikasi berhasil, karena kenaikan sesi bukan urusan paket `mfa`).

- [x] **Step 1: Test** vektor RFC 6238 (SHA-1, waktu 59, 1111111109, 1111111111, 1234567890, 2000000000, 20000000000); kode yang sama pada langkah yang sama ditolak (anti-replay: hanya `step > last_used_step` diterima, update atomik bersyarat; kode langkah sebelumnya dalam jendela ±1 juga ditolak); Review Focus 5: dua verifikasi serentak dengan kode sama, tepat satu sukses (pembaruan `last_used_step` bersyarat); 10 recovery code, hash disimpan, sekali pakai, dua pemakaian serentak tepat satu sukses; verifikasi memakai lockout FR-101; secret TOTP tersimpan tersegel; pendaftaran tidak aktif sebelum `ConfirmEnrollment`.
- [x] **Step 2:** gagal. **Step 3:** implementasi RFC 6238 sendiri (`crypto/hmac`, `crypto/sha1` hanya untuk HOTP per RFC 4226/6238; pengecualian SHA-1 disetujui pemilik produk 3 Nov 2026 dan dicatat di security-guidelines §3.4 dan ADR-0013 §6; penekanan gosec G401/G505 hanya di paket `mfa` dengan `//nolint:gosec` beralasan tertulis dan komentar rujukan RFC), pembanding konstan-waktu. **Step 4:** hijau. **Step 5: Commit** `feat(identity): MFA TOTP, recovery code, dan anti-replay`.

### Task C1.10: Middleware `Authenticate`, Principal, dan `Authorizer` bertipe

**Kelas:** Sonnet. **Bergantung:** C1.5, C1.6, C1.9.

**Files:** Create `internal/platform/identity/middleware.go`, `*_test.go`; Modify `internal/platform/httpserver/server.go` (urutan: header, `tenantContext`, batas laju IP, `Authenticate` dengan pencocokan `tid`, batas laju per principal (`guard.PrincipalLimiter`), permission), `internal/platform/modules/runtime/mount.go` (`Authorizer` bertipe, `permissionMW` memetakan 401/403/`mfa_required`), `sdk/module` (opsi rute `RequireMFA`), `internal/platform/modules/registry/{recorder.go,types}` (`RouteInfo.RequireMFA`), `internal/app/assemble.go` (mengganti stub authN; **perakitan pembatas hash serentak dari C1.7 (semaphore dari konfigurasi) ke `Hasher`/service identity agar berlaku di semua jalur login, ubah sandi, dan reset (tindak lanjut C1.4, PRD-001 Q17)**; `identity.Service`/`identity.Admin` dipasang **hanya** pada context modul `core` dan alasan sistem `cli:core:*`, tidak pada modul lain; keputusan pemilik produk 3 Nov 2026, dijaga aturan `IDENT`; `identity.Principal` tetap dipasang `Authenticate` untuk semua route), `tests/security/public_routes.txt`, `contracts/openapi/platform.yaml` (`/.well-known/jwks.json`).

**Interfaces:** `runtime.Authorizer.Decide(ctx, permission string, opts RouteOptions) Decision` (`Allow`, `Unauthenticated`, `TokenExpired`, `Forbidden`, `MFARequired`); `Allow(ctx, permission) bool` lama dipertahankan sebagai adaptor agar stub dan modul tidak berubah; `identity.Authenticate(deps) func(http.Handler) http.Handler` (membaca `Authorization: Bearer`, memverifikasi token, memeriksa `sid` pada daftar tercabut, membangun `Principal`, mencocokkan `tid` dengan tenant context, menolak token `scope`-terbatas di endpoint refresh).

**Catatan Q24 (3 Nov 2026):** pemeriksa `Origin` endpoint refresh membandingkan header `Origin` dengan `settings.OriginOf(PublicURL(ctx))`; URL publik tidak pernah diambil dari header permintaan. Perakit memanggil `PublicURL` saat start `serve` di production dan gagal start bila `settings.ErrNotConfigured`; di development memakai `PublicURLOrDevDefault(ctx, settings.DevURLFromHTTPAddr(cfg.HTTPAddr))`.

- [x] **Step 1: Test** token valid menghasilkan Principal di context; tanpa header 401 `unauthenticated`; `exp` lewat 401 `token_expired`; `sid` tercabut 401; `tid` tidak cocok 401; aturan MFA ADR-0013 §4 untuk **semua** pengguna: sesi berfaktor aktif tetapi belum terverifikasi hanya boleh `mfa/verify`, `mfa/recovery`, `logout`; administrator tanpa faktor hanya `mfa/enroll`, `mfa/confirm`, `logout`; route lain 403 `mfa_required`, kecuali `GET /api/bootstrap` (Q29 butir 2); pengguna non-admin dengan TOTP aktif tidak lolos hanya dengan sandi; `RequireMFA` pada route menuntut `amr` berisi `otp`; stub lama masih lolos tes modul yang ada; rute `Public()` tak diautentikasi; semua kegagalan token tidak membedakan sebab ke klien selain `token_expired`; `GET /.well-known/jwks.json` publik dan terdaftar di allowlist keamanan.
- [x] **Step 2:** gagal. **Step 3:** implementasi; autentikasi nyata menggantikan stub authN sepenuhnya; `ERP_AUTHZ_STUB` hanya tetap menentukan keputusan permission sampai C2 (K-C1.9), tanpa flag baru untuk menonaktifkan autentikasi (ADR-0013 menetapkan perilaku development, mis. admin dibuat lewat `core:create-admin`). **Step 4:** hijau; `task test:integration` dan `tests/security`. **Step 5: Commit** `feat(identity): Authenticate, Principal, dan keputusan otorisasi bertipe`.

### Task C1.11: Modul `core` dan kontrak API

**Kelas:** Sonnet. **Bergantung:** C1.10, C1.8, C1.9.

**Catatan Q27 (3 Nov 2026, disetujui pemilik produk) untuk handler:** `ConfirmMFA` mengembalikan `ConfirmMFAResult{Codes, Session}` dan `VerifyMFA` mengembalikan `LoginResult`; **keduanya membawa `RefreshToken`/`RefreshExpiresAt` baru yang wajib ditulis ulang handler ke cookie refresh** (token pra-MFA mati). Respons `mfa/confirm` memuat `codes` **dan** access token otp (`session.accessToken`, `expiresIn`); respons `mfa/verify`/`mfa/recovery` memuat `accessToken` dan `mfaState`. Allowlist `x-erp-credential-response` tingkat properti harus mencakup `accessToken` pada confirm dan verify MFA. Frontend tidak memanggil `refresh` setelah confirm bila respons memuat token. Pemetaan galat memakai `sdk/identity/errors.go` (termasuk `*RateLimitedError` dan `*PasswordPolicyError`); `RefreshInput.Origin` diisi dari header `Origin`; `Admin.SetPublicURL` tersedia untuk `core:settings:set` (C1.12).

**Files:** Create `modules/core/**` (dibangkitkan `task erpora -- make:module core` lalu disesuaikan; `module.json` `required: true`, `capabilities` berawalan `core.`, `desk`), `modules/core/openapi.yaml`, `modules/core/i18n/{id,en}.json`; Modify `internal/app/modules.go`, `go.work`, `contracts/spectral-functions/*` (rule `erp-credential-response-valid`/`-exception`; allowlist **(modul, operationId, properti)** tingkat properti: `accessToken` login/refresh, `secret`/`otpauthUri` enroll MFA, `codes` confirm MFA dan regenerate recovery codes; penanda pada properti; `refresh*` di body selalu ERROR; penanda tingkat operasi saja tidak cukup) dan `docs/security/security-guidelines.md` §2:48 (sudah ditulis di C1.0), `internal/app/assemble.go` (menyuntik `identity.Service` dan `identity.Admin` ke `runtime.Services` khusus modul `core`, dan `mail.Sender` ke modul yang membutuhkannya).

**Interfaces:** handler `POST /api/core/auth/{login,logout,refresh}`, `POST /api/core/auth/password/{forgot,reset,change}`, `POST /api/core/auth/mfa/{enroll,confirm,verify,recovery}`; respons login `{data: {accessToken, expiresIn, mfaState}}` dengan penanda `x-erp-credential-response: true` pada properti `accessToken` (dan pada `secret`/`otpauthUri` di enroll MFA serta `codes` di confirm MFA dan regenerate recovery codes); refresh token **hanya** lewat cookie; kode galat: `unauthenticated`, `token_expired`, `invalid_credentials` (seragam), `account_locked`, `mfa_required`, `rate_limited`, `password_policy_violation` (+ details tanpa membocorkan daftar).

- [x] **Step 1: Test** `task contracts:check` hijau; kesesuaian route, permission, dan klasifikasi; handler memakai `httpx.DecodeJSON`; `Cache-Control: no-store` pada semua respons auth; `Set-Cookie` refresh dengan atribut `HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh`; login benar mengembalikan `accessToken` dan tidak mengembalikan refresh token di body; **IDOR:** `password/change`, `logout`, dan MFA hanya untuk Principal sendiri (tes pengguna A vs B: A tak dapat mencabut sesi B atau mengubah MFA B); `module:check` lolos; modul `core` tidak dapat di-`disable` atau `uninstall` (`module_required`).
- [x] **Step 2:** gagal. **Step 3:** implementasi (handler tipis memanggil `identity.Service` dari context, tidak mengimpor platform). **Step 4:** hijau; `task contracts:check`; `task erpora -- module:check`. **Step 5: Commit** `feat(core): modul core required dengan API autentikasi`.

### Task C1.12: Command CLI `core:create-admin`, `core:rotate-keys`, `core:reset-mfa`, `core:unlock`

**Kelas:** Sonnet. **Bergantung:** C1.9, C1.11.

**Files:** Modify `sdk/module/module.go` (`Invocation.In io.Reader`, aditif), `internal/app/command.go` (mengisi `In`, memastikan `AsSystem("cli:<name>")` tercatat), `cmd/erpora/module_commands.go`; Create command di `modules/core/module.go` dan `modules/core/internal/application`.

**Interfaces:** `core:create-admin --email <e> [--name <n>]` membaca sandi dari prompt tersembunyi (TTY, `x/term`) atau stdin, tidak ada flag sandi; menerapkan kebijakan sandi; memberi role `administrator`; `core:rotate-keys` (hanya membuat kunci `pending` dan melaporkan waktu aktivasi; promosi oleh pekerja latar `serve`); `core:reset-mfa --email <e>`; `core:unlock --email <e>` (membuka akun `locked`, di luar teks asli FR-101, Q16). Semua command memakai `identity.AdminFrom(ctx)` (`CreateAdmin`, `RotateSigningKeys`, `ResetMFA`, `UnlockAccount`) di bawah `authz.AsSystem("cli:<name>")`; command `core:*` mewajibkan `ERP_SECRET_KEY` valid (menyegel/membuka data), sedangkan `migrate`/`inspect`/`doctor`/`modules`/`i18n` tidak; `core:reseal` ditunda ke fase kelak.

**Tambahan Q24 (3 Nov 2026):** command `core:settings:set` (mis. `--key platform.public_url --value <url>`; hanya kunci `platform.public_url` yang dikenal di C1) memanggil `settings.Service.SetPublicURL(ctx, url, "cli:core:settings:set")` dengan hak sistem teraudit; ini satu-satunya jalur pengisian awal URL publik sampai UI Settings (C3).

**Catatan CI (4 November 2026):** ganti langkah SQL `platform.public_url` pada job smoke CI (`.github/workflows/ci.yml`, varian Linux dan Windows) dengan `erpora core:settings:set --key platform.public_url --value https://erp.example.com`.

**Catatan Q28 (disetujui pemilik produk 4 November 2026):** `Admin.CreateAdmin` idempoten untuk sisa kegagalan (akun nonaktif, tanpa role, belum login, tanpa sesi): memanggil ulang dengan email sama melanjutkan akun; bukan lagi `ErrEmailTaken` untuk sisa seperti itu. **Varian stack CLI (tanpa `Service`/server HTTP) dikerjakan di C1.12**, bukan di C1.10b.

- [x] **Step 1: Test** admin dibuat dan dapat login; sandi tidak muncul di argumen proses, log, atau keluaran; sandi lemah ditolak; `create-admin` untuk email akun aktif/berrole/pernah login menolak (`ErrEmailTaken`), sedangkan sisa kegagalan (nonaktif, tanpa role, belum login, tanpa sesi) dilanjutkan (Q28); `rotate-keys` hanya membuat kunci `pending` baru (terpublikasi di JWKS, tidak dipakai menandatangani) dan melaporkan `KeyInfo.Status`/`ActivatesAt`; setelah >= 300 detik pekerja latar `serve` mempromosikannya menjadi `active` dan yang lama `retiring` (uji dengan jam tiruan; promosi juga terjadi saat `serve` start bila waktu aktivasi sudah lewat, dan memicu `identity.keys_changed`); `reset-mfa` menghapus faktor dan tercatat; `unlock` membuka akun `AccountLocked` dan pasangan terblokir, tercatat; semua command hanya berjalan bila modul `core` enabled (perilaku yang ada) dan memakai hak sistem teraudit.
- [x] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau; `tests/integration`. **Step 5: Commit** `feat(core): command create-admin, rotate-keys, reset-mfa`.

### Task C1.13: Pensiun aktor `api:dev`, bootstrap berautentikasi

**Kelas:** Sonnet. **Bergantung:** C1.10.

**Files:** Modify `internal/platform/modules/admin/api.go:35`, `internal/platform/i18n/admin/overrides.go:34` (aktor dari `identity.FromContext`; CLI tetap `cli:<name>`), `internal/platform/bootstrap/handler.go` dan `contracts/openapi/platform.yaml` (bootstrap tidak lagi `Public()`, menambah `user` dan `permRev` secara aditif; `permissions` tetap sampai C2), `internal/platform/httpserver/routes.go:205`.

- [x] **Step 1: Test** (dikerjakan bersama C1.10 di commit `a070c0b`) aksi admin modul/i18n mencatat `actor` = `user:<id>` dari Principal (bukan `api:dev`); `/api/bootstrap` tanpa token 401; dengan token mengembalikan `user` dan `permRev`; perubahan kontrak hanya aditif (`contracts:breaking` hijau); ETag bootstrap bervariasi per pengguna (tidak ada cache lintas pengguna: `Vary: Authorization`, `Cache-Control: private`).
- [x] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau. **Step 5: Commit** `feat(platform): aktor dari Principal dan bootstrap berautentikasi`.

### Task C1.14: Frontend login, MFA, reset, dan klien API berautentikasi

**Kelas:** Sonnet. **Bergantung:** C1.11, C1.13.

**Keputusan PRD-001 Q33 (4 Nov 2026):** layar auth milik modul core, dipasang shell sebagai gerbang pra-bootstrap di `/login`, `/forgot-password`, `/reset-password` (slug tercadang); `/core/change-password` halaman modul biasa; desk core minimal (manifest `desk`); QR `uqr` lazy; kode memakai shadcn `input-otp`.

**Files:** Create `modules/core/web/**` (gerbang `login`, `mfa`, `forgot`, `reset`; halaman `change-password`; i18n; `defineModule`; `desk` di `module.json`), `packages/platform-web/src/auth/**` (penyimpan access token di memori, refresh single-flight pada 401, deteksi `mfa_required`); Modify `packages/platform-web/src/api/client.ts` (hook `headers`, `credentials: "include"` hanya untuk endpoint refresh), `src/bootstrap/{fetcher.ts,provider.tsx}`, `src/sources/*` (CurrentUserSource sungguhan menggantikan `anonymousUserSource`), `web/src/app/{providers,router,shell/user-menu.tsx}` (guard sesi, tombol keluar), `packages/platform-web/src/permissions/index.ts` (tetap optimistis sampai C2; tidak diubah di C1).

- [x] **Step 1: Test** `bun test`: token tidak pernah ditulis ke `localStorage`/`sessionStorage`/`document.cookie` (uji dengan spy storage); refresh single-flight (tiga 401 serentak memicu satu refresh); 401 setelah refresh gagal mengarahkan ke login dan membersihkan cache query (`scopedKey`); `returnTo` hanya path relatif (tolak `//evil`, `https://...`, `javascript:`); layar MFA muncul saat `mfa_required`; semua teks lewat i18n; tidak ada `dangerouslySetInnerHTML`; `task web:check` dalam anggaran bundle 210 KB gzip (halaman auth dimuat lazy).
- [x] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau; `task web:check` (anggaran 210 KB, PRD-000 Q43; JS awal 200,3 KB). **Step 5: Commit** `feat(web): login, MFA, reset sandi, dan klien API berautentikasi`.

### Task C1.15: Keamanan, gerbang, review akhir, verifikasi browser (checkpoint C1)

**Kelas:** Sonnet (gerbang), review akhir Opus independen.

- [ ] **Step 1: Gerbang** `task lint arch check:gowork-off test`; `task test:integration` (VPS); `task contracts:check`; `task web:check`; `task test:generator`; `govulncheck ./...`; jalankan ulang semua tes Review Focus 1 sampai 5.
- [ ] **Step 2: Checklist ASVS sesi** (V2, V3, V6, V7 yang relevan) di `docs/security/security-guidelines.md` §6 dan §7 diperbarui; `docs/module-sdk/security-primitives.md` memuat `sdk/identity` dan `sdk/mail`; ADR-0013 Accepted; PRD-001 tabel keputusan diperbarui; milestone C1 ditandai selesai.
- [ ] **Step 3: Review akhir independen** (Opus, tanpa konteks implementer): kesesuaian dengan PRD-001 dan ADR-0013 serta `security-audit` terfokus pada permukaan C1; temuan yang sesuai spek diperbaiki implementer yang sama, yang di luar spek dikonfirmasi dulu; satu review atas diff perbaikan.
- [ ] **Step 4: Verifikasi browser** (aplikasi penuh dengan DB sementara di VPS): `core:create-admin`, login, wajib MFA Administrator, logout, refresh setelah 10 menit (token kedaluwarsa), reset sandi lewat driver `log`, lockout; inspeksi storage browser kosong dari token; DB sementara dihapus.
- [ ] **Step 5: Commit** dan laporkan: yang dikerjakan, yang menunggu konfirmasi, yang sengaja tidak dikerjakan.

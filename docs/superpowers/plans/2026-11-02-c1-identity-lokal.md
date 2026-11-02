# PRD-001 C1 Identity Lokal: Rencana Implementasi

> **Untuk agentic worker:** WAJIB memakai `superpowers:subagent-driven-development` (disarankan) atau `superpowers:executing-plans`. Langkah memakai kotak centang (`- [ ]`). Alur sprint dan pemilihan model subagent mengikuti `~/.claude/CLAUDE.md` (Sonnet implementer, review akhir Opus independen, maksimal 3 subagent bersamaan, saya yang commit).

**Tujuan:** mengganti stub autentikasi PRD-000 dengan identitas sungguhan: akun dan login lokal, JWT EdDSA + refresh berotasi, MFA TOTP, mailer minimal, Principal di context, modul `core` (`required`) beserta command `core:create-admin`, `core:rotate-keys`, `core:reset-mfa`.

**Arsitektur:** mesin (hash, token, sesi, revokasi, rate limit, MFA, middleware, store) di `internal/platform/identity` (+ `identity/store` untuk GORM); antarmuka yang dipakai modul di `sdk/identity` dan `sdk/mail` (dependency inversion D5/D9); modul `modules/core` hanya berisi API, use case admin, i18n, dan frontend; tabel identitas dimiliki platform (control DB). `Authorizer` diperluas ke keputusan bertipe agar 401, 403, dan `mfa_required` terbedakan tanpa mengubah modul.

**Tech Stack:** Go 1.27 stdlib (`crypto/ed25519`, `crypto/hmac`, `crypto/aes`, `net/smtp`, `crypto/tls`), `golang.org/x/crypto/argon2`, `golang.org/x/time/rate`, `golang.org/x/term`; goose (migrasi control dan tenant), GORM di `identity/store` saja; React/TS (`modules/core/web`, `packages/platform-web`).

**Spec:** `docs/prd/PRD-001-core.md` (FR-101, FR-102, FR-106, FR-107, FR-108, §3, §4, NFR-101 s.d. 106); `docs/superpowers/plans/2026-11-02-c-overview.md` (keputusan K-C1.1 sampai K-C1.9 yang **wajib disetujui pemilik produk sebelum Wave 0**); `docs/security/security-guidelines.md` §3.4 sampai §3.6, §3.10, §3.12; `docs/architecture/api-guidelines.md` §8, §12.

## Global Constraints

- Password: **argon2id**, parameter dari konfigurasi (default OWASP terkini), panjang minimum **12**, cek terhadap daftar sandi umum, tanpa aturan komposisi.
- Access token: JWT **EdDSA (Ed25519)**, umur **10 menit**, klaim `iss sub sid tid aud exp iat jti perm_rev amr scope`; daftar permission **tidak** masuk JWT. Refresh token: string acak opaque, hash disimpan, **14 hari sliding, absolut 90 hari**, rotasi tiap pakai, reuse mencabut seluruh *family*.
- Cookie refresh: `HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh`, ditambah pemeriksaan header `Origin`. Access token hanya di memori klien (tidak pernah `localStorage`/`sessionStorage`).
- Revokasi: middleware memeriksa `sid` terhadap daftar sesi tercabut **di memori**, bukan query per request; disebarkan lewat `StateNotifier`; instance baru memuat daftar awal saat startup.
- TOTP RFC 6238: 30 detik, 6 digit, anti-replay `last_used_step`; **10** recovery code sekali pakai (hash); wajib untuk role Administrator.
- Reset password: tautan sekali pakai **30 menit**, hash disimpan; reset mencabut semua sesi lain.
- Lockout: 5 gagal memicu jeda bertingkat, 20 gagal memicu `locked` (sampai di-unlock admin atau reset); pesan galat login **tidak** membedakan "email tidak ada" dan "password salah"; waktu respons seragam.
- Rahasia: `ERP_SECRET_KEY` 32 byte base64, **startup gagal** bila tidak valid (kecuali mode yang didefinisikan ADR-0013 untuk development); tanpa rahasia di URL atau log; `Cache-Control: no-store` pada respons auth.
- Endpoint: `POST /api/core/auth/{login,logout,refresh,password/forgot,password/reset,password/change}` (semua publik kecuali `change` dan `logout`; `refresh` publik berbasis cookie), `GET /.well-known/jwks.json`; 401 `unauthenticated`/`token_expired`, 403 `permission_denied`/`mfa_required`, 429 `rate_limited` + `Retry-After`.
- Arsitektur: modul tidak mengimpor platform (D9); GORM hanya di `identity/store` (setelah allowlist D3b diperbarui); tanpa `AutoMigrate`; tidak ada breaking change API; teks UI lewat i18n; `dangerouslySetInnerHTML` dilarang.
- TDD; satu commit per unit kerja; pesan konvensional; tanpa push atau PR.

## Review Focus

Lima kelas masukan/kegagalan yang tidak dicakup tes tiap task dan paling mungkin menggigit pengguna, dan tes yang memastikannya (ditambahkan ke task pemilik, dijalankan ulang di C1.15):

1. **Waktu respons login** untuk email tak ada vs sandi salah vs akun terkunci tidak boleh berbeda signifikan (tes statistik pada hash tiruan, bukan sekadar pesan seragam): C1.4 dan C1.7.
2. **Refresh paralel**: dua permintaan `refresh` serentak dengan token sama harus menghasilkan tepat satu rotasi sukses dan pencabutan family untuk yang kalah (bukan dua token valid, bukan deadlock): C1.6.
3. **JWT**: `alg: none`/HS256 dengan kunci publik sebagai rahasia, `kid` tak dikenal atau berisi path/SQL, `aud`/`iss` salah, `exp` lewat tepi leeway, token dari kunci `retired`: ditolak 401 tanpa membedakan sebab ke klien: C1.5.
4. **Revokasi lintas instance**: sesi tercabut harus ditolak instance lain segera setelah notifikasi (dan setelah restart instance lewat daftar awal); notifikasi hilang tidak boleh membuat sesi tercabut valid selamanya (batas atas umur access token): C1.6.
5. **Penyalahgunaan alur sekali pakai**: tautan reset dipakai dua kali serentak, kode TOTP/recovery code dipakai ulang serentak, user dengan banyak permintaan `forgot` (enumerasi akun lewat waktu/pesan, banjir email): C1.8 dan C1.9. Juga penguncian akun korban oleh penyerang (DoS lockout): mitigasi jeda bertingkat dan unlock via reset email; dicatat di ADR-0013.

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

- [ ] **Step 1: Test.** tes allowlist D3b yang sudah ada (`internal/archcheck/rules_test.go` dan `tests/architecture`) gagal sampai `rules.go`, `CLAUDE.md`, `AGENTS.md`, dan PRD-000 §4.2 memuat daftar identik; tambah kasus untuk `internal/platform/identity/store`.
- [ ] **Step 2: Jalankan** `go test ./internal/archcheck/... ./tests/architecture/...`, harapkan gagal.
- [ ] **Step 3: Tulis ADR-0013** (mekanisme token, rotasi kunci, revokasi, urutan middleware dan kontrak 401/403/`mfa_required`, rate limit, penyimpanan klien, ancaman DoS lockout) dan catat K-C1.1 sampai K-C1.9 lengkap dengan tanggal absolut dan pemilik produk sebagai penyetuju; perbarui allowlist di empat tempat sekaligus.
- [ ] **Step 4: Jalankan** tes yang sama, hijau; `task lint arch`.
- [ ] **Step 5: Commit** `docs(core): ADR-0013 diterima dan keputusan C1; allowlist D3b identity/store`.

### Task C1.1: Konfigurasi `ERP_SECRET_KEY` dan paket crypto

**Kelas:** Sonnet.

**Files:** Create `internal/platform/crypto/{seal.go,seal_test.go}`; Modify `internal/platform/config/{config.go,redact.go}`, `.env.example`, `docs/operations/deployment.md` bila perlu.

**Interfaces:** Produces `crypto.NewSealer(key []byte) (*Sealer, error)`; `(*Sealer).Seal(plaintext, aad []byte) ([]byte, error)`; `(*Sealer).Open(sealed, aad []byte) ([]byte, error)`; `config.Config.SecretKey []byte` (diurai dari `ERP_SECRET_KEY`, 32 byte base64; kesalahan format menggagalkan startup sesuai ADR-0013).

- [ ] **Step 1: Test** `TestSealOpenRoundTrip`, `TestOpenRejectsTamperedCiphertext`, `TestOpenRejectsWrongAAD`, `TestSealUsesFreshNonce` (dua Seal atas masukan sama berbeda), `TestConfigSecretKeyInvalidFailsStartup`, `TestRedactHidesSecretKey` (nilai tidak muncul di `inspect config`).
- [ ] **Step 2:** jalankan, harapkan gagal.
- [ ] **Step 3: Implementasi** AES-256-GCM, nonce acak dari `crypto/rand` diawali pada ciphertext, versi format satu byte di depan; parse dan validasi kunci di `config`; samarkan di `redact.go`.
- [ ] **Step 4:** tes hijau. **Step 5: Commit** `feat(platform): ERP_SECRET_KEY dan crypto.Sealer AES-256-GCM`.

### Task C1.2: Migrasi dan store identitas

**Kelas:** Sonnet. **Bergantung:** C1.0 (allowlist), C1.1.

**Files:** Create `migrations/control/0005_identity.sql`, `migrations/tenant/0003_roles_min.sql`, `internal/platform/identity/store/{models.go,store.go,store_test.go,store_integration_test.go}`; Modify `internal/app/assemble.go` (wiring `identitystore.New(database.ControlDB{DB: db})`).

**Interfaces:** Produces tabel control `users`, `tenant_memberships`, `user_preferences`, `signing_keys`, `auth_sessions`, `refresh_tokens`, `user_mfa_factors`, `user_recovery_codes`, `password_resets`, `login_attempts`; tabel tenant `roles`, `user_roles` (seed role sistem `administrator`); `store.Store` dengan metode per entitas (nama metode ditetapkan task ini dan didokumentasikan di `doc.go`, dipakai C1.4 sampai C1.9).

- [ ] **Step 1: Test integrasi** (`pgtest`, tag `integration`): migrasi naik dan turun bersih; `email` unik ternormalisasi (huruf kecil, trim); `refresh_tokens.parent_id` dan `family_id` konsisten; `used_at` tidak dapat di-set dua kali (kondisi `WHERE used_at IS NULL` mengembalikan 0 baris); semua query menyaring `tenant_id` (pola `desk_overrides`); tidak ada `AutoMigrate`.
- [ ] **Step 2:** jalankan (VPS lewat tunnel), harapkan gagal.
- [ ] **Step 3: Implementasi** migrasi dengan CHECK pada kolom enum dan indeks `(user_id)`, `(session_id)`, `(family_id)`, `(token_hash)`; rahasia (secret TOTP, kunci privat) disimpan sebagai `bytea` hasil `Sealer`; hash token sebagai `bytea` SHA-256. Struct `row` dengan `TableName()` mengikuti `desks/store`.
- [ ] **Step 4:** hijau; `task test:integration` untuk paket. **Step 5: Commit** `feat(identity): migrasi skema identitas dan store`.

### Task C1.3: `sdk/identity`, `sdk/mail`, dan helper uji

**Kelas:** Sonnet.

**Files:** Create `sdk/identity/{identity.go,identity_test.go}`, `sdk/identity/identitytest/identitytest.go`, `sdk/mail/{mail.go,mail_test.go}`.

**Interfaces:**
- `identity.Kind` (`KindHuman`, `KindService`); `identity.Principal{UserID string; Kind Kind; SessionID string; AuthMethod string; TokenScope []string; MFAVerified bool}`; `identity.WithPrincipal(ctx, Principal) context.Context`; `identity.FromContext(ctx) (Principal, bool)`.
- `identity.Service` (dipakai modul `core`): `Login(ctx, LoginInput) (LoginResult, error)`, `Refresh(ctx, RefreshInput) (LoginResult, error)`, `Logout(ctx, sessionID string) error`, `ForgotPassword(ctx, email string) error`, `ResetPassword(ctx, ResetInput) error`, `ChangePassword(ctx, ChangeInput) error`, serta metode MFA (`BeginMFAEnrollment`, `ConfirmMFA`, `VerifyMFA`, `RegenerateRecoveryCodes`); tipe input/hasil berada di paket yang sama (tanpa tipe platform). `identity.WithService/ServiceFrom(ctx)` mengikuti pola `authz.WithFieldMasker`.
- `mail.Message{To []string; Subject, Text, HTML string}`; `mail.Sender` (`Send(ctx, Message) error`); `mail.WithSender/From(ctx)`; `mail.Send(ctx, Message) error` memakai sender di context.

- [ ] **Step 1: Test** `TestFromContextEmpty`, `TestWithPrincipalRoundTrip`, `TestPrincipalIsValueCopy`, `TestMailSendWithoutSenderFailsClosed`; arsitektur: `sdk/identity` dan `sdk/mail` hanya mengimpor stdlib (D9).
- [ ] **Step 2:** gagal. **Step 3:** implementasi (tanpa dependensi baru; `identitytest.WithPrincipal(t, ctx, ...)` untuk uji modul). **Step 4:** hijau; `task arch`. **Step 5: Commit** `feat(sdk): identity.Principal, identity.Service, dan mail.Sender`.

### Task C1.4: Password dan kebijakan sandi

**Kelas:** Sonnet. **Bergantung:** C1.1, C1.3.

**Files:** Create `internal/platform/identity/password/{hash.go,policy.go,commonpasswords.txt,*_test.go}`; Modify `internal/platform/config/config.go` (parameter argon2 `ERP_ARGON2_*` dengan default OWASP).

**Interfaces:** `password.Hasher{Params}` dengan `Hash(plain string) (phc string, err error)`; `Verify(plain, phc string) (ok bool, needsRehash bool, err error)`; `password.Dummy() *Hasher` untuk hash tiruan pada email tak dikenal; `password.CheckPolicy(plain string) []Violation` (panjang minimal 12, tidak di daftar sandi umum, maksimum 1024 byte).

- [ ] **Step 1: Test** format PHC (`$argon2id$v=19$m=..,t=..,p=..$salt$hash`); `Verify` benar/salah dan konstan-waktu (`subtle.ConstantTimeCompare`); `needsRehash` saat parameter naik; kebijakan menolak `"password1234"` (daftar), `"short"`, 1025 byte; **Review Focus 1:** `TestVerifyUnknownUserTimingMatchesKnownUser` (rasio median waktu ≤ 1,2 pada 200 iterasi dengan parameter uji kecil).
- [ ] **Step 2:** gagal. **Step 3:** implementasi dengan `golang.org/x/crypto/argon2`; daftar sandi umum di-embed (`//go:embed`) sebagai himpunan dalam memori; sandi tidak pernah masuk log atau pesan galat. **Step 4:** hijau; `govulncheck` untuk dependensi. **Step 5: Commit** `feat(identity): argon2id, kebijakan sandi, dan hash tiruan`.

### Task C1.5: Kunci penandatangan, JWT EdDSA, JWKS

**Kelas:** Sonnet. **Bergantung:** C1.1, C1.2.

**Files:** Create `internal/platform/identity/token/{keys.go,jwt.go,jwks.go,*_test.go}`.

**Interfaces:** `token.Keyring` (`Active() Key`, `ByKID(kid string) (Key, bool)`, `Rotate(ctx) (Key, error)`, status `active/retiring/retired`); `token.Issuer.Issue(Claims) (string, error)`; `token.Verifier.Verify(raw string) (Claims, error)` (hanya `alg=EdDSA`, `kid` wajib dan dicari di keyring, validasi `iss/aud/exp/iat` dengan leeway 30 detik); `token.JWKSHandler(Keyring) http.Handler` (`Cache-Control: public, max-age=300`, hanya kunci `active` dan `retiring`).

- [ ] **Step 1: Test** vektor Ed25519 RFC 8032; Review Focus 3: `TestVerifyRejectsNoneAlg`, `TestVerifyRejectsHS256WithPublicKeyAsSecret`, `TestVerifyRejectsUnknownKID`, `TestVerifyRejectsKIDWithPathOrSQL`, `TestVerifyRejectsWrongIssuerAudience`, `TestVerifyExpiryEdgeWithLeeway`, `TestVerifyRejectsRetiredKey`; `TestRotateKeepsOldTokensValidWhileRetiring` (rotasi tanpa downtime); `TestPrivateKeyStoredSealed` (kolom privat tidak berisi bytes kunci mentah); JWKS tidak membocorkan kunci privat dan tidak memuat `retired`.
- [ ] **Step 2:** gagal. **Step 3:** implementasi JWT kompak (header `{"alg":"EdDSA","kid":...}`), parser ketat (tiga segmen base64url tanpa padding, batas ukuran 4 KB, tolak header ekstra yang tidak dikenal), kunci privat disegel `Sealer`, cache keyring di memori dengan muat ulang saat `Rotate`. **Step 4:** hijau. **Step 5: Commit** `feat(identity): kunci Ed25519, JWT EdDSA, dan JWKS`.

### Task C1.6: Sesi, refresh berotasi, dan revokasi

**Kelas:** Sonnet. **Bergantung:** C1.2, C1.5.

**Files:** Create `internal/platform/identity/session/{service.go,revocation.go,*_test.go,*_integration_test.go}`; Modify `internal/app/assemble.go` (muat daftar awal dan langganan `StateNotifier`).

**Interfaces:** `session.Service.Start(ctx, StartInput) (Session, RefreshToken, error)`; `Rotate(ctx, rawRefresh string, origin string) (Session, RefreshToken, error)` (deteksi reuse mencabut family dan sesi, mencatat event keamanan); `Revoke(ctx, sessionID, reason string) error`; `RevokeAllForUser(ctx, userID string) error`; `session.RevocationList` (`IsRevoked(sid string) bool`, `Add(sid string, until time.Time)`, `Load(ctx)`), dengan penyebaran lewat `StateNotifier` kanal `identity.session_revoked`.

- [ ] **Step 1: Test integrasi** `TestRotateIssuesNewTokenAndMarksParentUsed`; `TestReuseOfUsedTokenRevokesFamilyAndSession`; Review Focus 2: `TestConcurrentRefreshSameTokenExactlyOneSucceeds` (20 goroutine, tepat satu sukses, family tercabut, tanpa deadlock); `TestAbsoluteLifetime90DaysEnforced`; `TestSlidingExpiry14Days`; Review Focus 4: `TestRevocationVisibleToOtherInstanceViaNotifier`, `TestRevocationListLoadedOnStartup`, `TestRevocationEntriesExpireAfterAccessTokenTTL`; `TestRefreshRejectsMissingOrCrossOriginOrigin`; `TestDisabledUserSessionsRevokedImmediately`.
- [ ] **Step 2:** gagal. **Step 3:** implementasi rotasi dalam satu transaksi dengan `UPDATE ... SET used_at=now() WHERE id=$1 AND used_at IS NULL RETURNING` (pemenang tunggal), token acak 32 byte `crypto/rand` base64url dengan hash SHA-256 disimpan; daftar tercabut `sync.RWMutex` + map berumur. **Step 4:** hijau (VPS). **Step 5: Commit** `feat(identity): sesi, refresh berotasi, dan revokasi lintas instance`.

### Task C1.7: Rate limit, lockout, dan 429

**Kelas:** Sonnet. **Bergantung:** C1.2, C1.4.

**Files:** Create `internal/platform/identity/guard/{limiter.go,lockout.go,*_test.go}`; Modify `sdk/httpx` (tambah `TooManyRequests(w, r, retryAfter)` dengan header `Retry-After` dan kode `rate_limited`), katalog i18n platform bila kunci galat belum ada.

**Interfaces:** `guard.IPLimiter.Allow(ip string) (ok bool, retryAfter time.Duration)` (`x/time/rate`, per IP); `guard.Lockout.RegisterFailure(ctx, userID, ip string) (State, error)`; `guard.Lockout.RegisterSuccess(ctx, userID string) error`; `State{Locked bool; RetryAfter time.Duration}` (5 gagal: jeda bertingkat; 20: `locked`).

- [ ] **Step 1: Test** jeda bertingkat tepat pada gagal ke-5 dan seterusnya; `locked` pada gagal ke-20 dan tidak terbuka sendiri; sukses mereset hitungan; penghitung disimpan di DB (`login_attempts`) agar berlaku lintas instance; IP limiter menjawab 429 + `Retry-After` bilangan bulat; Review Focus 1: respons terkunci tidak lebih cepat dari respons sandi salah (waktu diseragamkan); **DoS lockout:** penyerang tak dapat mengunci akun korban lebih cepat dari jeda bertingkat tanpa mengganggu login sah dari IP lain (uji: IP A gagal 25 kali, user dari IP B tetap bisa mencoba setelah jeda); unlock lewat reset sandi.
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau. **Step 5: Commit** `feat(identity): rate limit per IP, lockout bertingkat, dan 429 Retry-After`.

### Task C1.8: Mailer dan reset password

**Kelas:** Sonnet. **Bergantung:** C1.2, C1.3, C1.4, C1.7.

**Files:** Create `internal/platform/mail/{smtp.go,log.go,*_test.go}`, `internal/platform/identity/reset/{service.go,templates/{id,en}/*.tmpl,*_test.go}`; Modify `internal/platform/config/config.go` (`ERP_MAIL_DRIVER`, `ERP_SMTP_*`, `ERP_SMTP_PASSWORD` tersamarkan), `internal/app/assemble.go`.

**Interfaces:** `mail/smtp.New(cfg) mail.Sender` (STARTTLS atau TLS, autentikasi, timeout, retry singkat); `mail/log.New(log) mail.Sender` (default di development, tanpa koneksi jaringan, isi tautan tidak ditulis ke log produksi); `reset.Service.Request(ctx, email, locale string) error` (selalu sukses secara eksternal, tanpa membedakan akun ada/tidak); `reset.Service.Complete(ctx, rawToken, newPassword string) error` (sekali pakai, 30 menit, mencabut semua sesi, membuka `locked`).

- [ ] **Step 1: Test** driver `log` tidak membuka soket; SMTP diuji terhadap server uji lokal (container Mailpit di VPS atau fake `net.Listener`) termasuk STARTTLS wajib bila dikonfigurasi; sandi SMTP tidak muncul di log atau `inspect config`; `Request` untuk email tak ada dan ada: respons dan waktu setara (Review Focus 5); token disimpan sebagai hash; `Complete` dua kali serentak: tepat satu sukses; token kedaluwarsa ditolak; sandi lama tidak berlaku dan sesi lain dicabut; batas permintaan `forgot` per email dan per IP (banjir email dicegah, 429); templat `id` dan `en` sesuai preferensi (`user_preferences.locale`).
- [ ] **Step 2:** gagal. **Step 3:** implementasi (`net/smtp` + `crypto/tls`, `html/template` untuk HTML). **Step 4:** hijau. **Step 5: Commit** `feat(identity): mailer smtp/log dan reset password`.

### Task C1.9: MFA TOTP dan recovery code

**Kelas:** Sonnet. **Bergantung:** C1.2, C1.4, C1.7.

**Files:** Create `internal/platform/identity/mfa/{totp.go,recovery.go,service.go,*_test.go}`.

**Interfaces:** `mfa.TOTP.Code(secret []byte, t time.Time) string`, `Verify(secret []byte, code string, t time.Time, lastStep int64) (step int64, ok bool)` (jendela ±1 langkah); `mfa.Service.BeginEnrollment(ctx, userID) (EnrollmentInfo{OTPAuthURI string, Secret string}, error)`; `ConfirmEnrollment(ctx, userID, code string) (recoveryCodes []string, error)`; `Verify(ctx, userID, code string) error`; `UseRecoveryCode(ctx, userID, code string) error`; `Reset(ctx, actor, userID)` (tercatat); `Required(ctx, userID) (bool, error)` (true bila role `administrator`).

- [ ] **Step 1: Test** vektor RFC 6238 (SHA-1, waktu 59, 1111111109, 1111111111, 1234567890, 2000000000, 20000000000); kode yang sama pada langkah yang sama ditolak (anti-replay, `last_used_step`); Review Focus 5: dua verifikasi serentak dengan kode sama, tepat satu sukses (pembaruan `last_used_step` bersyarat); 10 recovery code, hash disimpan, sekali pakai, dua pemakaian serentak tepat satu sukses; verifikasi memakai lockout FR-101; secret TOTP tersimpan tersegel; pendaftaran tidak aktif sebelum `ConfirmEnrollment`.
- [ ] **Step 2:** gagal. **Step 3:** implementasi RFC 6238 sendiri (`crypto/hmac`, `crypto/sha1` hanya untuk HOTP per RFC, dikecualikan dari larangan SHA-1 dengan komentar rujukan dan catatan di ADR-0013), pembanding konstan-waktu. **Step 4:** hijau. **Step 5: Commit** `feat(identity): MFA TOTP, recovery code, dan anti-replay`.

### Task C1.10: Middleware `Authenticate`, Principal, dan `Authorizer` bertipe

**Kelas:** Sonnet. **Bergantung:** C1.5, C1.6, C1.9.

**Files:** Create `internal/platform/identity/middleware.go`, `*_test.go`; Modify `internal/platform/httpserver/server.go` (urutan: pencocokan `tid` setelah `tenantContext`), `internal/platform/modules/runtime/mount.go` (`Authorizer` bertipe, `permissionMW` memetakan 401/403/`mfa_required`), `sdk/module` (opsi rute `RequireMFA`), `internal/platform/modules/registry/{recorder.go,types}` (`RouteInfo.RequireMFA`), `internal/app/assemble.go` (mengganti stub authN), `tests/security/public_routes.txt`, `contracts/openapi/platform.yaml` (`/.well-known/jwks.json`).

**Interfaces:** `runtime.Authorizer.Decide(ctx, permission string, opts RouteOptions) Decision` (`Allow`, `Unauthenticated`, `TokenExpired`, `Forbidden`, `MFARequired`); `Allow(ctx, permission) bool` lama dipertahankan sebagai adaptor agar stub dan modul tidak berubah; `identity.Authenticate(deps) func(http.Handler) http.Handler` (membaca `Authorization: Bearer`, memverifikasi token, memeriksa `sid` pada daftar tercabut, membangun `Principal`, mencocokkan `tid` dengan tenant context, menolak token `scope`-terbatas di endpoint refresh).

- [ ] **Step 1: Test** token valid menghasilkan Principal di context; tanpa header 401 `unauthenticated`; `exp` lewat 401 `token_expired`; `sid` tercabut 401; `tid` tidak cocok 401; administrator tanpa MFA terverifikasi mendapat 403 `mfa_required` di semua route kecuali pendaftaran MFA dan logout; `RequireMFA` pada route menuntut `amr` berisi `otp`; stub lama masih lolos tes modul yang ada; rute `Public()` tak diautentikasi; semua kegagalan token tidak membedakan sebab ke klien selain `token_expired`; `GET /.well-known/jwks.json` publik dan terdaftar di allowlist keamanan.
- [ ] **Step 2:** gagal. **Step 3:** implementasi; autentikasi nyata menggantikan stub authN sepenuhnya; `ERP_AUTHZ_STUB` hanya tetap menentukan keputusan permission sampai C2 (K-C1.9), tanpa flag baru untuk menonaktifkan autentikasi (ADR-0013 menetapkan perilaku development, mis. admin dibuat lewat `core:create-admin`). **Step 4:** hijau; `task test:integration` dan `tests/security`. **Step 5: Commit** `feat(identity): Authenticate, Principal, dan keputusan otorisasi bertipe`.

### Task C1.11: Modul `core` dan kontrak API

**Kelas:** Sonnet. **Bergantung:** C1.10, C1.8, C1.9.

**Files:** Create `modules/core/**` (dibangkitkan `task erpora -- make:module core` lalu disesuaikan; `module.json` `required: true`, `capabilities` berawalan `core.`, `desk`), `modules/core/openapi.yaml`, `modules/core/i18n/{id,en}.json`; Modify `internal/app/modules.go`, `go.work`, `contracts/spectral-functions/*` dan `docs/security/security-guidelines.md` §2:48 (pengecualian terbatas K-C1.4), `internal/app/assemble.go` (menyuntik `identity.Service` dan `mail.Sender` ke `runtime.Services`).

**Interfaces:** handler `POST /api/core/auth/{login,logout,refresh}`, `POST /api/core/auth/password/{forgot,reset,change}`, `POST /api/core/auth/mfa/{enroll,confirm,verify,recovery}`; respons login `{data: {accessToken, expiresIn, mfaRequired?}}` dengan penanda `x-erp-credential-response: true`; refresh token **hanya** lewat cookie; kode galat: `unauthenticated`, `token_expired`, `invalid_credentials` (seragam), `account_locked`, `mfa_required`, `rate_limited`, `password_policy_violation` (+ details tanpa membocorkan daftar).

- [ ] **Step 1: Test** `task contracts:check` hijau; kesesuaian route, permission, dan klasifikasi; handler memakai `httpx.DecodeJSON`; `Cache-Control: no-store` pada semua respons auth; `Set-Cookie` refresh dengan atribut `HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh`; login benar mengembalikan `accessToken` dan tidak mengembalikan refresh token di body; **IDOR:** `password/change`, `logout`, dan MFA hanya untuk Principal sendiri (tes pengguna A vs B: A tak dapat mencabut sesi B atau mengubah MFA B); `module:check` lolos; modul `core` tidak dapat di-`disable` atau `uninstall` (`module_required`).
- [ ] **Step 2:** gagal. **Step 3:** implementasi (handler tipis memanggil `identity.Service` dari context, tidak mengimpor platform). **Step 4:** hijau; `task contracts:check`; `task erpora -- module:check`. **Step 5: Commit** `feat(core): modul core required dengan API autentikasi`.

### Task C1.12: Command CLI `core:create-admin`, `core:rotate-keys`, `core:reset-mfa`

**Kelas:** Sonnet. **Bergantung:** C1.9, C1.11.

**Files:** Modify `sdk/module/module.go` (`Invocation.In io.Reader`, aditif), `internal/app/command.go` (mengisi `In`, memastikan `AsSystem("cli:<name>")` tercatat), `cmd/erpora/module_commands.go`; Create command di `modules/core/module.go` dan `modules/core/internal/application`.

**Interfaces:** `core:create-admin --email <e> [--name <n>]` membaca sandi dari prompt tersembunyi (TTY, `x/term`) atau stdin, tidak ada flag sandi; menerapkan kebijakan sandi; memberi role `administrator`; `core:rotate-keys`; `core:reset-mfa --email <e>`.

- [ ] **Step 1: Test** admin dibuat dan dapat login; sandi tidak muncul di argumen proses, log, atau keluaran; sandi lemah ditolak; `create-admin` dua kali untuk email sama menolak; `rotate-keys` menghasilkan kunci `active` baru dan lama `retiring`; `reset-mfa` menghapus faktor dan tercatat; semua command hanya berjalan bila modul `core` enabled (perilaku yang ada) dan memakai hak sistem teraudit.
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau; `tests/integration`. **Step 5: Commit** `feat(core): command create-admin, rotate-keys, reset-mfa`.

### Task C1.13: Pensiun aktor `api:dev`, bootstrap berautentikasi

**Kelas:** Sonnet. **Bergantung:** C1.10.

**Files:** Modify `internal/platform/modules/admin/api.go:35`, `internal/platform/i18n/admin/overrides.go:34` (aktor dari `identity.FromContext`; CLI tetap `cli:<name>`), `internal/platform/bootstrap/handler.go` dan `contracts/openapi/platform.yaml` (bootstrap tidak lagi `Public()`, menambah `user` dan `permRev` secara aditif; `permissions` tetap sampai C2), `internal/platform/httpserver/routes.go:205`.

- [ ] **Step 1: Test** aksi admin modul/i18n mencatat `actor` = `user:<id>` dari Principal (bukan `api:dev`); `/api/bootstrap` tanpa token 401; dengan token mengembalikan `user` dan `permRev`; perubahan kontrak hanya aditif (`contracts:breaking` hijau); ETag bootstrap bervariasi per pengguna (tidak ada cache lintas pengguna: `Vary: Authorization`, `Cache-Control: private`).
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau. **Step 5: Commit** `feat(platform): aktor dari Principal dan bootstrap berautentikasi`.

### Task C1.14: Frontend login, MFA, reset, dan klien API berautentikasi

**Kelas:** Sonnet. **Bergantung:** C1.11, C1.13.

**Files:** Create `modules/core/web/**` (halaman `login`, `mfa`, `forgot`, `reset`, `change-password`; i18n; `defineModule` dengan `desk`), `packages/platform-web/src/auth/**` (penyimpan access token di memori, refresh single-flight pada 401, deteksi `mfa_required`); Modify `packages/platform-web/src/api/client.ts` (hook `headers`, `credentials: "include"` hanya untuk endpoint refresh), `src/bootstrap/{fetcher.ts,provider.tsx}`, `src/sources/*` (CurrentUserSource sungguhan menggantikan `anonymousUserSource`), `web/src/app/{providers,router,shell/user-menu.tsx}` (guard sesi, tombol keluar), `packages/platform-web/src/permissions/index.ts` (tetap optimistis sampai C2; tidak diubah di C1).

- [ ] **Step 1: Test** `bun test`: token tidak pernah ditulis ke `localStorage`/`sessionStorage`/`document.cookie` (uji dengan spy storage); refresh single-flight (tiga 401 serentak memicu satu refresh); 401 setelah refresh gagal mengarahkan ke login dan membersihkan cache query (`scopedKey`); `returnTo` hanya path relatif (tolak `//evil`, `https://...`, `javascript:`); layar MFA muncul saat `mfa_required`; semua teks lewat i18n; tidak ada `dangerouslySetInnerHTML`; `task web:check` dalam anggaran bundle 200 KB gzip (halaman auth dimuat lazy).
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau; `task web:check`. **Step 5: Commit** `feat(web): login, MFA, reset sandi, dan klien API berautentikasi`.

### Task C1.15: Keamanan, gerbang, review akhir, verifikasi browser (checkpoint C1)

**Kelas:** Sonnet (gerbang), review akhir Opus independen.

- [ ] **Step 1: Gerbang** `task lint arch check:gowork-off test`; `task test:integration` (VPS); `task contracts:check`; `task web:check`; `task test:generator`; `govulncheck ./...`; jalankan ulang semua tes Review Focus 1 sampai 5.
- [ ] **Step 2: Checklist ASVS sesi** (V2, V3, V6, V7 yang relevan) di `docs/security/security-guidelines.md` §6 dan §7 diperbarui; `docs/module-sdk/security-primitives.md` memuat `sdk/identity` dan `sdk/mail`; ADR-0013 Accepted; PRD-001 tabel keputusan diperbarui; milestone C1 ditandai selesai.
- [ ] **Step 3: Review akhir independen** (Opus, tanpa konteks implementer): kesesuaian dengan PRD-001 dan ADR-0013 serta `security-audit` terfokus pada permukaan C1; temuan yang sesuai spek diperbaiki implementer yang sama, yang di luar spek dikonfirmasi dulu; satu review atas diff perbaikan.
- [ ] **Step 4: Verifikasi browser** (aplikasi penuh dengan DB sementara di VPS): `core:create-admin`, login, wajib MFA Administrator, logout, refresh setelah 10 menit (token kedaluwarsa), reset sandi lewat driver `log`, lockout; inspeksi storage browser kosong dari token; DB sementara dihapus.
- [ ] **Step 5: Commit** dan laporkan: yang dikerjakan, yang menunggu konfirmasi, yang sengaja tidak dikerjakan.

# ADR-0013: Autentikasi, otorisasi, dan audit

- **Status:** Accepted
- **Tanggal:** 2 November 2026
- **Penyetuju:** pemilik produk, untuk keputusan K-C1.1 sampai K-C1.9 (sesuai rekomendasi, 2 November 2026) dan keputusan 3 November 2026 (PRD-001 Q16, bagian 7a). Butir §8 adalah temuan review independen yang diterapkan sesuai spek, bukan keputusan pemilik produk.
- **Rujukan:** PRD-001 (Q10, Q14, Q15, Q16), PRD-000 (Q42), `docs/security/security-guidelines.md` §2, §3.4 sampai §3.6, §3.10, §3.12, `docs/architecture/api-guidelines.md` §8, §12, rencana `docs/superpowers/plans/2026-11-02-c1-identity-lokal.md`.

> Catatan penomoran: berkas ini dahulu dirujuk sebagai "ADR-013"; rujukan lama sama dengan ADR-0013.

## Konteks

Platform membutuhkan autentikasi pengguna (login lokal, kelak SSO OIDC), token API, otorisasi berbasis permission/scope/field, dan audit. PRD-001 Q10 memutuskan JWT access + refresh berotasi untuk semua klien termasuk web. Stub PRD-000 (authN pass-through, `authorization.NewStub`, aktor `api:dev`) harus diganti, dan `Authorizer.Allow(ctx, permission) bool` terlalu sempit karena tidak membedakan 401, 403, dan `mfa_required`. ADR ini merinci mekanisme sebelum kode milestone C1 (Identity lokal).

## Keputusan

### 1. Token

- **Access token:** JWT **EdDSA (Ed25519)**, umur **10 menit**, klaim `iss sub sid tid aud exp iat jti perm_rev amr scope`. Daftar permission **tidak** masuk JWT (di-resolve server, cache per `perm_rev`). Verifier hanya menerima `alg=EdDSA`, `kid` wajib dan dicari di keyring, ukuran maksimum 4 KB, leeway 30 detik; `alg: none`, HS256, `kid` tak dikenal, `iss`/`aud` salah, dan kunci `retired` ditolak 401 tanpa membedakan sebab ke klien (selain `token_expired`).
- **Refresh token:** string acak opaque 32 byte (`crypto/rand`, base64url); hanya hash SHA-256 yang disimpan. **14 hari sliding, 90 hari absolut**, **rotasi** tiap pakai. Pemenang rotasi ditentukan atomik (`UPDATE ... SET used_at=now() WHERE id=$1 AND used_at IS NULL RETURNING`). **Deteksi reuse:** pemakaian token yang sudah terpakai mencabut seluruh *family* beserta sesinya dan mencatat event keamanan; refresh paralel dengan token sama menghasilkan tepat satu sukses.
- **Penyimpanan klien:** refresh token hanya di cookie `HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh`, ditambah pemeriksaan header `Origin` pada endpoint refresh. Access token hanya di **memori** klien (tidak pernah `localStorage`/`sessionStorage`/`document.cookie`); refresh single-flight di klien. Refresh token tidak pernah ada di body respons. Semua respons auth `Cache-Control: no-store`.

### 2. Revokasi

Middleware memeriksa `sid` terhadap **daftar sesi tercabut di memori** (peta berumur di belakang `sync.RWMutex`), bukan query per request. Pencabutan disebarkan antar instance lewat `StateNotifier` (kanal `identity.session_revoked`); instance baru memuat daftar awal dari DB saat startup. Entri disimpan selama **umur access token + leeway verifier (10 menit + 30 detik)** sejak dicabut, yaitu selama token terakhir yang mungkin beredar masih diterima verifier; entri tidak boleh dibuang lebih awal. Notifikasi yang hilang tidak membuat sesi tercabut valid selamanya: batas atas keberlakuannya adalah `exp` token ditambah leeway (**10 menit 30 detik**), dan instance yang dimulai ulang memuat daftar awal dari DB. Menonaktifkan user, reset sandi, dan ganti sandi mencabut sesi terkait segera.

### 3. Kunci penandatangan, JWKS, dan `ERP_SECRET_KEY`

- Kunci Ed25519 berstatus `pending`, `active`, `retiring`, `retired` (status `pending` baru; PRD-001 §4 `signing_keys.status` diperluas aditif). **Rotasi multi-instance tanpa downtime** (`core:rotate-keys`):
  1. `core:rotate-keys` **hanya membuat** kunci baru berstatus `pending` yang **dipublikasikan di JWKS lebih dulu** dan melaporkan waktu aktivasinya (`identity.KeyInfo.Status` dan `ActivatesAt`); kunci `pending` tidak dipakai menandatangani. Command tidak pernah mengaktifkan kunci sendiri.
  2. Setelah **minimal `max-age` JWKS (300 detik)** sejak dipublikasikan (agar verifier dan cache pihak ketiga sudah memilikinya), kunci `pending` **dipromosikan** menjadi `active` oleh **pekerja latar di proses `serve`**, yang memeriksa secara berkala dan memicu notifikasi `identity.keys_changed` agar semua instance memuat ulang keyring; kunci lama menjadi `retiring` (token lama tetap valid sampai kedaluwarsa). Bila `serve` tidak berjalan saat waktu aktivasi lewat, promosi terjadi saat `serve` start. Ini penjabaran AC FR-102 "rotasi tanpa downtime" (sesuai spek).
  3. Kunci `retiring` menjadi `retired` (ditolak, dikeluarkan dari JWKS) **hanya setelah** lewat umur access token + leeway (10 menit 30 detik) sejak berhenti menandatangani.
  4. Setiap instance `serve` memuat ulang keyring lewat `StateNotifier` (kanal `identity.keys_changed`) dan, sebagai pengaman, saat bertemu `kid` tak dikenal (muat ulang **sekali**, dibatasi laju agar `kid` sembarang dari penyerang tidak menjadi beban query DB); `kid` yang tetap tak dikenal ditolak 401.
- **JWKS:** `GET /.well-known/jwks.json` publik, `Cache-Control: public, max-age=300`, hanya kunci `pending`, `active`, dan `retiring`, tidak pernah kunci privat atau kunci `retired`. Rute ini didaftarkan **platform** langsung di router (di luar `/api/{slug}`, K-C1.8), masuk `tests/security/public_routes.txt` dan `contracts/openapi/platform.yaml`.
- Kunci privat disimpan **tersegel AES-256-GCM** (`crypto.Sealer`, nonce acak, versi format satu byte, AAD terikat konteks) dengan **`ERP_SECRET_KEY`**: 32 byte, base64. Format segel membawa **id kunci** (sidik jari kunci) dan seluruh header, termasuk id kunci, ikut AAD. **Startup `serve` gagal** bila kunci tidak ada atau tidak valid; hal yang sama berlaku untuk command yang menyegel atau membuka data (`core:*`). `migrate`, `inspect`, `doctor`, `modules`, dan `i18n` **tidak** membutuhkan kunci, tetapi format tetap divalidasi bila `ERP_SECRET_KEY` terisi (keputusan 3 November 2026, Q16). Tidak ada mode tanpa autentikasi untuk development: pengembang membuat admin lewat `core:create-admin`. Nilai kunci tidak pernah muncul di log, `inspect config`, atau URL.
- **Rincian format segel (C1.1, `internal/platform/crypto`):** `[versi 1 byte][id kunci 1 byte][nonce 12 byte][ciphertext + tag GCM]`; AAD efektif = header (versi + id kunci) || AAD pemanggil, sehingga header tidak dapat diubah tanpa terdeteksi. AAD pemanggil wajib tidak kosong (`ErrEmptyAAD`) dan mengikat blob ke konteksnya (tabel, kolom, id baris). Galat dipisah: `ErrUnknownVersion` (versi tak dikenal), `ErrUnknownKeyID` (id kunci tak dimiliki), `ErrOpen` (kegagalan lain, termasuk AAD/ciphertext rusak). `NewSealer(active Key, previous ...Key)` dan `NewSealerFromMaterial(active []byte, previous ...[]byte)`.
- **Perilaku `config.Load`:** format `ERP_SECRET_KEY` dan `ERP_SECRET_KEY_PREVIOUS` divalidasi bila terisi, sedangkan kewajiban kehadiran hanya ditegakkan `serve` dan jalur penyegel (`core:*`). Bila kunci tidak valid, `config.Load` mengembalikan **Config parsial bersama galat**; hanya `erpora doctor` yang boleh memakai Config parsial itu (agar dapat tetap memeriksa PostgreSQL dan melaporkan kunci sebagai temuan tersendiri), pemanggil lain **wajib berhenti**. Command `secret:generate` membuat kunci baru (nilai tidak pernah ditulis ke log).
- **Rotasi `ERP_SECRET_KEY`** (keputusan 3 November 2026, Q16): kunci lama dipasang di `ERP_SECRET_KEY_PREVIOUS` dan **hanya dipakai untuk membuka** (segel baru selalu memakai `ERP_SECRET_KEY`); data dipilih kunci pembukanya lewat id kunci pada header segel. Command `core:reseal` (disediakan kelak, bukan bagian C1) menyegel ulang semua data tersegel dengan kunci baru; setelah itu `ERP_SECRET_KEY_PREVIOUS` dapat dihapus. Data tersegel saat ini: kunci privat JWT dan secret TOTP.
- **Petunjuk pembuatan kunci pada pesan galat** (keputusan pemilik produk 3 November 2026, Q16 butir 8): command `secret:generate` hanya ada di build pengembang (tag `devtools`) dan **tidak dipindahkan** ke binary release. Pesan galat `config` (`RequireSecretKey` dan format kunci) karena itu menyebut tiga cara: `erpora secret:generate` (hanya build pengembang), `openssl rand -base64 32`, dan PowerShell 7+ `[Convert]::ToBase64String([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32))`, ditambah rujukan ke `docs/operations/deployment.md`. `Get-Random` PowerShell **tidak boleh** disarankan karena bukan CSPRNG. Alasan: operator production memakai binary release dan tidak boleh diarahkan ke command yang tidak ada di sana, sementara menambah command ke binary release memperluas permukaan binary itu tanpa kebutuhan lain.

### 4. Urutan middleware dan keputusan otorisasi bertipe

Urutan: request ID dan header keamanan, `tenantContext`, **batas laju per IP**, **`Authenticate`**, **batas laju per principal**, route permission. Batas laju berlaku untuk **semua endpoint auth/token** (login, forgot, reset, refresh, change, `mfa/*`): per IP selalu; per principal (`sub`) untuk endpoint terautentikasi (`change`, `mfa/*`, `logout`); dan per identitas sasaran untuk endpoint publik yang belum punya principal (email ternormalisasi pada `login`/`forgot`, *family* pada `refresh`). Melebihi batas: 429 `rate_limited` + `Retry-After`. Klaim **`tid` dicocokkan setelah `tenantContext`** (tidak cocok: 401). `Authenticate` membaca `Authorization: Bearer`, memverifikasi token, memeriksa `sid` pada daftar tercabut, membangun `Principal`, dan menolak token ber-`scope` terbatas di luar endpoint yang dimaksud. Rute `Public()` tidak diautentikasi.

`Authorizer` diperluas ke keputusan bertipe `Decide(ctx, permission, opts) Decision` (`Allow`, `Unauthenticated`, `TokenExpired`, `Forbidden`, `MFARequired`); `Allow(ctx, permission) bool` lama dipertahankan sebagai adaptor sehingga modul dan stub tidak berubah. Pemetaan: **401** `unauthenticated` / `token_expired`; **403** `permission_denied` / `mfa_required`; **429** `rate_limited` + `Retry-After`. Aturan MFA pada `Authenticate`/`Decide` (berlaku untuk **semua** pengguna, bukan hanya administrator):
- Sesi dengan **faktor MFA aktif tetapi belum terverifikasi** (`amr` tidak memuat `otp`) hanya boleh memanggil `mfa/verify`, `mfa/recovery`, dan `logout`; semua route lain **403 `mfa_required`**. Pengguna non-administrator dengan TOTP aktif tidak boleh lolos hanya dengan sandi.
- Pemilik role `administrator` **tanpa faktor** terdaftar hanya boleh memanggil `mfa/enroll`, `mfa/confirm`, dan `logout`; semua route lain 403 `mfa_required`.
- Untuk route non-MFA, bila pengguna punya faktor aktif, `amr` access token **harus memuat `otp`**; token hasil login sandi saja berisi `amr=["pwd"]` dan token hasil verifikasi MFA `["pwd","otp"]`.
- Opsi rute `RequireMFA` menuntut `amr` memuat `otp` apa pun status faktor pengguna.

### 5. Password, rate limit, dan lockout

- Penyimpanan sandi **argon2id** (format PHC; parameter dari konfigurasi dengan default OWASP; `needsRehash` saat parameter naik), panjang minimum 12, maksimum 1024 byte, cek daftar sandi umum, tanpa aturan komposisi. Email tak dikenal diverifikasi terhadap hash tiruan agar waktu respons seragam; pesan galat tidak membedakan "email tidak ada" dan "sandi salah".
- **Rate limit** per IP dan per principal/identitas sasaran (`x/time/rate`, urutan di §4) menjawab 429 + `Retry-After`.
- **Lockout** (keputusan pemilik produk 3 November 2026, Q16; penghitung di DB `login_attempts`, berlaku lintas instance): **jeda bertingkat per pasangan (akun, IP)**, dan status `locked` pada akun **hanya** bila kegagalan terkumpul lintas banyak IP melampaui ambang lebih tinggi. Nilai (dapat dikonfigurasi, default berikut):
  - **5 gagal** berturut-turut pada satu pasangan (akun, IP) memicu jeda bertingkat: 30 detik, berlipat dua tiap kegagalan berikutnya, maksimum 15 menit; respons 429 `rate_limited` + `Retry-After`.
  - **20 gagal** pada satu pasangan memicu **pemblokiran pasangan itu** selama 1 jam; akun tetap dapat dipakai dari IP lain.
  - Akun menjadi **`locked`** bila terkumpul **>= 50 gagal lintas >= 5 IP berbeda dalam 1 jam**. Akun `locked` ditolak dari semua IP sampai di-unlock.
  - Sukses login mereset hitungan pasangan itu; hitungan jendela 1 jam untuk ambang lintas IP tidak direset oleh sukses dari IP lain.
  - **Unlock:** lewat reset sandi (tautan email), command CLI baru **`core:unlock --email <e>`** (hak sistem teraudit; di luar teks asli FR-101, disetujui pemilik produk 3 November 2026 dan dicatat di FR-101), atau unlock admin.
  - Pesan galat dan waktu respons tidak membedakan akun ada/tidak/terkunci. Permintaan `forgot` dibatasi per email dan per IP.
- **Ancaman DoS lockout (risiko residual diterima pemilik produk, 3 November 2026, Q16):** penyerang dapat sengaja salah login untuk mengunci akun korban. Mitigasi yang sudah diputuskan (§5, Q16): jeda dan pemblokiran dihitung **per pasangan (akun, IP)** sehingga penyerang dari satu IP tidak menghalangi login sah dari IP lain, `locked` pada akun baru terjadi pada >= 50 gagal lintas >= 5 IP dalam 1 jam, rate limit per IP membatasi laju penyerang, waktu respons seragam, dan pemilik akun dapat membuka kunci sendiri lewat reset sandi (atau `core:unlock`). Risiko residual: penyerang dengan banyak IP (botnet) tetap dapat menjadikan akun `locked` sementara. **Risiko residual ini diterima pemilik produk pada 3 November 2026**, dengan unlock lewat reset sandi, `core:unlock`, atau admin.
- **Reset sandi:** tautan sekali pakai, 30 menit, hanya hash disimpan, dipakai serentak hanya satu sukses; reset mencabut semua sesi dan membuka `locked`; `forgot` selalu sukses secara eksternal.

### 6. MFA TOTP

TOTP RFC 6238 (30 detik, 6 digit, jendela ±1 langkah) ditulis sendiri dengan stdlib, anti-replay lewat `last_used_step`: kode hanya diterima bila `step > last_used_step`, dengan pembaruan bersyarat atomik (`UPDATE ... WHERE last_used_step < $step`), sehingga kode lama dalam jendela ±1 dan pemakaian serentak ditolak, 10 recovery code sekali pakai (hash), secret tersegel, wajib untuk role `administrator` (PRD-001 Q9). **Pengecualian SHA-1 (disetujui pemilik produk, 3 November 2026, Q16; dicatat di security-guidelines §3.4):** TOTP memakai HMAC-SHA-1 **hanya** karena RFC 6238 dan RFC 4226 (HOTP) serta kompatibilitas aplikasi autentikator mensyaratkannya; ini dikecualikan dari larangan SHA-1 di security-guidelines §3.4 karena HMAC-SHA-1 tidak terdampak serangan tabrakan SHA-1 dan tidak dipakai untuk integritas data, tanda tangan, atau hash sandi. Pengecualian **khusus HOTP/TOTP** (RFC 4226, RFC 6238) dan hanya di paket TOTP (`internal/platform/identity/mfa`), dengan komentar rujukan RFC; penekanan gosec G401/G505 dibatasi pada paket itu dengan `//nolint:gosec` beralasan tertulis, bukan pengecualian global. SHA-1 tetap dilarang untuk hash sandi, tanda tangan, integritas data, dan keperluan lain. Verifikasi MFA memakai lockout yang sama.

### 7. Keputusan pemilik produk K-C1.1 sampai K-C1.9 (2 November 2026)

Seluruhnya **disetujui oleh pemilik produk pada 2 November 2026 sesuai rekomendasi**.

| # | Keputusan | Alasan singkat |
| --- | --- | --- |
| K-C1.1 | ADR ini disahkan (Accepted) sebagai langkah pertama C1 | Mekanisme harus tetap sebelum kode |
| K-C1.2 | Dependensi baru terbatas `x/crypto/argon2`, `x/time/rate`, `x/term`; JWT EdDSA dan TOTP ditulis sendiri dengan stdlib dan vektor uji RFC; SMTP lewat `net/smtp` + `crypto/tls` | Minim rantai pasok; format kecil dan teruji vektor RFC |
| K-C1.3 | C1 membangun subset FR-113: tabel tenant `roles`, `user_roles`, seed role sistem `administrator`; matriks, scope, dan field tetap C2 | MFA wajib Administrator dan `create-admin` memerlukan role |
| K-C1.4 | Linter kontrak diperluas aditif: penanda `x-erp-credential-response: true` untuk respons login/refresh pada modul `core`/`platform` terdaftar allowlist; refresh token tidak pernah di body; dicatat di security-guidelines §2 (granularitas dirinci pada bagian 7a butir 3) | `accessToken` harus ada di respons; larangan kredensial tetap berlaku di tempat lain |
| K-C1.5 | `Invocation.In io.Reader` (aditif) dan prompt tersembunyi (`x/term`) atau stdin; tanpa flag `--password` | Sandi lewat flag bocor ke riwayat shell dan daftar proses |
| K-C1.6 | SMTP lewat env sementara `ERP_MAIL_DRIVER`, `ERP_SMTP_*` (`ERP_SMTP_PASSWORD` disamarkan); pindah ke Settings platform di C3 tanpa mengubah `sdk/mail` | Settings (C3) belum ada |
| K-C1.7 | `internal/platform/identity/store` masuk allowlist GORM D3b (`authorization/store` kelak C2); PRD-000 Q42 | GORM terkurung di satu paket, seperti `modules/store` |
| K-C1.8 | `/.well-known/jwks.json` (kelak `/oauth/token`) didaftarkan platform di luar `/api/{slug}` | Modul tidak dapat mendaftarkan rute di luar slugnya |
| K-C1.9 | `ERP_AUTHZ_STUB` dan `ERP_MODULES_ADMIN_API` dipertahankan sampai C2; C1 hanya mengganti authN dan aktor audit | Gerbang mutasi admin bergantung authz penuh |

### 7a. Keputusan pemilik produk 3 November 2026 (PRD-001 Q16)

Disetujui oleh pemilik produk pada 3 November 2026 setelah review independen Wave 0:

1. **Lockout:** jeda bertingkat per pasangan (akun, IP); `locked` per akun hanya bila gagal terkumpul lintas banyak IP melampaui ambang (nilai di §5); unlock lewat reset sandi atau command baru `core:unlock --email`. Alasan: penyerang dari satu IP tidak dapat mengunci pemilik akun yang login dari IP lain. FR-101 dan FR-107 diubah agar konsisten.
2. **`ERP_SECRET_KEY`:** wajib hanya untuk `serve` dan command `core:*`; `migrate`, `inspect`, `doctor`, `modules`, `i18n` tidak membutuhkannya (format divalidasi bila terisi); rotasi lewat `ERP_SECRET_KEY_PREVIOUS` (hanya untuk membuka) dan command `core:reseal` kelak; format segel membawa id kunci dan header ikut AAD (§3). NFR-104 direvisi. Alasan: operasi pemeliharaan (migrasi, diagnosis) tidak perlu rahasia, dan rotasi harus mungkin tanpa kehilangan data tersegel.
3. **Penanda kredensial linter:** allowlist **(modul, operationId, properti)**: `accessToken` pada login/refresh, `secret`/`otpauthUri` pada enroll MFA, dan `codes` (recovery code, tampil sekali sesuai FR-107) pada confirm MFA dan regenerate recovery codes, modul `core`/`platform` (`codes` ditambahkan pemilik produk 3 November 2026); penanda dipasang pada properti; properti `refresh*` di body selalu ERROR keras; penanda tingkat operasi saja tidak cukup. Alasan: pengecualian yang sesempit mungkin agar tidak menjadi celah umum bagi kredensial.

4. **Pengecualian SHA-1 dan risiko residual DoS lockout:** (a) SHA-1 dikecualikan dari larangan security-guidelines §3.4 khusus HOTP/TOTP (RFC 4226/6238; §6). Alasan: RFC dan aplikasi autentikator mensyaratkan HMAC-SHA-1, dan HMAC-SHA-1 tidak terdampak serangan tabrakan. (b) Risiko residual bahwa penyerang berIP banyak (botnet) dapat meng-`lock` akun diterima (§5), dengan mitigasi jeda per (akun, IP), ambang lintas IP, unlock via reset sandi, `core:unlock`, atau admin.
5. **Akses `Service`/`Admin` per modul:** `identity.Admin` dan `identity.Service` hanya dipasang pada context modul `core` dan alasan sistem `cli:core:*`; aturan arch `IDENT` diperluas sehingga kode non-test modul selain `modules/core` dilarang memanggil/mereferensikan `identity.AdminFrom` dan `identity.ServiceFrom`. Alasan: modul lain tidak membutuhkan operasi administratif identitas, dan membatasi permukaannya mempersulit eskalasi lewat modul pihak ketiga (mendeteksi, bukan menjamin; lihat butir 6). Modul lain membaca `Principal` lewat `identity.FromContext`.
6. **Pengerasan aturan `IDENT` (keputusan pemilik produk 3 Nov 2026, opsi (a)+(b)):** (a) kode non-test modul dilarang mengimpor `unsafe` (termasuk `import _ "unsafe"`) dan memuat direktif `//go:linkname`, karena `//go:linkname forge erpora/sdk/identity.WithPrincipal` (fungsi tanpa badan) memintas deteksi berbasis `types.Func` (Go 1.23+ hanya membatasi linkname ke simbol stdlib); hak core dikunci ke path Go module **dan** direktori `modules/core`, dengan aturan `MODPATH` (path go.mod harus `erpora/modules/<direktori>`); dan `LoadTyped` memeriksa paket untuk varian build tag `devtools` serta OS lain (windows/linux, `CGO_ENABLED=0`). (b) kotak tertutup di `sdk/identity` menutup jalur refleksi atas context (dikerjakan terpisah). Alasan: menutup bypass yang terbukti oleh reviewer. Batas jaminan: aturan statis ini **mempersulit dan mendeteksi**, bukan menjamin. Risiko residual yang dicatat jujur: refleksi pada tipe lain dan dependensi pihak ketiga yang memanggil API tanpa dipindai arch test; modul pihak ketiga dianggap kode yang di-review sebelum dipasang. Risiko terbuka "modul memintas IDENT lewat linkname" ditutup oleh (a).

## Konsekuensi

- Verifikasi token cukup dengan kunci publik dan daftar tercabut di memori per instance: tanpa store sesi bersama di jalur request, horizontal scaling sederhana.
- Pencabutan efektif segera bila notifikasi sampai, dan paling lama 10 menit 30 detik (umur access token + leeway) bila tidak.
- `ERP_SECRET_KEY` menjadi rahasia operasional kritis: hilang berarti kunci privat tak dapat dibuka (perlu kunci baru), bocor berarti kunci privat terbuka; rotasi lewat `ERP_SECRET_KEY_PREVIOUS` dan `core:reseal` (§3, Q16); prosedur operasional didokumentasikan di `docs/operations/deployment.md`.
- Menulis JWT dan TOTP sendiri menuntut tes vektor RFC dan tes penolakan serangan (alg confusion, injeksi `kid`, replay); perubahan pada `internal/platform/identity` butuh review keamanan khusus (security-guidelines §3).
- Respons kredensial adalah pengecualian linter yang sempit dan tampil sebagai temuan `INFO`. Granularitas: allowlist (modul, operationId, properti) dengan penanda pada properti (bagian 7a butir 3).
- Rute platform di luar `/api/{slug}` menambah jenis rute yang harus dijaga allowlist keamanan.

## Alternatif yang ditolak

- **Session cookie untuk web** (pola ERPNext/Odoo): ditolak karena horizontal scaling; memerlukan store sesi bersama pada tiap request (PRD-001 Q10).
- **Access token di `localStorage`/cookie non-HttpOnly:** ditolak karena risiko pencurian lewat XSS; access token hanya di memori.
- **Daftar permission di dalam JWT:** ditolak karena membengkak dan cepat basi; memakai `perm_rev` dan cache server (C2).
- **Pustaka JWT/TOTP pihak ketiga:** ditolak (K-C1.2).
- **Query DB per request untuk revokasi:** ditolak karena beban dan titik kegagalan di jalur request.

## 8. Temuan review independen yang diterapkan (sesuai spek)

Review independen Wave 0 (3 November 2026) menghasilkan penjabaran berikut. Semuanya **sesuai spek** sehingga diterapkan tanpa menunggu keputusan pemilik produk; ini bukan keputusan pemilik produk.

| # | Penjabaran | Dasar spek |
| --- | --- | --- |
| IDENT | Aturan arsitektur `IDENT`: modul dilarang memasang identitas, layanan identitas, hak admin, atau pengirim email di context (PRD-000 §4.2, Q42 butir 5); pembatasan `AdminFrom`/`ServiceFrom` ke modul `core` adalah keputusan pemilik produk (bagian 7a butir 5) | PRD-001 FR-119 butir 2 (modul penyedia menegakkan otorisasi terhadap principal asli) |
| pending | Status kunci `pending` dan alur rotasi multi-instance (§3) | AC FR-102 "rotasi tanpa downtime" |
| MFA | Aturan MFA bagi pengguna non-admin berfaktor aktif (§4) | FR-107 (MFA opsional tetapi bila aktif harus ditegakkan) |
| promosi | Promosi kunci `pending` menjadi `active` oleh pekerja latar `serve` (§3), `core:rotate-keys` hanya membuat `pending` | AC FR-102 "rotasi tanpa downtime" |
| IDENT-uji | Aturan `IDENT` juga menolak impor `erpora/sdk/identity/identitytest` dan `testing` dari kode non-test modul; `LoadTyped` memeriksa paket dengan dan tanpa tag `devtools` | FR-119 butir 2 |

## 9. Keputusan kecil implementer C1.1 (disetujui pemilik produk 3 November 2026)

Diambil implementer C1.1 saat membangun `crypto` dan `config`, lalu ditanyakan kepada pemilik produk pada 3 November 2026. **Disetujui pemilik produk 3 November 2026 (tidak ada yang dipilih untuk diubah)**. Keputusan ini mudah dibatalkan bila pemilik produk meninjaunya kembali (perubahannya terkurung di `internal/platform/crypto`, `internal/platform/config`, dan `doctor`).

| # | Keputusan | Alasan |
| --- | --- | --- |
| KeyIDOf | Id kunci segel diturunkan dari material: `KeyIDOf` = byte pertama SHA-256(material); tabrakan id (peluang 1/256) antara kunci aktif dan lama ditolak di `config` dan `NewSealer`, dengan pesan agar kunci diganti | Id tetap cocok saat kunci pindah dari `ERP_SECRET_KEY` ke `ERP_SECRET_KEY_PREVIOUS` tanpa konfigurasi id terpisah; hash searah tidak membocorkan material |
| doctor | `erpora doctor` melaporkan **FAIL** bila `ERP_SECRET_KEY` kosong, walau kehadiran tidak diwajibkan untuk `migrate`/`inspect`/dll. | Kunci kosong berarti `serve` tidak akan berjalan; lebih baik ketahuan saat diagnosis |
| Config parsial | `config.Load` mengembalikan Config parsial bersama galat kunci; hanya `doctor` yang memakainya, pemanggil lain wajib berhenti (§3) | `doctor` tetap dapat memeriksa komponen lain |

# Pedoman Keamanan Aplikasi

| Atribut | Nilai |
| --- | --- |
| Status | **Draft v1 — menunggu review** (21 Oktober 2026) |
| Berlaku untuk | Seluruh kode platform, SDK, modul (backend & frontend), tooling, dan konfigurasi deployment |
| Dirujuk oleh | `CLAUDE.md`, PRD-000, PRD-001, setiap Kiro spec (`docs/specs/*/design.md`) |
| Terkait | `docs/architecture/api-guidelines.md`, `docs/architecture/transaction-data-standards.md`, `docs/operations/deployment.md` |

Kata **WAJIB**, **DILARANG**, dan **SEBAIKNYA** bersifat normatif.

## 0. Standar acuan

| Standar | Dipakai untuk |
| --- | --- |
| **ISO/IEC 27001:2022** | Kerangka sistem manajemen keamanan informasi (ISMS). Proyek ini menyiapkan kontrol teknis yang dibutuhkan; sertifikasi ISMS tetap memerlukan proses organisasi (kebijakan, risk assessment, audit internal) di luar kode |
| **ISO/IEC 27002:2022** | Katalog kontrol. Setiap aturan di dokumen ini merujuk nomor kontrolnya (mis. `27002:8.28`) |
| **ISO/IEC 27701:2019** | Perluasan privasi untuk data pribadi (PII) |
| **ISO/IEC 27017 / 27018** | Kontrol tambahan untuk deployment cloud dan PII di cloud (SaaS) |
| **OWASP ASVS 5.0.0** — target **Level 2** untuk seluruh sistem + **kontrol Level 3 terpilih** untuk modul berisiko (keuangan, HR) | Persyaratan verifikasi teknis yang dapat diuji (PRD-001 NFR-101). Persyaratan dirujuk dengan format berversi `v5.0.0-<bab>.<seksi>.<butir>` (mis. `v5.0.0-8.2.1`) karena ID berubah antarversi. Bab yang paling relevan: V8 Authorization, V9 Self-contained Tokens, V10 OAuth & OIDC, V14 Data Protection, V16 Logging |
| **OWASP Top 10 & OWASP API Security Top 10 (2023)** | Katalog celah yang paling sering dieksploitasi |
| **UU No. 27 Tahun 2022 tentang Pelindungan Data Pribadi (UU PDP)** | Kewajiban hukum Indonesia atas data pribadi |

> Dokumen ini **bukan** bukti kepatuhan ISO. Ia menerjemahkan kontrol ISO menjadi aturan teknis yang dapat ditegakkan di kode dan CI. Kepatuhan formal dibuktikan lewat audit ISMS.

## 1. Prinsip

1. **Aman secara default** (`27002:8.27`): konfigurasi bawaan adalah yang paling ketat (mis. `ERP_ENV=production`, listen `127.0.0.1`, telemetry mati, API admin mati).
2. **Gagal tertutup**: error saat memeriksa izin, scope, signature, atau konfigurasi → akses ditolak.
3. **Hak minimum** (`27002:5.15`, `8.2`): user, service account, akun database, dan modul hanya mendapat hak yang dibutuhkan.
4. **Pertahanan berlapis**: satu kontrol yang gagal tidak boleh langsung membuka data (contoh: scope + field group + audit).
5. **Backend adalah otoritas**: semua yang dikirim klien dianggap tidak tepercaya, termasuk total harga, status, ID, dan penanda hak akses.
6. **Dapat diaudit**: setiap aksi yang mengubah data atau hak akses meninggalkan jejak (`27002:8.15`).

## 2. Klasifikasi data (`27002:5.12`, `5.13`, `8.11`, `8.12`)

Setiap field group sensitif di manifest (PRD-001 FR-110) **WAJIB** menyatakan kelasnya.

| Kelas | Contoh | Penyimpanan | Log/trace/audit | Offline (IndexedDB) | Ekspor |
| --- | --- | --- | --- | --- | --- |
| **Publik** | Nama produk di katalog publik | Biasa | Boleh | Boleh | Boleh |
| **Internal** | Stok, nomor dokumen, nama gudang | Biasa | Boleh | Sesuai `OfflinePolicy` | Sesuai permission |
| **Rahasia** | Harga pokok, margin, data pelanggan, data karyawan (PII) | Biasa + field group berakses terbatas | **Disamarkan** | Hanya bila kebijakan modul mengizinkan + dienkripsi | Butuh permission `export` + tercatat di audit |
| **Sangat rahasia** | Password hash, secret settings, kunci, token, NIK/NPWP, rekening bank, gaji | **Dienkripsi** (AES-256-GCM, `ERP_SECRET_KEY`) atau hanya hash | **Tidak pernah** dicatat | **Dilarang** | **Dilarang** kecuali fitur khusus yang disetujui |

Manifest: `"field_groups": [{ "key": "salary", "classification": "highly_confidential", … }]`.

**Kontrak OpenAPI (Q32).** Linter kontrak mencocokkan nama properti dan parameter **per token camelCase** (`newPassword`, `clientSecret`, `resetToken`, `bankAccountNumber`). Nama yang mengandung `password`, `secret`, `token`, `apiKey`, `privateKey`, **`nik`, `npwp`, `bankAccount`, atau `salary`** wajib `x-erp-classification: highly_confidential`. Properti `highly_confidential` **tidak boleh ada di respons** dan data sensitif tidak boleh lewat path/query. Satu-satunya jalan keluar adalah pengecualian eksplisit pada properti respons 2xx, `x-erp-allow-highly-confidential: "<alasan>"` (string tidak kosong, mis. modul HR yang sah menampilkan gaji kepada pengguna HR berizin), yang **dibatasi ke data HR**: hanya properti bernama `nik`, `npwp`, `bankAccount*`, atau `salary` (per token, termasuk bentuk gabungan); nama kredensial, nama lain di luar kelompok itu, dan penempatan di luar properti respons 2xx (requestBody, parameter, respons non-2xx, skema akar) adalah ERROR (Q32 butir 14, disetujui pemilik produk 28 Okt 2026): pengecualian tidak menurunkan klasifikasi (route tetap `highly_confidential`, jadi `Cache-Control: no-store` berlaku, ekspor dan offline tetap dilarang), harus melewati field group + permission yang sesuai, dan setiap pengecualian dicetak `erpora contracts:lint` sebagai temuan `INFO` (`erp-highly-confidential-exception`) agar reviewer meninjaunya. Pengecualian `x-erp-allow-highly-confidential` untuk kredensial (`password`, `secret`, `token`, `apiKey`, `privateKey`) tidak pernah diizinkan. **Satu-satunya pengecualian terbatas (K-C1.4, disetujui pemilik produk 2 Nov 2026; granularitas ditetapkan 3 Nov 2026, ADR-0013, PRD-000 Q42, PRD-001 Q16):** properti kredensial pada respons 2xx hanya boleh lewat penanda `x-erp-credential-response: true` **pada properti itu**. Allowlist linter berbentuk **(modul, operationId, properti)** dan saat ini hanya: `accessToken` pada operasi login dan refresh, `secret` dan `otpauthUri` pada operasi pendaftaran MFA (enroll), serta `codes` (recovery code, tampil sekali sesuai FR-107) pada operasi confirm MFA dan regenerate recovery codes, semuanya modul `core` (modul `platform` didukung tetapi belum punya entri). Penanda `x-erp-credential-response: true` dipasang **pada properti respons 2xx** dan hanya sah bila tiga unsur (modul, operationId, properti) cocok dengan allowlist; penanda tingkat operasi saja **tidak cukup**, dan penanda di luar allowlist, pada requestBody, parameter, atau respons non-2xx adalah ERROR. Properti berawalan token `refresh` (mis. `refreshToken`) di body respons **selalu ERROR keras**, dengan atau tanpa penanda dan terdaftar atau tidak; refresh token hanya lewat cookie `HttpOnly; Secure; SameSite=Strict`. Route tetap `highly_confidential` (`Cache-Control: no-store`, tanpa ekspor atau offline), dan tiap pemakaian dicetak sebagai temuan `INFO` agar reviewer meninjaunya.

## 3. Katalog celah dan aturan pencegahan

Setiap baris memiliki **penegak**. "CI" berarti dicek otomatis; "Review" berarti masuk checklist code review dan Kiro spec.

### 3.1. Broken Access Control: IDOR / BOLA, BFLA, mass assignment (`27002:5.15`, `8.3`; OWASP A01, API1, API3, API5)

| Aturan | Penegak |
| --- | --- |
| Setiap endpoint dan kontrak antarmodul yang menerima ID memuat record lewat `authz.Check`/`authz.Filter` (PRD-001 FR-119) | Analyzer `go vet` + test IDOR wajib |
| Record di luar scope → 404, bukan 403 | Test IDOR |
| ID referensi di body diperiksa scope-nya | Test IDOR |
| Setiap route punya tepat satu permission; route tanpa permission harus `Public` dan masuk allowlist | Security test (PRD-000 FR-05) |
| **Mass assignment**: request di-decode ke DTO khusus per endpoint (bukan langsung ke model GORM); field yang tidak dikenal ditolak (`DisallowUnknownFields`) atau diabaikan secara eksplisit; field read-only (`id`, `status`, `createdBy`, `version`, total) tidak pernah diambil dari body | Analyzer: DILARANG `json.Decode` ke tipe di `infrastructure`; test |
| Perubahan field tanpa akses `write` → 403 `field_forbidden` | `authz.CheckWrite` + test |
| Hak sistem hanya lewat `authz.AsSystem(ctx, reason)` yang teraudit | Analyzer + audit |

### 3.2. Injection (`27002:8.28`; OWASP A03)

| Aturan | Penegak |
| --- | --- |
| **SQL**: hanya query builder GORM atau `db.Raw`/`Exec` dengan parameter binding. DILARANG membentuk SQL dengan `fmt.Sprintf`, `+`, atau template | Analyzer (string non-konstan sebagai argumen pertama `Raw`/`Exec`/`Where`/`Order`) + gosec G201/G202 |
| Nama kolom untuk sort/filter dinamis hanya dari **allowlist** di kontrak (`x-erp-sortable`/`x-erp-filterable`), dipetakan ke nama kolom tetap | Test + api-guidelines §5 |
| **Command injection**: DILARANG `os/exec` dengan input pengguna; bila perlu proses eksternal, argumen sebagai slice terpisah tanpa shell | gosec G204 |
| **Template injection**: template email/dokumen memakai `html/template` (escape otomatis); template buatan admin dijalankan di mode terbatas tanpa akses fungsi berbahaya | Review |
| **LDAP/expression/header injection**: nilai dari pengguna tidak dimasukkan ke header email, nama file, atau ekspresi tanpa validasi ketat (tolak `\r`, `\n`) | Test helper |
| **CSV/formula injection**: sel ekspor yang diawali `=`, `+`, `-`, `@`, tab, atau CR diberi awalan `'`, kecuali string desimal murni (`^-?[0-9]+(\.[0-9]+)?$`, mis. `-1250.00`) yang tidak dapat menjadi formula (PRD-000 Q30 butir 7) | Helper ekspor bersama (`sdk/export`) + test |

### 3.3. SSRF (OWASP A10, API7)

Setiap koneksi keluar yang tujuannya dipengaruhi data (webhook, OIDC discovery, impor dari URL, unduh gambar, integrasi) **WAJIB** melalui **egress client platform** (`sdk/egress`):

- **Allowlist skema**: hanya `https` (dan `http` hanya bila dikonfigurasi eksplisit untuk host tertentu).
- **Blokir alamat internal**: setelah DNS di-resolve, tolak loopback (`127.0.0.0/8`, `::1`), private (`10/8`, `172.16/12`, `192.168/16`, `fc00::/7`), link-local (`169.254/16` termasuk metadata cloud `169.254.169.254`, `fe80::/10`), CGNAT (`100.64/10`), `0.0.0.0/8`, multicast, dan alamat IPv4-mapped-IPv6 untuk rentang tersebut. Juga ditolak (Q32, M-1): IPv4-compatible `::/96`, SIIT `::ffff:0:0:0/96` (IPv4-mapped `::ffff:0:0/96` di-unmap lebih dulu lalu diperiksa sebagai IPv4), NAT64 `64:ff9b::/96` dan local-use `64:ff9b:1::/48` (IPv4 tertanam diperiksa), 6to4 `2002::/16` (IPv4 tertanam diperiksa), Teredo `2001::/32`, discard-only `100::/64`, serta rentang dokumentasi `192.0.2.0/24`, `198.51.100.0/24`, `203.0.113.0/24`, dan `2001:db8::/32`.
- **Anti DNS-rebinding**: koneksi dibuat ke IP yang sudah divalidasi (custom `DialContext`), bukan me-resolve ulang hostname.
- **Redirect**: maksimal 3, dan setiap hop divalidasi ulang dengan aturan yang sama.
- **Batas**: timeout koneksi 5 detik, total 30 detik; ukuran respons maksimal (default 10 MB); hanya content-type yang diharapkan.
- **Allowlist host per integrasi** bila memungkinkan (mis. issuer OIDC yang dikonfigurasi admin).
- Respons error dari host tujuan **tidak** diteruskan mentah ke pengguna.
- Penegak: **arch test**: `modules/**` DILARANG memakai `http.DefaultClient`, `http.Get/Post`, atau membuat `http.Client` sendiri; hanya `sdk/egress`. Test unit egress client dengan tabel alamat terlarang.

### 3.4. Sensitive Data Exposure / Cryptographic Failures (`27002:8.24`, `8.11`, `8.12`, `5.34`; OWASP A02, API3)

| Aturan | Penegak |
| --- | --- |
| Semua trafik eksternal lewat TLS ≥ 1.2 (reverse proxy); koneksi DB remote memakai `sslmode=verify-full` | deployment.md + review |
| Data kelas *Sangat rahasia* dienkripsi at-rest (AES-256-GCM, nonce acak per nilai) atau di-hash (password: argon2id) | Review + test |
| **Respons API minimal**: hanya field yang dibutuhkan; field group terlarang dihapus dari JSON (PRD-001 FR-112) | `authz.MaskFields` + test |
| DILARANG mengembalikan hash password, secret, token, kunci, atau `secret_reference` di respons API apa pun | Test kontrak + linter skema (`x-erp-classification`) |
| DILARANG menaruh data *Rahasia*/*Sangat rahasia* atau token di **URL** (query string tercatat di log proxy dan riwayat browser) | Linter kontrak + review |
| Respons API yang berisi data *Rahasia*: `Cache-Control: no-store` | Middleware + test |
| Error 5xx generik; detail hanya di log dengan `requestId` | api-guidelines §8 |
| **Log/trace/audit**: field *Sangat rahasia* tidak pernah dicatat; *Rahasia* disamarkan. Parameter SQL tidak direkam di trace | Pemindai log di integration test (PRD-001 NFR-103) |
| Kriptografi hanya dari stdlib/library terkurasi: AES-256-GCM, ChaCha20-Poly1305, Ed25519, SHA-256/512, HMAC-SHA-256, argon2id, `crypto/rand`. DILARANG MD5/SHA-1 untuk keamanan, `math/rand` untuk token, ECB, kunci hard-coded. **Satu-satunya pengecualian SHA-1 (disetujui pemilik produk 3 Nov 2026, ADR-0013 §6, PRD-001 Q16): HMAC-SHA-1 khusus HOTP/TOTP** (RFC 4226 dan RFC 6238, kompatibilitas aplikasi autentikator; HMAC-SHA-1 tidak terdampak serangan tabrakan), hanya di paket `internal/platform/identity/mfa`. Penekanan temuan gosec G401/G505 hanya di paket itu dengan `//nolint:gosec` beralasan tertulis (rujukan RFC), tidak pernah global; SHA-1 tetap dilarang untuk hash sandi, tanda tangan, dan integritas data | gosec G401–G505 + review |
| Rotasi kunci terdokumentasi (`erpora core:rotate-keys`, `ERP_SECRET_KEY`) | deployment.md |
| Offline: data *Rahasia* di IndexedDB hanya bila `OfflinePolicy` modul mengizinkan, dienkripsi, dan dihapus saat logout | PRD Offline & Sync |

### 3.5. Autentikasi & sesi (`27002:5.17`, `8.5`; OWASP A07, API2)

Diatur PRD-001 FR-101–FR-108: argon2id, lockout & rate limit, MFA wajib Administrator, JWT berumur pendek + refresh berotasi dengan deteksi reuse, pencabutan sesi (seketika di instance yang memprosesnya; instance/proses lain paling lama ±1 menit lewat muat ulang daftar tercabut, PRD-001 Q32 butir 2), OIDC dengan PKCE/state/nonce, PAT ber-hash dengan prefix. Tambahan:

- **Open redirect**: parameter `returnTo`/`redirect_uri` hanya menerima path relatif same-origin atau URL dari allowlist.
- Pesan login tidak membedakan "user tidak ada" dan "password salah"; respons waktu dibuat seragam.
- **Password**: dinormalisasi ke Unicode NFKC sebelum di-hash, diverifikasi, dan diperiksa kebijakan (ASVS 5.0; keputusan pemilik produk 3 Nov 2026, PRD-001 Q17). Batas 1024 byte diperiksa pada masukan mentah sebelum normalisasi (cegah DoS); panjang minimum 12 dalam rune pasca-NFKC; daftar sandi umum dicari pada NFKC + huruf kecil.

### 3.6. Celah sisi browser (OWASP A03, A05)

| Aturan | Penegak |
| --- | --- |
| **XSS**: React melakukan escape otomatis. `dangerouslySetInnerHTML` DILARANG kecuali melalui komponen `@erpora/ui` `SafeHtml` yang memakai DOMPurify dengan konfigurasi ketat | Biome lint rule + review |
| CSP ketat tanpa `unsafe-inline` untuk script (PRD-000 Global Constraints); tanpa `eval`/`new Function` | Header test |
| **CSRF**: API memakai Bearer token; satu-satunya endpoint berbasis cookie (`refresh`) memakai `SameSite=Strict` + cek `Origin` | PRD-001 FR-102 |
| **Clickjacking**: `frame-ancestors 'none'` | Header test |
| Token tidak disimpan di `localStorage`/`sessionStorage` | E2E (PRD-001 FR-102) |
| Tautan eksternal `target="_blank"` selalu `rel="noopener noreferrer"` | Lint |

### 3.7. Upload & pemrosesan file (`27002:8.7`; OWASP A04)

- Batas ukuran per endpoint (default 10 MB) ditegakkan **sebelum** membaca seluruh body (`http.MaxBytesReader`).
- Tipe file divalidasi dari **isi** (magic bytes), bukan ekstensi atau `Content-Type` klien; allowlist per fitur.
- Nama file disimpan sebagai ID acak; nama asli hanya metadata dan disanitasi saat diunduh (`Content-Disposition: attachment; filename*=`).
- File disimpan di luar direktori yang disajikan dan diunduh lewat endpoint ber-otorisasi (IDOR berlaku juga untuk file).
- Gambar di-*re-encode* untuk membuang metadata dan payload tersembunyi; SVG dari pengguna DILARANG ditampilkan inline.
- Hook pemindaian malware (mis. ClamAV) disediakan; wajib aktif untuk deployment SaaS.
- Arsip (zip) diekstrak dengan batas jumlah file, total ukuran, rasio kompresi, dan validasi path (zip-slip).

### 3.8. Parsing & deserialisasi (OWASP A08)

- Batas ukuran body JSON per endpoint (default 1 MB) dan batas kedalaman/jumlah elemen array.
- XML: tanpa resolusi entitas eksternal (XXE); gunakan `encoding/xml` (tidak me-resolve DTD eksternal) dan tolak DOCTYPE bila tidak dibutuhkan.
- DILARANG deserialisasi format yang dapat menjalankan kode atau memuat tipe arbitrer (mis. `gob` dari input tidak tepercaya).

### 3.9. Abuse logika bisnis & race condition (OWASP API6; lihat `transaction-data-standards.md`)

- Total, harga, pajak, diskon, dan saldo **dihitung server**; nilai dari klien hanya untuk validasi silang.
- Validasi angka: tolak kuantitas/nominal negatif kecuali alur yang memang mengizinkannya (retur, penyesuaian).
- Operasi ganda dicegah dengan `Idempotency-Key` dan referensi sumber unik; konflik konkurensi dengan `If-Match` + locking.
- Alur bertahap (approve, posting) memeriksa status saat ini di dalam transaksi, bukan status yang dibaca sebelumnya.

### 3.10. Pembatasan sumber daya (OWASP API4)

- Rate limit per IP dan per principal untuk endpoint auth, token, ekspor, impor, dan pencarian.
- `pageSize` maksimal 200, `limit` cursor maksimal 1000, `[in]` maksimal 100 nilai, `expand` satu tingkat (api-guidelines).
- Timeout server: `ReadHeaderTimeout` 10 detik; timeout query database per request; job panjang dipindah ke antrean.

### 3.11. Konfigurasi & komponen (`27002:8.8`, `8.9`, `8.19`; OWASP A05, A06, API8, API9)

- Binary release tanpa developer tools (build tag `devtools`, PRD-000 FR-22); `ERP_ENV=production` default.
- Dependensi dikunci (`go.sum`, `bun.lock`); `govulncheck`, `bun audit`, dan pembaruan otomatis (Renovate/Dependabot) di CI.
- **SBOM** (CycloneDX) dibuat untuk setiap rilis.
- Inventaris endpoint selalu sesuai kode (test kesesuaian kontrak ↔ route, PRD-000 FR-23); endpoint lama dihapus hanya lewat proses deprecation.

### 3.12. Logging, monitoring, dan sinkronisasi waktu (`27002:8.15`, `8.16`, `8.17`; OWASP A09)

- Audit log append-only (PRD-001 FR-130) untuk autentikasi, perubahan hak akses, settings, lifecycle modul, ekspor data, dan penolakan akses.
- Log terstruktur dengan `requestId`/`traceId`; tanpa data *Sangat rahasia*.
- Alert untuk: lonjakan login gagal, refresh token reuse, lonjakan 403/404 dari satu principal (indikasi enumerasi/IDOR), operasi modul `failed`.
- Server memakai sinkronisasi waktu (NTP); semua timestamp UTC.

## 4. Siklus pengembangan aman (`27002:8.25`–`8.29`, `8.31`–`8.33`)

| Tahap | Kewajiban |
| --- | --- |
| **Spec** (Kiro `design.md` / PRD) | Bagian **Keamanan** wajib (sekaligus memenuhi "documented security decisions" ASVS 5.0): daftar persyaratan ASVS yang relevan (`v5.0.0-…`), **kontrol L3 yang dipilih** bila modul berisiko, aset & klasifikasi data, permission/scope/field group, ancaman STRIDE singkat, titik masuk ID (IDOR), koneksi keluar (SSRF), penanganan file, data yang disimpan offline |
| **Implementasi** | Mengikuti dokumen ini; generator menghasilkan kode aman secara default |
| **Review** | Checklist §6; perubahan pada `internal/platform/identity`, `authorization`, `egress`, atau kriptografi butuh review keamanan khusus |
| **CI** | gosec, analyzer kustom, arch test, test IDOR, secret scan (gitleaks), `govulncheck`, `bun audit`, linter kontrak (`x-erp-classification`), header test |
| **Pra-rilis** | Pemindaian DAST (OWASP ZAP baseline) terhadap environment staging; penetration test sebelum rilis MAJOR atau sebelum marketplace dibuka |
| **Environment** | Dev, staging, dan production terpisah; **DILARANG** memakai data production di dev/test; data uji dari seeder/fixture atau hasil anonimisasi |

## 5. Manajemen kerentanan & insiden (`27002:5.24`–`5.28`, `8.8`)

- `SECURITY.md` di root repository: cara melaporkan kerentanan secara privat.
- Target waktu perbaikan sejak dikonfirmasi: **Critical 7 hari, High 30 hari, Medium 90 hari, Low** sesuai rilis berikutnya.
- Insiden data pribadi mengikuti UU PDP (pemberitahuan kepada subjek data dan lembaga berwenang dalam batas waktu yang ditetapkan peraturan).
- Setiap insiden menghasilkan postmortem tanpa menyalahkan individu dan test regresi.

## 6. Checklist keamanan (PR & Kiro spec)

- [ ] Klasifikasi data untuk field baru sudah ditetapkan (§2).
- [ ] Endpoint/kontrak yang menerima ID memakai `authz.Check`/`authz.Filter` dan punya test IDOR (A vs B).
- [ ] Request di-decode ke DTO; field read-only tidak diambil dari body.
- [ ] Tidak ada SQL dari string dinamis; sort/filter memakai allowlist.
- [ ] Koneksi keluar lewat `sdk/egress`.
- [ ] Tidak ada secret/PII di URL, log, trace, atau pesan error.
- [ ] Respons data *Rahasia* memakai `Cache-Control: no-store`.
- [ ] Upload: batas ukuran, validasi magic bytes, nama acak, unduh ber-otorisasi.
- [ ] Ekspor CSV memakai helper anti formula injection.
- [ ] Nilai uang/total dihitung server; konkurensi ditangani (`If-Match`, idempotency, locking).
- [ ] Aksi baru tercatat di audit log.
- [ ] Rate limit untuk endpoint mahal atau sensitif.

Butir umum di atas berlaku untuk setiap PR. Status penegakan autentikasi dan sesi per milestone C1 (ASVS 5.0.0 bab V6 Authentication, V7 Session Management, V9 Self-contained Tokens, dan V3 untuk penyimpanan token di browser) dicantumkan di bawah. Nomor requirement ASVS tidak dirujuk satu per satu; pemetaan per kategori ada di PRD-001 NFR-101.

**Autentikasi (V6)**

- [x] Sandi minimal 12 karakter (dihitung dalam rune setelah NFKC), maksimal 1024 byte pada masukan mentah, dan tidak boleh ada di daftar sandi umum. Sandi dinormalisasi NFKC sebelum di-hash dan diverifikasi. (`internal/platform/identity/password/policy.go`, `hash.go`)
- [x] Sandi di-hash argon2id dengan parameter bawaan OWASP (m=19456 KiB, t=2, p=1); parameter dapat diatur lewat `ERP_ARGON2_*` dengan batas bawah. Pembatas konkurensi hash (`guard/hashgate.go`) membatasi pemakaian memori serentak.
- [x] Login gagal menjawab `invalid_credentials` (401) yang seragam untuk email tak dikenal, sandi salah, akun `locked`, dan akun nonaktif. Waktu respons dibuat seragam dengan lantai 200 ms dan hash tiruan (`password.NewDummy`). (`guard/authenticator.go`, `identity/service.go`)
- [x] Lockout per pasangan (akun, IP): jeda bertingkat mulai kegagalan ke-5 (30 detik, berlipat, maksimum 15 menit); blokir pasangan 1 jam pada kegagalan ke-20; status `locked` hanya bila ada ≥50 kegagalan lintas ≥5 IP dalam 1 jam. Buka kunci lewat reset sandi atau `core:unlock`. (`guard/lockout.go`; `guard/lockout_test.go`)
- [x] Rate limit per IP dan per principal pada endpoint auth; respons `429 rate_limited` dengan `Retry-After`. (`guard/limiter.go`, `httpserver/server.go`)
- [x] MFA TOTP (RFC 6238) dengan anti-replay lewat `last_used_step`, dan recovery code sekali pakai yang disimpan sebagai SHA-256 terikat pengguna. (`mfa/totp.go`, `mfa/recovery.go`, `store/mfa.go`)
- [x] MFA wajib untuk role `administrator`: login mengembalikan `mfaState` `enroll_required` atau `verify_required`, dan operasi administratif menuntut sesi ber-`otp` (`identity/admin.go`). Pengguna non-admin dengan faktor aktif tetap diminta verifikasi (FR-107). (`identity/mfastate.go`, `mfa/service.go`)
- [x] Reset sandi: tautan sekali pakai berlaku 30 menit, hanya hash SHA-256 yang disimpan, `forgot` menjawab seragam, dan tautan milik akun nonaktif ditolak seperti token tidak sah. Reset mencabut seluruh sesi pengguna. (`reset/service.go`, `store/resets.go`)
- [x] Ganti sandi mencabut seluruh sesi lain dan mempertahankan sesi yang sedang dipakai. (`store/resets.go` `ChangePassword`)
- [x] Respons login, refresh, dan MFA diberi `Cache-Control: no-store` (`httpserver/server.go`). Refresh token tidak pernah masuk body JSON (`json:"-"`).
- [ ] Re-autentikasi untuk aksi berisiko tinggi (ASVS Level 3 terpilih, NFR-101): **belum**, menyusul bersama modul keuangan dan HR.

**Sesi (V7)**

- [x] Access token JWT berumur 10 menit (`token/keys.go` `AccessTokenTTL`). Refresh token opak 32 byte dari `crypto/rand`, hanya hash SHA-256 yang disimpan. (`session/service.go`)
- [x] Refresh berotasi. Pemakaian ulang token lama mencabut family dan sesi (`refresh_reuse`). Dua refresh serentak dengan token yang sama menghasilkan tepat satu rotasi sukses. (`session/service.go`; `session/session_test.go`)
- [x] Umur sesi: sliding 14 hari dan absolut 90 hari. (`session/service.go`; `TestSlidingExpiry14Days`, `TestAbsoluteLifetime90DaysEnforced`)
- [x] Cookie refresh `HttpOnly; Secure; SameSite=Strict; Path=/api/core/auth/refresh`. Refresh dengan header `Origin` hilang atau tidak diizinkan ditolak 403. (`modules/core/internal/presentation/http/cookie.go`, `session/origin.go`)
- [x] Logout mencabut sesi aktif. `Admin.LogoutAllDevices` mencabut seluruh sesi pengguna. `core:reset-mfa` mencabut sesi pengguna yang bersangkutan. (`identity/service.go`, `identity/admin.go`)
- [x] Pencabutan dipatuhi lintas instance: setiap instance memuat ulang daftar sesi tercabut paling lama tiap 1 menit dan menerima notifikasi `ChannelSessionRevoked`. Notifikasi yang hilang tidak membuat sesi tercabut berlaku selamanya karena muat ulang berkala. (`session/revocation.go`; PRD-001 Q32 butir 2)
- [x] Status `locked` tidak mencabut sesi yang sudah ada (PRD-001 Q30 butir 4); sesi itu tetap tidak dapat mengganti sandi atau memverifikasi MFA selama akun `locked` (PRD-001 Q34). Pembaruan sesi menolak pengguna yang tidak aktif (`ErrUserNotActive`). (`session/service.go`)
- [ ] Penonaktifan akun mencabut seluruh sesinya (`OnUserDisabled`, alasan `user_disabled`): hook sudah ada, tetapi belum ada jalur penonaktifan akun di C1 yang memanggilnya. Menyusul bersama operasi pengguna di C2/C3.
- [x] Access token hanya di memori klien. Refresh token hanya di cookie HttpOnly. Tidak ada `localStorage`, `sessionStorage`, `document.cookie`, atau IndexedDB untuk token (V3). (`packages/platform-web/src/auth/controller.ts`)

**Token (V9)**

- [x] Verifikasi JWT hanya algoritma EdDSA (Ed25519). `alg: none` dan HS256 dengan kunci publik sebagai rahasia ditolak. `kid` hanya dicocokkan ke daftar kunci dan tidak pernah menjadi path atau SQL. `iss` dan `aud` diperiksa, leeway 30 detik, dan kunci berstatus `pending` atau `retired` ditolak. (`token/jwt.go`; `token/jwt_test.go`: `TestVerifyRejectsNoneAlg`, `TestVerifyRejectsHS256WithPublicKeyAsSecret`, `TestVerifyRejectsKIDWithPathOrSQL`, `TestVerifyRejectsRetiredKey`)
- [x] Rotasi kunci dengan tahap `pending`, `active`, `retiring`, `retired` lewat `core:rotate-keys`. JWKS dipublikasikan di `/.well-known/jwks.json` tanpa kunci privat. (`token/keys.go`, `token/jwks.go`, `internal/app/identity.go`)

**Belum ditegakkan (menyusul milestone berikutnya)**

- [ ] Otorisasi per objek dan per field (ASVS V8): **C2** (PRD-001 FR-110 sampai FR-119).
- [ ] Audit log append-only untuk login, perubahan hak akses, dan pencabutan sesi: **C3** (PRD-001 FR-130). Tabel `login_attempts` dan log server bukan audit log.
- [ ] OIDC/SSO, PAT, dan OAuth2 client credentials (ASVS V10): **C4** (PRD-001 FR-103 sampai FR-105).
- [x] Pemindaian log lintas autentikasi untuk memastikan tidak ada rahasia tercatat (NFR-103): `internal/app/core_api_secretscan_integration_test.go` menangkap seluruh log selama alur auth API penuh (login, refresh, ubah sandi, logout, lupa/reset, enroll/verify MFA, recovery code) dan memastikan tidak ada sandi, JWT, refresh token, secret/kode TOTP, recovery code, atau token reset; ditambah test CLI (`core_cli_integration_test.go`, `tests/integration/core_cli_test.go`).

## 7. Status penegakan

| Kontrol | Status |
| --- | --- |
| Arch test D1–D11, header keamanan, request ID, DSN redaction, secret scan | **M1** (sedang dibangun) |
| Route permission allowlist, kontrak ↔ route, linter kontrak | **M2d ✅** (PRD-000 FR-23) |
| Timeout query database per request (`statement_timeout`) | **M2c ✅** (PRD-000 Q26, Q30) |
| Autentikasi lokal (argon2id, kebijakan sandi, lockout, rate limit 429), MFA TOTP dan recovery code, MFA wajib administrator, reset sandi, sesi dan refresh berotasi, JWT EdDSA dan JWKS, rotasi kunci, pemisahan `identity.Service`/`Admin` | **C1 ✅** (implementasi; PRD-001 FR-101, FR-102, FR-106, FR-107; ADR-0013 Accepted 2 Nov 2026. Gerbang C1.15 Step 1 dan review akhir belum dijalankan) |
| Pengiriman email (`sdk/mail`, driver `smtp` dan `log`; driver `log` di produksi tidak mencatat isi, tautan, atau token) | **C1 ✅** (PRD-001 FR-108; `ERP_SMTP_*` sementara sampai Settings di C3) |
| Settings platform minimal `platform.public_url` (dipakai tautan reset dan pemeriksaan `Origin`) | **C1 ✅** (PRD-001 Q24; `internal/platform/settings`, tanpa UI) |
| Authorization (helper `authz`, test IDOR, analyzer authz) | **C2** (PRD-001 FR-110 sampai FR-119; menyusul) |
| Audit append-only (login, perubahan hak akses, pencabutan sesi) | **C3** (PRD-001 FR-130 sampai FR-132; menyusul) |
| OIDC/SSO, PAT, OAuth2 client credentials | **C4** (PRD-001 FR-103 sampai FR-105; menyusul) |
| SBOM, `SECURITY.md`, gosec | **M1** (PRD-000 FR-24.1) |
| `sdk/egress` (anti-SSRF) + arch test `EGRESS` | **M2c ✅** (PRD-000 FR-24.2) |
| `httpx.DecodeJSON` + arch test `DECODE` | **M2c ✅** |
| `Cache-Control: no-store` berbasis klasifikasi route (`module.Classify`) | **M2c ✅** |
| `sdk/export` (CSV/XLSX, masking, audit) | **M2c ✅** |
| Klasifikasi field group di manifest | **M2c ✅** |
| Linter kontrak `x-erp-classification`; kesesuaian klasifikasi route ↔ kontrak | **M2d ✅** (PRD-000 FR-23, FR-24.2; rule `erp-*-classification`, `erp-no-highly-confidential-in-response`, `erp-no-sensitive-in-url`; `TestContracts_ClassificationMatchesRoute`) |
| Penanda lingkungan database + guard binary dev (`production` ditolak), `db:fresh` berlapis | **M2d ✅** (PRD-000 FR-22.5) |
| `SafeHtml`, OWASP ZAP baseline | **M3** (PRD-000 FR-24.3) |
| Pipeline upload file | **PRD Files** (sebelum fitur upload pertama) |
| Mass assignment pada DTO bangkitan metadata: decoder (`httpx.DecodeJSON`) menolak field tak dikenal dan field `readOnly` dengan `422 validation_failed` rinci per field; galat validasi tanpa nilai field; kode bangkitan lolos arch test tanpa pengecualian | **Resource metadata T1 ✅** (spec `resource-metadata` Req. 9, 19; `task metadata:generate:check`) |
| Validasi server otoritatif atas tipe, wajib, panjang, rentang, pola (RE2, waktu linear), enum, desimal string, dan `rules` lintas field; `422 business_rule_violated` untuk aturan bisnis | **Resource metadata T1 ✅** (`docs/module-sdk/generators.md` §10) |
| Metadata klien publik tanpa aturan `scope: server`, ambang atau field `confidential` ke atas, field `clientVisible: false`, dan ekspresi accessor `stored`; pemuat menolak chunk yang bocor (`checkClientMeta`) | **Resource metadata T1 ✅** (design D-8, D-21) |
| Pembangkit aman: hanya menulis di bawah `modules/<dir>/`, tanpa symlink, gagal tanpa menulis bila check gagal atau penanda `Code generated` hilang, literal lewat satu fungsi kutip per bahasa, tes golden masukan berbahaya; field `highly_confidential` ditolak (K17) | **Resource metadata T1 ✅** (design D-32; Req. 19) |
| Pemicu database penguncian dokumen, mesin selisih, dan migrasi bangkitan dengan penjaga data | **Resource metadata T2** (menyusul) |
| `authz.Check/Filter/CheckWrite/MaskFields` pada CRUD bangkitan, tes IDOR A vs B per titik masuk ID (matriks design), `SELECT ... FOR UPDATE` pada transisi, relasi dan salinan field lintas modul | **Resource metadata T4** (menyusul; bergantung PRD-001 FR-110 sampai FR-115, FR-119) |

## 8. Pertanyaan terbuka

1. ~~Penjadwalan kontrol~~ → diputuskan: PRD-000 FR-24 (Q22).
2. ~~Target ASVS~~ → diputuskan: ASVS 5.0.0 L2 + kontrol L3 terpilih per modul berisiko (§0, §4).
3. Pemindaian malware wajib juga untuk self-hosted, atau opsional?

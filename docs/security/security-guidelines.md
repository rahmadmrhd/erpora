# Pedoman Keamanan Aplikasi

| Atribut | Nilai |
| --- | --- |
| Status | **Draft v1 — menunggu review** (21 Oktober 2026) |
| Berlaku untuk | Seluruh kode platform, SDK, modul (backend & frontend), tooling, dan konfigurasi deployment |
| Dirujuk oleh | `CLAUDE.md`, PRD-000, PRD-001, setiap Kiro spec (`.kiro/specs/*/design.md`) |
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
| **CSV/formula injection**: sel ekspor yang diawali `=`, `+`, `-`, `@`, tab, atau CR diberi awalan `'` | Helper ekspor bersama + test |

### 3.3. SSRF (OWASP A10, API7)

Setiap koneksi keluar yang tujuannya dipengaruhi data (webhook, OIDC discovery, impor dari URL, unduh gambar, integrasi) **WAJIB** melalui **egress client platform** (`sdk/egress`):

- **Allowlist skema**: hanya `https` (dan `http` hanya bila dikonfigurasi eksplisit untuk host tertentu).
- **Blokir alamat internal**: setelah DNS di-resolve, tolak loopback (`127.0.0.0/8`, `::1`), private (`10/8`, `172.16/12`, `192.168/16`, `fc00::/7`), link-local (`169.254/16` termasuk metadata cloud `169.254.169.254`, `fe80::/10`), CGNAT (`100.64/10`), `0.0.0.0/8`, multicast, dan alamat IPv4-mapped-IPv6 untuk rentang tersebut.
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
| Kriptografi hanya dari stdlib/library terkurasi: AES-256-GCM, ChaCha20-Poly1305, Ed25519, SHA-256/512, HMAC-SHA-256, argon2id, `crypto/rand`. DILARANG MD5/SHA-1 untuk keamanan, `math/rand` untuk token, ECB, kunci hard-coded | gosec G401–G505 + review |
| Rotasi kunci terdokumentasi (`erp core:rotate-keys`, `ERP_SECRET_KEY`) | deployment.md |
| Offline: data *Rahasia* di IndexedDB hanya bila `OfflinePolicy` modul mengizinkan, dienkripsi, dan dihapus saat logout | PRD Offline & Sync |

### 3.5. Autentikasi & sesi (`27002:5.17`, `8.5`; OWASP A07, API2)

Diatur PRD-001 FR-101–FR-108: argon2id, lockout & rate limit, MFA wajib Administrator, JWT berumur pendek + refresh berotasi dengan deteksi reuse, pencabutan instan, OIDC dengan PKCE/state/nonce, PAT ber-hash dengan prefix. Tambahan:

- **Open redirect**: parameter `returnTo`/`redirect_uri` hanya menerima path relatif same-origin atau URL dari allowlist.
- Pesan login tidak membedakan "user tidak ada" dan "password salah"; respons waktu dibuat seragam.

### 3.6. Celah sisi browser (OWASP A03, A05)

| Aturan | Penegak |
| --- | --- |
| **XSS**: React melakukan escape otomatis. `dangerouslySetInnerHTML` DILARANG kecuali melalui komponen `@erp/ui` `SafeHtml` yang memakai DOMPurify dengan konfigurasi ketat | Biome lint rule + review |
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

## 7. Status penegakan

| Kontrol | Status |
| --- | --- |
| Arch test D1–D11, header keamanan, request ID, DSN redaction, secret scan | **M1** (sedang dibangun) |
| Route permission allowlist, kontrak ↔ route, linter kontrak | **M2** (PRD-000) |
| Authz helper, test IDOR, analyzer authz, MFA, audit | **PRD-001** |
| SBOM, `SECURITY.md`, gosec | **M1** (PRD-000 FR-24.1) |
| `sdk/egress`, klasifikasi data + `x-erp-classification`, `DecodeJSON`, `no-store`, `sdk/export` | **M2** (PRD-000 FR-24.2) |
| `SafeHtml`, OWASP ZAP baseline | **M3** (PRD-000 FR-24.3) |
| Pipeline upload file | **PRD Files** (sebelum fitur upload pertama) |

## 8. Pertanyaan terbuka

1. ~~Penjadwalan kontrol~~ → diputuskan: PRD-000 FR-24 (Q22).
2. ~~Target ASVS~~ → diputuskan: ASVS 5.0.0 L2 + kontrol L3 terpilih per modul berisiko (§0, §4).
3. Pemindaian malware wajib juga untuk self-hosted, atau opsional?

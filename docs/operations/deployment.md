# Panduan Deployment

> **Status: DRAF.** Dokumen ini ditulis berdasarkan PRD-000 sebelum binary-nya ada. Setiap perintah wajib diverifikasi terhadap binary nyata sebelum milestone M2 ditutup (PRD-000 FR-16). Bagian yang belum terverifikasi ditandai 🔶.

Aplikasi ini didistribusikan sebagai **satu binary** (`erpora` / `erpora.exe`). Binary tersebut sudah berisi API dan frontend (PWA). Satu-satunya dependensi eksternal wajib adalah **PostgreSQL**.

---

## 1. Persyaratan hosting

### 1.1. Yang didukung

| Lingkungan | Status | Catatan |
| --- | --- | --- |
| VPS / bare metal Linux (x86_64, arm64) | ✅ Direkomendasikan | systemd + Caddy |
| Windows Server / Windows 10+ | ✅ Didukung | Dijalankan sebagai Windows service (§5) |
| Container (Docker/Podman) | ✅ Didukung 🔶 | Image dari `infrastructure/docker/Dockerfile` (🔶 belum tersedia) |
| PaaS berbasis container (Fly.io, Railway, Render, dsb.) | ✅ Didukung | Harus menyediakan cara menjalankan perintah one-off (`erpora migrate`, `erpora modules`) |
| **Shared hosting PHP (cPanel, DirectAdmin, Plesk shared)** | ❌ **Tidak didukung** | Lihat §1.3 |

### 1.2. Spesifikasi minimum

| Komponen | Minimum | Disarankan |
| --- | --- | --- |
| CPU | 1 vCPU | 2 vCPU |
| RAM (aplikasi + PostgreSQL di satu mesin) | 1 GB | 2 GB |
| Disk | 10 GB SSD | 20 GB SSD + ruang backup terpisah |
| PostgreSQL | Versi minimum di ADR-003 🔶 | Major stabil terbaru |
| Akses | Shell (SSH / RDP / console) | — |
| Jaringan | Port 80/443 terbuka untuk reverse proxy | — |

Go binary sendiri hanya memakai sekitar puluhan MB RAM saat idle. Sebagian besar kebutuhan memori berasal dari PostgreSQL.

### 1.3. Kenapa shared hosting tidak didukung

Aplikasi ini adalah **proses yang berjalan terus** (long-running). Beberapa fitur bergantung pada sifat tersebut:
- status modul disimpan di memori untuk pemeriksaan cepat di setiap request,
- operasi lifecycle modul memegang lock database selama operasi berlangsung,
- telemetry dikirim secara batch.

Shared hosting berbasis PHP menjalankan kode per request dan biasanya mematikan proses latar. Selain itu umumnya hanya menyediakan MySQL dan tidak memberi akses shell untuk `erpora migrate` / `erpora modules`.

---

## 2. Topologi

### 2.1. Single node (rekomendasi awal)

```
Internet ──443──▶ Caddy (TLS) ──▶ erpora serve (127.0.0.1:8080) ──▶ PostgreSQL (localhost:5432)
```

Semuanya berada di satu VPS. Karena `erpora` hanya listen di `127.0.0.1`, aplikasi tidak bisa diakses langsung dari internet tanpa melewati reverse proxy.

### 2.2. Aplikasi + managed PostgreSQL

```
Internet ──443──▶ Caddy ──▶ erpora serve ──TLS──▶ Managed PostgreSQL
```

Cocok bila backup dan high availability database ingin diserahkan ke penyedia.

> Horizontal scaling (beberapa instance `erpora` di belakang load balancer) **belum didukung** pada PRD-000. Snapshot status modul masih in-process (FR-19.7), dan sinkronisasi antar-instance via `LISTEN/NOTIFY` menyusul.

---

## 3. Konfigurasi

Semua konfigurasi memakai environment variable. Daftar lengkapnya ada di `.env.example`.

| Variabel | Wajib | Default | Keterangan |
| --- | --- | --- | --- |
| `ERP_DATABASE_URL` | ✅ | — | `postgres://erp:PASSWORD@localhost:5432/erp?sslmode=disable` (gunakan `sslmode=require`/`verify-full` untuk DB remote) |
| `ERP_DB_SLOW_QUERY_MS` | | `200` | Ambang (milidetik, bilangan bulat ≥ 0) untuk mencatat query database yang melebihi batas sebagai `slow query` (level WARN). `0` menonaktifkan pencatatan query lambat |
| `ERP_SECRET_KEY` | ✅ untuk `serve` dan command penyegel (`core:*`), semua mode | — | 32 byte acak (base64 standar dengan padding) untuk menyegel (AES-256-GCM) kunci JWT, secret settings, dan secret MFA. Kosong menggagalkan `serve` (exit 2) di semua mode termasuk development (tanpa kunci sementara otomatis); `migrate`, `inspect`, `doctor`, `modules`, dan command lain tidak membutuhkannya, tetapi nilai yang terisi selalu divalidasi (format atau panjang salah = gagal) dan tidak pernah dicetak. **Simpan cadangannya terpisah dari backup database**: tanpa kunci ini, data terenkripsi di backup tidak dapat dibaca. Buat SEKALI per lingkungan: di mesin developer `task erpora -- secret:generate` (hanya mencetak, tidak menyimpan); di server (binary release tanpa developer tools) `openssl rand -base64 32` |
| `ERP_SECRET_KEY_PREVIOUS` | | — | Opsional, untuk rotasi kunci: kunci lama yang hanya dipakai membuka data tersegel sebelumnya (id kunci pada header segel memilih kunci). Pasang kunci lama di sini dan kunci baru di `ERP_SECRET_KEY`; lepas setelah semua data disegel ulang (`core:reseal`, menyusul). Tidak boleh sama dengan `ERP_SECRET_KEY`. Prosedur lengkap dan peringatannya di [§3.2](#32-rotasi-erp_secret_key) |
| `ERP_MAIL_DRIVER` | ✅ untuk `serve` di production | `log` di development | Pengirim surat (FR-108): `log` (tidak mengirim apa pun; hanya mencatat penerima tersamar dan subjek, isi surat penuh hanya bila `ERP_ENV=development`) atau `smtp`. **Di production `serve` gagal start (exit 2) bila variabel ini tidak diisi** (fail-closed; keputusan pemilik produk 3 Nov 2026, PRD-001 Q21): isi `smtp` agar surat (mis. tautan reset sandi) terkirim, atau `log` hanya bila Anda sengaja tidak mengirim surat. `migrate`, `inspect`, `doctor`, `modules`, dan `i18n` berjalan tanpa konfigurasi surat. URL publik untuk tautan di email berasal dari settings platform, bukan env |
| `ERP_SMTP_HOST` | ✅ bila `smtp` | — | Nama host atau IP server SMTP (tanpa skema/port) |
| `ERP_SMTP_PORT` | | `587` (`starttls`), `465` (`tls`), `25` (`none`) | Port server SMTP (1–65535) |
| `ERP_SMTP_USERNAME` | | — | Username autentikasi (PLAIN/LOGIN); harus diisi bersama `ERP_SMTP_PASSWORD`. Autentikasi hanya di atas TLS yang terverifikasi |
| `ERP_SMTP_PASSWORD` | | — | Sandi SMTP (tidak dipotong spasinya). Disamarkan (`****`) di log dan `erpora inspect config`; simpan di file env dengan izin akses terbatas |
| `ERP_SMTP_FROM` | ✅ bila `smtp` | — | Alamat pengirim polos ASCII (mis. `noreply@example.com`; tanpa nama tampilan) |
| `ERP_SMTP_TLS_MODE` | | `starttls` | `starttls` (STARTTLS wajib; server tanpa STARTTLS ditolak, tidak ada downgrade), `tls` (TLS implisit), atau `none` (hanya development, tanpa autentikasi; ditolak saat `ERP_ENV=production`). Sertifikat server selalu diverifikasi |
| `ERP_TRUSTED_PROXIES` | | *(kosong)* | Daftar IP atau CIDR proxy tepercaya, dipisah koma (mis. `10.0.0.0/8,192.0.2.5`); entri IP tunggal menjadi `/32` atau `/128`. **Wajib diisi bila aplikasi berjalan di belakang reverse proxy** (nginx, Caddy, Traefik, load balancer): batas laju per IP dan lockout per pasangan (akun, IP) memakai IP klien, dan tanpa variabel ini semua klien tampak berasal dari IP proxy (saling mengunci dan batas per IP menjadi batas global). Bila peer TCP ada di daftar, `X-Forwarded-For` dibaca dari kanan ke kiri dan hop pertama yang bukan proxy tepercaya menjadi IP klien (maks. 32 hop terkanan, header maks. 4096 byte; header cacat dianggap tidak tepercaya dan dipakai alamat peer); `X-Real-IP` tidak dipakai. Bila kosong (bawaan), `X-Forwarded-For` diabaikan sehingga tidak dapat dipalsukan. Proxy Anda harus **menambahkan** IP peer ke `X-Forwarded-For` (bukan menggantinya). Maks. 64 entri, tanpa `/0`, tanpa hostname atau port; nilai tidak valid membuat start gagal (exit 2). Keputusan pemilik produk 3 Nov 2026 (PRD-001 Q26) |
| `ERP_ARGON2_MEMORY_KIB` | | `19456` (19 MiB) | Memori per hash argon2id dalam KiB (parameter hash sandi, bawaan OWASP Password Storage Cheat Sheet). Rentang `19456`–`262144`. Nilai di luar rentang membuat `erpora` gagal start (exit 2). Memori dipakai per hash yang sedang berjalan, sehingga naikkan hanya dengan memperhitungkan RAM. Menaikkan nilai tidak membatalkan hash lama: login berikutnya yang sandinya benar menyimpan ulang hash dengan parameter baru |
| `ERP_ARGON2_TIME` | | `2` | Jumlah iterasi argon2id. Rentang `1`–`10`; nilai `1` hanya diterima bila `ERP_ARGON2_MEMORY_KIB` minimal `47104` (46 MiB, setara OWASP) |
| `ERP_ARGON2_PARALLELISM` | | `1` | Jumlah lane paralel argon2id. Rentang `1`–`16` |
| `ERP_ARGON2_MAX_CONCURRENT` | | `min(jumlah CPU, 4)`, minimal `1` | Batas jumlah hash argon2id yang berjalan serentak dalam satu proses (hash sandi, verifikasi, dan verifikasi tiruan), rentang `1`–`64`. Batas ini menjaga RAM: memori terburuk ≈ batas × memori per hash (±19 MiB per verifikasi pada bawaan, ±76 MiB untuk batas 4). Saat penuh, permintaan login menunggu sebentar lalu dijawab `429 rate_limited` dengan header `Retry-After` |
| `ERP_ENV` | | `production` | `development` mengaktifkan API mutasi modul, deteksi N+1, & log teks |
| `ERP_HTTP_ADDR` | | `127.0.0.1:8080` | Alamat listen. Jangan pakai `0.0.0.0` kecuali ada firewall/proxy |
| `ERP_LOG_FORMAT` | | `json` | `json` atau `text` |
| `ERP_TENANCY_MODE` | | `single` | Satu-satunya nilai valid saat ini |
| `ERP_DEFAULT_TENANT_ID` | | (dibuat otomatis) | UUID tenant default |
| `ERP_AUTHZ_STUB` | | `deny` (`allow` bila `ERP_ENV=development`) | Mode stub otorisasi sampai PRD-001: `allow` atau `deny`. **`allow` ditolak saat `ERP_ENV=production`** (keputusan Q25) |
| `ERP_MODULES_ADMIN_API` | | `false` | `true` mengaktifkan endpoint **mutasi** `POST /api/platform/modules/{key}/{operation}` di luar `development` (selain itu `404`). **Biarkan `false` di production** sampai autentikasi (PRD-001) tersedia; kelola modul lewat CLI. Endpoint baca tetap tunduk pada permission (`ERP_AUTHZ_STUB`) |
| `ERP_DB_NPLUSONE_THRESHOLD` | | `10` | Hanya di `development`: peringatan `indikasi N+1` bila satu request menjalankan lebih dari N query dengan bentuk SQL yang sama. `0` mematikan |
| `ERP_DB_STATEMENT_TIMEOUT_MS` | | `30000` | Batas waktu setiap statement SQL pada koneksi proses `erpora serve` (milidetik, 1000–600000; di luar rentang proses gagal start dengan exit 2). Statement yang melewatinya dibatalkan (SQLSTATE `57014`). Tidak berlaku untuk CLI (`migrate`, `modules`, command modul), sehingga migrasi panjang tidak terputus; operasi lifecycle lewat API admin di dalam `serve` ikut dibatasi |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | | (kosong = telemetry nonaktif) | Endpoint OpenTelemetry Collector |
| `OTEL_SERVICE_NAME` | | `erpora` | |

Simpan konfigurasi di file env dengan izin terbatas, lalu muat dengan `erpora serve --env-file <path>` 🔶.

### 3.1. Binary release, binary dev, dan penanda lingkungan

- **Production selalu memakai binary release** (`task build`, tanpa `-tags devtools`; artifact CI `erpora-<os>-<arch>`). Binary release **tidak memuat** developer tools: `make:*`, `db:fresh`, `db:seed`, `contracts:*`, `module:check`, `doctor`, dan `release:verify-tag` menghasilkan `unknown command`, dan `erpora version` menampilkan `flavor: release`. CI membuktikannya terhadap artifact (langkah smoke) dan lewat `go list -deps`.
- **Jangan pernah memakai binary dev di production** (`task build:dev` atau `go run -tags devtools`). Binary dev menulis banner peringatan ke stderr di setiap command.
- **Penanda lingkungan:** `erpora migrate up` pertama kali menulis `platform_state.environment` = nilai `ERP_ENV` (`development` atau `production`) pada database baru; nilai ini tidak pernah diubah oleh CLI (hanya SQL manual yang disengaja). Binary dev menolak (exit 3) semua command yang menulis (`serve`, `migrate up`, mutasi `modules`, command modul) terhadap database berpenanda `production`, atau terhadap database tanpa penanda bila `ERP_ENV=production`. Penanda tidak bergantung pada host database. `erpora inspect config` menampilkan `database.environment` (membuka koneksi dengan batas 3 detik; `unknown (...)` bila DB tidak terjangkau, tanpa membocorkan DSN).
- Dengan binary release, penanda tidak memengaruhi perilaku apa pun; sebab itu **set `ERP_ENV=production` sebelum `migrate up` pertama** agar database production berpenanda `production`.

---

### 3.2. Rotasi `ERP_SECRET_KEY`

> **Status:** command `core:reseal` (menyegel ulang semua data tersegel dengan kunci baru) **belum ada**; dijadwalkan kelak, bukan bagian C1. Akibatnya **rotasi lengkap belum dapat diselesaikan saat ini**: langkah 1 sampai 3 di bawah aman dijalankan (data lama tetap terbaca lewat `ERP_SECRET_KEY_PREVIOUS`), tetapi langkah 4 dan 5 menunggu command tersebut. Selama itu, pertahankan `ERP_SECRET_KEY_PREVIOUS`.

Data tersegel (kunci privat JWT, secret MFA, secret settings) membawa **id kunci** pada headernya; id itu diturunkan dari material kunci (byte pertama SHA-256), sehingga kunci yang dipindah dari `ERP_SECRET_KEY` ke `ERP_SECRET_KEY_PREVIOUS` tetap dikenali. Kunci di `ERP_SECRET_KEY_PREVIOUS` **hanya dipakai membuka**; segel baru selalu memakai `ERP_SECRET_KEY`.

1. **Cadangkan** backup database terbaru **dan** nilai kunci lama (terpisah satu sama lain, mis. di password manager tim). Jangan menimpa kunci lama sebelum langkah 5 selesai dan terverifikasi.
2. **Buat kunci baru** (32 byte acak, base64): `openssl rand -base64 32`, atau PowerShell (Windows PowerShell 5.1 dan 7+) `$b = New-Object byte[] 32; [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b)`, atau di mesin developer `task erpora -- secret:generate`. Jangan memakai `Get-Random` (bukan CSPRNG).
3. **Pasang di file env, lalu restart `serve`:** salin nilai `ERP_SECRET_KEY` yang lama persis ke `ERP_SECRET_KEY_PREVIOUS`, dan isi `ERP_SECRET_KEY` dengan kunci baru. Setelah restart, data lama tetap terbuka dan data yang baru ditulis tersegel dengan kunci baru. `erpora inspect config` menampilkan kedua variabel sebagai `****` (nilainya tidak pernah dicetak); `erpora doctor` (build pengembang) melaporkan kunci yang tidak valid sebagai FAIL.
4. **Segel ulang semua data dengan kunci baru** memakai `core:reseal`. ⚠️ *Command ini belum ada (lihat status di atas); langkah ini tidak dapat dijalankan sekarang.*
5. **Lepas `ERP_SECRET_KEY_PREVIOUS`** dari file env dan restart, hanya setelah `core:reseal` selesai tanpa galat. ⚠️ **Melepasnya sebelum reseal membuat semua data yang masih tersegel dengan kunci lama tidak terbaca** (kunci privat JWT lama, secret MFA, secret settings); pemulihannya hanya dengan memasang kembali kunci lama di `ERP_SECRET_KEY_PREVIOUS`, sehingga kunci lama harus tetap tersimpan sampai langkah ini terverifikasi.

**Penanganan galat saat startup (nilai kunci tidak pernah dicetak):**

- `ERP_SECRET_KEY_PREVIOUS: id kunci pada header segel bertabrakan dengan ERP_SECRET_KEY`: dua kunci berbeda kebetulan berid sama (peluang 1/256), sehingga header segel tidak dapat membedakannya. **Buat kunci baru lain** (ulangi langkah 2; `secret:generate` atau `openssl` menghasilkan nilai berbeda tiap kali), isi ke `ERP_SECRET_KEY`, dan simpan kunci lama tetap di `ERP_SECRET_KEY_PREVIOUS`.
- `ERP_SECRET_KEY_PREVIOUS: tidak boleh sama dengan ERP_SECRET_KEY`: kunci lama dan baru identik; kosongkan `ERP_SECRET_KEY_PREVIOUS` bila tidak sedang rotasi.
- Format atau panjang salah (bukan base64 standar dengan padding, atau bukan 32 byte): koreksi nilainya; startup (`serve`) dan `erpora doctor` melaporkannya.

### 3.3. URL publik aplikasi (settings platform)

URL publik aplikasi (dipakai untuk tautan reset sandi di email dan pemeriksaan `Origin`) **tidak** diisi lewat env. Nilainya disimpan di tabel `settings` tenant dengan kunci `platform.public_url` (PRD-001 Q24).

- **Production:** `serve` gagal start (exit 2) bila `platform.public_url` belum diisi. Nilainya harus http(s) absolut dengan host, **https wajib**, tanpa userinfo, query, atau fragmen, maksimum 512 karakter.
- **Development:** bila belum diisi, nilai bawaan diturunkan dari `ERP_HTTP_ADDR`.
- **Pengisian awal:** lewat `erpora core:settings:set --key platform.public_url --value <url>` (PRD-001 C1.12; lihat §3.4a). Nilai divalidasi dan dinormalisasi (production menolak `http`), dan perubahan tercatat di log dengan pelaku `cli:core:settings:set`. `serve` yang sedang berjalan membaca nilai baru tanpa restart.

### 3.4a. Command CLI `core:*` (PRD-001 C1.12)

Berlaku untuk binary release maupun dev. Semua command ini **hanya berjalan bila modul `core` enabled** (`erpora migrate up` memasangnya) dan **mewajibkan `ERP_SECRET_KEY` valid** di semua mode (tanpa kunci: exit `2` sebelum database dibuka). Mereka berjalan di bawah hak sistem teraudit `cli:core:<command>` dan mencatat aksinya di log (`identity.admin_created`, `identity.signing_key_rotated`, `identity.mfa_reset`, `identity.account_unlocked`, `identity.public_url_set`).

| Command | Fungsi |
| --- | --- |
| `core:create-admin --email <e> [--name <n>]` | Membuat administrator. **Sandi tidak pernah lewat flag**: dibaca dari prompt tersembunyi (dua kali) bila stdin terminal, atau satu baris dari stdin (`printf '%s\n' "$PW" \| erpora core:create-admin --email ...`). Kebijakan sandi FR-101 berlaku. Bila stdin bukan terminal (mis. mintty/Git Bash di Windows tanpa `winpty`), tidak ada prompt dan command mencetak pemberitahuan ke stderr bahwa sandi dibaca satu baris dan **tidak disembunyikan**; jalankan di terminal biasa (PowerShell, cmd, bash/ssh) atau awali dengan `winpty`. Dua `create-admin` serentak untuk email yang sama diserialkan (kunci per email di database): yang kedua menunggu lalu ditolak karena akun sudah ada. Memanggil ulang dengan email yang sama hanya melanjutkan sisa pembuatan yang terputus (PRD-001 Q28); akun yang sudah aktif atau berrole ditolak. |
| `core:rotate-keys` | Membuat kunci penandatangan JWT baru berstatus `pending` (terpublikasi di JWKS, belum menandatangani) dan melaporkan waktu aktivasinya. Promosi menjadi `active` dilakukan oleh `serve` yang berjalan, atau saat `serve` dimulai bila waktu aktivasi sudah lewat. |
| `core:reset-mfa --email <e>` | Menghapus faktor MFA dan recovery code pengguna serta mencabut seluruh sesinya. Instance `serve` yang berjalan menerima pencabutan sesi lewat muat ulang daftar tercabut (paling lama 1 menit; risiko diterima, PRD-001 Q32 butir 2). |
| `core:unlock --email <e>` | Membuka akun `locked` dan menghapus jeda login pasangannya. |
| `core:settings:set --key platform.public_url --value <url>` | Mengisi URL publik (satu-satunya kunci di C1). |

### 3.4. Retensi `login_attempts`

Tabel `login_attempts` memuat user_id dan alamat IP percobaan login (data pribadi; UU PDP). Proses `serve` menjalankan pekerja latar yang menghapus baris lebih tua dari **30 hari** segera saat start dan lalu tiap **jam** (PRD-001 Q30, ADR-0013 §5). Tidak ada konfigurasi atau cron tambahan; pekerja berhenti bersama proses saat shutdown. Pekerja boleh berjalan di beberapa instance (penghapusan idempoten). Galat pemangkasan dicatat di log (`identity: pemangkasan login_attempts gagal`) dan dicoba lagi pada putaran berikutnya; log tidak memuat data pengguna.

## 4. Instalasi di Linux (systemd)

### 4.1. PostgreSQL

```bash
# Debian/Ubuntu — gunakan repositori resmi PostgreSQL (apt.postgresql.org) untuk versi terbaru
sudo apt install postgresql
sudo -u postgres psql -c "CREATE ROLE erp LOGIN PASSWORD 'GANTI_DENGAN_PASSWORD_KUAT';"
sudo -u postgres psql -c "CREATE DATABASE erp OWNER erp;"
```

### 4.2. User sistem & direktori

```bash
sudo useradd --system --home /opt/erpora --shell /usr/sbin/nologin erpora
sudo mkdir -p /opt/erpora/bin /etc/erpora
sudo cp erpora /opt/erpora/bin/erpora && sudo chmod 755 /opt/erpora/bin/erpora
```

`/etc/erpora/erpora.env` (izin `640`, owner `root:erpora`):

```bash
ERP_DATABASE_URL=postgres://erp:GANTI_DENGAN_PASSWORD_KUAT@localhost:5432/erp?sslmode=disable
ERP_ENV=production
ERP_HTTP_ADDR=127.0.0.1:8080
# Buat sekali: openssl rand -base64 32 (cadangkan terpisah dari backup database)
ERP_SECRET_KEY=GANTI_DENGAN_HASIL_OPENSSL
```

### 4.3. Migrasi & aktivasi modul (sekali, saat instalasi)

```bash
sudo -u erpora /opt/erpora/bin/erpora migrate up --env-file /etc/erpora/erpora.env
sudo -u erpora /opt/erpora/bin/erpora modules install --all --enable --env-file /etc/erpora/erpora.env
sudo -u erpora /opt/erpora/bin/erpora modules list --env-file /etc/erpora/erpora.env

# URL publik aplikasi (wajib di production sebelum serve; lihat §3.3) dan administrator pertama (PRD-001 C1.12);
# sandi diminta secara interaktif (prompt tersembunyi) atau dibaca dari stdin; core:* membutuhkan ERP_SECRET_KEY di env file
sudo -u erpora /opt/erpora/bin/erpora core:settings:set --key platform.public_url --value https://erp.example.com --env-file /etc/erpora/erpora.env
sudo -u erpora /opt/erpora/bin/erpora core:create-admin --email admin@example.com --env-file /etc/erpora/erpora.env
```

Catatan operasional CLI `erpora modules`:

- **Exit code**: `0` sukses atau tidak ada perubahan; `2` validasi (argumen, key/operasi tidak dikenal, `--tenant` bukan tenant default, konfirmasi `uninstall` hilang: pakai `--yes` di skrip); `3` konflik atau prasyarat tidak terpenuhi (`invalid_transition`, `dependents_active`, `module_required`, `operation_in_progress`, `module_incompatible`); `4` operasi gagal (lihat `modules status <key>`); `1` lainnya.
- Perintah idempoten: operasi yang status targetnya sudah tercapai mencetak `tidak ada perubahan` dan exit `0`, sehingga `modules install --all --enable` aman diulang.
- `erpora migrate up` juga meng-install dan meng-enable modul `required` (beserta dependensinya) dengan actor `system:migrate`.
- `--all` melewati modul yang transisinya tidak sah dan menolak seluruh rencana bila ada prasyarat yang gagal (exit `3`); pakai `modules plan <operasi> --all` untuk dry-run.
- **Propagasi**: `serve` memeriksa revision status modul setiap 2 detik, jadi perubahan dari CLI terlihat di proses `serve` yang sedang berjalan dalam ≤ 2 detik tanpa restart.
- Operasi yang gagal atau terputus dilanjutkan dengan `erpora modules retry <operation-id>` (ID ada di `modules status <key>`).
- API mutasi (bila diaktifkan) menunggu operasi sampai 30 detik; operasi yang lebih lama dijawab `202` dengan header `Location: /api/platform/modules/operations/{id}` untuk dipantau.

Saat login pertama, Administrator wajib mendaftarkan MFA (TOTP). Simpan recovery code di tempat aman. Bila Administrator terakhir kehilangan akses MFA, jalankan `erpora core:reset-mfa` di server.

Pengaturan SMTP untuk email reset password untuk sementara diisi lewat env `ERP_MAIL_DRIVER` dan `ERP_SMTP_*` (lihat §3; PRD-001 K-C1.6). Pemindahannya ke menu **Settings → Platform → Email** dijadwalkan pada PRD-001 C3 tanpa mengubah `sdk/mail`.

### 4.4. Service systemd

`/etc/systemd/system/erpora.service`:

```ini
[Unit]
Description=ERPora
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=erpora
Group=erpora
ExecStart=/opt/erpora/bin/erpora serve --env-file /etc/erpora/erpora.env
Restart=on-failure
RestartSec=5
# Memberi waktu graceful shutdown (FR-03)
TimeoutStopSec=30
# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadOnlyPaths=/opt/erpora

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now erpora
sudo systemctl status erpora
curl -fsS http://127.0.0.1:8080/readyz
```

### 4.5. Reverse proxy + TLS (Caddy)

`/etc/caddy/Caddyfile`:

```
erpora.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080
}
```

Caddy mengurus sertifikat TLS (Let's Encrypt) secara otomatis. Nginx juga bisa dipakai, tetapi sertifikatnya harus dikelola sendiri (mis. certbot).

---

## 5. Instalasi di Windows

1. Pasang PostgreSQL untuk Windows (installer resmi), lalu buat role & database seperti §4.1 melalui `psql`.
2. Letakkan `erpora.exe` di `C:\erpora\bin\` dan file env di `C:\erpora\erpora.env` (batasi izin akses ke Administrators dan akun service). Isi `ERP_SECRET_KEY` di file env itu (wajib untuk `serve`); buat sekali di PowerShell:
   ```powershell
   $b = New-Object byte[] 32; [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b); [Convert]::ToBase64String($b)
   ```
3. Migrasi & modul (PowerShell sebagai Administrator):
   ```powershell
   C:\erpora\bin\erpora.exe migrate up --env-file C:\erpora\erpora.env
   C:\erpora\bin\erpora.exe modules install --all --enable --env-file C:\erpora\erpora.env
   ```
4. Daftarkan sebagai Windows service memakai pembungkus seperti **WinSW** atau **NSSM** 🔶 (dukungan service native di binary belum termasuk PRD-000). Contoh dengan NSSM:
   ```powershell
   nssm install erpora C:\erpora\bin\erpora.exe serve --env-file C:\erpora\erpora.env
   nssm set erpora AppStopMethodConsole 30000
   nssm start erpora
   ```
5. Gunakan Caddy untuk Windows atau IIS (ARR + URL Rewrite) sebagai reverse proxy + TLS.

---

## 6. Instalasi dengan Docker

Berkas `erpora.env` untuk container wajib memuat `ERP_SECRET_KEY` (buat dengan `openssl rand -base64 32`); tanpanya `serve` berhenti dengan exit 2.

```bash
docker run -d --name erpora \
  --env-file ./erpora.env \
  -p 127.0.0.1:8080:8080 \
  -e ERP_HTTP_ADDR=0.0.0.0:8080 \
  ghcr.io/rahmadmrhd/erpora:<versi>        # 🔶 lokasi registry final ditentukan saat rilis

docker exec erpora erpora migrate up
docker exec erpora erpora modules install --all --enable
```

Di dalam container, `ERP_HTTP_ADDR` harus `0.0.0.0:8080` agar bisa dijangkau. Pembatasan aksesnya dilakukan di port mapping (`127.0.0.1:8080:8080`).

---

## 7. Upgrade

Urutannya penting. **Migrasi tidak pernah berjalan otomatis** saat `serve` (PRD-000 FR-07).

1. **Backup database** (§8), lalu pastikan file backup bisa dibaca.
2. Unduh binary versi baru dan cek checksum-nya.
3. Hentikan service: `sudo systemctl stop erpora`.
4. Ganti binary (simpan binary lama sebagai `erpora.prev` untuk rollback).
5. Jalankan migrasi platform: `erpora migrate up --env-file ...`
6. Lihat modul yang perlu di-upgrade, lalu jalankan upgradenya:
   ```bash
   erpora modules list --env-file /etc/erpora/erpora.env          # cari status upgrade_pending
   erpora modules plan upgrade --all --env-file /etc/erpora/erpora.env   # dry-run
   erpora modules upgrade --all --env-file /etc/erpora/erpora.env
   ```
7. Jalankan service: `sudo systemctl start erpora`, lalu cek `/readyz` dan `erpora modules list`.

Selama langkah 6 belum dijalankan, modul berstatus `upgrade_pending` dan route-nya mengembalikan `503`. Ini disengaja, agar kode baru tidak berjalan di atas skema lama.

### 7.1. Rollback

- **Sebelum** `migrate up` / `modules upgrade`: cukup kembalikan binary lama.
- **Sesudah** migrasi: binary lama akan menolak modul yang versinya lebih baru (`downgrade_not_supported`). Rollback berarti **restore database dari backup langkah 1** dan memakai binary lama. Karena itu backup sebelum upgrade wajib dilakukan.
- Operasi modul yang gagal di tengah jalan bisa dilanjutkan dengan `erpora modules retry <operation-id>` setelah penyebabnya diperbaiki.

---

## 8. Backup & restore (dasar)

```bash
# Backup (format custom, terkompresi)
pg_dump --format=custom --file=erpora-$(date +%F).dump "$ERP_DATABASE_URL"

# Restore ke database kosong
pg_restore --clean --if-exists --no-owner --dbname="$ERP_DATABASE_URL" erpora-2026-10-21.dump
```

- Jadwalkan backup harian (cron / systemd timer / Task Scheduler) dan simpan **di luar server** aplikasi.
- Uji restore secara berkala. Backup yang belum pernah di-restore belum terbukti bisa dipakai.
- Prosedur backup per tenant dan point-in-time recovery dibahas di dokumen operasi lanjutan (PRD Tenancy).

---

## 9. Health check & monitoring

| Endpoint | Fungsi | Dipakai oleh |
| --- | --- | --- |
| `GET /healthz` | Proses hidup | Restart otomatis / liveness probe |
| `GET /readyz` | Database terhubung & migrasi platform sudah diterapkan | Load balancer / readiness probe |

- Log aplikasi: `journalctl -u erpora -f` (Linux) atau log NSSM/WinSW (Windows).
- Telemetry: isi `OTEL_EXPORTER_OTLP_ENDPOINT` untuk mengirim trace/metric ke OpenTelemetry Collector (`infrastructure/monitoring/otel-collector.yaml`).

---

## 10. Checklist hardening

- [ ] `erpora` listen di `127.0.0.1` dan hanya dapat diakses lewat reverse proxy ber-TLS.
- [ ] Firewall hanya membuka 22 (atau port SSH lain), 80, dan 443.
- [ ] PostgreSQL tidak listen di interface publik (`listen_addresses = 'localhost'`), kecuali memakai managed DB dengan TLS.
- [ ] File env berizin `640` dan tidak berada di repositori.
- [ ] `ERP_ENV=production` dan `ERP_MODULES_ADMIN_API=false`.
- [ ] Service berjalan sebagai user non-root (`erpora`).
- [ ] Backup harian aktif dan restore sudah diuji.
- [ ] `ERP_SECRET_KEY` dicadangkan di tempat terpisah dari backup database (mis. password manager tim).
- [ ] Binary diverifikasi dengan checksum sebelum dipasang.
- [ ] `erpora version` menampilkan flavor **`release`**. Binary flavor `dev` (berisi `make:*`, `db:fresh`) tidak boleh dipasang di server production.
- [ ] Database production dibuat dengan `ERP_ENV=production` saat `erpora migrate up` pertama, sehingga penanda lingkungannya `production`. Cek dengan `erpora inspect config`.
- [ ] Repositori GitHub: branch protection pada `main` mewajibkan pull request dan status check **`contracts`** (R-10, 28 Oktober 2026). Gate breaking CI menerima breaking change pada event `push` ke `main` (`ERP_API_BREAKING_APPROVED=true`) dengan asumsi perubahan itu sudah melewati PR yang statusnya `contracts` hijau dan label `api-breaking-approved`; tanpa branch protection, push langsung ke `main` melewati gate ini. Asumsi ini tidak dapat diverifikasi dari repositori dan harus dicek di pengaturan GitHub.

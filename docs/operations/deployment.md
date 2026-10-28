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
| `ERP_SECRET_KEY` | ✅ (sejak PRD-001) | — | 🔶 Belum tersedia (menunggu PRD-001). 32 byte acak (base64) untuk mengenkripsi kunci JWT, secret settings, dan secret MFA. **Simpan cadangannya terpisah dari backup database**: tanpa kunci ini, data terenkripsi di backup tidak dapat dibaca. Buat dengan `openssl rand -base64 32` |
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

---

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
```

### 4.3. Migrasi & aktivasi modul (sekali, saat instalasi)

```bash
sudo -u erpora /opt/erpora/bin/erpora migrate up --env-file /etc/erpora/erpora.env
sudo -u erpora /opt/erpora/bin/erpora modules install --all --enable --env-file /etc/erpora/erpora.env
sudo -u erpora /opt/erpora/bin/erpora modules list --env-file /etc/erpora/erpora.env

# Administrator pertama (sejak PRD-001); password diminta secara interaktif
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

Pengaturan SMTP untuk email reset password diisi lewat menu **Settings → Platform → Email** setelah login (bukan env).

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
2. Letakkan `erpora.exe` di `C:\erpora\bin\` dan file env di `C:\erpora\erpora.env` (batasi izin akses ke Administrators dan akun service).
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

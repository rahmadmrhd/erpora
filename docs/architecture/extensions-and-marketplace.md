# Ekstensi & Marketplace

| Atribut | Nilai |
| --- | --- |
| Status | **Draft desain** (diekstrak dari dokumen awal `Blueprint.md` §8 dan `arsitektur.md` §8). Akan diformalkan di **PRD Marketplace** (fase 06) dan ADR-008 |
| Terkait | PRD-000 FR-06 (field marketplace opsional, `trust`), FR-19 (status `verified`), §8 (kandidat wazero) |

## 1. Build-time modules vs runtime plugins

| Kemampuan | Build-time modules (sekarang) | Runtime plugins (nanti) |
| --- | --- | --- |
| Aktif/nonaktif per tenant | Ya | Ya |
| Lazy loading UI | Ya | Ya |
| Registrasi route | Saat build | Butuh mekanisme runtime |
| Tambah modul tanpa rebuild frontend | Tidak | Bisa |
| Tambah kode backend tanpa rebuild Go | Tidak | Butuh proses/layanan plugin terpisah |
| Type safety | Lebih mudah | Lebih kompleks |
| Keamanan & versioning | Lebih sederhana | Kontrol lebih ketat |

Versi pertama: **build-time modules + runtime activation**. Binary Go tidak bisa memuat modul Go baru sebagai kode native lintas platform; marketplace tanpa rebuild butuh kontrak ekstensi tersendiri.

## 2. Dua kelas ekstensi

- **Class A — Trusted in-process modules.** Modul internal dan publisher yang sudah melalui persetujuan. Berjalan dalam proses aplikasi dan diperlakukan sebagai kode tepercaya (contoh: Inventory, Purchasing, laporan khusus yang disetujui).
- **Class B — Isolated extensions.** Ekstensi publik yang belum tepercaya dijalankan **di luar proses utama** (worker/service terisolasi) dengan API terbatas (contoh: integrasi eksternal, otomasi kustom, transformasi dokumen pihak ketiga). Kandidat implementasi: WebAssembly via `wazero` atau proses terpisah via gRPC (ADR-008).

Bila siapa pun boleh mengunggah kode yang berjalan di server ERP, **sandboxing bukan fitur opsional**. Tahap awal: format paket dan SDK publik dibangun sejak awal, tetapi eksekusi dibatasi pada modul bawaan dan publisher tepercaya. Marketplace terbuka diluncurkan setelah model keamanan dan isolasinya matang.

## 3. Pipeline distribusi modul

```
Publisher submit paket
  → validasi manifest
  → scan dependency & lisensi
  → static analysis & test
  → security review
  → build artifact immutable
  → sign & publish
  → customer meminta instalasi
  → verifikasi signature & kompatibilitas
  → install di environment yang disetujui
  → health check & audit
```

## 4. Persyaratan minimum marketplace

- Identitas publisher terverifikasi. Nilai `publisher.id` di manifest **bukan** bukti kepercayaan.
- Paket immutable dengan checksum.
- Tanda tangan digital diverifikasi dengan public key tepercaya.
- Versi dan dependency constraint divalidasi.
- Hak akses yang diminta ditampilkan sebelum instalasi.
- Review keamanan dan pemindaian dependency.
- Kebijakan update, penghentian dukungan, dan pencabutan paket.
- Audit log untuk instalasi, aktivasi, upgrade, dan uninstall.
- Prosedur penanganan vulnerability dan distribusi patch darurat.

Modul publik tidak boleh mengubah core framework atau mengakses secret seluruh tenant tanpa batas. Gunakan capability-based access, API yang sempit, dan identitas workload terpisah untuk ekstensi terisolasi.

## 5. Kandidat format paket

Arsip (`tar.zst`) + manifest + checksum SHA-256 + signature Ed25519 (atau Sigstore). Field-nya sudah dicadangkan di manifest (PRD-000 FR-06): `artifact`, `checksum`, `signature`, `requested_permissions`, `license`.

## 6. Risiko kritis

**Ekstensi publik berjalan dengan privilege aplikasi utama.** Manifest dan interface tidak cukup; tanpa trust boundary yang benar, modul pihak ketiga dapat mengakses data atau secret yang seharusnya tidak terjangkau.

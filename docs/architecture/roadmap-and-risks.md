# Roadmap, Kualitas, dan Risiko Platform

| Atribut | Nilai |
| --- | --- |
| Status | **Draft desain** (diekstrak dari dokumen awal `Blueprint.md` §3.3, §11.2, §12–§17 dan `arsitektur.md` §9). Bagian yang sudah tercakup PRD ditandai |
| Terkait | PRD-000 (fase 01 + 03), PRD-001 (fase 02 tanpa tenancy), `docs/adr/` |

## 1. Roadmap fase

| Fase | Isi | Exit gate | Status |
| --- | --- | --- | --- |
| 01 Architecture foundation | ADR, database engine, threat model, struktur monorepo, CI, manifest & SDK awal | Keputusan arsitektur dapat diuji dan diterjemahkan ke kode | **PRD-000** |
| 02 Core platform & tenancy | Control DB, tenant registry, provisioning, secret management, auth, membership, RBAC, audit, tenant resolver, isolation test, backup per tenant | Tenant dapat dibuat & dipakai tanpa akses silang | Core: **PRD-001**; tenancy: PRD Tenancy (lihat `tenancy-design.md`) |
| 03 Module Registry & lifecycle | Manifest parser, dependency resolver, install/enable/disable/upgrade, capability, settings modul, orkestrasi migrasi, audit | Modul contoh aktif/nonaktif aman tanpa hapus data | **PRD-000** |
| 04 UI platform & vertical slice | App shell, design system, navigation registry, Item, Warehouse, Purchase Order, Goods Receipt, stock ledger, E2E satu alur | Satu alur bisnis lengkap dapat dijalankan, diuji, diaudit | Belum |
| 05 Domain expansion | Sales & Purchasing lengkap, Finance (posting jurnal, payment, tax), Assets, outbox/retry/idempotensi, integrasi | Alur lintas modul memenuhi invariant domain | Belum |
| 06 Hybrid release & marketplace beta | Container image, deployment profile, installer & upgrade self-hosted, signed package, publisher verification, isolated execution, release channel & rollback | SaaS dan self-hosted memakai release yang sama; lifecycle paket teraudit | Belum (lihat `extensions-and-marketplace.md`) |
| 07 Production hardening & scaling | Load test, query profiling, horizontal scaling, worker, observability per tenant, backup drill, DR, security review | Target performa, availability, recovery, biaya terukur | Belum |

### 1.1. PRD-002 Document Framework — keputusan awal (22 Okt 2026)

Urutan: **PRD-000 → PRD-001 Core → PRD-002 Document Framework → modul bisnis pertama.** Kapabilitas bersama harus ada sebelum modul bisnis agar tidak ada modul yang menulis versinya sendiri lalu di-refactor.

Tujuannya meniru model ERPNext/Frappe: **setiap resource yang dideklarasikan modul otomatis mendapat kapabilitas standar** (approval, penomoran, lampiran, komentar/timeline, cetak, email, saved view, custom field, impor/ekspor, notifikasi) tanpa modul harus memasangnya sendiri. Pemasangan otomatis itu dilakukan berdasarkan manifest yang dibaca registry, sehingga **mesinnya berada di platform** (pertanyaan penentu: platform perlu memanggilnya).

| Lapisan | Isi |
| --- | --- |
| **Platform** (`internal/platform/doc*` + `sdk/doc`) | **Pipeline aksi dokumen**, approval engine, numbering series, attachment, comment & activity timeline, rendering print & email, saved view, custom field & tag, framework impor/ekspor, notifikasi. **Platform memiliki tabel datanya** |
| **Modul Core** (`modules/core/web` + use case admin) | **Layar administrasi**: konfigurasi skema approval & kotak masuk approval, editor email/print template, seri penomoran, custom field, pengaturan notifikasi. Memakai API admin dari SDK (pola sama dengan PRD-001 §3) |
| **Modul bisnis** | Mendeklarasikan resource, aksi, dan kapabilitas di manifest; mengisi **hook** domain (`Validate`, `Apply`, data untuk print/email). Tidak membuat ulang kapabilitas standar dan **tidak perlu** bergantung pada `erp.core` untuk memakainya |

**Pipeline aksi dokumen** (dijalankan platform, sama untuk semua modul; *inversion of control* ala Frappe `validate`/`on_submit`):

```
POST /api/{slug}/{koleksi}/{id}/{aksi}
  authz (permission + scope + field)        ← PRD-001
  → muat & kunci dokumen, cek status & version
  → hook modul: Validate()                   ← logika bisnis modul
  → approval gate (skema dari Core)          → bila perlu: pending_approval, berhenti
  → numbering series (saat submit)
  → hook modul: Apply()                      ← efek domain (ledger, dsb.)
  → audit + timeline + outbox + notifikasi
  → commit (satu transaksi, transaction-data-standards §1)
```

**Endpoint otomatis per resource** (mengikuti permission & scope resource, api-guidelines): `/attachments`, `/comments`, `/timeline`, `/print`, saved views, custom field, impor/ekspor.

**Batas desain** (menghindari risiko "module system terlalu generik"): platform mengatur **urutan dan kewajiban** (otorisasi → approval → nomor → audit), sedangkan **isi aturan bisnis tetap di hook modul**. Proses yang tidak berbentuk dokumen (mis. penutupan periode, rekonsiliasi) boleh memakai endpoint biasa lewat `Registrar.Handle`.

Kandidat cakupan (difinalisasi saat PRD-002 ditulis):

| Kapabilitas | Isi |
| --- | --- |
| **Approval Workflow** | Rincian di tabel "Approval Workflow" di bawah |
| **Email Template** | Template per event/resource, multi-bahasa, variabel dari kontrak resource, pratinjau; dikirim lewat mailer (PRD-001 FR-108) |
| **Print Template** | Template cetak/PDF per resource (faktur, PO, slip), kop & tanda tangan per company, permission `print`, ter-versi |
| **Saved Filter / Saved View** | Filter, kolom, dan urutan tersimpan per user, dapat dibagikan per role, untuk data grid setiap resource |
| **Document Numbering Series** | Seri penomoran per company/tipe dokumen/periode, gapless bila diwajibkan (transaction-data-standards §5) |
| **Attachment** | Lampiran file per record lewat pipeline upload aman (security-guidelines §3.7), mengikuti permission record |
| **Activity Timeline & Comment** | Riwayat aktivitas, komentar, dan mention per record |
| **Notification** | Notifikasi in-app + email per event (dasar untuk push notification di kemudian hari) |
| **Import / Export framework** | Impor dengan dry-run dan ekspor aman (`sdk/export`) yang seragam untuk semua resource |
| **Tag / Custom Field** | Penanda bebas dan field tambahan per resource yang dikonfigurasi admin |

#### Approval Workflow

| Keputusan | Isi |
| --- | --- |
| Sifat | **Wajib di semua modul.** Setiap aksi dokumen melewati approval gate di pipeline; ada atau tidaknya tahap persetujuan ditentukan skema (tanpa skema aktif = otomatis lolos) |
| Lokasi | **Mesin di platform** (approval gate di pipeline aksi dokumen, `sdk/doc`); **layar konfigurasi skema & kotak masuk approval di modul Core** |
| Skema approval (diatur di Core) | Per resource + aksi (`submit`, `post`, `cancel`, …): tingkat bertahap (sekuensial/paralel), penentuan approver (role, unit organisasi, user tertentu, atasan pembuat), kondisi/batas nilai (mis. total > Rp 10 juta → tambah level), maker-checker, delegasi saat berhalangan, eskalasi/batas waktu. Perubahan skema tercatat di audit dan **di-snapshot** ke permintaan approval yang sedang berjalan |
| Deklarasi di manifest | Resource dengan aksi dokumen (`submit`/`post`/`cancel`/`amend`) mendeklarasikan aksinya; platform otomatis memasang approval gate pada aksi tersebut. Registry menolak aksi dokumen yang didaftarkan di luar pipeline (mis. lewat `Registrar.Handle` biasa) |
| Penegakan | **Struktural**: aksi dokumen hanya bisa dijalankan lewat pipeline platform, sehingga approval tidak mungkin terlewat. Template test: aksi dengan skema aktif tidak boleh langsung `submitted`; audit mencatat setiap keputusan |
| Status dokumen | Tambahan `pending_approval`; `reject` mengembalikan ke `draft` dengan alasan (transaction-data-standards §2) |
| Bergantung pada PRD-001 | Principal, role & matriks (aksi `approve`), unit organisasi (approver per cabang/departemen), audit, mailer, settings |

### Urutan modul bisnis

| # | Modul | Alasan |
| --- | --- | --- |
| 1 | Core + Settings | Identitas, konfigurasi, permission, lifecycle (**PRD-001**) |
| 2 | Master Data + Inventory | Menguji kepemilikan data, ledger, transaksi |
| 3 | Purchasing | Menguji integrasi dokumen & penerimaan barang |
| 4 | Sales | Menguji reservasi stok, pengiriman, invoice |
| 5 | Finance | Menguji posting jurnal & konsistensi lintas dokumen |
| 6 | Assets | Menguji depresiasi & integrasi akuntansi |
| 7 | Marketplace extensions | Menguji SDK & kontrak eksternal |

Finance bukan modul CRUD: sebelum posting jurnal, tentukan aturan pembukuan, multi-currency, pajak, reversal, periode akuntansi, dan sumber kebenaran saldo.

Rekomendasi awal: bangun Core dan Inventory sebagai **reference modules**; generalisasi kontrak registry, permission, routing, dan offline policy setelah keduanya bekerja baik.

## 2. Entitas domain ERP (konseptual, untuk PRD modul bisnis)

| Area | Entitas utama |
| --- | --- |
| Organisasi | `companies` (nama, mata uang dasar) |
| Inventory | `warehouses` (kode unik), `items` (SKU unik, satuan stok), `stock_movements` (item, gudang, kuantitas desimal, sumber dokumen, waktu posting) |
| Purchasing | `suppliers`, `purchase_orders` (nomor unik, status, mata uang, total), `purchase_order_lines` (item, kuantitas, harga), `goods_receipts` (status), `goods_receipt_lines` (kuantitas diterima) |
| Finance | `accounts` (kode unik, tipe), `journal_entries` (nomor unik, tanggal posting, status), `journal_lines` (akun, debit, kredit) |

Implementasi produksi menambahkan audit trail, referensi dokumen sumber, konversi satuan, dimensi gudang, presisi mata uang, detail pajak, periode akuntansi, entri reversal, dan aturan posting.

## 3. Fondasi UX sebelum memperluas modul bisnis

- Navigation registry dan command palette.
- Data grid dengan filter, sorting, pagination, dan saved views.
- Komponen form dengan validasi konsisten.
- Relation selector dengan pencarian lintas layar sesuai otorisasi.
- Document status & workflow action.
- Activity timeline & audit history.
- Settings schema untuk konfigurasi tenant.
- Aksesibilitas, navigasi keyboard, layout responsif.

Jangan membuat semua halaman generik berbasis metadata sejak awal: UI metadata-driven cocok untuk konfigurasi & CRUD standar, tetapi proses seperti posting jurnal, reservasi stok, atau penutupan periode butuh interaksi dan validasi eksplisit.

## 4. Strategi testing

| Jenis | Fokus |
| --- | --- |
| Unit | Aturan domain, pajak, kurs, pembulatan, status dokumen, validasi transaksi |
| Integration | Repository, transaksi database, migrasi, locking, outbox, tenant resolver |
| Contract & architecture | Kompatibilitas SDK, larangan dependensi antarmodul, public API, versi manifest |
| Security | Isolasi tenant, otorisasi, integritas paket, privilege escalation, penanganan secret |
| End-to-end | Alur pengguna dari pembuatan dokumen hingga posting, dengan browser & DB realistis |

Skenario wajib Module System (sebagian besar sudah menjadi AC PRD-000 FR-19): aktif/nonaktif, dependency hilang, manifest tidak valid, versi tidak kompatibel, migrasi gagal, job lama setelah disable, signature tidak valid, akses silang tenant, dua transaksi stok bersamaan.

## 5. Versi, migrasi, dan rilis hybrid

Tiga versi yang dipisahkan: **platform version** (core + SDK + kontrak publik), **module version**, **schema version** (struktur data tenant). Ketiganya tidak selalu berubah bersamaan.

Aturan migrasi: versioned & dapat diaudit; tidak dijalankan otomatis per request; dijalankan orchestrator dengan status persisten; perubahan berisiko memakai **expand-and-contract**; backup & restore diuji sebelum upgrade berisiko; upgrade tenant bertahap dengan retry; versi core, modul, dan skema setiap tenant dapat diketahui.

Rilis hybrid: **artifact yang sama** untuk SaaS dan self-hosted; perbedaan hanya di deployment profile & konfigurasi.

- SaaS: tenant registry terpusat, provisioning otomatis, upgrade bertahap (canary), observability & backup terpusat.
- Self-hosted: artifact/container yang dapat diverifikasi, konfigurasi & secret lokal, perintah install/migrate/upgrade/backup/restore terdokumentasi, health check sebelum menerima traffic, lisensi/update yang tetap berfungsi pada konektivitas terbatas (bila dibutuhkan produk).

## 6. Strategi scaling

Jangan menetapkan ambang scaling dari jumlah pengguna; tetapkan SLO dan ukur bottleneck nyata.

| Tahap | Isi |
| --- | --- |
| A — Single deployment | Satu deployment, database per tenant, fokus konsistensi domain, kualitas query, backup |
| B — Horizontal application scaling | Beberapa instance, load balancer, worker, pengelolaan koneksi terkontrol, tenant context aman di setiap instance |
| C — Data & workload scaling | Optimasi query, indeks, pooling, read replica, reporting database, pemisahan tenant besar |
| D — Selective service extraction | Pisahkan rendering dokumen, integration gateway, atau reporting bila scaling/isolasi/deployment independen membenarkannya |

## 7. ADR yang harus dibuat

| ADR | Keputusan |
| --- | --- |
| 001 | Modular Monolith sebagai struktur awal |
| 002 | Clean Architecture & aturan dependensi modul |
| 003 | Database engine utama |
| 004 | Database-per-tenant & tenant resolution |
| 005 | Control Plane vs ERP Runtime |
| 006 | Module manifest, SDK, semantic versioning |
| 007 | Semantik install/enable/disable/uninstall |
| 008 | Extension trust model & sandboxing |
| 009 | Domain event, outbox, idempotensi |
| 010 | Migrasi database & kompatibilitas upgrade |
| 011 | Kontrak UI extension & design system |
| 012 | Strategi rilis SaaS/self-hosted |
| 013 | Autentikasi, otorisasi, audit |
| 014 | Observability, SLO, backup, recovery |

Setiap ADR memuat konteks masalah, opsi, keputusan, konsekuensi, dan kondisi pemicu evaluasi ulang.

## 8. Risiko arsitektur terbesar

| Tingkat | Risiko |
| --- | --- |
| Kritis | Ekstensi publik berjalan dengan privilege aplikasi utama |
| Kritis | Kebocoran tenant context (worker, queue, cache, file storage, koneksi DB yang tidak dibersihkan) |
| Mayor | Module system terlalu generik (aturan domain dipaksa masuk manifest/CRUD engine) |
| Mayor | Upgrade tenant tidak terkendali (migrasi gagal di tengah, versi tidak kompatibel, tanpa recovery plan) |
| Menengah | Premature microservices |

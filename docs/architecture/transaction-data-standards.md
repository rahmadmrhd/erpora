# Standar Transaksi & Integritas Data

| Atribut | Nilai |
| --- | --- |
| Status | **Draft v1 — menunggu review** (21 Oktober 2026) |
| Berlaku untuk | Setiap resource yang membuat, mengubah, atau memposting data bisnis (dokumen, ledger, master data) di modul mana pun |
| Dirujuk oleh | `CLAUDE.md`, Kiro spec (`design.md`), PRD modul bisnis |
| Terkait | `api-guidelines.md` (§4, §7, §9, §11), `security-guidelines.md` (§3.9), PRD-001 (scope, audit) |

Kata **WAJIB**, **DILARANG**, dan **SEBAIKNYA** bersifat normatif.

## 0. Standar acuan

| Standar | Dipakai untuk |
| --- | --- |
| **ISO 8601** | Format tanggal & waktu (api-guidelines §4.1) |
| **ISO 4217** | Kode mata uang **dan jumlah digit minor unit** (IDR 2, USD 2, JPY 0, KWD 3) |
| **ISO 3166-1** | Kode negara (alpha-2) |
| **ISO 639-1** | Kode bahasa (preferensi user, dokumen) |
| **ISO 15489-1:2016** | Pengelolaan rekaman (records): keaslian, keandalan, integritas, keterpakaian, retensi |
| **ISO/IEC 27002:2022** | `5.3` pemisahan tugas, `5.33` perlindungan rekaman, `8.15` logging, `8.17` sinkronisasi waktu, `8.10` penghapusan informasi |
| **ISO 8000 & ISO/IEC 25012** | Kualitas data (akurasi, kelengkapan, konsistensi, keterkinian) untuk master data |
| **ISO 20022** | Format pesan keuangan untuk integrasi bank/pembayaran (bila fitur itu dibangun) |
| **SAK/PSAK** dan peraturan perpajakan Indonesia | Aturan akuntansi & kewajiban dokumen. Bukan ISO, tetapi mengikat modul keuangan dan pajak |

> Kewajiban hukum (retensi, penomoran faktur pajak, dsb.) **WAJIB** dikonfirmasi dengan konsultan pajak/hukum sebelum modul terkait dirilis. Nilai di dokumen ini adalah titik awal desain.

## 1. Prinsip integritas

1. **Rekaman sah tidak diubah, hanya dikoreksi.** Dokumen yang sudah `submitted`/`posted` tidak pernah di-`UPDATE` pada field bisnisnya dan tidak pernah di-`DELETE`. Koreksi dilakukan dengan `cancel` + `amend`, atau dokumen pembalik (*reversal*).
2. **Satu transaksi bisnis = satu transaksi database.** Perubahan data, entri ledger, nomor dokumen, catatan idempotensi, event outbox, dan audit log ditulis bersama atau tidak sama sekali.
3. **Server adalah sumber kebenaran angka.** Total, pajak, diskon, kurs, dan saldo dihitung server.
4. **Setiap invariant domain ditegakkan dua kali**: di kode domain **dan** di database (constraint/trigger) untuk invariant kritis.
5. **Jejak lengkap**: siapa, kapan, apa, dari mana (dokumen sumber), mengapa (alasan cancel/amend).

## 2. Siklus hidup dokumen

```
draft ──submit──▶ [pending_approval ──approve──▶] submitted ──post──▶ posted
                         └──reject──▶ draft (dengan alasan)
(pending_approval hanya terjadi bila skema approval aktif untuk resource/aksi tersebut — §9;
 post opsional, untuk dokumen yang menghasilkan ledger)
  │                  │                   │
  │ PATCH/DELETE     └──cancel──▶ cancelled ──amend──▶ draft baru (amendedFromId)
  │                                      ▲
  └ hanya di draft                       └── posted: cancel = membuat dokumen/entri pembalik, bukan menghapus
```

| Aturan | Penegak |
| --- | --- |
| Field bisnis hanya dapat diubah di `draft` | Domain + **trigger DB** menolak `UPDATE` field bisnis bila status ≠ `draft` |
| `DELETE` hanya untuk `draft` yang belum pernah di-submit | Domain + trigger |
| `cancel` dan `amend` **wajib** menyertakan alasan (disimpan & diaudit) | Validasi DTO |
| Dokumen `posted` dibatalkan dengan entri pembalik bertanggal posting pembatalan (bukan menghapus entri lama) | Domain + test |
| Transisi status diperiksa di dalam transaksi dengan record terkunci (`SELECT … FOR UPDATE`) | Domain + test konkurensi |

## 3. Ledger (stok, jurnal, kartu piutang/hutang)

- Tabel ledger (`stock_movements`, `journal_lines`, …) **append-only**: aplikasi tidak punya izin `UPDATE`/`DELETE` (dipaksa trigger dan grant DB).
- Setiap baris ledger menyimpan **referensi dokumen sumber** (`source_type`, `source_id`, `source_line_id`) yang unik, sehingga satu dokumen tidak bisa terposting dua kali.
- **Jurnal double-entry**: per `journal_entry`, Σ debit = Σ kredit dalam mata uang dasar **dan** mata uang transaksi. Ditegakkan oleh constraint trigger `DEFERRABLE INITIALLY DEFERRED` yang memeriksa saat commit.
- Saldo (stok per gudang, saldo akun) adalah **proyeksi** dari ledger. Proyeksi boleh di-cache di tabel saldo, tetapi diperbarui dalam transaksi yang sama dan **direkonsiliasi** berkala (§10).
- Kebijakan stok negatif dideklarasikan per modul/gudang (`allow_negative_stock`); bila dilarang, diperiksa dengan baris saldo terkunci.

## 4. Konkurensi & idempotensi

| Situasi | Mekanisme |
| --- | --- |
| Dua user mengedit draft yang sama | Optimistic: `version` + `If-Match` → `412` (api-guidelines §7.1) |
| Dua transaksi mengubah saldo yang sama | Pessimistic: kunci baris saldo/counter dengan urutan kunci yang **konsisten** (urut ID) untuk mencegah deadlock |
| Retry jaringan, double-click, sync offline | `Idempotency-Key` (api-guidelines §7.2) + unique constraint pada referensi sumber |
| Proses latar memposting dokumen | Klaim kerja dengan `SELECT … FOR UPDATE SKIP LOCKED`; status diperiksa ulang setelah kunci didapat |

Isolation level default `READ COMMITTED`; operasi yang membaca lalu menulis berdasarkan agregat (mis. cek limit kredit) memakai kunci eksplisit atau `SERIALIZABLE` dengan retry otomatis untuk error serialization (`40001`).

## 5. Penomoran dokumen

- Nomor dokumen (`number`) **berbeda** dari ID teknis (UUID) dan unik per **seri** (`company`, tipe dokumen, prefix, periode).
- Format seri dapat dikonfigurasi (mis. `INV/{YYYY}/{MM}/{#####}`); nomor dibuat **saat submit**, bukan saat draft dibuat, agar draft yang dibatalkan tidak menimbulkan celah.
- **Seri tanpa celah (gapless)** untuk dokumen yang diwajibkan peraturan (mis. faktur/nota bernomor urut): counter di tabel `document_series` dikunci (`FOR UPDATE`) di transaksi yang sama dengan dokumen. Seri biasa boleh memakai sequence (boleh bercelah).
- Nomor yang sudah terpakai **tidak pernah** dipakai ulang, termasuk milik dokumen yang dibatalkan.

## 6. Nilai uang, kuantitas, dan pembulatan

| Aturan | Detail |
| --- | --- |
| Tipe | Desimal (`numeric` di PostgreSQL, `shopspring/decimal` di Go, string di JSON, library desimal di frontend). **DILARANG** float di semua lapisan |
| Presisi penyimpanan | Uang: `numeric(20,6)` untuk perhitungan menengah; nilai final dibulatkan ke **minor unit ISO 4217** mata uangnya. Kuantitas: `numeric(20,6)`, dibulatkan ke presisi satuan |
| Mata uang | Setiap nilai uang disimpan bersama kode mata uangnya. Dokumen multi-currency menyimpan **nilai transaksi** dan **nilai mata uang dasar** |
| Kurs | Disimpan pada dokumen: nilai kurs (min. 6 desimal), tanggal kurs, sumber kurs. Kurs tidak dihitung ulang setelah dokumen di-submit |
| Pembulatan | **Dapat dikonfigurasi** (settings), dalam tiga lapis — lihat §6.1. Selisih pembulatan selalu diposting ke akun selisih pembulatan, bukan dibuang |
| Pajak | Tarif pajak disimpan (snapshot) per baris dokumen; dasar pengenaan & pembulatan pajak mengikuti peraturan yang berlaku |
| Satuan | Kode satuan konsisten (mis. UN/ECE Rec. 20); faktor konversi satuan disimpan pada baris transaksi (snapshot) |

### 6.1. Konfigurasi pembulatan (tiga lapis)

| Lapis | Pengaturan | Contoh |
| --- | --- | --- |
| **1. Presisi mata uang** (per mata uang) | Jumlah digit desimal, default dari minor unit ISO 4217 | IDR 2, JPY 0, KWD 3 |
| **2. Metode pembulatan perhitungan** (per company, dapat ditimpa per modul) | `half_up` (pembulatan komersial, default), `half_even` (*banker's rounding*), `up`, `down`; dan **titik pembulatan**: per baris atau per total dokumen | Harga × kuantitas − diskon, pajak per baris |
| **3. Pembulatan total / kas** (per tipe dokumen atau per POS/kanal) | Kelipatan (`increment`), arah (`nearest`/`up`/`down`), dan akun selisih | POS tunai: total dibulatkan ke **Rp 100** atau **Rp 500**; faktur: tanpa pembulatan total |

- Pembulatan lapis 3 **tidak mengubah** nilai baris dan pajak. Ia menambah baris/komponen **"Pembulatan"** yang tercatat di dokumen dan diposting ke akun selisih pembulatan, sehingga jejaknya utuh dan pajak tetap dihitung dari nilai sebelum pembulatan.
- Konfigurasi yang dipakai **disalin (snapshot) ke dokumen** saat submit (§7): perubahan konfigurasi tidak mengubah dokumen lama.
- Konfigurasi pembulatan memakai Settings berbasis schema (PRD-001 FR-120), dan perubahannya tercatat di audit.

## 7. Snapshot data referensi

Transaksi **menyalin** nilai master data yang memengaruhi hasilnya pada saat dibuat/di-submit: harga, tarif pajak, akun, faktor konversi satuan, kurs, nama/alamat pihak (untuk dokumen legal). Perubahan master data sesudahnya **tidak** mengubah dokumen yang sudah ada.

## 8. Periode akuntansi & tanggal

- `posting_date` adalah **tanggal** (bukan timestamp) dalam zona waktu company; `created_at`/`updated_at` adalah timestamp UTC.
- Posting ke periode berstatus `closed` **ditolak**. Membuka kembali periode butuh permission khusus, alasan, dan tercatat di audit.
- Tanggal transaksi di masa depan atau terlalu jauh di masa lalu ditolak sesuai kebijakan modul.
- Server memakai sinkronisasi waktu (NTP); waktu dari klien tidak dipakai sebagai waktu rekaman.

## 9. Pemisahan tugas & persetujuan (`ISO/IEC 27002:5.3`)

> **Approval adalah kapabilitas wajib** di semua modul: setiap resource dokumen terintegrasi dengan approval, dan **mesin approval berada di pipeline aksi dokumen platform**, sedangkan **skemanya diatur di modul Core** (PRD-002 Document Framework). Bila tidak ada skema aktif untuk sebuah resource/aksi, approval dianggap otomatis disetujui, tetapi titik pemeriksaannya tetap ada di kode. Detail keputusan awal: `roadmap-and-risks.md` §1.1.

- Modul mendeklarasikan resource yang **mendukung** maker-checker di manifest (mis. pembayaran, jurnal manual, penyesuaian stok, perubahan harga massal). **Aktif atau tidaknya diatur lewat konfigurasi** (Settings per company, per resource): bila aktif, pembuat dokumen **tidak boleh** meng-`approve`/`post` dokumennya sendiri.
- **Batas nilai** (mis. penyesuaian > Rp 10.000.000 butuh approval level 2) juga dapat dikonfigurasi per resource dan dievaluasi server.
- Perubahan konfigurasi maker-checker dan batas nilai tercatat di audit (karena mengubah kontrol internal).
- Kombinasi role yang bertentangan (mis. "membuat vendor" + "membayar vendor") dapat ditandai sebagai **konflik SoD**. Penugasan role yang melanggar memberi peringatan, atau ditolak sesuai kebijakan, dan tercatat di audit.

## 10. Rekonsiliasi & kontrol berkala

- Job terjadwal membandingkan proyeksi saldo dengan agregat ledger, dan total sub-ledger dengan akun kontrol di buku besar.
- Selisih **tidak diperbaiki otomatis**: dicatat, memicu alert, dan diselesaikan lewat dokumen koreksi.
- Hasil rekonsiliasi disimpan sebagai rekaman yang dapat diaudit.

## 11. Kualitas master data (ISO 8000 / ISO/IEC 25012)

- Kode unik per company (SKU, kode akun, kode gudang), dinormalisasi (trim, case).
- Field wajib dan format divalidasi server dengan aturan yang sama untuk UI, API, dan impor.
- Deteksi kemungkinan duplikat (nama/NPWP/email mirip) saat membuat master data pihak.
- Master data yang sudah dipakai transaksi **tidak dihapus**, hanya dinonaktifkan.

## 12. Impor data massal

- Melewati **validasi dan otorisasi yang sama** dengan API (tidak ada jalur pintas ke database).
- Wajib menyediakan **dry-run** yang melaporkan error per baris tanpa menyimpan.
- Mode `atomic` (semua atau tidak sama sekali) atau per baris, dinyatakan jelas ke pengguna.
- Setiap impor tercatat: file sumber (checksum), pengguna, waktu, jumlah baris berhasil/gagal.

## 13. Retensi, arsip, dan penghapusan (ISO 15489, `27002:8.10`, UU PDP)

Retensi **dapat dikonfigurasi per kategori data** (Settings), dengan nilai default di bawah. Bila admin menetapkan nilai di bawah **batas minimum hukum** yang tercatat untuk kategori tersebut, sistem menampilkan peringatan, meminta konfirmasi eksplisit, dan mencatatnya di audit. Retensi juga **tidak berarti data harus tetap di database utama**: rekaman lama dapat dipindah ke **arsip** (penyimpanan murah, read-only, tetap dapat dicari untuk audit), sehingga database operasional tetap ramping.

| Data | Default |
| --- | --- |
| Dokumen & catatan pembukuan/transaksi | **10 tahun**: mengikuti kewajiban penyimpanan dokumen perpajakan di Indonesia (batas minimum hukum; konfirmasi dengan konsultan). Setelah melewati periode aktif (mis. 2 tahun), dipindah ke arsip |
| Audit log | Sesuai settings (default 365 hari, PRD-001 FR-130). Untuk event terkait transaksi keuangan, diselaraskan dengan retensi dokumen |
| Data pribadi | Diproses sesuai tujuan dan dasar hukum (UU PDP). Permintaan penghapusan dari subjek data yang berbenturan dengan kewajiban retensi diselesaikan dengan **pseudonimisasi** field pribadi, bukan menghapus rekaman transaksi |
| Draft yang tidak pernah di-submit | Boleh dihapus permanen |

Penghapusan permanen dicatat di audit (apa, kapan, siapa, dasar).

## 14. Testing integritas

- **Property-based test** untuk invariant: Σ debit = Σ kredit, saldo proyeksi = agregat ledger, kuantitas tidak negatif bila dilarang, total dokumen = Σ baris + pajak − diskon setelah pembulatan.
- **Test konkurensi**: N goroutine mem-posting/menyesuaikan saldo yang sama; hasil akhir konsisten dan tanpa deadlock.
- **Test idempotensi**: permintaan sama dikirim 2× → satu efek.
- **Test immutabilitas**: `UPDATE`/`DELETE` langsung ke dokumen `posted` dan ledger ditolak database.
- **Test periode**: posting ke periode tertutup ditolak.

## 15. Checklist resource transaksional (Kiro spec `design.md` & PR)

- [ ] Status dokumen & transisi didefinisikan; field yang boleh berubah per status jelas.
- [ ] Trigger/constraint DB untuk immutabilitas setelah submit dan untuk ledger append-only.
- [ ] Satu transaksi DB mencakup data, ledger, nomor, idempotensi, outbox, dan audit.
- [ ] Konkurensi: `version`/`If-Match` + kunci baris dengan urutan konsisten.
- [ ] Referensi sumber unik untuk ledger; `Idempotency-Key` didukung.
- [ ] Seri penomoran ditentukan (gapless atau tidak) beserta alasannya.
- [ ] Uang/kuantitas desimal; minor unit ISO 4217; metode & titik pembulatan dinyatakan; akun selisih pembulatan.
- [ ] Snapshot data referensi (harga, pajak, kurs, konversi satuan).
- [ ] Validasi periode akuntansi & tanggal.
- [ ] Maker-checker / batas nilai bila resource berisiko.
- [ ] Rekonsiliasi untuk saldo yang di-cache.
- [ ] Kebijakan retensi & penghapusan.
- [ ] Test properti, konkurensi, idempotensi, immutabilitas.

## 16. Keputusan & pertanyaan terbuka

| # | Pertanyaan | Status |
| --- | --- | --- |
| T1 | Metode pembulatan? | ✅ **Konfigurasi tiga lapis** (presisi mata uang, metode perhitungan default `half_up`, pembulatan total/kas mis. Rp 100 untuk POS) — §6.1 |
| T2 | Maker-checker wajib atau opsional? | ✅ **Diatur per konfigurasi** per resource yang mendukungnya — §9 |
| T3 | Retensi? | ✅ **Dapat dikonfigurasi per kategori** dengan default dan peringatan bila di bawah batas minimum hukum; data lama dipindah ke arsip — §13. Angka minimum hukum tetap perlu dikonfirmasi konsultan pajak |
| T4 | Dokumen apa saja yang wajib gapless pada rilis pertama? | ⏳ Terbuka — diputuskan di PRD modul Sales/Finance |

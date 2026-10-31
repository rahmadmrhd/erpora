# ADR-0015: Komponen `@erpora/ui` persis registry shadcn dengan kunci snapshot

- **Status:** Accepted. Menggantikan sebagian keputusan 1 dan konsekuensi "adaptasi manual" pada [ADR-0011](0011-kontrak-ui-extension-dan-design-system.md).
- **Tanggal:** 2026-10-31 (keputusan komponen ini dicatat di ADR-0015, bukan ADR-0011; disetujui pemilik produk 2026-11-01, lihat PRD-000 Q40 butir 26)
- **Sumber:** PRD-000 Q40 butir 1 (diubah oleh butir 17 sampai 19); spec `docs/specs/m3a-revisi-shell-shadcn/` (requirements R1.1, R1.3, R1.6, R2.2; design D-1, D-1a, D-1b; tasks 1.3, 1.4, grup 2). Keputusan disetujui pemilik produk pada 31 Okt 2026.
- **Diperbarui 2026-10-31 (putaran perbaikan 2):** penyimpangan terdaftar bertambah satu menjadi empat (patch perilaku, K12), CLI shadcn tetap lewat `bunx` terpin (K13, keputusan 31 Okt 2026, tanpa perubahan). Sumber: PRD-000 Q40 butir 24; spec `m3a-revisi-shell-shadcn` requirements R1.1, bagian "Keputusan putaran perbaikan" (K12, K13); design D-1a, D-11.

## Konteks

- M3a (ADR-0011) menyalin komponen shadcn ke `packages/ui` lalu mengadaptasinya di tempat (token, label, ukuran). Hasilnya menyimpang dari registry terbaru: `sidebar` +246/−90 baris, `avatar` +62, `alert-dialog` +61, `dropdown-menu` +49, `popover` +45, dan varian serta ukuran `button` berbeda (`primary`, `md`).
- Registry shadcn tidak berversi. Isi komponen di `ui.shadcn.com` dapat berubah tanpa perubahan di repo, sehingga pemeriksaan langsung ke registry di `web:check` tidak deterministik dan butuh jaringan.
- Registry menulis teks Inggris tertanam (`Close`, `More`, `Loading`, `aria-label="breadcrumb"`) dan palet langsung (`bg-black/50`, `text-white`). Keduanya melanggar aturan i18n (R1.7, R19.1) dan aturan token (R2.2).
- Pemilik produk menetapkan tujuan "komponen persis shadcn" agar pembaruan shadcn cukup dijalankan ulang tanpa merge manual (Q40 butir 1).
- Komponen tidak boleh diubah isinya. Kebutuhan DS yang tidak punya padanan shadcn harus berada di lapisan pembungkus.

## Opsi yang dipertimbangkan

1. **Opsi A (dipilih): persis registry dengan penyimpangan terdaftar dan kunci snapshot.** Penyimpangan yang diizinkan hanya jalur impor `cn`, patch label, dan patch token, yang semuanya dideklarasikan di `packages/ui/shadcn.patches.json`. Kesamaan diperiksa offline terhadap `packages/ui/shadcn.lock.json`. Kelebihan: pemeriksaan deterministik di CI, perubahan selalu lewat satu perintah, dan diff terhadap registry kecil. Kekurangan: patch harus ditinjau setiap sinkronisasi, dan kunci tidak mendeteksi drift upstream sampai `shadcn:verify` dijalankan.
2. **Opsi B: tetap salin lalu adaptasi di tempat (status quo ADR-0011).** Kelebihan: fleksibel per komponen. Kekurangan: selisih terhadap registry terus membesar, dan setiap pembaruan memerlukan merge manual.
3. **Opsi C: `shadcn add --dry-run --diff` terhadap registry hidup di `web:check`.** Kelebihan: langsung mendeteksi drift. Kekurangan: rapuh, butuh jaringan di setiap pemeriksaan, dan gagal sendiri saat upstream berubah. Ditolak.
4. **Opsi D: komponen shadcn sebagai dependensi terpasang tanpa salinan.** Ditolak seperti pada ADR-0011: token dan label tidak dapat diubah di sumbernya.

## Keputusan

1. **Komponen persis registry.** Setiap komponen di `packages/ui/src/components/ui` identik dengan hasil `shadcn add <komponen>` pada CLI terpin `shadcn@4.21.5` (registry `new-york-v4`), kecuali empat penyimpangan terdaftar:
   - (a) jalur impor `cn`: `from "cn"` menjadi `from "@erpora/ui/lib/utils"`, dilakukan oleh `shadcn:sync`;
   - (b) **patch label**: teks Inggris tertanam diganti `UiLabel` atau `useUiLabel(...)`;
   - (c) **patch token**: warna palet langsung diganti token semantik DS (`bg-black/50` menjadi `bg-overlay` dengan opasitas 0,8 mengikuti DS; `text-white` menjadi `text-destructive-foreground`);
   - (d) **patch perilaku** (K12, 31 Okt 2026): patch hanya menghapus penulisan cookie `sidebar_state` oleh `Sidebar` shadcn (`kind: behavior` di `shadcn.patches.json`). Penyimpanan keadaan ke `localStorage["erpora.sidebar"]` (nilai `open` boolean) ditulis shell lewat `onOpenChange` (`web/src/app/shell/sidebar-state.ts`), bukan oleh patch. Tanpa patch ini, registry tetap menulis cookie yang ikut terkirim ke server dan bertentangan dengan R9.1 dan D-10.
   Patch bersifat deklaratif (`find`, `replace`, jumlah kemunculan tetap) dan gagal keras bila teks asal tidak ditemukan. Patch dicatat dengan alasan di `shadcn.patches.json`.
2. **Kunci snapshot.** Kesamaan diperiksa offline terhadap `packages/ui/shadcn.lock.json`, yang berisi hash SHA-256 isi setiap berkas di `components/ui` dan `src/hooks/*.ts` setelah normalisasi `cn` dan patch. `task shadcn:check` berjalan di `web:check` dan gagal bila ada berkas berbeda dari kunci, berkas tanpa kunci, kunci tanpa berkas, atau patch yang belum terpasang. Kunci hanya diperbarui lewat `task shadcn:sync -- <komponen>`. `task shadcn:verify` (daring, di luar `web:check`) hanya melaporkan drift terhadap registry hidup dan selalu keluar dengan kode 0.
3. **Nama dan ukuran bawaan shadcn.** Varian dan ukuran mengikuti shadcn (`Button`: `default|destructive|outline|secondary|ghost|link`; ukuran `default|xs|sm|lg|icon*`). Varian dan ukuran buatan sendiri (`primary`, `md`) dihapus.
4. **Lapisan pembungkus `components/extended`.** Kebutuhan DS tanpa padanan shadcn berada di sini dan membungkus komponen shadcn tanpa mengubah berkasnya: `LoadingButton` (indikator loading, memakai `Spinner`), `StatusBadge` (badge bernada status, memakai `Badge`), dan `Toaster` (sonner bergaya token, entri sekunder `@erpora/ui/toaster`).
5. **Padanan komposit lama.** `BadgeStatus` menjadi `StatusBadge`; `EmptyState` dan `ErrorPage` menjadi `Empty`; `Kbd` buatan sendiri menjadi `Kbd` shadcn; `LoadingIcon` menjadi `Spinner`; `Button` varian `primary` menjadi `default`; prop `loading` menjadi `LoadingButton`.
6. **Lembar bawah tanpa `drawer`.** Komponen `drawer` (vaul) tidak dipasang. Lembar bawah memakai `Sheet side="bottom"`.
7. **Komponen baru dikonfirmasi dahulu.** Setiap komponen baru dari registry hanya dipasang setelah pemilik produk menyetujui, dengan perbandingan terhadap padanan Frappe UI dan DS (R1.6).
8. **Ukuran kontrol.** Tinggi kontrol shadcn 36 px diterima walau DS menetapkan 34 px. Kepadatan hanya dapat diatur lewat token.
9. **CLI shadcn tetap lewat `bunx` (K13, 31 Okt 2026).** Pemilik produk memutuskan CLI `shadcn@4.21.5` tetap dijalankan lewat `bunx shadcn@4.21.5` dari skrip `shadcn:sync` dan `shadcn:verify` (`shadcn:check` offline dan tidak menjalankan CLI), sesuai PRD-000 Q39 butir 5. Devdependency ditolak karena menambah ±270 entri `bun.lock` dan memperburuk `bun audit` (1 moderate menjadi 1 high, 1 moderate, dan 1 low). Temuan MINOR-1 (CLI via `bunx` tanpa lockfile, dependensi transitif di-resolve segar) ditutup sebagai risiko yang diterima: CI hijau, skrip hanya jalan di mesin pengembang, dan nama komponen divalidasi `parseComponentNames`.

## Konsekuensi

- **Positif:** pembaruan shadcn menjadi satu perintah dengan diff yang tertinjau; pemeriksaan kesamaan deterministik dan offline; teks dan palet mengikuti aturan i18n dan token.
- **Negatif dan biaya yang diterima:**
  - Setiap sinkronisasi perlu meninjau patch. Bila registry mengubah teks yang dipatch, sinkronisasi gagal dan harus diperbarui secara sadar.
  - Kunci tidak melihat perubahan upstream sampai `shadcn:verify` dijalankan secara manual.
  - Dependensi baru masuk: `cmdk` 1.1.1 (palet, hanya chunk malas) dan `next-themes` 0.4.6 (ditarik `sonner` 2.0.8; `theme` ditimpa di pembungkus `Toaster`).
  - Perilaku bawaan shadcn yang bertentangan dengan keputusan produk (`Sidebar` menulis cookie `sidebar_state`) dihapus lewat patch perilaku (K12, 31 Okt 2026), bukan diterima. Perilaku bawaan lain yang berbeda dari M3a tetap dicatat di `docs/specs/m3a-revisi-shell-shadcn/tasks.md` (catatan 2.1 sampai 2.6).
  - `shadcn-sync` menjalankan CLI lewat `bunx` terpin dengan argumen nama polos saja, sehingga URL, namespace registry, dan flag ditolak. Ini membatasi risiko rantai pasok dan SSRF. Risiko CLI tanpa lockfile diterima (K13, 31 Okt 2026).
- **Pekerjaan lanjutan wajib:**
  - Setiap komponen baru tetap melewati konfirmasi pemilik produk sebelum `shadcn:sync`.
  - Setiap sinkronisasi memperbarui `shadcn.lock.json` dalam commit yang sama dengan komponennya.

## Pemicu evaluasi ulang

- Registry shadcn berhenti dipelihara, mengubah lisensi, atau mengubah jalur impor yang membuat patch tidak lagi berlaku.
- Patch label atau token menjadi terlalu banyak sehingga komponen praktis tidak lagi persis registry.
- `shadcn:verify` secara rutin melaporkan drift yang tidak dapat diselesaikan lewat patch.
- DS berubah secara material (token atau komponen inti) dan pemilik produk memutuskan mengganti sumber desain.

# ADR-011: Kontrak UI extension dan design system

- **Status:** Accepted
- **Tanggal:** 2026-10-29
- **Sumber:** PRD-000 Q33 (pendekatan UI dan batas blok M3), Q36 (keputusan M3a, PK-1 sampai PK-6), Q38 (keputusan pelaksanaan M3a: entry sekunder F2, anggaran); spec `.kiro/specs/m3a-frontend-foundation-shell/` (design K-1 sampai K-34)

## Konteks

Frontend modul di-co-locate di `modules/<name>/web` (ADR-001, PRD-000 Q21). Komponen generik berada di `packages/ui` (`@erpora/ui`), layanan frontend platform di `packages/platform-web`, dan app shell di `web/`. Sebelum M3a, batas antara shell, modul, dan komponen UI belum tertulis sebagai keputusan, dan `packages/ui-contracts` masih stub.

Fakta yang memengaruhi keputusan:

- Pemilik produk menetapkan **ERP Design System (DS)** sebagai sumber token, tipografi (Figtree dan skala zinc), dan acuan komponen (Q33 butir 2). DS adalah artefak desain, bukan paket npm.
- **Frappe UI** adalah pustaka Vue. Stack frontend repo adalah React, sehingga Frappe UI hanya dipakai sebagai rujukan UX dan perilaku, tidak sebagai dependensi.
- DS memakai komponen shadcn/Radix dengan token yang perlu disesuaikan. Penyesuaian yang sudah tercatat mencakup kontras (K-14, sebagian pasangan token DS gagal 4,5:1), dimensi shell (K-15), dan label bawaan (K-5, K-24).
- Anggaran bundle entry shell adalah 200 KB gzip (NFR-04). Pengukuran spike menunjukkan ruang yang sempit. Entry shell terukur 187,9 KB gzip pada 29 Okt 2026 (Q38).
- `make:module --frontend` dan templat modul belum sepenuhnya selaras dengan bentuk frontend referensi (task 13.4 M3a belum selesai).

## Opsi yang dipertimbangkan

1. **Opsi A (dipilih): ERP DS sebagai sumber token, shadcn/ui React disalin ke `packages/ui`.** Komponen menjadi milik repo, lalu diadaptasi ke token DS, label, dan ukuran. Kelebihan: kendali penuh atas token, kontras, label, dan ukuran; dependensi tidak berubah diam-diam. Kekurangan: setiap `shadcn add` memerlukan langkah adaptasi terdokumentasi (lihat `packages/ui/README.md`) dan pemeliharaan salinan sendiri.
2. **Opsi B: Frappe UI sebagai pustaka komponen.** Kelebihan: API ringkas, banyak perilaku sudah jadi. Kekurangan: pustaka Vue, tidak sesuai stack React yang dipilih PRD-000 (Q5, FR-08), dan token DS tetap harus dipetakan. Ditolak; Frappe UI tetap menjadi rujukan UX per komponen (tabel C design M3a).
3. **Opsi C: komponen shadcn sebagai dependensi terpasang tanpa salinan.** Kelebihan: tidak ada salinan yang dipelihara. Kekurangan: token, label bawaan (`UiLabels`), dan kontras tidak dapat diubah di sumbernya, sehingga penyesuaian DS harus ditimpa ulang setiap versi. Ditolak.
4. **Opsi D: kontrak UI extension dan ekstensi tampilan dari modul diputuskan sekarang.** Ditolak untuk fase ini: kebutuhan konkret belum ada sebelum shell dan modul referensi berjalan, dan PRD marketplace (fase 06) belum dimulai.

## Keputusan

1. **Design system.** Sumber token dan acuan komponen adalah ERP Design System milik pemilik produk. Komponen dasar memakai shadcn/ui (React, Radix) yang **disalin** ke `packages/ui` sebagai paket `@erpora/ui` dan dimiliki repo. Tipografi memakai Figtree yang di-host sendiri (`@fontsource-variable/figtree`, tanpa CDN). Nilai warna hanya ada di `packages/ui/src/styles/theme.css`; komponen dilarang memakai nilai palet langsung (mis. `zinc-100`, `#fff`, `rgb(...)`). Frappe UI hanya rujukan UX dan perilaku.
2. **Batas paket frontend** (PRD-000 §4.2, aturan F1 sampai F4):
   - `@erpora/ui` adalah daun dan **tidak boleh** mengimpor paket `@erpora/*` mana pun (F4). Label dari luar lewat konteks `UiLabels` yang diisi shell.
   - `@erpora/platform-web` tidak mengimpor `@erpora/ui` dan tidak mengimpor paket modul. Komponen dijembatani di shell (mis. `UiLabelsBridge`).
   - `@erpora/module-sdk` tidak bergantung pada `@erpora/platform-web` maupun `@erpora/ui`.
   - Frontend modul hanya mengimpor `@erpora/ui`, `@erpora/platform-web`, `@erpora/module-sdk`, `@erpora/platform-contracts`, dan `@erpora/module-<x>` yang tercantum di `manifest.dependencies` (F2). Antarmodul hanya lewat `index.ts`. Entry sekunder `packages/<x>/src/entries/*` (mis. `@erpora/ui/toaster`) boleh diimpor langsung lewat `exports` paket untuk memisahkan kode yang dimuat malas (Q38 butir 1).
   - Hanya app shell `web/` yang merakit semua modul (F3).
3. **Registry dan route modul.** Modul mendeklarasikan `FrontendModule` lewat `defineModule()` di `@erpora/module-sdk`: `key`, `slug`, `version`, `title?`, `navigation[]`, `registerRoutes`, dan `offline` (mengikuti `docs/architecture/offline-strategy.md` §3). Shell mencocokkan modul dengan bootstrap berdasarkan `key`, hanya memunculkan navigasi untuk status `enabled`, dan membangun router sekali setelah bootstrap dari modul yang ada di bootstrap. Route modul menerima base route bertipe spesifik (`ModuleBaseRoute`), bukan `AnyRoute`, dan dimuat malas (`.lazy()`). Tidak ada deklarasi `Register` router global (K-28). Guard di shell memblokir route modul yang statusnya bukan `enabled` sebelum chunk halaman dimuat, dan `router.invalidate()` dijalankan saat `revision` bootstrap berubah (K-11). Backend tetap otoritas; filter di frontend hanya untuk UX.
4. **Teks UI dan aksi halaman.** Semua teks UI memakai kunci terjemahan (`useTranslation`, `msg()`, `crumb()`), kecuali layar bootstrap gagal (PK-4, K-34). Aksi halaman dideklarasikan halaman lewat store `usePageActions` dan dirender navbar oleh shell, tanpa modul mengimpor struktur shell (K-8).
5. **Di luar keputusan ini.** Kontrak UI extension (ekstensi tampilan oleh modul selain route dan navigasi) **tidak diputuskan** dan tetap fase lanjutan. `packages/ui-contracts` tetap stub. Keputusan itu dibuat saat kebutuhan konkretnya muncul.

## Konsekuensi

- **Positif:** modul baru memakai komponen dan token yang sama tanpa membuat versinya sendiri; arah dependensi frontend dapat diperiksa otomatis (`dependency-cruiser`, aturan F1 sampai F4); route dan navigasi modul tidak perlu mengubah kode shell.
- **Negatif dan biaya:**
  - Anggaran bundle sempit: entry shell 187,9 KB dari 200 KB gzip (Q38). Tambahan fitur harus memakai lazy loading (TanStack DB sudah lazy, K-21).
  - Setiap `shadcn add` memerlukan langkah adaptasi (ganti impor `cn`, pin versi, token DS). Langkah ini terdokumentasi, tetapi manual.
  - Salinan komponen harus dipelihara repo sendiri.
  - `bun-plugin-tailwind` 0.1.2 membawa mesin Tailwind 4.1.14 sementara paket `tailwindcss` 4.3.3 dipakai untuk tema; utilitas yang baru di versi lebih baru dapat hilang diam-diam (lihat Risiko design M3a).
- **Pekerjaan lanjutan wajib:**
  - Blok M3b sampai M3g di atas batas paket ini (PRD-000 Q33).
  - Identitas dan sesi nyata di PRD-001 untuk menggantikan `CurrentUserSource` sementara (K-4).
  - Backend notifikasi di PRD Notifications untuk menggantikan `emptyNotificationSource` (K-3).
  - Kontrak UI extension dirumuskan sebagai ADR baru saat kebutuhannya ada.
  - `make:page` dan templat `--frontend` diselaraskan dengan panduan frontend (PK-5 dan task 13.4 M3a).

## Pemicu evaluasi ulang

- Entry shell melampaui 200 KB gzip dan mitigasi K-12 sudah habis.
- DS berubah secara material (token, tipografi, atau komponen inti), atau pemilik produk memutuskan mengganti sumber desain.
- Modul pihak ketiga (marketplace, fase 06) membutuhkan ekstensi tampilan di luar route dan navigasi. Ini memicu keputusan kontrak UI extension.
- Pustaka komponen React yang dipakai (Radix, shadcn CLI, Tailwind) berhenti dipelihara atau mengubah lisensinya.
- Batas F1 sampai F4 terbukti tidak cukup, mis. shell perlu mengimpor komponen dari modul.

# ui

`@erpora/ui`: komponen generik milik repo. `src/components/ui` adalah salinan **persis** registry shadcn pada versi terpin, dengan
penyimpangan yang terdaftar saja (lihat ADR-0015 dan PRD-000 Q40 butir 17 sampai 19). `src/components/extended` berisi pembungkus tipis
untuk kebutuhan ERP Design System (DS) yang tidak punya padanan shadcn. Konsumen hanya mengimpor dari `@erpora/ui` (barrel `src/index.ts`)
dan memuat tema lewat `@import "@erpora/ui/theme.css"` di CSS shell.

## Aturan

- **Persis shadcn (ADR-0015).** Setiap berkas di `src/components/ui` sama dengan hasil `shadcn add` pada CLI terpin, kecuali tiga
  penyimpangan terdaftar: (a) jalur impor `cn` (`from "cn"` menjadi `from "@erpora/ui/lib/utils"`), (b) **patch label** (teks Inggris
  tertanam diganti `useUiLabel`), dan (c) **patch token** (palet langsung diganti token semantik DS). Jangan mengedit berkas ini dengan
  tangan. Perubahan hanya lewat `task shadcn:sync`.
- **Varian dan ukuran bawaan shadcn.** Nama varian dan ukuran mengikuti shadcn (`Button`: `default`, bukan `primary`; ukuran `default`
  dan `xs|sm|lg|icon*`, bukan `md`). Kebutuhan DS dipenuhi lewat token, bukan dengan mengubah komponen.
- **Kebutuhan DS di `src/components/extended`.** Pembungkus membungkus komponen shadcn tanpa mengubah berkasnya. Saat ini: `LoadingButton`
  (indikator loading), `StatusBadge` (badge bernada status), dan `Toaster` (sonner bergaya token).
- **Tanpa nilai palet langsung.** Komponen memakai token semantik (`bg-background`, `text-muted-foreground`, `bg-status-success-bg`, ...),
  tidak pernah `zinc-100`, `red-500`, `#fff`, atau `rgb(...)`. Nilai warna hanya ada di `src/styles/theme.css` (diuji).
- **Daun arsitektur (F4).** Paket ini tidak mengimpor paket `@erpora/*` lain; teks bawaan komponen dibaca lewat konteks `UiLabels`
  (`src/labels.tsx`) yang diisi shell. Impor ke `@erpora/ui/...` sendiri (alias self-reference lewat `exports`) diizinkan.
- **Tanpa string tampilan tertanam.** Teks bawaan (tutup dialog, label breadcrumb, ...) memakai `useUiLabel()`; kunci ada di namespace `ui`
  (`i18n/id.json`, `i18n/en.json`). Patch label menggantikan teks Inggris registry dengan kunci ini.

## Ekspor dan entry sekunder

- Konsumen mengimpor dari barrel `@erpora/ui` (`src/index.ts`). Barrel mengekspor komponen fondasi dan pembungkus `extended` (kecuali
  `Toaster` dan `Command`), `cn`, `useIsMobile`, `fitBreadcrumb`, `initialsOf`, `iconRegistry`/`resolveIcon`, serta `UiLabelsProvider`,
  `UiLabel`, dan `useUiLabel`.
- Komponen yang sengaja dimuat malas dipasang sebagai **entry sekunder** (Q38 butir 1, R18.2) agar tidak masuk JS awal:
  - `@erpora/ui/toaster` mengekspor `Toaster` dan `toast` (sonner bergaya token, dari `components/extended/toaster`);
  - `@erpora/ui/command` mengekspor `Command` (shadcn + cmdk 1.1.1) untuk palet perintah.
    Keduanya tidak diekspor dari barrel, sehingga entry shell tetap di bawah anggaran.
- Entry sekunder diimpor langsung lewat `exports` paket; aturan F2 mengizinkannya, aturan lain tetap berlaku.

## Kunci terjemahan

- Teks bawaan komponen berasal dari namespace `ui`: `i18n/id.json` dan `i18n/en.json`. Kunci yang dibangkitkan ada di `src/i18n/keys.gen.ts` (`task i18n:generate`).
- `uiKeys` di `src/labels.tsx` mendaftar kunci yang dibaca komponen. Tes memastikan setiap kunci di sana ada di `i18n/{id,en}.json`.
- Panduan umum ada di [`docs/module-sdk/i18n.md`](../../docs/module-sdk/i18n.md).

## Memasang atau memperbarui komponen

Alur ini menggantikan cara lama "salin lalu adaptasi di tempat" (ADR-0011, sudah sebagian digantikan oleh ADR-0015).

Sebelum memasang komponen baru dari registry, **minta konfirmasi pemilik produk** (R1.6) dengan perbandingan terhadap padanan Frappe UI dan DS.
Komponen di luar daftar yang sudah disetujui tidak dipasang tanpa konfirmasi.

```
task shadcn:sync -- badge tabs      # pasang atau timpa komponen dari registry, terapkan patch, perbarui kunci
task shadcn:check                   # offline: hash berkas vs kunci snapshot, dan patch terpasang (ikut web:check)
task shadcn:verify                  # daring: lapor drift terhadap registry hidup (tidak menggagalkan build)
task shadcn:verify -- badge         # hanya komponen tertentu
task shadcn:sync -- --prune         # buang entri kunci yang berkasnya dihapus sengaja
```

- `shadcn:sync` hanya menerima **nama komponen polos** (huruf kecil, angka, tanda hubung). URL, namespace registry (`@x/y`), jalur, dan
  flag ditolak, sehingga skrip tidak bisa diarahkan ke registry lain.
- `shadcn:sync` menjalankan CLI terpin (skrip `shadcn` di `package.json` memanggil `bunx shadcn@4.21.5`; CLI sengaja bukan devDependency,
  PRD-000 Q39). Naikkan versi hanya sambil memeriksa `npm view shadcn time` (rilis ≥ 14 hari) dan menjalankan `bun audit`.
- Urutan kerja `shadcn:sync`: `add -o -y` → ganti `from "cn"` menjadi `from "@erpora/ui/lib/utils"` → hapus dependensi `cn` yang ditulis CLI →
  terapkan patch dari `shadcn.patches.json` → pin versi pasti dependensi baru atau yang berubah → `bun install` → tulis `shadcn.lock.json`.
  Hash dihitung setelah patch.
- **Patch.** Penyimpangan selain jalur `cn` hanya lewat `shadcn.patches.json`: `{ component, find, replace, count?, reason }`. Patch gagal
  keras bila teks asal tidak ditemukan sebanyak `count`, sehingga perubahan registry yang memutus patch terlihat. Setiap patch wajib punya alasan
  yang menunjuk aturan (R1.7, R2.2). Patch label memakai `UiLabel` atau `useUiLabel(...)`; kunci barunya ditambahkan ke `uiKeys` dan ke
  `i18n/{id,en}.json`.
- **Kunci snapshot.** `shadcn.lock.json` menyimpan hash isi setiap berkas di `src/components/ui` dan `src/hooks/*.ts`. `shadcn:check` gagal bila ada
  berkas berbeda dari kunci, berkas tanpa kunci, kunci tanpa berkas, atau patch yang belum terpasang. Jangan menyunting kunci dengan tangan.
- Komponen dan kunci diperbarui dalam **satu commit**, dengan `package.json` dan `bun.lock` bila dependensinya berubah.
- Ringkas: `shadcn:verify` hanya melaporkan selisih. Pembaruan selalu lewat `shadcn:sync` dan ditinjau sebagai diff.
- Dampak perilaku dari shadcn persis (cookie `sidebar_state`, `aria-current`, `role="alert"` pada `Alert`, dan lain-lain) dicatat di
  `docs/specs/m3a-revisi-shell-shadcn/tasks.md` (catatan 2.1 sampai 2.6).

## Pengujian

`bun test` (dari root repo atau di dalam paket) memakai happy-dom dan Testing Library lewat preload `web/test/dom.ts` dan `web/test/setup.ts`.
Pemeriksaan kesamaan komponen (`shadcn:check`) dan tes alat sinkronisasi (`web/tools/shadcn-*.test.ts`) ikut berjalan di `web:check`.

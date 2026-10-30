# ui

`@erpora/ui`: komponen generik milik repo (shadcn/Radix yang disalin lalu diadaptasi ke token ERP Design System). Konsumen hanya
mengimpor dari `@erpora/ui` (barrel `src/index.ts`) dan memuat tema lewat `@import "@erpora/ui/theme.css"` di CSS shell.

## Aturan

- **Komponen dimiliki repo.** Hasil `shadcn add` disalin ke `src/components/ui`, diadaptasi (token, label, ukuran), dan dicatat di
  tabel K-24 `.kiro/specs/m3a-frontend-foundation-shell/design.md`. Jangan menyalin komponen manual.
- **Tanpa nilai palet langsung.** Komponen memakai token semantik (`bg-background`, `text-muted-foreground`, `bg-status-success-bg`, ...),
  tidak pernah `zinc-100`, `red-500`, `#fff`, atau `rgb(...)`. Nilai warna hanya ada di `src/styles/theme.css` (diuji).
- **Daun arsitektur (F4).** Paket ini tidak mengimpor paket `@erpora/*` lain; teks bawaan komponen dibaca lewat konteks `UiLabels`
  (`src/labels.tsx`) yang diisi shell. Impor ke `@erpora/ui/...` sendiri (alias self-reference lewat `exports`) diizinkan.
- **Tanpa string tampilan tertanam.** Teks bawaan (tutup dialog, label breadcrumb, ...) memakai `useUiLabel()`; kunci ada di namespace `ui`
  (`i18n/id.json`, `i18n/en.json`).

## Ekspor dan entry sekunder

- Konsumen mengimpor dari barrel `@erpora/ui` (`src/index.ts`). Barrel mengekspor komponen fondasi, `cn`, `initialsOf`, `collapseCrumbs`, `resolveIcon`/`iconRegistry`, dan `UiLabelsProvider`/`useUiLabel`.
- Komponen yang sengaja dimuat malas dipasang sebagai **entry sekunder** (Q38 butir 1): `@erpora/ui/toaster` mengekspor `Toaster` dan `toast` (sonner bergaya token). `Toaster` tidak diekspor dari barrel, sehingga entry shell tetap di bawah anggaran.
- Entry sekunder diimpor langsung lewat `exports` paket; aturan F2 mengizinkannya, aturan lain tetap berlaku.

## Kunci terjemahan

- Teks bawaan komponen berasal dari namespace `ui`: `i18n/id.json` dan `i18n/en.json`. Kunci yang dibangkitkan ada di `src/i18n/keys.gen.ts` (`task i18n:generate`).
- `uiKeys` di `src/labels.tsx` mendaftar kunci yang dibaca komponen. `labels.test.tsx` memastikan setiap kunci di sana ada di `i18n/{id,en}.json`.
- Panduan umum ada di [`docs/module-sdk/i18n.md`](../../docs/module-sdk/i18n.md).

## Memasang komponen baru

CLI `shadcn` dijalankan lewat `bunx` pada **versi terpin** (skrip `shadcn` di `package.json` memanggil `bunx shadcn@4.21.5`); ia sengaja bukan
devDependency, supaya pohon dependensinya (mis. `zod`, `deepmerge`, `undici`) tidak masuk `bun.lock` dan `bun audit` (keputusan 31 Okt 2026, PRD-000 Q39).
Naikkan versi di skrip itu hanya sambil memeriksa `npm view shadcn time` (rilis ≥ 14 hari) dan menjalankan `bun audit`:

```
cd packages/ui
bun run shadcn add <komponen> --dry-run   # pratinjau
bun run shadcn add <komponen> -y
```

Setelah **setiap** `add` (hasil uji spike, design K-10):
1. Ganti `from "cn"` menjadi `from "@erpora/ui/lib/utils"` di berkas hasil, dan hapus dependensi `cn` yang ditulis CLI ke `package.json`
   (registry shadcn memakai paket npm `cn`; repo memakai `cn` sendiri di `src/lib/utils.ts`).
2. Pin ulang versi dependensi yang ditulis CLI (ditulis dengan `^`) ke versi pasti di tabel B `design.md`, lalu `bun install` dan commit
   `package.json` bersama `bun.lock`.
3. Adaptasi ke token DS, label (`useUiLabel`), dan ukuran DS; tambahkan test; periksa bahwa kelas yang dipakai komponen dihasilkan oleh mesin
   Tailwind build (`bun-plugin-tailwind` membawa mesin 4.1.14, lihat Risiko di design).

## Pengujian

`bun test` (dari root repo atau di dalam paket) memakai happy-dom dan Testing Library lewat preload `web/test/dom.ts` dan `web/test/setup.ts`.

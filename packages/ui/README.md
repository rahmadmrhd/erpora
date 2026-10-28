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

## Memasang komponen baru

CLI `shadcn` terpin sebagai devDependency (tidak memakai `bunx`/`npx` tak terpin):

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

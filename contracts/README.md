# contracts

Kontrak milik platform dan aturan bersama, bukan kontrak modul.

- `openapi/common.yaml`: komponen bersama (skema, parameter, header, respons error); dirujuk lewat `erp:common#/components/...`.
- `openapi/platform.yaml`: endpoint platform (bootstrap dan manajemen modul).
- `module.schema.json`: JSON Schema manifest modul.
- `spectral.yaml` dan `spectral-functions/`: ruleset linter kontrak (api-guidelines dan klasifikasi data). Dijalankan
  `erpora contracts:lint` lewat `web/contracts-lint.ts` (Spectral terpin di `web/package.json`; tanpa `bun x`/`npx`).

Setiap modul punya kontrak sendiri di `modules/<dir>/openapi.yaml`. Rujukan lintas dokumen hanya `erp:<common|platform|slug>#/<pointer>`
dan hanya ke `common` atau dependensi manifest (rule `erp-ref-dependency`). Test kesesuaian di `internal/app` mencocokkan
route terdaftar dengan operasi kontrak (method, path, permission, `x-erp-public`, klasifikasi respons).

Pembangkitan tipe TS, pemeriksaan drift, dan deteksi breaking change menyusul pada Task 7 dan 8 (PRD-000 FR-23.3, FR-23.4).

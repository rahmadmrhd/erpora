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

`erpora contracts:generate` membangkitkan tipe TS (`packages/platform-contracts`, `modules/<dir>/web/api/contract.gen.ts`,
`packages/module-sdk/src/manifest.gen.ts`) dan `contracts/dist/openapi.json` (spec gabungan; tidak di-commit); `--check` menjaga
hasil generate tetap mutakhir. `erpora contracts:breaking` membandingkan dengan tag rilis terakhir (`oasdiff`). Semuanya digabung di
`task contracts:check`. Panduan lengkap: [`docs/module-sdk/contracts.md`](../docs/module-sdk/contracts.md).

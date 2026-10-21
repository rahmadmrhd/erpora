# ADR-006: Module manifest, SDK, dan semantic versioning

- **Status:** Proposed

## Konteks

Setiap modul memiliki manifest `module.json` dan dikompilasi sebagai Go module sendiri (ADR-001). Kontrak antara platform, SDK publik (`sdk/`), dan modul harus stabil, dapat divalidasi, dan memiliki aturan versi yang jelas. Pada M1, `module.json` minimal sudah ada di `modules/example/`, dan `SDKVersion` ada di `sdk/module`.

## Keputusan

Belum diputuskan secara final. Arah awal dicatat di PRD-000 (FR-05, FR-06, Q15): `version` di manifest harus sama dengan tag Go module, dan modul dengan MAJOR ≥ 2 mengikuti aturan path Go.

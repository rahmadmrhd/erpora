# contracts

Kontrak milik platform dan aturan bersama, bukan kontrak modul. Isinya `openapi/` (`common.yaml` untuk komponen bersama, `platform.yaml` untuk endpoint platform), `spectral.yaml` sebagai ruleset linter OpenAPI, serta `module.schema.json` sebagai JSON Schema manifest. `openapi/common.yaml` dan `openapi/platform.yaml` (bootstrap dan manajemen modul) ditulis di M2b sebagai dokumentasi; linter Spectral, pembangkitan tipe, dan test kesesuaian kontrak diselesaikan pada PRD-000 FR-06 dan FR-23 (M2d).

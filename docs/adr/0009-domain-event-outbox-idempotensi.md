# ADR-009: Domain event, outbox, dan idempotensi

- **Status:** Proposed

## Konteks

Modul perlu berkomunikasi tanpa dependensi langsung yang melanggar D1 dan D8. Perubahan data dan publikasi event harus konsisten, dan sinkronisasi offline membutuhkan operasi yang idempoten. Pada M1 hanya struktur `internal/platform/events` dan `internal/platform/sync` (stub).

Rujukan: `docs/architecture/module-integration.md` dan `docs/architecture/offline-strategy.md`.

## Keputusan

Belum diputuskan. Keputusan ini akan dirumuskan saat PRD Events & Jobs ditulis.

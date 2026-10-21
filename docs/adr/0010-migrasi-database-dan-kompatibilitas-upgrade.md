# ADR-010: Migrasi database dan kompatibilitas upgrade

- **Status:** Proposed

## Konteks

Migrasi dijalankan lewat goose per target (ADR-003) dan tidak pernah berjalan otomatis saat `serve`. Kompatibilitas upgrade antara versi platform, SDK, dan modul harus dijaga, termasuk urutan migrasi saat modul di-upgrade.

Rujukan: ADR-003 dan `docs/architecture/roadmap-and-risks.md` §5 (versi, migrasi, rilis hybrid).

## Keputusan

Belum diputuskan secara lengkap. Prinsip dasar sudah ditetapkan di ADR-003: migrasi versioned, terpisah per target, dan hanya dijalankan lewat `erp migrate` atau operasi lifecycle.

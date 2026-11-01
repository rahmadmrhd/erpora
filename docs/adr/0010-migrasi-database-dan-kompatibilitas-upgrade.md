# ADR-010: Migrasi database dan kompatibilitas upgrade

- **Status:** Proposed

## Konteks

Migrasi dijalankan lewat goose per target (ADR-003) dan tidak pernah berjalan otomatis saat `serve`. Kompatibilitas upgrade antara versi platform, SDK, dan modul harus dijaga, termasuk urutan migrasi saat modul di-upgrade.

Rujukan: ADR-003 dan `docs/architecture/roadmap-and-risks.md` §5 (versi, migrasi, rilis hybrid).

## Keputusan

Belum diputuskan secara lengkap. Prinsip dasar sudah ditetapkan di ADR-003: migrasi versioned, terpisah per target, dan hanya dijalankan lewat `erpora migrate` atau operasi lifecycle.

**Keputusan sebagian (1 Nov 2026, pemilik produk; PRD-000 Q41 butir 12):**

1. Upgrade yang melompati versi selalu menjalankan semua migrasi yang tertinggal secara berurutan. Karena itu migrasi yang sudah dirilis tidak boleh diedit atau digabung, dan tidak ada titik henti wajib antar-versi.
2. Penghapusan kolom, baik yang tampil di API maupun tidak, hanya pada rilis MAJOR modul, didahului penandaan `deprecated` dan pengisian kolom pengganti lewat migrasi (expand, migrate, contract; api-guidelines §14.3).
3. Migrasi penghapusan memuat penjaga terhadap data yang belum disalin, dan `Down`-nya ditandai tidak dapat dibalik.

Hal lain di ADR ini tetap belum diputuskan; status tetap Proposed.

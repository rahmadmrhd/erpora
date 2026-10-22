# ADR-005: Control Plane vs ERP Runtime

- **Status:** Proposed

## Konteks

Metadata platform (instalasi modul, operasi lifecycle, tenant terdaftar) perlu disimpan dan dikelola terpisah dari data bisnis tenant. Pada M1, migrasi target `control` sudah ada (`migrations/control/`, tabel `goose_control`), tetapi batas penuh antara control plane dan runtime ERP belum diputuskan.

## Keputusan

Belum diputuskan. Keputusan ini akan dirumuskan bersama ADR-004 dan PRD tenancy.

# ADR-004: Database-per-tenant dan tenant resolution

- **Status:** Proposed

## Konteks

Platform ini ditujukan untuk multi-tenant dengan isolasi data yang kuat. Pada M1 hanya tersedia seam single-tenant (`ERP_TENANCY_MODE=single`). Keputusan final tentang database per tenant, control DB, dan cara menentukan tenant dari request belum diambil.

Rujukan: `docs/architecture/tenancy-seams.md` (seam yang ada di kode M1) dan `docs/architecture/tenancy-design.md` (target database-per-tenant).

## Keputusan

Belum diputuskan. Keputusan ini akan dirumuskan saat PRD tenancy ditulis.

# ADR-013: Autentikasi, otorisasi, dan audit

- **Status:** Proposed

## Konteks

Platform membutuhkan autentikasi untuk pengguna (login lokal, SSO OIDC), token API, otorisasi berbasis permission, scope, dan field, serta audit trail. Ini dirinci di PRD-001 (Core).

Keputusan yang sudah diusulkan di PRD-001 (Q10): token JWT access dengan refresh token berotasi, dipakai untuk semua klien. Keputusan ini belum final sampai ADR ini diterima.

Rujukan: `docs/security/security-guidelines.md`.

## Keputusan

Usulan (belum diterima): JWT access token dan refresh token berotasi untuk semua klien, sesuai PRD-001 Q10. Detail mekanisme ditentukan oleh ADR ini sebelum implementasi PRD-001.

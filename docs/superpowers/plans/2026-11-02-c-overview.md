# PRD-001 Core (Identity, Authorization, Settings, Audit): Ikhtisar Rencana Implementasi

> **Untuk agentic worker:** rencana ini adalah ikhtisar. Eksekusi per milestone memakai rencana rinci masing-masing (`superpowers:subagent-driven-development` atau alur sprint di `~/.claude/CLAUDE.md`). Rencana C1 ada di `2026-11-02-c1-identity-lokal.md`; C2 sampai C4 ditulis setelah milestone sebelumnya selesai (bergantung pada keputusan dan kode C1).

**Tujuan:** mengganti stub autentikasi/otorisasi PRD-000 (FR-05) dengan Core sungguhan agar `sdk/authz` memenuhi FR-115 dan T4 spec `resource-metadata` (grup 19 sampai 25) terbuka.

**Spec:** `docs/prd/PRD-001-core.md` (Accepted). Urutan milestone mengikuti PRD §10 atas keputusan pemilik produk 2 Nov 2026 ("Ikuti PRD: C1 lalu C2").

## Peta milestone

| Milestone | Cakupan | Membuka | Rencana |
| --- | --- | --- | --- |
| **C1 Identity lokal** | FR-101, 102, 106, 107, 108; modul `core` (`required`); `core:create-admin`, `core:rotate-keys`, `core:reset-mfa`; role `Administrator` minimal (lihat keputusan K-C1) | Principal nyata di context, 401 vs 403, pengganti `api:dev` | `2026-11-02-c1-identity-lokal.md` (rinci) |
| **C2 Authorization** | FR-110 s.d. 119 (manifest `resources`/`field_groups`, scope, field group, `sdk/authz` penuh, IDOR, cache `perm_rev`, UI matriks) | **T4 resource-metadata** (grup 19 sampai 25), lalu grup 26 | ditulis setelah C1 |
| **C3 Settings dan Audit** | FR-120, 130, 131, 132 (settings berskema, preferensi, audit append-only) | Sumber konfigurasi SMTP yang proper menggantikan env sementara C1 | ditulis setelah C2 |
| **C4 Integrasi** | FR-103 (OIDC), FR-104 (PAT), FR-105 (client credentials) | | ditulis setelah C3 |

## Kondisi awal kode (hasil pemetaan 2 Nov 2026)

- `sdk/authz` hanya `AsSystem`, `SystemReason`, `FieldMasker`/`MaskFields`; tidak ada `sdk/identity`, `sdk/mail`, Principal.
- Stub: `internal/app/assemble.go:123-137` (authN pass-through, `authorization.NewStub`), `internal/platform/authorization/stub.go`; `permissionMW` (`internal/platform/modules/runtime/mount.go:135-147`) selalu menjawab 403 dan `Authorizer.Allow(ctx, permission) bool` tak dapat membedakan 401, 403, dan `mfa_required`.
- `modules/core` belum ada; dukungan `required` sudah ada (`sdk/module/manifest.go`, `lifecycle/required.go`).
- Tidak ada dependensi JWT, TOTP, SMTP, rate limiter, AES-GCM; `ERP_SECRET_KEY` hanya di dokumen.
- Frontend: tanpa login, guard, atau penyimpanan token; `/api/bootstrap` publik; `usePermission` optimistis; aktor audit `api:dev` di `modules/admin/api.go:35` dan `i18n/admin/overrides.go:34`.

## Keputusan yang butuh persetujuan pemilik produk (sebelum kode C1)

Tiap butir berisi rekomendasi; keputusan dicatat di tabel keputusan PRD-001 (Q-number) atau ADR-0013 sesuai `CLAUDE.md`.

| # | Keputusan | Rekomendasi |
| --- | --- | --- |
| K-C1.1 | **ADR-0013** (autentikasi/otorisasi/audit) masih *Proposed* dan hanya menyalin PRD-001 Q10; perlu detail mekanisme dan status Accepted sebelum kode | Tulis dan sahkan ADR-0013 sebagai langkah pertama C1 (token, rotasi, revokasi, kunci, urutan middleware, kontrak 401/403/mfa) |
| K-C1.2 | **Dependensi baru** (menambah `go.sum`, SBOM, govulncheck) | `golang.org/x/crypto/argon2` (sudah indirect), `golang.org/x/time/rate`, `golang.org/x/term` (prompt sandi CLI); JWT EdDSA dan TOTP RFC 6238 **ditulis sendiri** di `internal/platform/identity` memakai stdlib (`crypto/ed25519`, `crypto/hmac`) dengan vektor uji RFC, bukan pustaka pihak ketiga; SMTP lewat `net/smtp` + `crypto/tls` |
| K-C1.3 | **Role Administrator di C1**: MFA wajib berlaku untuk pemilik role Administrator (FR-107) dan `core:create-admin` harus memberi role itu, tetapi tabel role milik C2 | Bangun subset FR-113 di C1: tabel tenant `roles`, `user_roles`, dan seed role sistem `administrator`; matriks, scope, dan field access tetap C2 |
| K-C1.4 | **Linter kontrak vs respons kredensial**: security-guidelines §2:48 mewajibkan properti bernama `password`/`secret`/`token` berklasifikasi `highly_confidential` dan dilarang di respons, tanpa pengecualian; respons login/refresh membawa `accessToken` | Perluas linter secara aditif (seperti K21) dengan penanda eksplisit `x-erp-credential-response: true` hanya untuk operasi di modul `core`/`platform` yang diizinkan allowlist; catat sebagai keputusan di security-guidelines §2 |
| K-C1.5 | **`core:create-admin` dan sandi**: `Invocation` hanya flag string tanpa stdin; sandi lewat flag bocor ke riwayat shell | Tambah `Invocation.In io.Reader` dan prompt tersembunyi (`x/term`) bila TTY, atau baca stdin; tanpa flag sandi (flag `--password` tidak disediakan) |
| K-C1.6 | **Konfigurasi SMTP sebelum Settings (C3)** | Env sementara `ERP_MAIL_DRIVER`, `ERP_SMTP_*` (sandi lewat `ERP_SMTP_PASSWORD`, disamarkan); dipindah ke settings platform di C3 tanpa mengubah `sdk/mail` |
| K-C1.7 | **Allowlist GORM (D3b)** menambah `internal/platform/identity/store` (dan kelak `authorization/store`) | Tambahkan di `internal/archcheck/rules.go`, `rules_test.go`, `CLAUDE.md` dan `AGENTS.md` (identik), PRD-000 §4.2, dan Q-number baru seperti Q35 |
| K-C1.8 | **Rute platform di luar `/api/{slug}`** (`/.well-known/jwks.json`, kelak `/oauth/token`) tak dapat didaftarkan modul | Didaftarkan platform langsung di router (`internal/app`), masuk allowlist `tests/security/public_routes.txt` dan `contracts/openapi/platform.yaml` |
| K-C1.9 | **Penghapusan stub**: `ERP_AUTHZ_STUB` dan `ERP_MODULES_ADMIN_API` | Pertahankan sampai C2 (gerbang mutasi modules/i18n admin bergantung authz penuh); C1 hanya mengganti authN dan aktor audit |

## Risiko lintas milestone

1. `Authorizer.Allow(ctx, permission) bool` terlalu sempit; C1 memperluas ke keputusan bertipe (`Allow`, `Unauthenticated`, `Forbidden`, `MFARequired`) tanpa mengubah modul (D5/D9).
2. `Authenticate` berjalan sebelum `tenantContext` (`httpserver/server.go:93-123`); klaim `tid` harus dicocokkan setelah tenant masuk context.
3. Tiap perubahan menyentuh frontend `web/` (guard, login, bootstrap) dan modul `core` berfrontend sendiri (`modules/core/web`, `desk` wajib).
4. Semua titik masuk ID baru (sesi, token, user) butuh tes IDOR pengguna A vs B (FR-119).
5. Dokumen yang harus ikut diperbarui tiap milestone: PRD-001 tabel keputusan, `docs/security/security-guidelines.md` §6 dan §7, `docs/module-sdk/security-primitives.md`, ADR-0013, `docs/architecture/api-guidelines.md` bila kode galat baru.

## Gerbang tiap milestone

`task lint arch check:gowork-off test`, `task test:integration` (Postgres VPS lewat tunnel SSH, `ERP_TEST_DATABASE_URL`), `task contracts:check`, `task web:check`, `task test:generator` bila menyentuh generator/template; review independen model lebih tinggi (kesesuaian + `security-audit` terfokus); verifikasi browser untuk alur login. Tanpa push atau PR kecuali diminta.

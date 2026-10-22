# ERPora — Aturan Proyek

Platform ERP modular: **Go modular monolith + React/TypeScript PWA**, satu binary dengan frontend ter-embed. Semua komunikasi dengan user dalam **Bahasa Indonesia**; identifier kode tetap bahasa Inggris.

## Sumber kebenaran (baca sebelum mengubah perilaku)

| Dokumen | Isi |
| --- | --- |
| `docs/prd/PRD-000-inisialisasi-project.md` | Struktur repo, Module Registry, SDK, CLI, observability, seam tenancy (Accepted) |
| `docs/prd/PRD-001-core.md` | Identity, RBAC, Settings, Audit (Accepted) |
| `docs/architecture/api-guidelines.md` | Bentuk WAJIB setiap kontrak & endpoint API |
| `docs/security/security-guidelines.md` | Pedoman keamanan (ISO/IEC 27001/27002, ASVS 5.0.0 L2 + L3 terpilih, OWASP, UU PDP): IDOR, injection, SSRF, sensitive data exposure, dll. |
| `docs/architecture/transaction-data-standards.md` | Standar transaksi & integritas data (ISO 8601/4217/15489, SoD, immutabilitas, ledger, pembulatan, retensi) |
| `docs/operations/deployment.md` | Deployment (draft) |
| `docs/adr/` | Keputusan arsitektur |
| `docs/architecture/` | Draft desain untuk PRD berikutnya (tenancy, offline, events, marketplace, roadmap); indeks & pemetaan rujukan lama di `docs/architecture/README.md` |

Rujukan "Blueprint §x" / "arsitektur §x" di PRD menunjuk dokumen awal yang sudah dihapus (asli ada di commit `c42d676`); lihat tabel pemetaan di `docs/architecture/README.md`. Dokumen **draft desain** bukan spesifikasi mengikat — hanya PRD, ADR, dan api-guidelines yang mengikat.

Bila permintaan bertentangan dengan PRD/ADR/api-guidelines, **tanyakan dulu** sebelum mengerjakan. Keputusan baru dicatat di tabel keputusan PRD terkait (Q-number) atau ADR.

## Alur kerja fitur baru

- **Fitur yang sudah tercakup PRD** → ikuti PRD; rencana implementasi di `docs/superpowers/plans/`.
- **Fitur baru di luar PRD** (modul bisnis baru, integrasi baru, perubahan arsitektur, pekerjaan 4+ file) → **wajib memakai Kiro spec** (skill `kiro-spec`, command `/spec`):
  - Lokasi: `.kiro/specs/<nama-spec>/` berisi `.config.kiro`, `requirements.md`, `design.md`, `tasks.md`.
  - Alur default requirements-first: `/spec-req` → `/spec-design` → `/spec-task`, lalu eksekusi dengan `/spec-status`.
  - Spec **harus patuh** pada PRD-000, PRD-001, api-guidelines, security-guidelines, dan transaction-data-standards; bagian design wajib menyebut resource/aksi permission, field group + klasifikasi data, scope, kontrak OpenAPI modul, migrasi, bagian **Keamanan** (STRIDE, titik masuk ID/IDOR, koneksi keluar/SSRF, file, data offline), dan — untuk resource transaksional — checklist `transaction-data-standards.md` §15.
  - Bug fix kecil (1–3 file) atau perubahan minor boleh langsung dikerjakan tanpa spec.

## Aturan arsitektur (ditegakkan otomatis — jangan dilanggar)

- Monorepo **multi Go module** (`go.work`): root `erpora` (platform + `cmd/erpora` + `internal/app`), `sdk/`, dan setiap `modules/<name>/` = Go module sendiri. Semua `go.mod`: `go 1.27`, `toolchain go1.27.3`.
- Modul hanya boleh mengakses modul lain lewat `modules/<x>/contract` **dan** hanya bila tercantum di `dependencies` manifest + `require` go.mod (D1, D8). Modul tidak boleh mengimpor root module; akses platform hanya lewat `sdk/` (D9).
- `internal/platform/**` tidak boleh mengimpor `modules/**` (D2); hanya `internal/app` yang merakit modul (D7).
- Frontend modul berada di dalam modulnya: `modules/<name>/web` (package `@erpora/module-<name>`). Komponen generik dari `@erpora/ui` (`packages/ui`), layanan frontend platform dari `@erpora/platform-web` — **bukan** dari modul Core. `web/` hanya app shell yang merakit modul. Dependency `package.json` modul harus sama dengan `manifest.dependencies` (F1–F3).
- Domain tanpa I/O (D3); GORM hanya di `*/internal/infrastructure`, `internal/platform/database`, `internal/platform/modules/store`, `sdk/data` (D3b); `AutoMigrate` dilarang — skema hanya lewat migrasi goose.
- Router: `net/http.ServeMux`; modul mendaftarkan route lewat `Registrar.Handle(method, path, permission, h)` / `Public(...)`, tidak pernah menerima router.
- Backend adalah otoritas: permission frontend hanya untuk UX.
- **Cegah IDOR di setiap titik masuk ID** (PRD-001 FR-119): handler memuat record hanya lewat `authz.Check`/`authz.Filter`; ID referensi di body juga dicek; record di luar scope → 404. **Implementasi `contract/` menegakkan otorisasi sendiri** terhadap principal di context (jangan percaya pemanggil); parameter kontrak tidak boleh membawa `tenantId`/`userId`; hak sistem hanya lewat `authz.AsSystem(ctx, reason)` yang teraudit. Setiap endpoint/kontrak yang menerima ID wajib punya test IDOR (user A vs B).
- Tenancy: mode single-tenant; selalu ambil tenant dari context (`tenancy.FromContext`, `sdk/data.DB(ctx)`), jangan pernah memegang pool DB langsung di modul.
- Detail aturan & penegaknya: PRD-000 §4.2 dan `internal/archcheck`.

## API & kontrak

- Ikuti `docs/architecture/api-guidelines.md`: URL kebab-case jamak + endpoint aksi (`POST …/{id}/submit`), JSON camelCase, amplop `data`/`error`, uang & kuantitas **string desimal**, record di luar scope → 404.
- **Tidak boleh ada breaking change API** (tanpa versi di URL): hanya menambah; perubahan struktur memakai expand → migrate → contract.
- Setiap modul punya kontrak sendiri `modules/<x>/openapi.yaml`; jalankan `erpora contracts:lint` dan `erpora contracts:generate` (tersedia mulai M2).

## Perintah

- Semua perintah lewat **Taskfile** (`task --list`). Developer CLI: `task erpora -- <command>` (build tag `devtools`).
- Gate lokal sebelum commit: `task lint arch check:gowork-off test` (+ `task test:integration` bila menyentuh DB, butuh Docker).
- Di Windows, `-race` dilewati (tidak ada cgo/gcc); CI Linux tetap menjalankannya.
- Jangan menambah perintah Taskfile yang memakai `rm`/`cp`/`sed` atau utilitas khusus Unix.

## Konvensi kerja

- TDD: tulis test yang gagal dulu, lalu implementasi.
- Satu commit per unit kerja yang bisa diuji; pesan commit konvensional (`feat(scope): …`, `fix:`, `docs:`, `chore:`).
- Jangan push atau membuat PR tanpa diminta.
- Temuan review: kerjakan semua yang sesuai spek (termasuk Minor); yang di luar/bertentangan dengan spek ditanyakan dulu (lihat CLAUDE.md global).

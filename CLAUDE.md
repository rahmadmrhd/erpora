# ERP Platform (erp-go) — Aturan Proyek

Platform ERP modular: **Go modular monolith + React/TypeScript PWA**, satu binary dengan frontend ter-embed. Semua komunikasi dengan user dalam **Bahasa Indonesia**; identifier kode tetap bahasa Inggris.

## Sumber kebenaran (baca sebelum mengubah perilaku)

| Dokumen | Isi |
| --- | --- |
| `docs/prd/PRD-000-inisialisasi-project.md` | Struktur repo, Module Registry, SDK, CLI, observability, seam tenancy (Accepted) |
| `docs/prd/PRD-001-core.md` | Identity, RBAC, Settings, Audit (Accepted) |
| `docs/architecture/api-guidelines.md` | Bentuk WAJIB setiap kontrak & endpoint API |
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
  - Spec **harus patuh** pada PRD-000, PRD-001, dan api-guidelines; bagian design wajib menyebut resource/aksi permission, field group, scope, kontrak OpenAPI modul, dan migrasi.
  - Bug fix kecil (1–3 file) atau perubahan minor boleh langsung dikerjakan tanpa spec.

## Aturan arsitektur (ditegakkan otomatis — jangan dilanggar)

- Monorepo **multi Go module** (`go.work`): root `github.com/rahmadmrhd/erp-go` (platform + `cmd/erp` + `internal/app`), `sdk/`, dan setiap `modules/<name>/` = Go module sendiri. Semua `go.mod`: `go 1.27`, `toolchain go1.27.2`.
- Modul hanya boleh mengakses modul lain lewat `modules/<x>/contract` **dan** hanya bila tercantum di `dependencies` manifest + `require` go.mod (D1, D8). Modul tidak boleh mengimpor root module; akses platform hanya lewat `sdk/` (D9).
- `internal/platform/**` tidak boleh mengimpor `modules/**` (D2); hanya `internal/app` yang merakit modul (D7).
- Frontend modul berada di dalam modulnya: `modules/<name>/web` (package `@erp/module-<name>`). Komponen generik dari `@erp/ui` (`packages/ui`), layanan frontend platform dari `@erp/platform-web` — **bukan** dari modul Core. `web/` hanya app shell yang merakit modul. Dependency `package.json` modul harus sama dengan `manifest.dependencies` (F1–F3).
- Domain tanpa I/O (D3); GORM hanya di `*/internal/infrastructure`, `internal/platform/database`, `internal/platform/modules/store`, `sdk/data` (D3b); `AutoMigrate` dilarang — skema hanya lewat migrasi goose.
- Router: `net/http.ServeMux`; modul mendaftarkan route lewat `Registrar.Handle(method, path, permission, h)` / `Public(...)`, tidak pernah menerima router.
- Backend adalah otoritas: permission frontend hanya untuk UX.
- Tenancy: mode single-tenant; selalu ambil tenant dari context (`tenancy.FromContext`, `sdk/data.DB(ctx)`), jangan pernah memegang pool DB langsung di modul.
- Detail aturan & penegaknya: PRD-000 §4.2 dan `internal/archcheck`.

## API & kontrak

- Ikuti `docs/architecture/api-guidelines.md`: URL kebab-case jamak + endpoint aksi (`POST …/{id}/submit`), JSON camelCase, amplop `data`/`error`, uang & kuantitas **string desimal**, record di luar scope → 404.
- **Tidak boleh ada breaking change API** (tanpa versi di URL): hanya menambah; perubahan struktur memakai expand → migrate → contract.
- Setiap modul punya kontrak sendiri `modules/<x>/openapi.yaml`; jalankan `erp contracts:lint` dan `erp contracts:generate` (tersedia mulai M2).

## Perintah

- Semua perintah lewat **Taskfile** (`task --list`). Developer CLI: `task erp -- <command>` (build tag `devtools`).
- Gate lokal sebelum commit: `task lint arch check:gowork-off test` (+ `task test:integration` bila menyentuh DB, butuh Docker).
- Di Windows, `-race` dilewati (tidak ada cgo/gcc); CI Linux tetap menjalankannya.
- Jangan menambah perintah Taskfile yang memakai `rm`/`cp`/`sed` atau utilitas khusus Unix.

## Konvensi kerja

- TDD: tulis test yang gagal dulu, lalu implementasi.
- Satu commit per unit kerja yang bisa diuji; pesan commit konvensional (`feat(scope): …`, `fix:`, `docs:`, `chore:`).
- Jangan push atau membuat PR tanpa diminta.
- Temuan review: kerjakan semua yang sesuai spek (termasuk Minor); yang di luar/bertentangan dengan spek ditanyakan dulu (lihat CLAUDE.md global).

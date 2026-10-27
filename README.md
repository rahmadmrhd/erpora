# ERPora

Platform ERP modular monolith dalam satu binary `erpora`, dengan modul bisnis sebagai Go module terpisah dan frontend React di satu app shell. Keputusan arsitektur ada di [`docs/adr/`](docs/adr/).

## Prasyarat

- **Go 1.27.3** (toolchain dipin di `go.mod`; Go akan mengunduhnya sendiri bila perlu)
- **Bun 1.4.2** (dipin di `package.json` dan `.bun-version`)
- **Docker** untuk PostgreSQL 18 lokal
- **go-task v3** sebagai entry point perintah. Pasang dengan `go install github.com/go-task/task/v3/cmd/task@v3.54.0`. Untuk OS lain lihat komentar di bagian atas `Taskfile.yml`.
- **golangci-lint v2.15.0** (versi terpin) untuk `task lint`. Dipasang otomatis oleh `task setup` ke `$(go env GOPATH)/bin`; pastikan folder itu ada di `PATH`. Di CI dipasang oleh `golangci-lint-action` dengan versi yang sama.

## Quick start

```sh
task setup   # go mod download, bun install, dan golangci-lint terpin (dibutuhkan `task lint`)
```

Salin `.env.example` menjadi `.env`:

- PowerShell: `Copy-Item .env.example .env`
- Git Bash, macOS, Linux: `cp .env.example .env`

Lalu jalankan:

```sh
task db:up
task migrate
task dev
```

Setelah `task dev` berjalan, API ada di `http://127.0.0.1:8080` (cek `/healthz` dan `/readyz`) dan web dev server di `http://127.0.0.1:5173`.

Daftar perintah lengkap: `task --list`.

### Perintah developer

```sh
task dev:setup                                   # migrate up + install semua modul (setelah task db:up)
task erpora -- doctor                            # periksa go, bun, task, docker, PostgreSQL, .env
task erpora -- make:module inventory --frontend  # modul baru yang langsung lolos semua gate
task erpora -- make:handler inventory Approve --route "POST /items/{id}/approve" --permission inventory.items.approve
task erpora -- db:fresh --seed                   # HAPUS data, bangun ulang (hanya development)
task contracts:check                             # lint kontrak, hasil generate mutakhir, breaking change
task test:generator                              # bukti make:module lolos semua gate (lama; butuh Docker)
```

Panduan: [`docs/module-sdk/generators.md`](docs/module-sdk/generators.md) dan [`docs/module-sdk/contracts.md`](docs/module-sdk/contracts.md).
`erpora` flavor dev tidak boleh dipakai di production; binary release (`task build`) tidak memuat developer tools.

## Peta folder

| Folder | Isi |
| --- | --- |
| `cmd/erpora` | Entry point CLI `erpora` (serve, migrate, version) |
| `internal/platform` | Runtime platform: config, database, migrate, httpserver, observability, tenancy, dan modul-modul platform |
| `internal/app` | Composition root: merakit platform dan modul |
| `internal/archcheck`, `tests/architecture` | Pemeriksaan aturan dependensi D1–D11 |
| `sdk/` | SDK publik Go (`module`, `data`), Go module tersendiri |
| `modules/<name>/` | Modul ERP, setiap folder adalah Go module sendiri. Saat ini ada dua modul referensi: `example` dan `exampleaddon` |
| `web/` | App shell frontend (Bun workspace `@erpora/web`) |
| `packages/` | Paket TypeScript bersama |
| `contracts/` | Kontrak OpenAPI dan JSON Schema milik platform |
| `migrations/` | Migrasi goose untuk target `control` dan `tenant` |
| `tests/` | Test architecture, integration, security, dan e2e |
| `infrastructure/` | Docker Compose dev, deployment, monitoring, backup |
| `docs/` | PRD, ADR, arsitektur, SDK modul, keamanan, operasi |

## Dokumentasi

- PRD: [`docs/prd/`](docs/prd/) (PRD-000 inisialisasi, PRD-001 core)
- ADR: [`docs/adr/`](docs/adr/)
- Arsitektur: [`docs/architecture/README.md`](docs/architecture/README.md)
- Panduan API: [`docs/architecture/api-guidelines.md`](docs/architecture/api-guidelines.md)
- Keamanan: [`docs/security/security-guidelines.md`](docs/security/security-guidelines.md), dan [`SECURITY.md`](SECURITY.md) untuk pelaporan kerentanan
- Operasi dan deployment: [`docs/operations/deployment.md`](docs/operations/deployment.md)
- Aturan kerja untuk developer dan agen AI: [`CLAUDE.md`](CLAUDE.md)

# erp-go

Platform ERP modular monolith dalam satu binary `erp`, dengan modul bisnis sebagai Go module terpisah dan frontend React di satu app shell. Keputusan arsitektur ada di [`docs/adr/`](docs/adr/).

## Prasyarat

- **Go 1.27.3** (toolchain dipin di `go.mod`; Go akan mengunduhnya sendiri bila perlu)
- **Bun 1.4.2** (dipin di `package.json` dan `.bun-version`)
- **Docker** untuk PostgreSQL 18 lokal
- **go-task v3** sebagai entry point perintah. Pasang dengan `go install github.com/go-task/task/v3/cmd/task@latest`. Untuk OS lain lihat komentar di bagian atas `Taskfile.yml`.

## Quick start

```sh
task setup
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

## Peta folder

| Folder | Isi |
| --- | --- |
| `cmd/erp` | Entry point CLI `erp` (serve, migrate, version) |
| `internal/platform` | Runtime platform: config, database, migrate, httpserver, observability, tenancy, dan modul-modul platform |
| `internal/app` | Composition root: merakit platform dan modul |
| `internal/archcheck`, `tests/architecture` | Pemeriksaan aturan dependensi D1–D11 |
| `sdk/` | SDK publik Go (`module`, `data`), Go module tersendiri |
| `modules/<name>/` | Modul ERP, setiap folder adalah Go module sendiri. Saat ini ada `example` |
| `web/` | App shell frontend (Bun workspace `@erp/web`) |
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

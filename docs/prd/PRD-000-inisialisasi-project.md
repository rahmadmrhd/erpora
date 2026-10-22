# PRD-000 — Inisialisasi Project & Module Registry (Go + React)

| Atribut        | Nilai                                                                 |
| -------------- | --------------------------------------------------------------------- |
| Status         | **Accepted** (21 Oktober 2026), semua pertanyaan Q1–Q23 diputuskan     |
| Tanggal        | 21 Oktober 2026                                                       |
| Fase roadmap   | Blueprint §14 fase **01 — Architecture foundation** + fase **03 — Module Registry & lifecycle** |
| Sumber acuan   | Dokumen awal Blueprint & arsitektur (sudah dihapus; materinya ada di `docs/architecture/`, pemetaan rujukan di `docs/architecture/README.md`) |
| Dokumen lanjut | **PRD-001 Core** (berikutnya), lalu PRD Tenancy, PRD Offline & Sync, PRD Notifications, dst. |

---

## 1. Latar belakang

Kita membangun **ERPora**, sebuah platform ERP, dan bukan sekadar aplikasi ERP. Platform ini menyediakan identitas, tenancy, module runtime, otorisasi, konfigurasi, event, migrasi, dan mekanisme ekstensi. Modul bisnis (Inventory, Purchasing, Sales, Finance, Assets) dibangun di atas fondasi tersebut.

Blueprint aslinya ditulis untuk Laravel. Prinsip arsitekturnya tetap berlaku. PRD ini hanya mengganti implementasinya ke stack berikut:

| Area            | Keputusan                                                                 |
| --------------- | ------------------------------------------------------------------------- |
| Arsitektur      | Modular Monolith · Clean Architecture selektif · DDD pada domain kompleks |
| Backend         | Go (stdlib `net/http`, `log/slog`, GORM di atas driver `pgx`)             |
| UI system       | Tailwind CSS + shadcn/ui (Radix)                                          |
| Frontend        | React + TypeScript · TanStack Router · TanStack Query · TanStack DB       |
| Bundler/runtime | Bun (dev server, bundler, test runner, package manager/workspaces)        |
| Offline         | PWA · Service Worker · Workbox · Cache Storage · IndexedDB                |
| Database        | PostgreSQL · **mode single-tenant di fase ini**; struktur disiapkan untuk Control DB + database-per-tenant |
| Deployment      | Satu Go binary dengan frontend assets yang di-embed (`go:embed`)          |
| Sinkronisasi    | Go API + client-side sync engine (outbox di IndexedDB)                    |
| Observability   | OpenTelemetry (traces, metrics, logs, error tracking)                     |
| Extensibility   | Module Registry · manifest · SDK · lifecycle · dependency resolver · version compatibility |
| Marketplace     | Signed immutable packages · publisher verification · permission declaration · isolated execution |

## 2. Tujuan

Hasil fase ini ada dua:
1. **Walking skeleton**: repository berjalan end-to-end dengan struktur, batas modul, tooling, observability, dan pipeline build yang final.
2. **Module Registry yang berfungsi penuh** untuk modul bawaan (build-time): manifest, dependency resolver, version compatibility, lifecycle persisten (install → enable → disable → upgrade → uninstall), migrasi per modul, dan aktivasi runtime.

Multi-tenancy **bukan fokus**. Yang dibuat hanya struktur dan *seam*-nya, supaya implementasi berikutnya tidak menyentuh modul.

### 2.1. Goals

1. **G1 — Struktur repository final**: sesuai Blueprint §5 dan arsitektur §7, diterjemahkan ke Go + Bun.
2. **G2 — Batas modul ditegakkan secara teknis**: compiler Go dan architecture test menolak pelanggaran dependensi di backend maupun frontend.
3. **G3 — Satu perintah build menghasilkan satu binary**: binary tersebut menyajikan API dan SPA/PWA.
4. **G4 — Developer experience**: satu perintah menjalankan lingkungan dev (Go API + Bun dev server + PostgreSQL), dan hot reload berjalan.
5. **G5 — Kontrak tersedia**: `module.schema.json`, SDK Go `sdk/module`, dan SDK TS `@erpora/module-sdk` divalidasi oleh dua modul referensi.
6. **G6 — CI quality gates**: format, lint, type check, unit test, architecture test, integration test, dan build berjalan di setiap PR.
7. **G7 — ADR**: ADR-001, 002, 003, 006, dan 007 final; sisanya stub.
8. **G8 — Module Registry & lifecycle**: modul dapat di-install, di-enable, di-disable, di-upgrade, dan di-uninstall dengan aman, teraudit, dapat di-retry, dan tanpa menghapus data (Blueprint §7, exit gate fase 03).
9. **G9 — Tenancy-ready**: semua kode tenant-scoped mengambil tenant dari context lewat resolver. Mengganti resolver single-tenant ke multi-tenant tidak mengubah modul.

### 2.2. Non-goals (di luar cakupan PRD ini)

- **Implementasi multi-tenancy**: tenant registry, provisioning database, secret management, tenant resolution berbasis hostname, backup per tenant → PRD Tenancy. Fase ini hanya membuat seam-nya (FR-21).
- Authentication, RBAC, policy engine → PRD Identity & Authorization. Middleware-nya disediakan sebagai stub.
- Offline data, outbox, sync protocol → PRD Offline & Sync. Fase ini hanya membuat SW app-shell dan kerangka kontrak.
- Signed packages, publisher verification, sandbox, runtime plugin → fase 06. Status `verified` dan field manifest-nya sudah dicadangkan.
- Settings schema modul, event subscriber, scheduled jobs → PRD Settings/Events. Slot di Registrar sudah dicadangkan.
- Purge data modul saat uninstall (Blueprint §7: uninstall tidak menghapus histori).
- Modul bisnis apa pun (Inventory, dan seterusnya).
- Design system lengkap. Yang dibuat hanya application shell minimal.
- Dashboard dan visualisasi observability serta backend penyimpanan telemetry production.
- Push notification (lihat §8).

## 3. Pengguna PRD

| Persona            | Kebutuhan                                                                     |
| ------------------ | ----------------------------------------------------------------------------- |
| Core developer     | Struktur jelas, build cepat, batas modul tidak bisa dilanggar tanpa sengaja    |
| Module developer   | Template modul, SDK, dan aturan lifecycle yang jelas                           |
| Admin/operator     | Melihat status modul, mengaktifkan/menonaktifkan, upgrade, dan memulihkan operasi yang gagal |
| DevOps/self-hoster | Satu binary + konfigurasi via env, health check, perintah migrate & modules    |

---

## 4. Struktur repository target

Monorepo **multi Go module** yang disatukan dengan `go.work`, ditambah **Bun workspaces** untuk kode TypeScript:

| Go module                                        | Lokasi              | Isi                                              | Versi/tag               |
| ------------------------------------------------ | ------------------- | ------------------------------------------------ | ----------------------- |
| `erpora`                    | root                | Platform (`internal/platform`), composition root (`internal/app`), `cmd/erpora`, `web` (embed) | `vX.Y.Z` (= versi platform) |
| `erpora/sdk`                | `sdk/`              | SDK publik (`module`, `data`)                    | `sdk/vX.Y.Z` (= `SDKVersion`) |
| `erpora/modules/<name>`     | `modules/<name>/`   | Satu ERP module                                  | `modules/<name>/vX.Y.Z` (= `version` di manifest) |

Arah dependensi Go module (tidak boleh dibalik): **root → modules/* → sdk**, dan **root → sdk**. `sdk` tidak bergantung pada siapa pun di repo ini.

```
erpora/
├── go.work                        # menyatukan root, sdk, dan semua modules/* (di-commit, bersama go.work.sum)
├── go.mod                         # module erpora (platform + composition root)
├── package.json                   # Bun workspaces root: ["web", "packages/*", "modules/*/web"]
├── bun.lock
├── Taskfile.yml                   # go-task: entry point semua perintah (lintas OS)
├── .env.example
├── README.md
│
├── cmd/
│   └── erpora/                       # SATU CLI ala Artisan (FR-03, FR-22)
│       ├── main.go                # serve | migrate | modules | inspect | <modul>:<cmd> | version
│       ├── devtools_on.go         # //go:build devtools  → mendaftarkan developer tools
│       └── devtools_off.go        # //go:build !devtools → no-op (binary release)
│
├── internal/
│   ├── devtools/                  # HANYA di build -tags devtools (FR-22)
│   │   ├── commands/              # make:*, db:*, module:check, contracts:generate, doctor
│   │   └── templates/             # template generator (di-embed)
│   │
│   ├── app/                       # composition root: wiring platform + modul
│   │   ├── app.go
│   │   └── modules.go             # daftar modul yang dikompilasi (build-time registry)
│   │
│   ├── platform/                  # runtime platform; TIDAK boleh import modules/*
│   │   ├── config/                # load & validasi konfigurasi (env)
│   │   ├── database/              # GORM (driver pgx), transaksi, ControlDB vs TenantDB, TenantConnector
│   │   ├── migrate/               # runner goose: control, tenant, per-modul
│   │   ├── httpserver/            # server, middleware chain, error envelope, SPA handler
│   │   ├── observability/         # OTel SDK, slog bridge, error recovery, helper propagasi (FR-17)
│   │   ├── tenancy/               # SEAM: TenantID, context, Resolver, SingleTenantResolver (FR-21)
│   │   ├── identity/              # (stub) authN middleware
│   │   ├── authorization/         # (stub) permission check middleware
│   │   ├── modules/               # Module Registry (FR-05, FR-18–FR-20)
│   │   │   ├── manifest/          # parsing + validasi JSON Schema
│   │   │   ├── registry/          # katalog modul terkompilasi, route table, capability table
│   │   │   ├── resolver/          # dependency graph, topo sort, cycle detection
│   │   │   ├── compatibility/     # constraint SemVer platform/SDK/dependency
│   │   │   ├── lifecycle/         # state machine, operations, reconcile, hooks
│   │   │   ├── store/             # persistensi installations/operations/events (GORM)
│   │   │   ├── runtime/           # snapshot status aktif + revision + guard middleware
│   │   │   └── admin/             # handler API /api/platform/modules + perintah CLI
│   │   ├── settings/              # (stub)
│   │   ├── events/                # (stub) event bus + outbox
│   │   ├── audit/                 # (stub) audit umum; lifecycle modul memakai tabel sendiri dulu
│   │   ├── files/                 # (stub)
│   │   ├── sync/                  # (stub) sync engine server-side
│   │   ├── bootstrap/             # GET /api/bootstrap
│   │   └── extensions/            # (stub) packages, signing, sandbox — fase 06
│
├── modules/                       # ERP modules — SETIAP folder adalah Go module tersendiri
│   ├── example/                   # go.mod: .../modules/example (lihat §4.1)
│   └── exampleaddon/              # go.mod: .../modules/exampleaddon; require modules/example (FR-15)
│
├── sdk/                           # PUBLIC Go SDK — Go module tersendiri (go.mod: .../sdk)
│   ├── go.mod
│   ├── module/
│   │   ├── module.go              # interface Module + hook lifecycle opsional
│   │   ├── manifest.go            # tipe Manifest
│   │   ├── registrar.go           # interface Registrar (Handle, Public)
│   │   └── version.go             # SDKVersion
│   └── data/
│       └── data.go                # akses TenantDB untuk lapisan infrastructure modul
│
├── web/                           # APP SHELL saja: Bun workspace "@erpora/web" DAN Go package (embed)
│   ├── embed.go                   # //go:embed all:dist
│   ├── dist/.gitkeep              # placeholder agar `go build` tanpa build frontend tetap lolos
│   ├── package.json               # dependencies: @erpora/ui, @erpora/platform-web, @erpora/module-* (semua modul yang dikompilasi)
│   ├── tsconfig.json
│   ├── build.ts                   # Bun.build app + service worker + Workbox injectManifest
│   ├── dev.ts                     # Bun dev server + proxy /api → Go
│   ├── public/                    # manifest.webmanifest, icons
│   └── src/
│       ├── main.tsx
│       ├── index.html
│       ├── styles.css             # Tailwind; @source ke packages/ui dan modules/*/web
│       ├── sw.ts                  # service worker (Workbox)
│       └── app/
│           ├── modules.ts         # daftar modul frontend yang dirakit (penanda erp:generated:modules)
│           ├── router/            # root route + agregasi route modul
│           ├── bootstrap/         # fetch /api/bootstrap, snapshot, revision
│           ├── providers/         # QueryClient, DB, Router providers
│           └── settings/modules/  # halaman manajemen modul (FR-20)
│
├── packages/                      # paket TS bersama milik PLATFORM (bukan milik modul mana pun)
│   ├── ui/                        # "@erpora/ui": design system (shadcn), komponen generik tanpa pengetahuan bisnis
│   ├── platform-web/              # "@erpora/platform-web": registry modul, api client, permissions (UX), tenancy scope,
│   │                              #   offline, sync (stub), TanStack DB, observability (OTel Web lazy)
│   ├── platform-contracts/        # tipe hasil generate dari contracts/openapi/ (common + platform)
│   ├── module-sdk/                # defineModule(), tipe FrontendModule & OfflinePolicy
│   └── ui-contracts/              # kontrak UI extension (stub)
│
├── contracts/                     # kontrak MILIK PLATFORM + aturan bersama (bukan kontrak modul)
│   ├── openapi/
│   │   ├── common.yaml            # komponen bersama: amplop data/error/page, Decimal, UUID, Timestamp, header
│   │   └── platform.yaml          # endpoint platform: /api/bootstrap, /api/platform/*
│   ├── spectral.yaml              # ruleset linter OpenAPI (FR-23)
│   ├── spectral-functions/        # fungsi kustom linter (TS, dijalankan Bun)
│   └── module.schema.json
│
├── migrations/
│   ├── control/                   # metadata platform: module_installations, module_operations, ...
│   └── tenant/                    # platform pada tenant DB: module_migrations, ...
│
├── tests/
│   ├── architecture/              # aturan dependensi (Go)
│   ├── integration/               # PostgreSQL nyata (testcontainers): lifecycle, migrasi
│   ├── security/                  # route publik allowlist; (nanti) tenant isolation
│   └── e2e/                       # Playwright (skeleton + alur enable/disable modul)
│
├── infrastructure/
│   ├── docker/                    # docker-compose.dev.yml (PostgreSQL, collector), Dockerfile
│   ├── deployment/                # (stub) profil SaaS / self-hosted
│   ├── monitoring/                # otel-collector.yaml, alerts/ (FR-17); dashboard menyusul
│   └── backup/                    # (stub)
│
├── docs/
│   ├── prd/
│   ├── adr/
│   ├── architecture/
│   ├── module-sdk/
│   ├── security/
│   └── operations/
│
└── .github/workflows/ci.yml
```

Folder bertanda **(stub)** hanya berisi `doc.go` (Go) atau `README.md` yang menjelaskan tanggung jawab folder dan PRD yang akan mengisinya. Tujuannya agar struktur bisa direview sekarang tanpa kode spekulatif.

### 4.1. Struktur internal modul backend

Blueprint §5.1 diterjemahkan ke Go. Setiap ERP module adalah **Go module tersendiri**, dan detail implementasinya berada di `internal/` milik module itu. Batasnya ditegakkan oleh compiler di dua lapis: `internal/` dan daftar `require` di `go.mod`.

```
modules/<name>/
├── go.mod                   # module erpora/modules/<name>
│                            # require: .../sdk + HANYA modul yang ada di manifest.dependencies
│                            # replace ke path relatif untuk sibling module yang belum dirilis
├── module.go                # implementasi sdk/module.Module (+ hook lifecycle opsional)
├── module.json              # manifest (di-embed), divalidasi terhadap contracts/module.schema.json
├── openapi.yaml             # KONTRAK API MILIK MODUL INI (FR-23); di-embed, hanya path /api/{slug}/…
├── contract/                # PUBLIC port modul untuk modul lain (interface + DTO)
├── migrations/              # migrasi tenant milik modul (di-embed, versioned)
├── README.md                # deskripsi, capability, dependency, catatan upgrade
├── web/                     # FRONTEND MILIK MODUL INI — Bun workspace package "@erpora/module-<name>"
│   ├── package.json         # dependencies: @erpora/ui, @erpora/platform-web, @erpora/module-sdk,
│   │                        #   + @erpora/module-<x> HANYA untuk modul di manifest.dependencies
│   ├── index.ts             # SATU-SATUNYA public entry frontend modul
│   ├── module.ts            # frontend manifest (routes, navigasi, offline policy UI)
│   ├── api/contract.gen.ts  # tipe & client hasil generate dari ../openapi.yaml
│   ├── routes/              # halaman (lazy)
│   └── components/          # komponen khusus modul
└── internal/                # TIDAK dapat diimpor dari luar modules/<name>/
    ├── domain/              # entity, value object, domain event, policy (tanpa I/O)
    ├── application/         # command/query handler, use case
    ├── infrastructure/      # model GORM + repository, integrasi, messaging
    └── presentation/
        └── http/            # handler, request/response DTO
```

Clean Architecture bersifat **selektif** (Blueprint §5.1). Modul CRUD sederhana boleh meringkas `application`/`domain`. Satu-satunya yang tidak boleh dilanggar adalah aturan `internal/`, karena itu yang menjamin batas modul.

**Frontend modul berada di dalam modulnya** (`modules/<name>/web/`, package `@erpora/module-<name>`), sehingga satu modul = satu folder mandiri (backend, frontend, kontrak, migrasi) yang ter-versi dan ter-tag bersama. Modul tetap **dibundel saat build** oleh app shell `web/` (build-time module, §7.5); co-location hanya memindahkan lokasi source. Komponen generik berasal dari `@erpora/ui` dan layanan frontend platform dari `@erpora/platform-web`, keduanya milik platform, bukan milik modul Core.

**Versi & rilis modul**:
- `version` di `module.json` **harus sama** dengan tag Go module (`modules/<name>/vX.Y.Z`). Ini diperiksa oleh CI saat tag dibuat.
- Modul dengan versi MAJOR ≥ 2 mengikuti aturan Go (`.../modules/<name>/v2`). Aturan ini didokumentasikan di `docs/module-sdk/versioning.md`, tetapi belum dibutuhkan di fase ini.
- Binary platform tetap memuat modul pada **build-time** (`internal/app/modules.go` + `require` di root `go.mod`). Enable/disable per modul terjadi di runtime (FR-19), tidak bergantung pada pembagian Go module.

### 4.2. Aturan dependensi (wajib, diuji otomatis)

| #   | Aturan                                                                                  | Penegak             |
| --- | --------------------------------------------------------------------------------------- | ------------------- |
| D1  | Modul lain hanya boleh mengimpor `modules/<x>/contract`; `modules/<x>/internal/**` tertutup | Compiler (`internal/`) |
| D2  | `internal/platform/**` tidak boleh mengimpor `modules/**`                               | Arch test (root `go.mod` me-`require` semua modul demi composition root, jadi compiler tidak mencegahnya) |
| D3  | `*/internal/domain` tidak boleh mengimpor `net/http`, `gorm.io/**`, `pgx`, `database/sql`, `infrastructure`, `presentation` | Arch test |
| D3b | `gorm.io/**` hanya boleh diimpor oleh `modules/*/internal/infrastructure/**`, `internal/platform/database`, `internal/platform/modules/store`, dan `sdk/data` | Arch test |
| D4  | `*/internal/application` tidak boleh mengimpor `presentation` atau `infrastructure`     | Arch test           |
| D5  | `sdk` tidak boleh mengimpor root module maupun `modules/*`                              | Compiler (`sdk/go.mod` tidak me-`require` keduanya; `internal/` root tidak bisa diimpor dari Go module lain) |
| D6  | `contract/` modul tidak boleh mengimpor `internal/` modul lain                          | Compiler            |
| D7  | Hanya `internal/app` yang boleh mengimpor seluruh modul (composition root)              | Arch test           |
| D8  | Modul hanya boleh mengimpor modul yang tercantum di `dependencies` manifest-nya          | Compiler: `GOWORK=off go build ./...` per modul (impor tanpa `require` gagal) + arch test: `require` di `go.mod` == `dependencies` di `module.json` |
| D9  | `modules/**` tidak boleh mengimpor root module (`internal/platform/**`, `internal/app`); akses platform hanya lewat `sdk` | Compiler (`internal/` root tidak bisa diimpor dari Go module lain) + arch test (`go.mod` modul tidak boleh me-`require` root module) |
| D10 | `modules/**` dan `sdk` tidak boleh mengimpor library router pihak ketiga                | Arch test           |
| D11 | Dependensi antar-Go-module tidak boleh siklik (`go.mod` require graph)                  | Compiler/`go mod` + arch test |
| F1  | `@erpora/ui`, `@erpora/platform-web`, `@erpora/module-sdk` tidak boleh bergantung pada `@erpora/module-*` | `package.json` (Bun workspace) + dependency-cruiser |
| F2  | Frontend modul hanya boleh mengimpor `@erpora/ui`, `@erpora/platform-web`, `@erpora/module-sdk`, `@erpora/platform-contracts`, dan `@erpora/module-<x>` yang ada di `manifest.dependencies`; antarmodul hanya lewat `index.ts` (deep import dilarang) | `package.json` + dependency-cruiser + arch test: dependencies `package.json` modul == `manifest.dependencies` |
| F3  | Hanya app shell `web/` yang boleh bergantung pada semua `@erpora/module-*` dan merakitnya   | `package.json` + dependency-cruiser |

D8 dan D9 membuat graf dependensi kode **identik** dengan graf dependensi di manifest. Resolver (FR-18) pun bisa dipercaya: tidak ada dependensi tersembunyi yang lolos dari pemeriksaan lifecycle.

---

## 5. Kebutuhan fungsional

Setiap kebutuhan memiliki kriteria penerimaan (AC) yang dapat diverifikasi.

### FR-01 — Inisialisasi repository & tooling

- Git repository, `.gitignore` (Go, Bun, `web/dist/*` kecuali `.gitkeep`, `.env`), `.editorconfig`, `.gitattributes` (LF).
- `go.work` di root (di-commit bersama `go.work.sum`) mencakup root, `sdk`, dan setiap `modules/*`. Semua `go.mod` memakai versi Go dan `toolchain` yang sama, dikunci ke rilis stabil terbaru saat inisialisasi. Konsistensinya diperiksa oleh `task check:gomod`.
- Taskfile menyediakan task yang berjalan untuk **setiap Go module** (`task tidy`, `task lint`, `task test`, `task vuln`), sehingga developer tidak perlu masuk ke setiap folder. Pembuatan kode (modul baru, migrasi, dsb.) memakai developer tools di CLI `erp` flavor dev (FR-22), bukan Taskfile. Task `task erpora -- <command>` menjalankan `go run -tags devtools ./cmd/erpora <command>`, dan `task build:dev` menghasilkan `bin/erpora{{exeExt}}` flavor dev.
- Root `package.json` dengan Bun workspaces dan versi Bun dikunci (`packageManager` / `.bun-version`).
- Go module path: **`erpora`**.
- **`Taskfile.yml` ([go-task](https://taskfile.dev))** sebagai satu-satunya entry point perintah. Alasan pemilihan: go-task adalah satu binary Go dan menjalankan perintah lewat interpreter shell POSIX bawaannya (`mvdan/sh`). Akibatnya `task dev` berperilaku sama di PowerShell, cmd, Git Bash, WSL, macOS, dan Linux tanpa `make` atau bash.
  - Bagian atas `Taskfile.yml` **wajib** berisi komentar cara instalasi dan cara menjalankan per OS/terminal:
    - Semua OS (sudah ada Go): `go install github.com/go-task/task/v3/cmd/task@latest`
    - Tanpa install global (sudah ada Bun): `bunx @go-task/cli <task>`
    - Windows: `winget install Task.Task` atau `scoop install task`. Pemanggilannya sama di PowerShell, cmd, dan Git Bash: `task <nama>`
    - macOS: `brew install go-task`
    - Linux: `sh -c "$(curl -fsSL https://taskfile.dev/install.sh)" -- -d -b ~/.local/bin`, atau paket distro/snap
  - Setiap task memiliki `desc`. Task yang perilakunya berbeda per OS memakai `platforms:` atau `{{OS}}`/`{{exeExt}}` (mis. `bin/erpora{{exeExt}}`).
  - Perintah di dalam task tidak memakai utilitas khusus Unix (`rm -rf`, `cp`, `sed`). Operasi file ditulis sebagai skrip Go/Bun kecil, atau memakai fitur bawaan Taskfile.

**AC:** `task --list` menampilkan semua task beserta deskripsinya. Clone baru lalu `task setup` memasang semua dependensi tanpa langkah manual lain (selain Docker), dan ini diverifikasi di PowerShell (Windows) serta bash (Linux/macOS).

### FR-02 — Struktur folder sesuai §4

**AC:** semua folder di §4 ada. Setiap folder stub memiliki `doc.go`/`README.md` yang menjelaskan tanggung jawabnya. `go build` dan `go vet` lolos untuk setiap Go module, baik dalam workspace mode maupun dengan `GOWORK=off`.

### FR-03 — Backend binary & subcommand

`cmd/erpora` menyediakan:

| Perintah                     | Perilaku                                                                 |
| ---------------------------- | ------------------------------------------------------------------------ |
| `erpora serve`                  | Reconcile status modul (FR-19), lalu menjalankan HTTP server (API + SPA)  |
| `erpora migrate up`             | Migrasi platform: target `control` lalu `tenant` (tenant default)        |
| `erpora migrate status`         | Status migrasi platform dan per modul                                    |
| `erpora modules <subcommand>`   | Manajemen modul (FR-20)                                                  |
| `erpora inspect routes [--module <key>] [--json]` | Tabel route dari `Registry.Routes()`: method, pattern, permission, public, modul |
| `erpora inspect permissions [--module <key>]` | Daftar capability per modul                                   |
| `erpora inspect config`         | Konfigurasi efektif dengan nilai rahasia disensor (`ERP_DATABASE_URL` → `postgres://erp:****@...`), flavor build, dan penanda lingkungan database (FR-22.5) |
| `erpora <slug>:<command>`       | Command runtime yang didaftarkan modul (FR-22.4)                         |
| `erpora version`                | Versi platform, versi SDK, commit, waktu build (di-inject via `-ldflags`) |

- CLI dibangun dengan **`spf13/cobra`**: help per command, `erpora list` untuk melihat semua command (setara `php artisan list`), dan shell completion (`erpora completion bash|zsh|fish|powershell`). Tipe cobra **tidak** diekspos ke modul (lihat FR-22.4), mengikuti prinsip yang sama dengan router di FR-05.
- Semua command `inspect` bersifat read-only, sehingga aman dijalankan di production.

- Konfigurasi dibaca dari environment variable (prefix `ERP_`) dengan validasi saat startup. Bila ada yang tidak valid, proses gagal cepat dengan pesan yang jelas. Variabel minimum: `ERP_DATABASE_URL` (wajib), `ERP_ENV` (`development | production`, default `production`), `ERP_HTTP_ADDR` (default `127.0.0.1:8080`, yaitu hanya localhost karena diasumsikan berada di belakang reverse proxy), `ERP_LOG_FORMAT` (`json | text`), ditambah variabel FR-17, FR-20, dan FR-21. PRD-001 menambahkan `ERP_SECRET_KEY` (wajib sejak PRD-001).
- Opsional: `erpora serve` dapat membaca file env lewat flag `--env-file <path>`. Ini memudahkan systemd/Windows service tanpa mengekspor variabel secara global.
- Graceful shutdown pada SIGINT/SIGTERM.
- Logging terstruktur `log/slog` (JSON di production, text di dev) dengan `request_id`.

**AC:** `erpora version` menampilkan keempat nilai. `erpora serve` tanpa `ERP_DATABASE_URL` gagal dengan pesan yang menyebut variabel tersebut. Shutdown menunggu request aktif sampai batas timeout.

### FR-04 — HTTP server & endpoint dasar

| Endpoint             | Keterangan                                                                  |
| -------------------- | --------------------------------------------------------------------------- |
| `GET /healthz`       | Liveness, tanpa dependensi eksternal                                        |
| `GET /readyz`        | Readiness: koneksi PostgreSQL + migrasi platform sudah diterapkan           |
| `GET /api/bootstrap` | Bootstrap manifest: `appVersion`, `tenantId` (tenant default), `modules[]` dari runtime snapshot (FR-19: `key`, `slug`, `version`, `status`, `basePath`, `offline`), `permissions: []`, `revision`. Header `ETag` = revision |
| `GET /*`             | SPA handler: file statis dari embed. Path tanpa ekstensi dan bukan `/api/*` diarahkan ke `index.html` |

- Router: `net/http.ServeMux` dengan pola method+path (Go ≥ 1.22). Tidak memakai framework router. ServeMux adalah **detail implementasi platform** dan tidak pernah diekspos ke modul (lihat FR-05). Karena itu platform bisa beralih ke router lain (mis. chi) tanpa breaking change pada modul.
- **Semua endpoint WAJIB mengikuti [`docs/architecture/api-guidelines.md`](../architecture/api-guidelines.md)**: struktur URL, amplop `data`/`error`, camelCase, desimal sebagai string, pagination, filter, `If-Match`, `Idempotency-Key`, dan katalog kode error. Ini berlaku juga untuk `/api/bootstrap` dan endpoint platform.
- Error API memakai envelope JSON yang konsisten (`{"error": {"code", "message", "details", "requestId", "traceId"}}`, didefinisikan di `contracts/openapi/common.yaml`).
- Header cache: aset ber-hash → `immutable, max-age=31536000`; `index.html` dan `sw.js` → `no-cache`.
- `/api/*` yang tidak ditemukan mengembalikan 404 JSON, **bukan** `index.html`.

**AC:** keempat endpoint lulus integration test. `curl /api/unknown` menghasilkan 404 JSON. `curl /inventory/products` mengembalikan `index.html`. `sw.js` dikirim dengan `Cache-Control: no-cache`. `GET /api/bootstrap` dengan `If-None-Match` revision terkini menghasilkan 304.

### FR-05 — Kontrak modul (SDK Go) & registry

Definisikan di `sdk/module` (kontrak publik, mengikuti arsitektur §2 dan lima kontrak Blueprint §6). **Modul tidak menerima router.** Modul mendeklarasikan route lewat API milik SDK, dan platform yang memasangnya. Ini menyimpang secara sengaja dari contoh `RegisterRoutes(mux *http.ServeMux)` di dokumen arsitektur awal (disetujui; dicatat di ADR-006).

```go
// Runtime contract
type Module interface {
    Manifest() Manifest
    Register(r Registrar) error
}

type Registrar interface {
    // Route terproteksi: permission WAJIB dan harus ada di manifest.capabilities modul ini.
    Handle(method, path, permission string, h http.Handler)
    // Route tanpa autentikasi/otorisasi; harus eksplisit dan diaudit.
    Public(method, path string, h http.Handler)
    // Command CLI runtime milik modul (FR-22.4); nama otomatis berprefiks slug.
    Command(c Command)
    // Dicadangkan: Settings, EventSubscribers, Jobs (PRD Settings/Events).
}

// Lifecycle contract — semua opsional, dideteksi lewat type assertion.
type MigrationSource interface { Migrations() fs.FS }
type Installer      interface { Install(ctx context.Context, env Env) error }
type Upgrader       interface { Upgrade(ctx context.Context, env Env, from, to string) error }
type EnableHook     interface { OnEnable(ctx context.Context, env Env) error }
type DisableHook    interface { OnDisable(ctx context.Context, env Env) error }
type HealthChecker  interface { Health(ctx context.Context, env Env) error }
// Data contoh untuk development; hanya dipanggil oleh `erpora db:seed` (flavor dev), tidak pernah di production.
type DevSeeder      interface { SeedDev(ctx context.Context, env Env) error }

// Env: konteks terbatas yang diberikan platform ke hook (tenant ID, logger, akses data
// lewat sdk/data). BUKAN akses ke container/platform internal (Blueprint §6.2).
```

- **Capability contract** = `manifest.capabilities`. **Integration contract** = package `contract/` + `manifest.provides`.
- Modul hanya bergantung pada `net/http` stdlib (`http.Handler`, `*http.Request`) dan tidak pada library router mana pun.
- Permission dideklarasikan satu kali di manifest (`capabilities`), sehingga manifest menjadi satu-satunya sumber deklarasi.
- `path` ditulis relatif terhadap prefix modul. Platform memasangnya di `/api/{slug}{path}` (`slug` dari manifest, FR-06), jadi modul tidak bisa mendaftarkan route di luar namespace-nya.
- Saat registrasi, platform membungkus setiap handler dengan rantai middleware yang urutannya tetap:
  `recover → otel (FR-17) → request-id/log → authN → tenant context (FR-21) → module status guard (FR-19) → permission check → handler`.
  Di fase ini authN dan permission check masih stub yang dapat dikonfigurasi (allow-all di dev, terlihat di log). Implementasinya menyusul di PRD masing-masing **tanpa mengubah modul**.
- Route `Public` melewati authN dan permission, tetapi tetap melewati recover, otel, log, tenant context, dan module status guard.
- `platform.core` adalah pseudo-modul milik platform (slug `platform`, versi = versi platform, selalu `enabled`). Semua modul bergantung padanya, dan route manajemen modul (FR-20) berada di namespace-nya.

Registry (`internal/platform/modules/registry`) memvalidasi saat startup:
1. Menolak `key` modul duplikat dan `slug` duplikat.
2. Memvalidasi manifest terhadap `contracts/module.schema.json`.
3. Menolak capability key duplikat antarmodul, dan capability yang tidak berprefiks `slug` modul (mis. `example.*`).
4. Menolak `Handle` dengan permission kosong atau yang tidak dideklarasikan di `capabilities` modul itu.
5. Menolak route konflik (method + path setelah prefix) dengan error yang menyebut **kedua modul** yang bentrok, bukan sekadar panic ServeMux.
6. Menyediakan tabel route read-only `Registry.Routes() []RouteInfo{Module, Method, Pattern, Permission, Public}` untuk audit, security test, dan pembuatan dokumentasi/OpenAPI.
7. Semua kesalahan dikumpulkan dan dilaporkan sekaligus (bukan berhenti di error pertama), dengan nama modul dan field terkait.

Pemeriksaan dependency dan kompatibilitas ada di FR-18.

**AC:**
- Unit test untuk ketujuh aturan di atas.
- Security test: setiap entri `Registry.Routes()` memiliki permission non-kosong **atau** `Public: true`. Daftar route publik dibandingkan dengan allowlist di `tests/security/public_routes.txt`, sehingga route publik baru wajib di-review.
- Test urutan middleware: handler modul menerima context yang sudah berisi request-id, span aktif, tenant ID, dan `erp.module.id`.

### FR-06 — Manifest schema

`contracts/module.schema.json` (JSON Schema 2020-12) mencakup field Blueprint §6.1 tanpa field PHP/Laravel:

`schema_version`, `key` (pola `publisher.name`), `slug` (kebab-case, unik; dipakai sebagai prefix API `/api/{slug}` dan `basePath` frontend `/{slug}`), `name`, `version` (SemVer), `publisher {id, name}`, `trust` (enum `builtin | verified | untrusted`; fase ini hanya `builtin` yang diterima), `compatibility {platform, sdk}` (constraint SemVer), `dependencies[] {module, version, optional}`, `capabilities[]`, `provides[]`, `migrations {strategy: "versioned" | "none"}`, `offline {mode, ...}` (enum mode: `online-only | offline-read | offline-draft | offline-write`).

Field marketplace (`artifact`, `checksum`, `signature`, `requested_permissions`, `license`) **didefinisikan sebagai opsional** sekarang agar format tidak berubah breaking di fase 06.

> **Perluasan oleh PRD-001** (aditif, `schema_version` tetap `1`): `required`, `resources` (aksi, aksi kustom, `owner_field`, `scope_fields`, `field_groups`, `audit`), `scope_dimensions`, `default_roles`, dan `settings`. `capabilities` tetap diterima dan diturunkan otomatis dari `resources`. Schema di M2 PRD-000 sebaiknya sudah menyiapkan field-field ini sebagai opsional.

**AC:** schema memvalidasi manifest `example` dan `exampleaddon`. Test negatif membuktikan bahwa versi non-SemVer, `key` tanpa publisher, constraint SemVer tidak valid, dan `trust` selain `builtin` ditolak. Tipe TS di `packages/module-sdk` dibangkitkan atau divalidasi dari schema yang sama.

### FR-07 — Database & migrasi

- `infrastructure/docker/docker-compose.dev.yml` menjalankan PostgreSQL (versi major dikunci) dengan **satu database dev `erp`**. Pada mode single-tenant, control DB dan tenant DB default adalah **database fisik yang sama**, tetapi diakses lewat tipe dan target migrasi yang terpisah (FR-21).
- Akses data: **GORM** (`gorm.io/gorm` + `gorm.io/driver/postgres`, yang memakai `pgx/v5`). Guardrail wajib:
  1. **GORM hanya di lapisan infrastructure** (aturan D3/D3b). Model GORM (struct bertag `gorm:`) terpisah dari entity domain, dan repository melakukan mapping. Modul CRUD sederhana boleh memakai model GORM langsung di `application` hanya jika ADR modulnya mencatat pengecualian itu.
  2. **`AutoMigrate` dilarang** di luar test. Skema dikelola oleh migrasi SQL versioned (goose). Arch test atau linter menolak pemanggilan `AutoMigrate` di kode non-test.
  3. **Tipe koneksi terpisah**: `database.ControlDB` dan `database.TenantDB` membungkus `*gorm.DB`, sehingga keduanya tidak bisa tertukar saat compile.
  4. **Transaksi eksplisit**: operasi kritis memakai `db.Transaction(...)` dan locking eksplisit (`clause.Locking{Strength: "UPDATE"}`). Query kompleks (laporan, CTE) boleh memakai `db.Raw` dengan parameter binding, tanpa string concatenation.
  5. **Observabilitas query**: logger GORM diarahkan ke `slog`, mencatat slow query (ambang dapat dikonfigurasi), tanpa nilai parameter SQL. **(M2, keputusan Q23)** `PrepareStmt` aktif di semua mode; di mode development, log memberi peringatan bila satu request menjalankan lebih dari N query dengan bentuk SQL yang sama (indikasi N+1; N dapat dikonfigurasi, default 10).
  6. **Preload, bukan lazy load**: relasi selalu dimuat eksplisit (`Preload`/`Joins`). Relasi lintas modul dilarang (Blueprint §3.3), sehingga model GORM hanya punya asosiasi di dalam modulnya sendiri.
- Tool migrasi: `goose` (Provider API, migrasi di-embed), dipisah per target dengan tabel versi masing-masing:

| Target   | Sumber                              | Tabel versi                 | Dijalankan oleh                 |
| -------- | ----------------------------------- | --------------------------- | ------------------------------- |
| control  | `migrations/control/`               | `goose_control`             | `erpora migrate up`                |
| tenant   | `migrations/tenant/`                | `goose_tenant`              | `erpora migrate up` (tenant default) |
| modul    | `modules/<x>/migrations/`           | `goose_module_<slug_snake>` (slug `example-addon` → `goose_module_example_addon`) | Lifecycle install/upgrade (FR-19) |

- Migrasi platform awal:
  - **control**: `platform_state`, `tenant_state`, `module_installations`, `module_operations`, `module_lifecycle_events`. Detail di FR-19.
  - **tenant**: `module_migrations` (Blueprint §13.2: `module_key`, `module_version`, `migration_key`, `checksum`, `applied_at`, `execution_status`) sebagai catatan audit di samping tabel versi goose.
- Migrasi **tidak pernah** berjalan otomatis pada HTTP request maupun saat `serve` (Blueprint §13.2). Migrasi hanya berjalan lewat `erpora migrate` dan operasi lifecycle.

**AC:** `task db:up` lalu `task migrate` lolos pada DB kosong. Menjalankannya ulang menghasilkan no-op. `erpora migrate status` menampilkan versi tiap target, termasuk tiap modul yang terpasang. `erpora serve` pada DB yang migrasi platformnya belum diterapkan gagal dengan pesan "jalankan erpora migrate up".

### FR-08 — Frontend application shell

- React + TypeScript `strict` (plus `noUncheckedIndexedAccess`).
- **TanStack Router** dengan **code-based routing**. Setiap modul mengekspor fungsi registrasi route yang menerima parent route bertipe spesifik (bukan `AnyRoute`). `app/router` mengagregasi route modul secara statis saat build. Halaman modul memakai `.lazy()` untuk code splitting.
- **Route guard**: route modul yang statusnya bukan `enabled` di bootstrap menampilkan halaman "Modul tidak aktif" (UX saja; backend tetap menolak, FR-19).
- **TanStack Query**: satu `QueryClient`. Query key berprefiks scope `[tenantId, ...]`, dengan `tenantId` dari bootstrap (FR-21). Bootstrap di-refetch saat window focus dan setelah operasi modul, memakai `ETag`/revision.
- **TanStack DB**: inisialisasi provider dan satu collection contoh (`example`) yang bersumber dari Query, sebagai bukti integrasi. Persistence IndexedDB menyusul di PRD offline.
- UI system: **Tailwind CSS** (via `bun-plugin-tailwind` pada `Bun.build`/dev server) + **shadcn/ui**. Komponen shadcn disalin ke `packages/ui` (`@erpora/ui`) dan menjadi milik repo; Tailwind memindai `packages/ui` dan `modules/*/web` lewat `@source`. Fase ini hanya memasang komponen yang dipakai shell dan halaman modul (button, sidebar, sheet, toast, table, badge, dialog).
- Shell: layout dengan sidebar yang dibangun dari frontend module registry dan difilter oleh modul `enabled` dari `/api/bootstrap`.
- Halaman: `/` (dashboard kosong), `/example` dan `/example-addon` (route modul referensi, lazy), `/settings/modules` (FR-20), 404.

**AC:** `bun run typecheck` lolos tanpa error. Membuka `/example` memuat chunk terpisah. Men-disable `example` lewat FR-20 menghilangkan menunya **tanpa reload halaman** (revision berubah → bootstrap di-refetch).

### FR-09 — Frontend module registry & SDK TS

- `packages/module-sdk` menyediakan `defineModule()` dan tipe `FrontendModule` (`key`, `slug`, `version`, `navigation[]`, `registerRoutes`, `offline: OfflinePolicy` UI-side). `OfflinePolicy` mengikuti `docs/architecture/offline-strategy.md` §3.
- Frontend registry mencocokkan setiap modul frontend dengan entri bootstrap berdasarkan `key`:
  - Tidak ada di bootstrap → diabaikan, warning dicatat.
  - Versi frontend berbeda dari versi backend → warning (indikasi build tidak sinkron).
  - Hanya status `enabled` yang memunculkan navigasi.
- **Backend tetap otoritas** (prinsip §7.2).

**AC:** unit test (`bun test`) untuk filter modul aktif, penolakan `key` duplikat, dan perilaku bila modul tidak ada di bootstrap.

### FR-10 — PWA & Service Worker (app shell saja)

- `web/public/manifest.webmanifest` dan ikon.
- `src/sw.ts` di-bundle oleh `Bun.build`, lalu `workbox-build` **`injectManifest`** menyuntikkan precache manifest. Yang di-precache hanya app shell (`index.html`, entry JS/CSS, ikon). Chunk modul memakai strategi on-demand (`CacheFirst` untuk aset ber-hash). `/api/*` **tidak** di-cache oleh SW pada fase ini.
- Update flow: SW baru menunggu (`waiting`), UI menampilkan prompt "Versi baru tersedia", dan pengguna memicu `skipWaiting`.
- SW **nonaktif di mode dev** secara default (dapat diaktifkan dengan flag).

**AC:** build production dapat di-install sebagai PWA (Lighthouse "installable"). Reload offline menampilkan app shell. Deploy versi baru memunculkan prompt update.

### FR-11 — Kerangka sync engine (struktur saja)

- Backend `internal/platform/sync` dan frontend `src/platform/sync` berisi `README.md` plus tipe kontrak `OutboxOperation` (field mengikuti `docs/architecture/offline-strategy.md` §4: `operationId`, `module`, `action`, `tenantId`, `entityId`, `baseVersion`, `payload`, `status`, `traceparent?`).
- Belum ada logika. Protokol didefinisikan di PRD sync.

**AC:** tipe `OutboxOperation` ada di Go dan TS, dan tes kontrak memastikan nama field JSON sama.

### FR-12 — Build tunggal & embed

`task build` (flavor **release**, tanpa developer tools; `task build:dev` menambahkan `-tags devtools`, lihat FR-22):
1. `bun run --filter @erpora/web build` → `web/dist/` (aset ber-hash + `sw.js` + precache manifest).
2. `go build -trimpath -ldflags "-s -w -X ...version=..." -o bin/erpora{{exeExt}} ./cmd/erpora`.

- `web/embed.go`: `//go:embed all:dist`.
- `web/dist/.gitkeep` memastikan `go build` tetap lolos tanpa build frontend. Dalam kondisi itu SPA handler menampilkan halaman "frontend belum di-build" di mode dev.

**AC:** `bin/erpora serve` di mesin bersih (tanpa Bun/Node) menyajikan SPA dan API. Cross-compile `GOOS=linux GOARCH=amd64` dan `GOOS=windows` berhasil.

### FR-13 — Lingkungan pengembangan

- `task dev` menjalankan paralel: PostgreSQL (docker compose), Go API flavor dev (`-tags devtools`) dengan live reload (`air`) di `:8080`, dan Bun dev server (`web/dev.ts`, HMR) di `:5173` yang mem-proxy `/api/*` ke `:8080`.
- `task dev:setup` = `erpora migrate up` + `erpora modules install --all --enable` agar lingkungan dev langsung berisi modul aktif.
- `.env.example` mendokumentasikan semua variabel.

**AC:** mengubah file `.tsx` memicu HMR tanpa reload penuh. Mengubah file `.go` me-restart API dalam ≤ 3 detik pada modul skeleton. Clone baru → `task setup` → `task dev:setup` → `task dev` menampilkan kedua modul referensi aktif.

### FR-14 — Quality gates & CI

| Gate                  | Backend                                   | Frontend                                 |
| --------------------- | ----------------------------------------- | ---------------------------------------- |
| Format                | `gofmt`/`goimports`                       | Biome format                             |
| Lint/static analysis  | `golangci-lint` (incl. `govet`, `staticcheck`, `errcheck`, `gosec`) | Biome lint |
| Type check            | `go build`                                | `tsc --noEmit` (semua workspace)         |
| Unit test             | `go test -race ./...` per Go module (via `task test`) | `bun test`                    |
| Isolasi Go module     | `GOWORK=off go build ./...` per modul; cek `require` == manifest `dependencies`; versi Go/toolchain seragam | — |
| Architecture test     | `tests/architecture` (aturan D1–D11)      | dependency-cruiser (aturan F1–F3)        |
| Integration test      | testcontainers-go + PostgreSQL (lifecycle & migrasi modul) | —                       |
| E2E                   | Playwright: enable/disable modul dari UI  |                                          |
| Security/dependency   | `govulncheck`, gitleaks                   | `bun audit` (atau setara)                |
| Observability rules   | `promtool check rules`                    |                                          |
| Kontrak API (FR-23)   | Test kesesuaian route ↔ kontrak; test kontrak respons | `erpora contracts:lint` (Spectral); hasil `contracts:generate` sinkron |
| Build                 | `task build` → artifact binary            |                                          |

GitHub Actions `ci.yml` menjalankan semua gate pada PR dan menyimpan binary sebagai artifact.

CI juga memiliki workflow rilis: tag `modules/<name>/vX.Y.Z` atau `sdk/vX.Y.Z` memicu validasi bahwa versi tag sama dengan `version` di manifest atau `SDKVersion`.

**AC:** PR yang menambahkan import `internal/platform/x` → `modules/example` **gagal** di CI dengan pesan yang menyebut aturan D2. PR yang membuat `example` mengimpor `exampleaddon/contract` (tidak ada di `dependencies` maupun `require`) gagal di build `GOWORK=off` (D8). PR yang menambah `require` di `go.mod` tanpa menambah `dependencies` di manifest gagal di arch test D8. PR yang membuat frontend `exampleaddon` mengimpor modul yang tidak ada di `dependencies`-nya, atau melakukan deep import `@erpora/module-example/components/...`, gagal di F2.

### FR-15 — Modul referensi `example` & `exampleaddon`

Dua modul minimal yang membuktikan seluruh kontrak sekaligus menjadi template:

| Aspek          | `acme.example` (slug `example`)                         | `acme.example-addon` (slug `example-addon`)                  |
| -------------- | ------------------------------------------------------- | ------------------------------------------------------------ |
| Dependency     | `platform.core ^1.0`                                    | `platform.core ^1.0`, `acme.example ^1.0`                    |
| Endpoint       | `GET /api/example/ping` (`example.ping.read`); `GET /api/example/notes` (`example.notes.read`) | `GET /api/example-addon/summary` (`example-addon.summary.read`) yang memanggil `example/contract` |
| Migrasi        | `0001_create_example_notes.sql` (+ seed via `Installer`) | tanpa migrasi (`strategy: none`)                             |
| Hook lifecycle | `Installer`, `HealthChecker`, `DevSeeder`               | `EnableHook`, `DisableHook` (mencatat log)                   |
| Command CLI    | `erpora example:recount` (menghitung ulang jumlah notes)   | —                                                            |
| Frontend       | `module.ts`, route lazy, satu item navigasi              | `module.ts`, route lazy, satu item navigasi                  |

- Modul fixture tambahan untuk test (manifest rusak, dependensi siklik, versi tidak kompatibel, migrasi gagal, health check gagal) berada di `internal/platform/modules/testdata/` dan **tidak** dikompilasi ke binary.
- Versi `1.1.0` modul `example` (dengan migrasi `0002_...`) disimulasikan di integration test untuk menguji upgrade.
- Dokumentasi `docs/module-sdk/creating-a-module.md` dan `docs/module-sdk/lifecycle.md` menggunakan kedua modul ini sebagai contoh.

**AC:** semua tes FR-05, FR-08, FR-09, FR-18, dan FR-19 memakai modul-modul ini. Menyalin `example` → `foo` dan mengganti `key`/`slug` menghasilkan modul baru yang lolos semua gate tanpa menyentuh kode `platform`, kecuali satu baris pendaftaran di `internal/app/modules.go` dan `web/src/app/modules.ts` (+ dependency di `web/package.json`).

### FR-16 — ADR & dokumentasi

- `docs/adr/0000-template.md` (konteks, opsi, keputusan, konsekuensi, pemicu evaluasi ulang — Blueprint §16).
- **Final** di fase ini:
  - ADR-001 Modular Monolith
  - ADR-002 Clean Architecture selektif & aturan dependensi (merujuk §4.2)
  - ADR-003 PostgreSQL + GORM (dengan guardrail FR-07) + goose
  - ADR-006 Module manifest, SDK (termasuk Registrar `Handle`/`Public`), SemVer & compatibility, serta struktur multi Go module + `go.work` dan aturan tag versi
  - ADR-007 Install/enable/disable/upgrade/uninstall semantics (state machine FR-19)
- **Stub** (judul + konteks): ADR lain sesuai Blueprint §16. ADR-004 mencatat seam tenancy FR-21, dan ADR-008 menyebut kandidat isolated execution (lihat §8).
- `README.md`: prasyarat, quick start, peta folder, dan tautan ADR.
- `docs/operations/deployment.md`: persyaratan hosting (NFR-11), topologi yang didukung, instalasi langkah demi langkah (Linux systemd, Windows service, Docker), konfigurasi env, reverse proxy + TLS, prosedur upgrade & rollback, backup dasar, health check, dan hardening. Drafnya dibuat sekarang, lalu setiap perintah diverifikasi terhadap binary nyata di exit gate M1/M2.

**AC:** kelima ADR final telah direview. Setiap perintah di `deployment.md` telah dijalankan pada VM Linux bersih dan pada Windows minimal sekali sebelum M2 ditutup. README cukup bagi developer baru untuk menjalankan `task dev` dalam ≤ 15 menit.

### FR-17 — Observability berbasis OpenTelemetry

Cakupan: semua lapisan observability **kecuali dashboard/visualisasi** (Grafana, dsb.). Mengacu ke Blueprint §1 (logs, metrics, error tracking, tracing) dan ADR-014.

**17.1. Backend SDK & konfigurasi** (`internal/platform/observability`)
- Inisialisasi `TracerProvider`, `MeterProvider`, dan propagator W3C (`traceparent`, `baggage`) saat startup. Flush saat graceful shutdown.
- Konfigurasi memakai env standar `OTEL_*` (`OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `OTEL_TRACES_SAMPLER`, dll.). Resource attributes berisi `service.name`, `service.version` (dari `-ldflags`), dan `deployment.environment`.
- **Exporter default no-op** bila endpoint tidak diisi, demi NFR-01.
- Bridge `slog` → setiap log dalam request menyertakan `trace_id` dan `span_id`.

**17.2. Instrumentasi otomatis**
- HTTP server: `otelhttp` membungkus semua route. Nama span memakai **pola route** (`GET /api/example/ping`), bukan URL mentah, agar kardinalitas tetap rendah.
- Database: `gorm.io/plugin/opentelemetry` (span per query dan metrik koneksi pool). Nilai parameter SQL **tidak** direkam.
- Runtime metrics Go (goroutine, GC, memori).

**17.3. Atribut tenant & modul**
- Helper `observability.WithTenant(ctx, tenantID)` dan `WithModule(ctx, moduleKey)` menempelkan `erp.tenant.id` dan `erp.module.key` ke span, metric, dan log.
- Middleware registry modul (FR-05) mengisi `erp.module.key` secara otomatis. Middleware tenant context (FR-21) mengisi `erp.tenant.id`; di fase ini nilainya selalu tenant default.
- Aturan privasi: tidak ada PII (email, nama) di atribut telemetry. Hanya ID.

**17.4. Instrumentasi lifecycle modul & kontrak untuk komponen yang belum ada**
- Setiap operasi lifecycle (FR-19) menjadi span `module.<operation>` dengan child span per langkah (resolve, migrate, hook, health). Metrik: `erp.module.operations` (counter per operation/result) dan `erp.module.operation.duration` (histogram).
- `observability` menyediakan helper `StartJobSpan`, `InjectContext(carrier)`, dan `ExtractContext(carrier)` untuk meneruskan trace context lewat payload job, pesan outbox, dan operasi sync (field `traceparent` opsional di `OutboxOperation`, FR-11). Komponen-komponen itu dibangun di PRD masing-masing dan **wajib** memakai helper ini.

**17.5. Frontend tracing** (`packages/platform-web`, modul observability)
- OTel Web SDK: instrumentasi `fetch` (menyuntikkan `traceparent` ke `/api/*` agar trace browser → Go menyambung) dan `document-load`. Navigasi TanStack Router dicatat sebagai span.
- **Dimuat lazy setelah app shell interaktif**, agar tidak masuk anggaran bundle NFR-04.
- Ekspor ke endpoint same-origin `POST /api/platform/telemetry/v1/{traces,metrics}` (route `Public` milik `platform.core`, masuk allowlist). Go meneruskannya ke OTLP endpoint dengan batas ukuran payload dan rate limit. Collector tidak diekspos langsung ke browser.
- Bila backend memakai exporter no-op, frontend tidak mengirim telemetry (diatur lewat flag di `/api/bootstrap`).

**17.6. Error tracking (vendor-neutral)**
- Backend: middleware recover panic → `span.RecordError` + status `Error` + log `ERROR` dengan stack trace + respons 500 envelope tanpa bocoran detail.
- Frontend: React error boundary, `window.onerror`, dan `unhandledrejection` → span event `exception` (OTel semconv), dikirim lewat jalur 17.5.
- Integrasi ke layanan error tracking tertentu (Sentry, dsb.) ditunda dan cukup ditambahkan lewat exporter.

**17.7. Collector (dev & referensi deployment)**
- `infrastructure/monitoring/otel-collector.yaml`: receiver OTLP (gRPC + HTTP), processor `batch` + `memory_limiter`, exporter `debug` (dev) dan exporter OTLP generik (diisi saat deploy).
- `docker-compose.dev.yml` profil `observability`: `task dev:otel` menjalankan collector, dan output trace/metric terlihat di log collector.

**17.8. SLO & alerting (definisi)**
- `docs/operations/slo.md`: SLI awal (availability `/api/*` non-5xx, latency p95 `http.server.request.duration`, readiness), target sementara, dan error budget.
- `infrastructure/monitoring/alerts/`: aturan alert dalam format Prometheus rules (burn-rate SLO, 5xx rate, readiness gagal, operasi modul `failed`), divalidasi di CI dengan `promtool check rules`. Aturan aktif setelah backend metrics dipasang di fase deployment.

**AC:**
- Request ke `/api/example/notes` dengan collector aktif menghasilkan **satu trace** yang memuat span browser (fetch) → span HTTP Go → span query GORM, dan log request memiliki `trace_id` yang sama.
- `erpora modules enable acme.example` menghasilkan span `module.enable` beserta child span langkah-langkahnya.
- Tanpa `OTEL_EXPORTER_OTLP_ENDPOINT`, aplikasi berjalan normal, tidak ada koneksi keluar, dan frontend tidak memuat SDK OTel.
- Panic pada handler uji menghasilkan respons 500 envelope, span berstatus error dengan event `exception`, dan log ERROR.
- Unit test: `InjectContext`/`ExtractContext` mempertahankan trace ID lewat carrier map. `erp.module.key` dan `erp.tenant.id` terisi otomatis pada span route modul.
- Nama span HTTP memakai pola route (diuji dengan route ber-parameter).
- CI: `promtool check rules` lolos.

### FR-18 — Dependency resolver & version compatibility

`internal/platform/modules/resolver` dan `.../compatibility`:

- **SemVer**: memakai `github.com/Masterminds/semver/v3`. Constraint yang didukung: `^`, `~`, `>=`, `<`, rentang (`>=1.2 <2.0`), dan `||`. Pre-release hanya cocok bila constraint menyebutnya secara eksplisit.
- **Kompatibilitas platform**: `manifest.compatibility.platform` dicocokkan dengan versi platform (`-ldflags`); `manifest.compatibility.sdk` dicocokkan dengan `sdk/module.SDKVersion`. Modul yang tidak kompatibel **tidak dapat di-install atau di-enable**, dan registry menandainya `incompatible` (lihat FR-19).
- **Graf dependensi**:
  - Dependency wajib (`optional: false`) harus ada di katalog dan versinya memenuhi constraint.
  - Dependency opsional yang ada harus memenuhi constraint. Kalau tidak ada, cukup dicatat (modul harus berfungsi tanpanya).
  - **Cycle detection** melaporkan jalur siklus lengkap (`a → b → c → a`).
  - **Urutan topologis deterministik** (Kahn + urutan alfabet untuk tie-break) dipakai untuk urutan registrasi, install `--with-deps`, enable, dan reconcile. Kebalikannya dipakai untuk disable/uninstall.
- **`provides`**: dicatat dan diekspos di registry/bootstrap. Dependency berbasis `provides` (bukan `key`) **di luar cakupan** dan dicatat sebagai pertimbangan ADR-006.
- **Pelaporan error**: semua masalah dikumpulkan dalam satu `ResolutionError` terstruktur (`code`, `module`, `dependency`, `constraint`, `found`, `path`). CLI dan API menampilkannya dalam bahasa yang jelas.

**AC (unit test, tabel kasus):** dependency hilang; versi dependency tidak memenuhi constraint; siklus 2 dan 3 node; dependency opsional hilang (lolos) dan opsional salah versi (gagal); platform tidak kompatibel; SDK tidak kompatibel; urutan topologis stabil di 100 kali eksekusi dengan input teracak; beberapa error sekaligus dilaporkan bersamaan.

### FR-19 — Lifecycle modul & runtime activation

Mengimplementasikan Blueprint §7 untuk modul bawaan (build-time). Semua operasi berlaku **per tenant**. Di fase ini selalu tenant default (FR-21).

**19.1. Status** (kolom `module_installations.status`)

| Status            | Makna di fase ini                                                       |
| ----------------- | ----------------------------------------------------------------------- |
| `available`       | Modul dikompilasi di binary tetapi belum ter-install (virtual: tidak ada baris) |
| `verified`        | Dicadangkan untuk paket bertanda tangan (fase 06). Modul `trust: builtin` dianggap terverifikasi oleh build |
| `installed`       | Migrasi diterapkan dan hook `Install` sukses; belum aktif               |
| `enabled`         | Aktif: route, menu, dan capability tersedia                             |
| `disabled`        | Terpasang tetapi tidak aktif; data tetap ada                            |
| `upgrade_pending` | Versi di binary lebih baru dari versi ter-install                        |
| `failed`          | Operasi terakhir gagal; perlu `retry` atau penanganan manual             |
| `uninstalled`     | Tidak aktif dan tidak terpasang; **data dan histori tetap disimpan**    |

Kondisi turunan yang tidak disimpan sebagai status: `incompatible` (FR-18) dan `missing` (baris ada di DB tetapi modul tidak ada di binary).

**19.2. Transisi yang diizinkan** (selain ini ditolak dengan error `invalid_transition`)

| Operasi     | Dari                                         | Ke                         | Prasyarat                                                         |
| ----------- | -------------------------------------------- | -------------------------- | ----------------------------------------------------------------- |
| `install`   | `available`, `uninstalled`                   | `installed`                | Kompatibel. Dependency wajib minimal `installed` (atau `--with-deps`) |
| `enable`    | `installed`, `disabled`                      | `enabled`                  | Semua dependency wajib `enabled`. Health check lolos              |
| `disable`   | `enabled`                                    | `disabled`                 | Tidak ada modul `enabled` yang bergantung wajib padanya           |
| `upgrade`   | `upgrade_pending`                            | status sebelum upgrade     | Kompatibel. Dependency memenuhi constraint versi baru             |
| `uninstall` | `installed`, `disabled`                      | `uninstalled`              | Tidak ada modul ter-install yang bergantung wajib padanya         |
| `retry`     | `failed`                                     | target operasi yang gagal  | Melanjutkan operasi terakhir dari langkah yang gagal              |

- Modul bertanda **`required: true`** (diperkenalkan PRD-001, mis. `erp.core`) di-install & di-enable otomatis oleh `erpora migrate up`, serta menolak `disable`/`uninstall` dengan error `module_required`.
- Downgrade (versi binary < versi ter-install) **ditolak**. Modul ditandai `failed` dengan alasan `downgrade_not_supported` dan route-nya tidak aktif.
- `uninstall` **tidak** menghapus tabel atau data (Blueprint §7). Install ulang setelah uninstall menjalankan migrasi yang belum diterapkan saja (no-op bila versi sama).

**19.3. Langkah operasi** (Blueprint §7.1, disederhanakan untuk modul bawaan)

```
install:  resolve deps → check compatibility → migrate (goose_module_<slug>) → Install hook → status installed
enable:   check deps enabled → OnEnable hook → health check → status enabled → revision++
            └─ health check gagal → OnDisable hook → kembali ke status asal (installed/disabled) → operation failed
disable:  check dependents → OnDisable hook → status disabled → revision++
upgrade:  resolve deps → check compatibility → migrate (versi baru) → Upgrade hook(from,to) → status sebelumnya → revision++
uninstall: check dependents → status uninstalled → revision++
```

**19.4. Persistensi & audit** (migrasi control, FR-07)
- `module_installations`: `id`, `tenant_id` (UUID, NOT NULL), `module_key`, `installed_version`, `status`, `previous_status` (untuk upgrade/retry), `last_operation_id`, `settings jsonb` (dicadangkan), `created_at`, `updated_at`. **Unique `(tenant_id, module_key)`** (Blueprint §3.2).
- `module_operations`: `id`, `tenant_id`, `module_key`, `operation`, `from_version`, `to_version`, `status` (`running | succeeded | failed`), `steps jsonb` (langkah + status + durasi), `error jsonb`, `actor`, `trace_id`, `started_at`, `finished_at`.
- `module_lifecycle_events`: log audit append-only (`tenant_id`, `module_key`, `event`, `actor`, `operation_id`, `details`, `occurred_at`). Blueprint §7 mensyaratkan pencatatan siapa dan kapan.
- `platform_state`: key-value platform (`default_tenant_id`, `environment` (penanda lingkungan, FR-22.5), dsb.).
- `tenant_state`: `tenant_id` (PK), `module_state_revision`.
- `actor` di fase ini: `cli:<os-user>` atau `api:dev` (belum ada identitas). Kolomnya berbentuk string bebas agar nanti bisa diisi user ID.

**19.5. Konsistensi & konkurensi**
- Operasi lifecycle per tenant **diserialisasi** dengan PostgreSQL session advisory lock (`pg_advisory_lock(hash(tenant_id))`). Lock diambil pada **satu koneksi khusus** yang dipegang selama operasi (`(*gorm.DB).Connection(...)`), karena operasi mencakup beberapa transaksi dan koneksi pool biasa bisa berganti di antara transaksi. Operasi kedua menunggu dengan timeout lalu gagal dengan `operation_in_progress`.
- Setiap langkah dicatat ke `module_operations.steps` **sebelum dan sesudah** dijalankan, sehingga proses yang mati di tengah jalan (crash/kill) meninggalkan operasi `running` yang terdeteksi saat startup. Status operasi itu diubah menjadi `failed` dengan alasan `interrupted` dan dapat di-`retry`.
- Migrasi modul memakai transaksi per file migrasi (DDL PostgreSQL bersifat transaksional). `retry` melanjutkan dari migrasi berikutnya yang belum diterapkan.
- Hook modul harus idempoten (didokumentasikan di `docs/module-sdk/lifecycle.md`), karena `retry` dapat memanggilnya ulang.

**19.6. Reconcile saat startup** (`erpora serve`)
1. Muat katalog modul terkompilasi, lalu validasi (FR-05) dan resolve (FR-18). Error validasi katalog = **startup gagal** (bug build, bukan kondisi runtime). Karena seluruh modul berada di binary yang sama, pemeriksaan ini juga menjamin bahwa versi baru sebuah modul tetap memenuhi constraint modul lain yang bergantung padanya, **sebelum** `upgrade` dijalankan.
2. Bandingkan dengan `module_installations` tenant default:
   - Versi binary > versi ter-install → `upgrade_pending`.
   - Versi binary < versi ter-install → `failed` (`downgrade_not_supported`).
   - Ada di DB tetapi tidak ada di binary → `missing` (dicatat sebagai warning; route tentu tidak ada).
   - Operasi `running` sisa → `failed` (`interrupted`).
3. Reconcile **tidak pernah** menjalankan migrasi atau meng-install/enable secara otomatis.

**19.7. Runtime activation**
- `modules/runtime` menyimpan snapshot status per tenant di memori beserta `revision`. Snapshot diperbarui setelah setiap operasi sukses.
- **Module status guard** (middleware FR-05) pada setiap route modul:

| Status modul                              | Respons                                           |
| ----------------------------------------- | ------------------------------------------------- |
| `enabled`                                 | Diteruskan ke handler                             |
| `installed`, `disabled`                   | `403` `{"code": "module_disabled"}`               |
| `available`, `uninstalled`, `missing`     | `404` `{"code": "module_not_found"}`              |
| `upgrade_pending`, `failed`               | `503` `{"code": "module_unavailable"}` + `Retry-After` untuk `upgrade_pending` |

- Guard juga berlaku untuk route `Public`. Modul yang tidak aktif tidak memiliki permukaan HTTP sama sekali.
- **Seam multi-instance**: perubahan snapshot dipublikasikan melalui interface `StateNotifier`. Implementasi fase ini in-process. Implementasi `LISTEN/NOTIFY` PostgreSQL ditambahkan saat horizontal scaling (Blueprint §15 tahap B), tanpa mengubah guard.

**AC (integration test dengan PostgreSQL nyata, mengacu Blueprint §12.1):**
- **Modul diaktifkan**: route, menu (bootstrap), dan capability tersedia. `revision` bertambah.
- **Modul dinonaktifkan**: request baru → 403 `module_disabled`, data tabel `example_notes` tetap ada, dan event audit tercatat dengan actor.
- **Dependency tidak tersedia**: enable `exampleaddon` saat `example` disabled ditolak dengan alasan yang menyebut `acme.example`. Disable `example` saat `exampleaddon` enabled juga ditolak.
- **Manifest tidak valid**: fixture → startup gagal dengan pesan field yang salah.
- **Versi tidak kompatibel**: fixture → install ditolak dengan `ResolutionError`.
- **Migration gagal**: fixture dengan migrasi ke-2 yang error → operasi `failed`, migrasi ke-1 tetap diterapkan, migrasi ke-2 di-rollback. Setelah fixture diperbaiki, `retry` sukses dan melanjutkan dari migrasi ke-2.
- **Health check gagal saat enable**: modul kembali ke status asalnya (`installed` atau `disabled`), route tidak pernah aktif, dan operasi `failed` tercatat.
- **Upgrade**: simulasi `example` 1.0.0 → 1.1.0: reconcile → `upgrade_pending` (route 503) → `upgrade` → migrasi `0002` diterapkan, hook `Upgrade("1.0.0","1.1.0")` terpanggil, status kembali `enabled`.
- **Downgrade** ditolak.
- **Uninstall**: status `uninstalled`, tabel dan data tetap ada, install ulang tidak menjalankan ulang migrasi yang sudah diterapkan.
- **Konkurensi**: dua `enable` paralel → satu sukses, satu `operation_in_progress` (atau keduanya berurutan tanpa korupsi status).
- **Crash di tengah operasi**: proses dihentikan setelah langkah migrate → startup berikutnya menandai operasi `interrupted`, dan `retry` sukses.
- **Idempotensi**: `enable` pada modul yang sudah `enabled` → no-op sukses tanpa event duplikat.

### FR-20 — Manajemen modul: CLI, API, dan UI

**20.1. CLI** (`erpora modules ...`). Ini antarmuka utama, termasuk untuk production.

| Perintah                                         | Keterangan                                              |
| ------------------------------------------------ | ------------------------------------------------------- |
| `list [--json]`                                  | Semua modul: key, versi binary, versi ter-install, status, dependency, kompatibilitas |
| `status <key>`                                   | Detail + operasi terakhir + langkah-langkahnya           |
| `install <key> [--with-deps] [--enable]`         | Install (dan dependency-nya dalam urutan topologis)      |
| `install --all [--enable]`                       | Semua modul terkompilasi (untuk dev/self-hosted awal)    |
| `enable <key>` / `disable <key>`                 |                                                          |
| `upgrade <key>` / `upgrade --all`                | Semua modul `upgrade_pending` dalam urutan topologis     |
| `uninstall <key>`                                | Dengan konfirmasi interaktif (`--yes` untuk skrip)       |
| `retry <operation-id>`                           |                                                          |
| `history <key>`                                  | Log audit lifecycle                                      |
| `plan <operation> <key>` / `plan <operation> --all` | Dry-run: menampilkan langkah, urutan topologis, dan dampak tanpa mengeksekusi. `--all` berlaku untuk `install`, `enable`, dan `upgrade` |

Exit code berbeda untuk error validasi (2), konflik/prasyarat (3), dan kegagalan operasi (4), agar mudah dipakai di skrip.

**20.2. HTTP API** (namespace `platform.core`, prefix `/api/platform/modules`)

| Endpoint                                     | Permission                 |
| -------------------------------------------- | -------------------------- |
| `GET /api/platform/modules`                  | `platform.modules.read`    |
| `GET /api/platform/modules/{key}`            | `platform.modules.read`    |
| `GET /api/platform/modules/{key}/history`    | `platform.modules.read`    |
| `POST /api/platform/modules/{key}/{operation}` | `platform.modules.manage` |
| `GET /api/platform/modules/operations/{id}`  | `platform.modules.read`    |

- Operasi mutasi berjalan sinkron dengan timeout dan mengembalikan `operation_id` beserta hasilnya. Untuk operasi panjang, respons `202` + polling `operations/{id}`.
- **Pengamanan sementara (sebelum ada auth)**: endpoint mutasi **hanya aktif** bila `ERP_ENV=development` atau `ERP_MODULES_ADMIN_API=true`. Di luar itu endpoint mengembalikan `404`. Production memakai CLI sampai PRD Identity & Authorization selesai.
- Seluruh endpoint didokumentasikan di `contracts/openapi/platform.yaml` (endpoint milik platform), dan tipe TS-nya dibangkitkan ke `packages/platform-contracts` (FR-23).

**20.3. UI** (`/settings/modules`)
- Tabel modul: nama, key, versi (binary vs ter-install), badge status, dependency, dan modul yang bergantung padanya.
- Aksi (install/enable/disable/upgrade/uninstall/retry) hanya tampil bila API mutasi aktif. Dialog konfirmasi menampilkan hasil `plan`, misalnya "akan menonaktifkan route X, Y".
- Riwayat operasi dan langkah per modul.
- Setelah aksi sukses: bootstrap di-invalidate, sehingga sidebar dan route guard ter-update tanpa reload.

**AC:**
- Semua perintah CLI memiliki test (golden output untuk `list` dan `plan`, serta exit code).
- API: test permission (stub authz dikonfigurasi deny → 403), test endpoint mutasi mati di `ERP_ENV=production`.
- E2E Playwright: disable `example` dari UI → menu hilang, membuka `/example` menampilkan "Modul tidak aktif", dan `GET /api/example/notes` → 403. Enable kembali → semua pulih.

### FR-21 — Seam multi-tenancy (struktur saja)

Tujuan: tenancy bisa diimplementasikan penuh di PRD Tenancy **tanpa mengubah modul, SDK, atau skema tabel platform**.

- **`internal/platform/tenancy`**:
  - Tipe `TenantID` (UUID) dan helper `WithTenant(ctx)` / `FromContext(ctx)`.
  - Interface `Resolver { Resolve(r *http.Request) (TenantID, error) }`.
  - Implementasi fase ini: **`SingleTenantResolver`**, yang selalu mengembalikan tenant default dari `ERP_DEFAULT_TENANT_ID`. Bila tidak diisi, nilainya dibuat oleh `erpora migrate up` pertama dan disimpan di control DB (`platform_state`).
  - `ERP_TENANCY_MODE=single` (satu-satunya nilai valid di fase ini; nilai `multi` dicadangkan).
- **`internal/platform/database`**:
  - `ControlDB` dan `TenantDB` sebagai tipe berbeda.
  - Interface `TenantConnector { For(ctx, TenantID) (TenantDB, error) }`. Implementasi single-tenant mengembalikan pool yang sama untuk tenant default dan **menolak** tenant ID lain.
- **`sdk/data`**: modul mendapatkan DB hanya lewat `data.DB(ctx)`. Karena `sdk/` tidak boleh mengimpor `internal/` (D5), `sdk/data` hanya mendefinisikan interface `Provider { DB(ctx) (*gorm.DB, error) }`. Implementasinya (adaptor `TenantConnector`) disuntikkan oleh platform ke context lewat middleware tenant dan ke `Env` hook lifecycle (dependency inversion). Modul tidak pernah memegang pool atau DSN.
- **Skema**: semua tabel platform yang tenant-scoped memiliki kolom `tenant_id` NOT NULL sejak migrasi pertama. Foreign key ke tabel `tenants` ditambahkan di PRD Tenancy.
- **Middleware**: langkah "tenant context" di rantai FR-05 memanggil `Resolver`, lalu mengisi context dan atribut OTel (FR-17.3).
- **Frontend**: `tenantId` dari bootstrap dipakai sebagai prefix query key (FR-08) dan disediakan lewat `platform/tenancy`.
- **Operasi lifecycle** (FR-19) menerima `TenantID` secara eksplisit. CLI memakai tenant default dan menyediakan flag `--tenant` yang di fase ini hanya menerima tenant default.

**AC:**
- Arch test: tidak ada package selain `internal/platform/database` yang membuka koneksi DB (`gorm.Open`, `pgxpool.New`).
- `modules/**` tidak dapat mengimpor `internal/platform/database` maupun `tenancy` (dijamin compiler, D9). Akses hanya lewat `sdk/data`.
- Unit test: `TenantConnector` single-tenant menolak tenant ID selain default. `data.DB(ctx)` tanpa tenant di context mengembalikan error, bukan DB default.
- Integration test: semua baris di `module_installations`, `module_operations`, dan `module_lifecycle_events` memiliki `tenant_id` = tenant default.
- Dokumen `docs/architecture/tenancy-seams.md` menjelaskan titik-titik yang diganti saat PRD Tenancy: resolver, connector, migrasi tenant per database, dan FK `tenants`.

### FR-22 — CLI `erpora` dengan developer tools (setara Artisan)

Tujuannya agar pekerjaan rutin, seperti membuat modul, migrasi, dan lapisan Clean Architecture, menghasilkan kode yang **langsung mengikuti aturan PRD ini** dan lolos semua quality gate tanpa disalin manual dari `example`. Pengalamannya satu pintu seperti `php artisan`: **semua command ada di bawah `erp`**.

**22.1. Satu CLI, dua flavor build**

| Flavor build | Cara membuat | Command tersedia | Dipakai di |
| ------------ | ------------ | ---------------- | ---------- |
| **dev**      | `task build:dev` → `go build -tags devtools ./cmd/erpora`; atau langsung `task erpora -- <command>` (`go run -tags devtools ./cmd/erpora`) | Semua: runtime (`serve`, `migrate`, `modules`, `inspect`, command modul) **+** developer tools (22.2) | Mesin developer, CI |
| **release**  | `task build` → `go build ./cmd/erpora` (tanpa tag) | Runtime saja. Developer tools **secara fisik tidak ada** di binary | Production / rilis |

- Kode developer tools berada di `internal/devtools/` (commands + templates). Satu-satunya titik masuknya adalah `cmd/erpora/devtools_on.go` dengan `//go:build devtools`, sedangkan `cmd/erpora/devtools_off.go` (`//go:build !devtools`) adalah no-op.
- `erpora version` menampilkan flavor build (`release` / `dev`). Binary dev menampilkan banner peringatan di setiap command.
- **Binary dev menolak menjalankan command yang menulis** (`serve`, `migrate`, `modules` mutasi, command modul, `db:*`) terhadap database yang ditandai `production` (22.5). Hanya command read-only (`inspect`, `modules list/status`) yang diizinkan.

**22.2. Daftar developer tools** (hanya di flavor dev)

| Grup | Command | Fungsi |
| ---- | ------- | ------ |
| **make** | `make:module <name> [--frontend] [--depends <key,...>]` | Membuat Go module baru di `modules/<name>` (go.mod, manifest, `module.go`, `contract/`, `internal/` lengkap, README, test), mendaftarkannya ke `go.work`, root `go.mod`, `internal/app/modules.go`, dan (dengan `--frontend`) membuat package `modules/<name>/web` (`@erpora/module-<name>`) lalu mendaftarkannya di `web/package.json` + `web/src/app/modules.ts` |
| | `make:migration <module> <name>` | File migrasi goose bernomor urut berikutnya di `modules/<module>/migrations/` (atau `migrations/control|tenant` dengan `--platform control|tenant`) |
| | `make:entity <module> <Name>` | Entity + test di `internal/domain` |
| | `make:usecase <module> <Name> [--query]` | Command/query handler + DTO + test di `internal/application` |
| | `make:repository <module> <Name>` | Interface repository (domain) + model GORM + implementasi (infrastructure) + mapping + integration test |
| | `make:handler <module> <Name> --route "GET /items/{id}" --permission <cap>` | HTTP handler + pendaftaran `Registrar.Handle` di `module.go`. Capability ditambahkan ke `module.json` bila belum ada |
| | `make:contract <module> <Name>` | Interface + DTO di `contract/` untuk dipakai modul lain |
| | `make:command <module> <name>` | Command runtime modul (FR-22.4) |
| | `make:page <module> <path>` | Route lazy TanStack Router + komponen di frontend modul |
| **db** | `db:fresh [--seed]` | Drop semua tabel → `migrate up` → `modules install --all --enable` → (opsional) seed |
| | `db:seed [module]` | Menjalankan `DevSeeder` modul dalam urutan topologis |
| **module** | `module:check [name]` | Validasi manifest (schema), `require` go.mod == `dependencies`, prefix capability, versi Go/toolchain seragam |
| **inspect** | `route:list`, `permission:list` | Alias ke `erpora inspect ...` agar familier bagi pengguna Artisan (alias ini juga ada di flavor release karena read-only) |
| **contracts** | `contracts:generate [module]` | Kontrak platform → `packages/platform-contracts`; kontrak setiap modul → `modules/<name>/web/api/contract.gen.ts`; `module.schema.json` → `packages/module-sdk`; serta spec gabungan untuk dokumentasi (FR-23) |
| | `contracts:lint [module]` | Menjalankan linter Spectral (FR-23) pada kontrak platform dan/atau modul |
| **lainnya** | `doctor` | Memeriksa versi Go, Bun, Task, Docker, koneksi PostgreSQL, dan file `.env` |
| | `list` | Semua command beserta deskripsinya |

**22.3. Aturan generator**
- Template disimpan di `internal/devtools/templates` (di-embed, `text/template`). Kode hasil generate sudah memenuhi aturan D1–D11, guardrail GORM (FR-07), dan konvensi penamaan.
- **Tidak menimpa file**: generator gagal bila file target sudah ada, kecuali diberi `--force`. Opsi `--dry-run` menampilkan daftar file yang akan dibuat atau diubah beserta diff-nya.
- Perubahan pada file yang sudah ada (`go.work`, `internal/app/modules.go`, router frontend, `module.go`, `module.json`) dilakukan lewat **penanda eksplisit** (mis. `// erp:generated:modules`), bukan pencarian teks bebas. Bila penanda tidak ditemukan, generator berhenti dengan instruksi manual.
- Setelah menulis file, generator menjalankan `gofmt`/`goimports` (Go) dan Biome (TS) pada file yang dibuat.

**22.4. Command runtime milik modul** (setara command kustom Artisan dari package)
- Modul mendaftarkan command lewat `Registrar.Command(sdk.Command{Name, Short, Flags, Run})`. Tipe `sdk.Command` milik SDK; cobra tidak bocor ke modul.
- Nama akhir otomatis `<slug>:<name>` (mis. `erpora example:recount`), sehingga tidak bisa bentrok dengan command platform atau modul lain. Bentrokan nama ditolak saat registrasi, mengikuti aturan route di FR-05.
- Saat dijalankan, command mendapat context yang sama dengan request HTTP: tenant (FR-21), `sdk/data`, logger, dan span OTel. Command **hanya berjalan bila modul berstatus `enabled`**. Bila tidak, command gagal dengan exit code 3 dan pesan status modul.
- Setiap eksekusi command modul dicatat ke log dengan actor `cli:<os-user>`.

**22.5. Pengaman berlapis**

1. **Lapis build (utama):** developer tools tidak di-compile ke binary release. Salah konfigurasi apa pun di server production tidak bisa memunculkan `db:fresh` atau `make:*`.
2. **Lapis data, penanda lingkungan di database:** `erpora migrate up` pertama kali menulis `platform_state.environment` = nilai `ERP_ENV` (`development` / `production`). Nilai ini **tidak dapat diubah lewat CLI** (hanya lewat SQL manual yang disengaja).
   - `db:fresh` dan `db:seed` hanya berjalan bila penanda DB = `development` **dan** `ERP_ENV=development`.
   - Binary dev menolak semua command yang menulis ke DB berpenanda `production` (22.1).
   - Penanda ini sengaja **tidak bergantung pada host database**, karena di topologi single-node production (deployment.md §2.1) PostgreSQL juga berada di `localhost`.
3. **Lapis interaksi:** `db:fresh` meminta pengguna **mengetik nama database** sebagai konfirmasi. `--yes` (untuk CI/skrip) hanya diterima bila syarat lapis 2 terpenuhi.

**AC:**
- `erpora make:module inventory --frontend --depends acme.example` (flavor dev) menghasilkan modul yang **langsung** lolos `task lint`, `task test`, arch test D1–D11, build `GOWORK=off`, dan `erpora module:check`. Modul itu juga terdaftar dan dapat di-install/enable lewat `erpora modules`. Ini diuji otomatis di CI pada direktori sementara.
- Setiap generator `make:*` memiliki golden test (output template dibandingkan dengan file acuan).
- `--dry-run` tidak mengubah file apa pun (diverifikasi lewat checksum seluruh repo sebelum dan sesudah).
- `db:fresh` gagal tanpa menyentuh database bila `ERP_ENV=production`, **atau** bila penanda DB = `production` walaupun `ERP_ENV=development` (simulasi salah konfigurasi).
- Binary dev yang diarahkan ke DB berpenanda `production`: `erpora serve`, `erpora migrate up`, dan `erpora modules enable` ditolak; `erpora inspect routes` tetap berjalan.
- `erpora example:recount` (command contoh di modul `example`) berjalan saat modul `enabled` dan gagal dengan exit code 3 saat `disabled`.
- **Binary release tidak memuat developer tools**: `go list -deps ./cmd/erpora` (tanpa tag) tidak mengandung `internal/devtools`, dan `erpora make:module` pada binary release menghasilkan "unknown command". Keduanya diperiksa di CI terhadap artifact rilis.
- Arch test: hanya `cmd/erpora/devtools_on.go` yang boleh mengimpor `internal/devtools/**`.
- `erpora list` menampilkan semua command yang tersedia di flavor tersebut, dikelompokkan (runtime, inspect, make, db, modul).

### FR-23 — Kontrak API per modul & linter kontrak

Setiap modul **memiliki kontrak API-nya sendiri**. Tidak ada satu file OpenAPI yang diedit bersama oleh semua modul. Bentuk kontrak mengikuti [`api-guidelines.md`](../architecture/api-guidelines.md).

**23.1. Kepemilikan kontrak**

| Pemilik | File | Isi |
| --- | --- | --- |
| Platform (bersama) | `contracts/openapi/common.yaml` | **Hanya komponen**: amplop `data`/`error`/`page`, `Decimal`, `Uuid`, `Timestamp`, `Date`, parameter & header standar, respons error standar. Tanpa path |
| Platform | `contracts/openapi/platform.yaml` | Endpoint platform: `/api/bootstrap`, `/api/platform/*` |
| Setiap modul | `modules/<x>/openapi.yaml` | Endpoint & skema milik modul itu saja |

- Kontrak modul berada di **Go module modul tersebut** (ikut di-tag dan di-versi bersama modul) dan di-embed ke binary.
- Kontrak modul **hanya boleh** mendefinisikan path di bawah `/api/{slug}/`, dan nama skemanya otomatis diberi namespace modul saat digabung (`Item` di modul `inventory` menjadi `inventory.Item`), sehingga dua modul boleh memakai nama skema yang sama tanpa bentrok.
- **Rujukan lintas kontrak** memakai skema URI `erp:` yang diselesaikan oleh tooling, bukan path file relatif (path relatif akan rusak bila modul kelak dipindah ke repo sendiri):
  - `$ref: 'erp:common#/components/schemas/Decimal'`: selalu diizinkan.
  - `$ref: 'erp:inventory#/components/schemas/ItemSummary'`: hanya bila `acme.inventory` tercantum di `dependencies` manifest modul perujuk (analog D8 untuk kontrak).
- Skema yang dirujuk modul lain dianggap **API publik modul**. Mengubahnya secara breaking mengikuti aturan evolusi di api-guidelines §14.

**23.2. Kesesuaian kontrak dan kode** (test otomatis, gagal = CI gagal)
- Setiap route di `Registry.Routes()` milik modul X **harus** punya operasi di `modules/X/openapi.yaml` dengan method, path, dan `x-erp-permission` yang **sama persis** dengan `Registrar.Handle`. Sebaliknya, setiap operasi di kontrak harus punya route terdaftar.
- Setiap `x-erp-permission` harus merujuk resource/aksi yang dideklarasikan di `module.json`.
- **Test kontrak respons**: integration test memvalidasi respons nyata endpoint modul referensi terhadap skema di kontraknya.

**23.3. Linter kontrak (Spectral)**
- Ruleset di `contracts/spectral.yaml` (+ fungsi kustom TS di `contracts/spectral-functions/`, dijalankan Bun) menegakkan api-guidelines, antara lain:

| Aturan | Merujuk |
| --- | --- |
| Path kebab-case, koleksi jamak, nesting ≤ 1, tanpa trailing slash, tanpa kata kerja CRUD | §2 |
| Path kontrak modul diawali `/api/{slug}/` sesuai manifest | §2, 23.1 |
| Setiap operasi punya `x-erp-permission` berformat `<resource>.<action>` dan `operationId` unik | §1, §13 |
| Nama properti camelCase; enum snake_case | §4.1 |
| Properti bernama/berformat uang & kuantitas tidak boleh bertipe `number` (harus `$ref` ke `Decimal`) | §4.1 |
| Respons sukses memakai amplop `data`; respons 4xx/5xx merujuk skema error standar | §4.2, §8 |
| Operasi `GET` koleksi mendefinisikan parameter pagination standar | §6 |
| `POST` mendukung header `Idempotency-Key`; `PATCH`/`DELETE`/aksi mendukung `If-Match` | §7 |
| Tidak ada `PUT` kecuali ditandai `x-erp-allow-put: true` | §3 |
| Rujukan `erp:<modul>` hanya ke modul yang ada di `dependencies` | 23.1 |
| Setiap operasi punya contoh request/respons dan mendokumentasikan kode errornya | §13 |

- Dijalankan lewat `erpora contracts:lint`, di pre-commit (opsional, via Taskfile), dan di CI.
- **Deteksi breaking change**: CI membandingkan kontrak setiap modul (dan platform) dengan kontrak pada tag rilis terakhirnya memakai `oasdiff`. Perubahan breaking menggagalkan CI kecuali memenuhi syarat api-guidelines §14.3 langkah 3 (rilis MAJOR + bukti tidak dipakai + label persetujuan di PR). Modul yang belum pernah dirilis dikecualikan.
- Metrik `erp.api.deprecated_usage` (OTel, FR-17) dicatat otomatis oleh middleware untuk setiap operasi/field yang ditandai `deprecated` di kontrak.

**23.4. Generate & publikasi**
- `erpora contracts:generate` (FR-22):
  - kontrak platform → `packages/platform-contracts`;
  - kontrak setiap modul → `modules/<name>/web/api/contract.gen.ts` (tipe + client fetch bertipe), sehingga frontend modul hanya memakai kontrak modulnya sendiri dan kontrak dependency-nya;
  - **spec gabungan** (`common` + `platform` + semua modul) → artifact build `contracts/dist/openapi.json` untuk dokumentasi integrator.
- File hasil generate di-commit dan diberi header "jangan diedit". CI memastikan hasil generate selalu sinkron (`contracts:generate` lalu `git diff --exit-code`).
- `erpora make:module` membuat kerangka `openapi.yaml`, dan `erpora make:handler` menambahkan operasi ke kontrak modul bersamaan dengan pendaftaran route (FR-22).

**AC:**
- Kontrak `example` dan `exampleaddon` lolos linter; `exampleaddon` merujuk `erp:example#/…` dan lolos karena `acme.example` ada di dependencies-nya.
- Rujukan `erp:exampleaddon#/…` dari `example` ditolak linter (bukan dependency).
- Kontrak dengan field `total: {type: number}`, path `/api/example/getItems`, atau operasi tanpa `x-erp-permission` ditolak linter dengan pesan yang menunjuk aturan api-guidelines terkait.
- Menambah `Registrar.Handle` tanpa operasi di kontrak (atau sebaliknya) menggagalkan test kesesuaian 23.2.
- Dua modul fixture dengan skema bernama sama (`Item`) digabung tanpa bentrok di spec gabungan.
- Mengubah kontrak tanpa menjalankan `contracts:generate` menggagalkan CI.
- Menghapus field dari kontrak `example` yang sudah di-tag (fixture tag di test) menggagalkan pemeriksaan `oasdiff`; menambah field opsional lolos.

---

### FR-24 — Primitif keamanan platform

Menerapkan `docs/security/security-guidelines.md`. Prinsip penjadwalan: **setiap primitif keamanan sudah tersedia sebelum konsumen pertamanya ditulis**, sehingga modul tidak pernah membuat versinya sendiri yang kelak harus di-refactor.

**24.1. M1 — rantai pasok & pelaporan**
- `gosec` aktif di `golangci-lint` (sudah termasuk FR-14).
- **SBOM** CycloneDX dibuat di CI untuk setiap Go module (`cyclonedx-gomod`) dan untuk dependensi frontend, diunggah sebagai artifact build.
- `SECURITY.md` di root: cara melaporkan kerentanan secara privat dan target waktu perbaikan (security-guidelines §5).

**24.2. M2 — primitif untuk modul**
- **`sdk/egress`**: satu-satunya HTTP client keluar untuk modul dan platform (OIDC di PRD-001 memakainya). Aturan anti-SSRF sesuai security-guidelines §3.3 (allowlist skema, blokir alamat internal/metadata setelah resolve DNS, dial ke IP tervalidasi, validasi setiap redirect, timeout, batas ukuran). Arch test: `modules/**` dilarang memakai `http.DefaultClient`, `http.Get/Post/Head/PostForm`, atau membuat `http.Client` sendiri.
- **Klasifikasi data**: field group di manifest memiliki `classification` (`public | internal | confidential | highly_confidential`); kontrak OpenAPI memakai `x-erp-classification` per properti. Linter kontrak (FR-23) menolak properti `highly_confidential` di respons, di parameter URL, dan tanpa klasifikasi untuk field sensitif yang dikenali (mis. nama `password`, `secret`, `token`, `nik`, `npwp`).
- **Helper decode request** (`sdk/httpx.DecodeJSON`): batas ukuran body (default 1 MB, dapat ditimpa per route), batas kedalaman, `DisallowUnknownFields`, decode hanya ke DTO. Arch test: decode JSON ke tipe dari `internal/infrastructure` ditolak.
- **Middleware `Cache-Control: no-store`** untuk respons route yang mengembalikan data `confidential`/`highly_confidential` (berdasarkan klasifikasi di kontrak).
- **Helper ekspor** (`sdk/export`): CSV/XLSX dengan proteksi formula injection, menghormati `authz.MaskFields`, dan mencatat ekspor ke audit (audit aktif setelah PRD-001).

**24.3. M3 — frontend**
- Komponen `SafeHtml` di `@erpora/ui` (DOMPurify, konfigurasi ketat); lint rule melarang `dangerouslySetInnerHTML` di luar komponen ini.
- **OWASP ZAP baseline** di CI terhadap `erpora serve` lokal (binary release + DB container), dengan daftar pengecualian yang di-review.

**24.4. Di luar PRD-000** — pipeline upload file (security-guidelines §3.7) dibangun di **PRD Files**, sebelum fitur upload pertama.

**AC:**
- 24.1: artifact SBOM tersedia di setiap build CI; `SECURITY.md` ada dan dirujuk README.
- 24.2: test tabel egress menolak `http://127.0.0.1`, `http://169.254.169.254`, `http://[::1]`, `http://10.0.0.1`, hostname yang resolve ke alamat privat, dan redirect ke alamat privat; arch test menolak fixture modul yang memakai `http.Get`; linter kontrak menolak fixture dengan `password` di respons atau token di query; `DecodeJSON` menolak body > batas, field tak dikenal, dan kedalaman berlebih; ekspor CSV mengawali `=1+1` dengan `'`.
- 24.3: lint menolak `dangerouslySetInnerHTML` di luar `SafeHtml`; ZAP baseline tanpa temuan High di CI.

## 6. Kebutuhan non-fungsional

| ID     | Kebutuhan                                                                                     |
| ------ | --------------------------------------------------------------------------------------------- |
| NFR-01 | Binary production berjalan tanpa runtime eksternal selain PostgreSQL                           |
| NFR-02 | Build reproducible: versi Go, Bun, dan semua dependensi dikunci (`go.sum`, `bun.lock`)         |
| NFR-03 | `task build` bersih ≤ 2 menit di mesin dev standar; CI total ≤ 10 menit                        |
| NFR-04 | Entry JS app shell ≤ 200 KB gzip (anggaran awal, dipantau di CI)                               |
| NFR-05 | Tidak ada secret di repository. `.env` di-ignore dan CI menjalankan secret scan (gitleaks)     |
| NFR-06 | Lintas platform dev: Windows, macOS, Linux. Skrip build ditulis di Go/Bun dan tidak bergantung pada bash |
| NFR-07 | Semua endpoint mengembalikan `X-Request-Id`, dan log menyertakannya                            |
| NFR-08 | Header keamanan dasar: CSP ketat untuk SPA, `X-Content-Type-Options`, `Referrer-Policy`        |
| NFR-09 | Module status guard menambah overhead ≤ 50 µs p99 per request (lookup snapshot in-memory, tanpa query DB) |
| NFR-10 | Startup (validasi + resolve + reconcile) ≤ 1 detik untuk 50 modul (diuji dengan fixture sintetis) |
| NFR-11 | **Lingkungan hosting minimum**: lingkungan yang mengizinkan proses long-running (VPS, bare metal, container, atau PaaS), PostgreSQL sesuai versi minimum di ADR-003, akses shell/CLI untuk `erpora migrate` dan `erpora modules`, serta RAM ≥ 1 GB untuk aplikasi + PostgreSQL di satu mesin (2 GB disarankan). **Shared hosting berbasis PHP (cPanel/DirectAdmin) tidak didukung.** Rinciannya ada di `docs/operations/deployment.md` |

## 7. Prinsip yang dibawa dari dokumen acuan

1. **Module owns its capabilities**: route, permission, konfigurasi, dan offline policy dideklarasikan oleh modul.
2. **Backend owns authority**: frontend registry hanya untuk UX, sedangkan aktivasi modul dan permission diputuskan Go.
3. **Frontend owns presentation**: Router untuk navigasi dan lazy loading, Query/DB untuk state, SW/IndexedDB untuk offline.
4. **Offline is explicitly governed**: tidak ada data API yang di-cache sampai kebijakannya didefinisikan.
5. **Build-time modules + runtime activation** pada versi pertama. Runtime plugin tidak dibahas di sini.
6. **Kontrak eksplisit milik masing-masing pemiliknya**: platform di `contracts/`, setiap modul di `modules/<x>/openapi.yaml`. Kontrak menjadi sumber kebenaran Go ↔ TS.
7. **Disable bukan delete**: menonaktifkan atau meng-uninstall modul tidak pernah menghapus histori transaksi (Blueprint §7).
8. **Lifecycle teraudit dan dapat dipulihkan**: setiap operasi memiliki status persisten, langkah tercatat, dan jalur retry (Blueprint §7.1).

## 8. Catatan desain untuk fase lanjutan (tidak dikerjakan sekarang)

- **Multi-tenancy penuh → PRD Tenancy**: tenant registry, `tenants`/`tenant_databases` (dengan `secret_reference`, Blueprint §3.2), provisioning database, resolver berbasis hostname, `TenantConnector` multi-pool dengan batas koneksi, migrasi tenant terjadwal dengan concurrency limit, dan test isolasi adversarial (Blueprint §12.1). Seam-nya sudah tersedia di FR-21.
- **Isolated execution (Class B extension, Blueprint §8)**: kandidat kuat untuk Go single-binary adalah **WebAssembly via `wazero`**, karena pure Go dan tanpa CGO, sehingga tetap satu binary. Ekstensi tidak tepercaya hanya bisa mengakses host function berbasis capability. Alternatif lain adalah proses terpisah via gRPC. Diputuskan di ADR-008.
- **Signed immutable packages**: kandidat format adalah arsip (tar.zst) + manifest + checksum SHA-256 + signature Ed25519 (atau Sigstore). Field-nya sudah dicadangkan di FR-06, dan status `verified` di FR-19.
- **Dependency berbasis `provides`** dan kebijakan cascade disable: dievaluasi di revisi ADR-006/007.
- **Push notification → PRD Notifications** (di luar spek Blueprint/arsitektur, disetujui untuk fase lanjutan): dibangun sebagai modul `platform.notifications`. Cakupannya notifikasi in-app, Web Push (VAPID + handler `push`/`notificationclick` di service worker FR-10), preferensi per user, dan pemicu dari domain event lewat outbox. Catatan: Web Push di iOS/iPadOS hanya berjalan bila PWA sudah di-install ke Home Screen.
- **Dashboard observability**: Grafana (atau setara) di atas data FR-17, termasuk dashboard per tenant dan per modul.

## 9. Keputusan (sebelumnya pertanyaan terbuka)

| #  | Pertanyaan                                     | Keputusan                                                        |
| -- | ---------------------------------------------- | ---------------------------------------------------------------- |
| Q1 | Go module path?                                | ✅ **`erpora`** (akun pribadi untuk sementara) |
| Q2 | Linter/formatter frontend?                     | ✅ **Biome**. Batas modul tetap memakai dependency-cruiser       |
| Q3 | Akses data?                                    | ✅ **GORM**: fleksibilitas & kecepatan pengembangan mendekati Eloquent, dengan guardrail di FR-07. Alternatif yang ditolak: sqlc (lambat untuk CRUD/filter dinamis), ent (kurva belajar & codegen) |
| Q4 | Task runner?                                   | ✅ **Taskfile (go-task)**, lintas OS, dengan komentar cara menjalankan per OS (FR-01) |
| Q5 | Library UI awal?                               | ✅ **Tailwind CSS + shadcn/ui (Radix)**, sesuai Blueprint §1     |
| Q6 | Versi PostgreSQL target minimum?               | ✅ Major stabil terbaru yang didukung hosting target, dikunci di docker-compose dan dicatat di ADR-003 |
| Q7 | HTTP router?                                   | ✅ **`net/http` ServeMux** sebagai implementasi; modul memakai `Registrar.Handle/Public` milik SDK (FR-05) |
| Q8 | Observability?                                 | ✅ **OpenTelemetry** penuh kecuali dashboard (FR-17)             |
| Q9 | Cakupan fase ini?                              | ✅ Inisialisasi + **Module Registry penuh**. Multi-tenancy hanya seam (FR-21) |
| Q10 | Antarmuka manajemen modul?                    | ✅ CLI + HTTP API + UI `/settings/modules`; API mutasi hanya aktif di dev sampai auth tersedia (FR-20) |
| Q11 | Respons modul tidak aktif?                    | ✅ `installed`/`disabled` → 403, belum terpasang → 404, `upgrade_pending`/`failed` → 503 (FR-19.7) |
| Q12 | Field manifest `trust`?                        | ✅ Ya; fase ini hanya `builtin` (FR-06)                          |
| Q13 | CLI `plan` & `history`?                        | ✅ Ya (FR-20.1)                                                  |
| Q14 | Target NFR-09 & NFR-10?                        | ✅ Disetujui sebagai target awal; dievaluasi ulang setelah M2    |
| Q16 | Command untuk development ala Artisan?        | ✅ **Satu CLI `erpora`** dengan dua flavor build: dev (`-tags devtools`, berisi generator & `db:*`) dan release (runtime saja). Ditambah penanda lingkungan di database dan konfirmasi nama DB. Framework CLI: cobra (FR-03, FR-22) |
| Q17 | Command runtime milik modul?                   | ✅ **Setiap modul** boleh mendaftarkan command miliknya lewat `Registrar.Command`; nama berprefiks slug dan hanya berjalan saat modul `enabled` (FR-22.4) |
| Q18 | Konfigurasi runtime dasar?                     | ✅ `ERP_HTTP_ADDR` (default `127.0.0.1:8080`), `ERP_LOG_FORMAT`, default `ERP_ENV=production`, flag `--env-file` (FR-03) |
| Q19 | Pedoman & penegakan kontrak API?               | ✅ `docs/architecture/api-guidelines.md` wajib diikuti; **setiap modul punya kontrak OpenAPI sendiri** (`modules/<x>/openapi.yaml`), platform hanya memiliki komponen bersama & endpoint platform; linter Spectral di CI (FR-04, FR-23) |
| Q20 | Konvensi API dasar?                            | ✅ JSON camelCase; tanpa nomor versi di URL dan **tanpa breaking change** (expand → migrate → contract, ditegakkan `oasdiff`); pagination halaman untuk UI + cursor untuk sync/integrasi (api-guidelines §4, §6, §14) |
| Q21 | Lokasi frontend modul?                         | ✅ **Co-location**: frontend setiap modul di `modules/<name>/web` (package `@erpora/module-<name>`); komponen generik di `packages/ui`, layanan frontend platform di `packages/platform-web`; `web/` hanya app shell perakit. Batas ditegakkan lewat `package.json` (F1–F3) |
| Q22 | Penjadwalan primitif keamanan? | ✅ Primitif tersedia sebelum konsumen pertama: M1 SBOM + `SECURITY.md` + gosec; M2 `sdk/egress`, klasifikasi data, `DecodeJSON`, `no-store`, `sdk/export`; M3 `SafeHtml` + ZAP; upload di PRD Files (FR-24) |
| Q23 | Temuan review akhir M1 yang butuh keputusan | ✅ (M4) kode `tenant_unresolved`, `method_not_allowed`, `not_ready` masuk katalog api-guidelines §8; (M10) workflow validasi tag rilis ditunda ke **M2**, gate `bun test` ke **M3**; (M11) FR-07 guardrail 5 diperjelas dan ditunda ke **M2**; (NV1) server memakai `otelhttp.WithPublicEndpoint()` — `traceparent`/`baggage` dari klien tidak dipercaya |
| Q24 | Penamaan produk? | ✅ (22 Okt 2026) Produk bernama **ERPora**. (1) Path Go module tanpa domain: root `erpora`, sdk `erpora/sdk`, modul `erpora/modules/<name>`; sengaja bare untuk saat ini, tidak dapat di-`go get` pihak ketiga, dan **wajib pindah ke vanity domain (mis. `erpora.dev/...`) sebelum SDK dipublikasikan** untuk modul pihak ketiga (ADR-001). (2) CLI/binary `erp` → `erpora` (`cmd/erpora`, `task erpora -- <command>`, artefak CI `erpora-<os>-<arch>`). (3) Prefix env var tetap `ERP_` (tidak diubah). (4) Scope npm `@erp/*` → `@erpora/*` (web, `packages/*`, `modules/*/web`; aturan dependency-cruiser F1–F3). (5) `service.name` OTel default `erpora`. (6) Nama tampilan "ERPora" di web (judul/PWA), README, SECURITY.md, CLAUDE.md, dan dokumen docs/. Dokumen historis (`docs/superpowers/plans/*`, `.superpowers/**`) dan rujukan "Blueprint §x" ke commit `c42d676` tidak diubah. Nama role/database PostgreSQL dev (`erp`), skema URI kontrak `erp:`, ekstensi `x-erp-*`, atribut OTel `erp.*`, dan key modul `erp.core` tidak berubah. |
| Q15 | Struktur Go module (`go.mod`) untuk SDK & modul? | ✅ **Multi-module + `go.work`**: root (platform), `sdk`, dan setiap `modules/<name>` adalah Go module sendiri dengan tag versi independen (§4, D5/D8/D9). Dicatat di ADR-006 |

## 10. Definition of Done

- [ ] Semua AC FR-01 s.d. FR-24 terpenuhi.
- [ ] CI hijau di branch utama. PR uji pelanggaran batas modul (D2, D8, F2) terbukti gagal.
- [ ] Seluruh skenario Blueprint §12.1 yang relevan untuk modul bawaan lulus di integration/E2E test (FR-19 AC).
- [ ] `bin/erpora` hasil CI dapat dijalankan di Linux dan Windows, menyajikan SPA yang installable sebagai PWA, dan dapat mengelola modul lewat CLI.
- [ ] ADR-001, 002, 003, 006, 007 berstatus *Accepted*.

## 11. Milestone & exit gate

PRD ini dikerjakan dalam empat milestone. Setiap milestone harus lolos exit gate-nya sebelum milestone berikutnya dianggap selesai. Pekerjaan boleh tumpang tindih bila tidak saling bergantung.

| Milestone | Cakupan | Exit gate |
| --------- | ------- | --------- |
| **M1 — Skeleton** | FR-01–FR-04, FR-07, FR-12–FR-14, FR-16 (ADR-001..003), FR-17.1–17.2, FR-21, FR-24.1 | `bin/erpora` hasil CI berjalan di Linux & Windows; `/healthz`, `/readyz` hijau; arch test D1–D11 dan build `GOWORK=off` per modul aktif dan PR pelanggaran terbukti gagal; migrasi platform berjalan & idempoten |
| **M2 — Module Registry** | FR-05, FR-06, FR-15, FR-18, FR-19, FR-20.1–20.2, FR-22, FR-23, FR-24.2, FR-16 (ADR-006/007), FR-07 guardrail 5, FR-14 workflow validasi tag rilis (Q23) | Semua AC FR-18/FR-19 lulus di integration test (skenario Blueprint §12.1 yang relevan); kedua modul referensi dapat dikelola penuh lewat CLI; `erpora make:module` menghasilkan modul yang lolos semua gate; binary release terbukti tanpa developer tools |
| **M3 — Frontend & PWA** | FR-08–FR-11, FR-20.3, FR-24.3, FR-14 gate `bun test` (Q23) | E2E enable/disable dari UI lulus; PWA installable; bundle dalam anggaran NFR-04 |
| **M4 — Observability lengkap** | FR-17.3–17.8 | Trace end-to-end browser → Go → DB terbukti; lifecycle modul ter-trace; `promtool` lolos |

## 12. Urutan pengerjaan yang disarankan

1. **Fondasi repo**: FR-01, FR-02 → repo + struktur + tooling.
2. **Keputusan tertulis**: FR-16 (ADR-001..003, draft ADR-006/007).
3. **Backend skeleton**: FR-03, FR-04, FR-07, FR-17.1–17.2 → server + DB + instrumentasi dasar (dipasang sejak awal agar semua handler berikutnya otomatis ter-trace).
4. **Seam tenancy**: FR-21 → resolver, connector, `sdk/data` (sebelum tabel modul dibuat, agar `tenant_id` ada sejak awal).
5. **Kontrak modul**: FR-06, FR-05 → manifest schema + SDK Go + registry & route table.
6. **Resolver**: FR-18 → SemVer, graf, cycle detection.
7. **Lifecycle**: FR-19 → state machine, persistensi, migrasi modul, reconcile, runtime guard.
8. **Modul referensi**: FR-15 → `example` + `exampleaddon` + fixture test.
9. **Manajemen**: FR-20.1–20.2 → CLI + API.
9a. **Developer tools**: FR-22 → flavor dev `erpora` (template diturunkan dari modul `example` yang sudah terbukti), penanda lingkungan DB, `erpora inspect`, command modul.
10. **Frontend**: FR-08, FR-09, FR-20.3 → shell, registry TS, halaman modul.
11. **PWA & binary**: FR-10, FR-12.
12. **Pelengkap**: FR-11, FR-17.3–17.8 → kerangka sync, atribut, frontend tracing, error tracking, collector, SLO & alert.
13. **DX & CI**: FR-13, FR-14 → `task dev`, E2E, semua gate di CI (architecture test aktif secara lokal sejak langkah 3).

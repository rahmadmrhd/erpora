# M2b — Lifecycle & Manajemen Modul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Modul `example` dan `exampleaddon` dapat di-install, di-enable, di-disable, di-upgrade, di-uninstall, dan di-retry per tenant lewat CLI `erpora modules …` dan API `/api/platform/modules`. Status tersimpan di control DB, migrasi modul berjalan lewat `goose_module_<slug_snake>`, operasi diserialisasi dengan advisory lock, dan `erpora serve` melakukan reconcile lalu mengaktifkan route sesuai snapshot status.

**Architecture:** Migrasi control `0002` menambah `tenant_state`, `module_installations`, `module_operations`, `module_lifecycle_events` (append-only). Migrasi tenant `0002` menambah `module_migrations`. `internal/platform/migrate` menjalankan migrasi modul per file. `modules/store` (satu-satunya pemakai GORM untuk tabel lifecycle) menyediakan repository, commit atomik, dan advisory lock pada koneksi khusus. `modules/lifecycle` berisi state machine murni (`Transition`, `Check`, `Effective`) dan engine `Service` (langkah, hook, telemetry, retry, reconcile, plan). `modules/runtime` memperoleh `Snapshot.Apply` (compare-and-set per revision), `StateNotifier`, dan `ModuleContext`. `modules/admin` berisi handler API. `internal/app` merakit semuanya untuk `serve`, `migrate up`, `modules`, dan command modul `<slug>:<name>`.

**Tech Stack:** Go 1.27.3 · goose v3 Provider API · GORM (`clause.Locking`, `Connection`) · PostgreSQL 18 (advisory lock, trigger) · cobra · OpenTelemetry (span + metric) · testcontainers-go

**Spec:** `docs/prd/PRD-000-inisialisasi-project.md` FR-03, FR-04, FR-07 (tabel migrasi + guardrail 5), FR-15, FR-16 (ADR-007), FR-17.4, FR-19 (19.1–19.7 + AC), FR-20.1, FR-20.2, FR-21 (AC tenant_id), FR-22.4, Q23, Q25–Q28; `docs/prd/PRD-001-core.md` FR-119; `docs/architecture/api-guidelines.md` §2, §3, §6, §7, §8, §10, §12; `docs/architecture/transaction-data-standards.md` (satu transaksi bisnis = satu transaksi DB, append-only); pembagian M2: `docs/superpowers/plans/2026-10-22-m2-overview.md`

## Global Constraints

- Go module path: root `erpora`, SDK `erpora/sdk`, modul `erpora/modules/<name>`. Semua `go.mod`: `go 1.27`, `toolchain go1.27.3`.
- Status tersimpan (`module_installations.status`): `installed | enabled | disabled | upgrade_pending | failed | uninstalled` (+ `verified` dicadangkan di CHECK). `available` virtual (tidak ada baris). Status turunan (tidak disimpan): `missing`, `incompatible`.
- Operasi: `install | enable | disable | upgrade | uninstall | retry`; operasi internal `reconcile`. Status operasi: `running | succeeded | failed`.
- Kode error lifecycle (FR-19): `invalid_transition`, `operation_in_progress`, `module_required`, `downgrade_not_supported`, `interrupted`, ditambah kode rencana `dependency_not_ready`, `dependents_active`, `module_incompatible`, `module_not_found`, `operation_not_found`.
- Langkah operasi FR-19.3 persis: install `resolve → compatibility → migrate → hook:install`; enable `dependencies → hook:enable → health` (health gagal → `hook:disable` → status asal); disable `dependents → hook:disable`; upgrade `resolve → compatibility → migrate → hook:upgrade`; uninstall `dependents`. Setiap langkah dicatat ke `module_operations.steps` **sebelum dan sesudah** dijalankan.
- Tabel versi goose modul: `"goose_module_" + strings.ReplaceAll(slug, "-", "_")` di **tenant DB**; tabel lifecycle di **control DB** (keduanya database fisik yang sama pada mode single).
- Semua baris tabel lifecycle membawa `tenant_id` = tenant default (FR-21). Modul tidak pernah memegang pool: hook dan command menerima ctx dari `runtime.ModuleContext`.
- Amplop API `{"data": …}` / `{"error": {"code","message","details","requestId","traceId"}}`, JSON camelCase, `{key}` = key modul, `{id}` = UUID operasi.
- Permission: `platform.modules.read`, `platform.modules.manage` (capability `platform.core`).
- API mutasi hanya aktif bila `ERP_ENV=development` atau `ERP_MODULES_ADMIN_API=true`; selain itu `404 not_found`.
- Actor: `cli:<os-user>` (CLI), `api:dev` (API), `system:reconcile` (reconcile saat startup), `system:migrate` (modul `required` di `migrate up`).
- Exit code CLI: `0` sukses/no-op, `2` validasi (argumen, key/operasi tidak dikenal, `--tenant` bukan default, konfirmasi hilang), `3` konflik/prasyarat, `4` operasi gagal, `1` lainnya.
- Migrasi tidak pernah berjalan pada HTTP request biasa atau saat `serve` (reconcile tidak memigrasi); hanya lewat `migrate up` dan operasi lifecycle.
- GORM hanya di `internal/platform/modules/store`, `internal/platform/database`, `sdk/data`, dan infrastructure modul (D3b). `database/sql` hanya di `database` dan `migrate` (DB-OPEN). `internal/platform/**` tidak mengimpor `modules/**` (D2); hanya `internal/app` (D7).
- Tidak ada perubahan breaking pada `/healthz`, `/readyz`, `/api/bootstrap` (bootstrap hanya terisi; bentuknya tetap).
- Windows: `-race` dilewati; Taskfile tanpa `rm`/`cp`/`sed`.
- TDD; satu commit per task; pesan konvensional diakhiri baris kosong + `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Gate sebelum commit: `task lint arch check:gowork-off test`, ditambah `task test:integration` untuk task yang menyentuh DB.

## Keputusan rencana (perlu disetujui pada review rencana)

1. **Pemetaan kode lifecycle ke HTTP memakai katalog api-guidelines §8**, dengan kode FR-19 di `error.details[].code`: `invalid_transition` → `409 invalid_state`; `operation_in_progress` → `409 request_in_progress`; `module_required`, `dependency_not_ready`, `dependents_active`, `module_incompatible` (satu detail per `resolver.Issue`), `downgrade_not_supported` → `422 business_rule_violated`; key/operasi tidak dikenal → `404 not_found`. CLI menampilkan kode FR-19 apa adanya. **(perlu konfirmasi owner)** — FR-19.2 menyebutnya "error `invalid_transition`", sedangkan api-guidelines §7.3 mewajibkan `409 invalid_state` untuk transisi tidak sah. Opsi ini memenuhi keduanya tanpa menambah kode ke katalog.
2. **No-op berdasarkan status target**: operasi yang status targetnya sudah tercapai adalah no-op sukses tanpa baris operasi, event, atau kenaikan revision. Berlaku untuk `install` pada `installed|enabled|disabled`, `enable` pada `enabled`, `disable` pada `disabled`, `uninstall` pada `uninstalled`, dan `upgrade` bila versi binary == versi ter-install. **(perlu konfirmasi owner)** — AC FR-19 hanya menyebut idempotensi `enable`, tetapi tabel 19.2 menolak semua transisi lain. Tanpa perluasan ini, `install --all --enable` (deployment.md §4.3) tidak dapat diulang.
3. **Revision naik pada setiap perubahan status yang di-commit**, termasuk `install` dan reconcile, bukan hanya pada operasi yang ditandai `revision++` di FR-19.3. Alasannya: status `installed` terlihat di `/api/bootstrap`, sehingga ETag (= revision) harus ikut berubah. **(perlu konfirmasi owner)**
4. **Propagasi lintas proses dengan polling revision.** `erpora serve` memeriksa `tenant_state.module_state_revision` setiap `2s` (konstanta `lifecycle.DefaultPollInterval`). Bila lebih baru dari snapshot, `serve` mempublikasikannya lewat `StateNotifier`, lalu subscriber memanggil `Service.Refresh`. Implementasi `StateNotifier` tetap in-process (FR-19.7), dan poller adalah sumber notifikasi kedua yang kelak diganti `LISTEN/NOTIFY`. **(perlu konfirmasi owner)** — FR-20.1 menjadikan CLI antarmuka utama production, padahal CLI adalah proses terpisah. Tanpa polling, snapshot `serve` basi sampai restart.
5. **`incompatible` berlaku transitif sebagai status turunan.** Modul yang tidak kompatibel, atau yang dependensi wajibnya (transitif) tidak kompatibel, dan status tersimpannya `installed|enabled|disabled|upgrade_pending`, dilaporkan `incompatible` (guard `503 module_unavailable`). Status `failed`, `uninstalled`, dan `available` tidak ditimpa. `disable` dan `uninstall` tetap diizinkan, sedangkan `install`, `enable`, dan `upgrade` ditolak `module_incompatible`. **(perlu konfirmasi owner)** — carry-over overview meminta keputusan ini. Opsi fail-closed dipilih karena kode dependensi yang tidak kompatibel tetap dipanggil lewat `contract/`.
6. **`sdk/authz` minimal**: `AsSystem(ctx, reason) context.Context` (panic bila `reason` kosong) dan `SystemReason(ctx) (string, bool)`. Platform memakainya untuk hook lifecycle (`module.lifecycle:<op>`) dan command modul (`cli:<slug>:<name>`), lalu setiap pemakaian dicatat di log dengan `system_reason` dan actor. **(perlu konfirmasi owner)** — FR-119 menyatakan `AsSystem` hanya dipanggil dari `internal/application` modul. Pemakaian oleh platform adalah pengecualian yang harus dicatat. Analyzer FR-115 (PRD-001) wajib mengizinkan `internal/platform/modules/**` dan `internal/app`.
7. **API mutasi sinkron dengan batas tunggu `30s`.** Bila operasi belum selesai dalam batas itu, API membalas `202` + `Location: /api/platform/modules/operations/{id}`, dan operasi berlanjut in-process dengan `context.WithoutCancel`. Shutdown di tengah operasi menghasilkan `interrupted`, lalu dapat di-`retry`. **(perlu konfirmasi owner)** — FR-20.2 meminta 202 + polling, sedangkan api-guidelines §10 melarang endpoint proses panjang sebelum infrastruktur Jobs. `module_operations` berfungsi sebagai catatan status, bukan antrean job.
8. **Gate API mutasi dipasang saat mount, bukan saat registrasi.** Route `POST /api/platform/modules/{key}/{operation}` selalu terdaftar di registry (agar `inspect routes`, security test, dan kesesuaian kontrak M2d stabil). `runtime.Mount` menerima opsi `runtime.SkipRoute(func(registry.RouteInfo) bool)`, dan `serve` tidak memasang route itu bila gate mati. Catch-all `/api/` lalu membalas `404 not_found` sebelum permission dievaluasi, sesuai AC FR-20 "mutasi mati di production → 404" walaupun stub `deny`.
9. **Scope sebelum PRD-001 = tenant.** Belum ada principal, jadi scope setiap endpoint dan perintah adalah tenant dari resolver/CLI. Operasi atau event milik tenant lain → `404 not_found` (CLI exit 2). Key modul berasal dari katalog terkompilasi dan tidak rahasia (Q28 butir 2). Baris DB untuk key yang tidak ada di tenant ini diperlakukan sebagai tidak ada. Test IDOR memakai baris tenant lain yang disisipkan langsung ke DB.
10. **Status hasil kegagalan:** kegagalan pada langkah yang mengubah data (`migrate`, `hook:install`, `hook:upgrade`) → modul `failed` dengan `previous_status` = status asal (`available` dicatat sebagai teks). Kegagalan `hook:enable`, `health`, atau `hook:disable` → status modul tidak berubah, operasi `failed` (FR-19.3 untuk health; diperluas ke hook enable/disable secara simetris). Penolakan prasyarat (transisi, dependensi, kompatibilitas, lock) tidak membuat baris operasi; hanya error dan log `WARN`. **(perlu konfirmasi owner)** — perilaku `OnEnable`/`OnDisable` yang gagal tidak diatur spec.
11. **Deteksi `interrupted` di bawah lock.** Setiap operasi lifecycle dan reconcile `serve`, setelah memperoleh advisory lock, menandai semua operasi `running` tenant itu sebagai `failed` dengan `error.code = interrupted`. Selama lock dipegang, tidak ada operasi lain yang hidup. Modul ikut `failed` hanya bila operasi yang terputus adalah `install` atau `upgrade`.
12. **`retry`** hanya untuk operasi `failed` yang menjadi `last_operation_id` modulnya. Retry membuat baris operasi baru (nama operasi = operasi asal), mencatat event `retried` dengan `details.retryOf`, dan melewati langkah yang sudah `succeeded` kecuali `migrate`. Langkah `migrate` selalu dijalankan karena goose hanya menerapkan file yang belum diterapkan. Prasyarat diperiksa ulang. Retry atas operasi `reconcile` (downgrade) → `invalid_transition`. API `POST …/{key}/retry` me-retry operasi terakhir modul itu. CLI `retry <operation-id>` memakai ID.
13. **Downgrade** dicatat reconcile sebagai operasi `reconcile` berstatus `failed` (`downgrade_not_supported`), dengan modul `failed` dan `previous_status` disimpan. Bila binary kembali ke versi == ter-install, reconcile memulihkan `previous_status`. **(perlu konfirmasi owner)** — pemulihan otomatis tidak disebut spec, tetapi tanpanya rollback binary (deployment.md §7.1) meninggalkan modul `failed` permanen.
14. **Advisory lock:** `pg_try_advisory_lock(hashtextextended('erp.module.lifecycle:' || tenant_id, 0))` diulang dengan backoff `50ms → 500ms` sampai `DefaultLockTimeout = 5s`, pada satu koneksi yang dipegang lewat `(*gorm.DB).Connection`. Unlock memakai `context.WithoutCancel`. Bila unlock gagal, koneksi menjalankan `pg_advisory_unlock_all()` sebelum kembali ke pool. Reconcile `serve` yang kehabisan waktu menunggu lock hanya memuat snapshot (read-only) dengan log `WARN`.
15. **"Dependensi minimal installed"** (FR-19.2 install) = status tersimpan `installed|enabled|disabled`. **"Modul ter-install yang bergantung"** (uninstall) = baris dengan status ≠ `uninstalled`. Upgrade memeriksa bahwa `installed_version` setiap dependensi wajib memenuhi constraint pada manifest baru.
16. **`--all` dan `--with-deps`** selalu melewati `platform.core`, berjalan dalam urutan topologis, dan memvalidasi seluruh rencana (`Plan`) sebelum langkah pertama. Prasyarat yang gagal di modul mana pun → tidak ada yang dieksekusi (exit 3). Kegagalan operasi menghentikan sisa rangkaian (exit 4) dan melaporkan hasil yang sudah selesai. `install --with-deps --enable` meng-enable seluruh rantai.
17. **Modul `required: true`** di-install dan di-enable (beserta dependensinya) oleh `erpora migrate up` setelah migrasi platform, dengan actor `system:migrate`. Belum ada modul referensi yang `required`, sehingga perilaku ini diuji dengan fixture.
18. **Bootstrap**: `modules[]` = semua modul terkompilasi selain `platform.core` beserta status efektifnya. `version` = versi binary, `basePath = "/" + slug`, `offline = manifest.offline.mode != "online-only"`, `revision` = revision tenant, `permissions` tetap `[]` (PRD-001). Modul `missing` tidak dimuat karena tidak punya slug di binary.
19. **`If-Match` opsional** pada aksi `POST …/{key}/{operation}`, dibandingkan dengan revision tenant (sama dengan ETag `GET /api/platform/modules` dan `/{key}`). Nilai yang tidak cocok → `412 version_mismatch`. **`Idempotency-Key` diterima tetapi tidak disimpan**, karena setiap operasi idempoten lewat aturan no-op (keputusan 2) dan kontrak mendokumentasikan hal ini. **(perlu konfirmasi owner)** — penyimpanan kunci 24 jam (api-guidelines §7.2) belum punya infrastruktur.
20. **Body aksi** (`{"withDeps": bool, "enable": bool}`, hanya untuk `install`) di-decode dengan decoder ketat lokal: maksimal 4 KiB, `DisallowUnknownFields`, satu objek. Decoder ini diganti `httpx.DecodeJSON` di M2c.
21. **Guardrail 5**: `PrepareStmt: true` di semua mode. Deteksi N+1 hanya bila `ERP_ENV=development`, dengan env baru `ERP_DB_NPLUSONE_THRESHOLD` (default `10`, `0` = mati). Peringatan muncul sekali per bentuk SQL per request saat hitungan mencapai `N+1`.
22. **Kontrak** `contracts/openapi/common.yaml` (komponen amplop/error/pagination minimal) dan `contracts/openapi/platform.yaml` (`/api/bootstrap` + `/api/platform/modules*`) ditulis di M2b sebagai dokumentasi (FR-20.2). Linter Spectral, `contracts:generate`, dan test kesesuaian route ↔ kontrak tetap di M2d.
23. **Simulasi upgrade `example` 1.0.0 → 1.1.0** (FR-15/FR-19 AC) memakai pembungkus khusus test di `internal/app`. Versi 1.0.0 hanya mengekspos `0001`, sedangkan versi 1.1.0 mengekspos `0001`+`0002` dan `Upgrader` perekam. Manifest asli `example` tetap `1.0.0` (FR-15), walaupun sudah membawa `0002` (Q26).
24. **Konfirmasi `uninstall`**: tanpa `--yes`, CLI membaca `y/N` dari stdin. Stdin non-interaktif atau kosong tanpa `--yes` → exit 2 "konfirmasi diperlukan; pakai --yes". Jawaban selain `y` → "dibatalkan", exit 0.

## Review Focus

1. **Dua `enable` paralel dari dua proses** → satu sukses; yang lain `operation_in_progress` (exit 3) setelah `DefaultLockTimeout`, atau no-op berurutan. Status dan revision tidak korup, dan hanya ada satu event `enabled`. Test: `TestService_ConcurrentEnableTwoStores` (Task 7, integration, dua pool terpisah) dan `TestCLI_ConcurrentEnableTwoProcesses` (Task 15).
2. **Proses mati setelah langkah `migrate`** → baris operasi `running`, langkah `migrate` `succeeded`, `hook:install` tidak ada. Startup berikutnya menandainya `failed/interrupted`, modul `failed`, lalu `retry` sukses tanpa menjalankan ulang migrasi. Test: `TestService_InterruptedOperationMarkedUnderLock` (Task 8) dan `TestCLI_CrashAfterMigrateThenRetry` (Task 15).
3. **Upgrade/install dengan migrasi ke-2 yang gagal** → migrasi 1 tetap diterapkan, migrasi 2 di-rollback (tidak ada tabel/kolom parsial), `module_migrations` mencatat `failed`, dan modul `failed`. Setelah fixture diperbaiki, `retry` menerapkan hanya migrasi 2. Test: `TestApplyModule_SecondFileFailsThenResumes` (Task 2) dan `TestService_FailingMigrationThenRetry` (Task 8).
4. **Disable modul yang punya dependen `enabled`** (`acme.example` saat `acme.example-addon` enabled) → `dependents_active` yang menyebut `acme.example-addon`, tanpa hook terpanggil dan tanpa baris operasi. Kebalikannya, enable addon saat `example` disabled → `dependency_not_ready` yang menyebut `acme.example`. Test: `TestCheck_Prerequisites` (Task 6) dan `TestExampleModules_DependencyRules` (Task 14).
5. **Modul `required` di-disable atau di-uninstall** → `module_required` (CLI exit 3, API `422` dengan detail `module_required`), bahkan dengan `--yes` dan `--all`. `migrate up` kedua kalinya tidak membuat operasi atau event baru. Test: `TestCheck_Prerequisites` (Task 6) dan `TestService_EnsureRequiredIdempotent` (Task 8).

---

## File Structure (M2b)

```
migrations/control/0002_module_lifecycle.sql            # Task 1
migrations/tenant/0002_module_migrations.sql            # Task 1
internal/platform/migrate/{module.go, module_integration_test.go, lifecycle_tables_integration_test.go}   # Task 1, 2
internal/platform/modules/store/{store.go, lock.go, models.go, store_integration_test.go}                # Task 3
internal/platform/modules/runtime/{snapshot.go, notifier.go, modctx.go, mount.go, *_test.go}             # Task 4, 5
sdk/authz/{system.go, system_test.go}                   # Task 5
internal/platform/database/{provider.go, nplusone.go, open.go}   # Task 5, 13
internal/platform/modules/lifecycle/
├── transitions.go, check.go, effective.go, errors.go             # Task 6
├── service.go, steps.go, telemetry.go, read.go                   # Task 7
├── retry.go, reconcile.go, required.go, plan.go, watch.go        # Task 8
└── *_test.go, *_integration_test.go, fixtures_test.go
internal/platform/modules/testdata/migrations/{ok, failing, failing_fixed}/*.sql   # Task 2, 8
internal/platform/modules/admin/{api.go, views.go, decode.go, api_test.go, api_integration_test.go}    # Task 12
internal/platform/bootstrap/handler.go, internal/platform/httpserver/{routes.go, server.go}           # Task 9
internal/platform/config/config.go                      # Task 12 (ERP_MODULES_ADMIN_API), Task 13 (ERP_DB_NPLUSONE_THRESHOLD)
internal/app/{app.go, assemble.go, lifecycle.go, registry.go, testhooks_integration.go, testhooks_default.go, *_integration_test.go}   # Task 9, 11, 14, 15
cmd/erpora/{modules.go, module_commands.go, migrate.go, root.go, testdata/modules_*.golden}            # Task 10, 11
contracts/openapi/{common.yaml, platform.yaml}          # Task 12
tests/integration/{modules_cli_test.go}                 # Task 15
modules/example/{go.mod, go.sum}; delete internal/infrastructure/notes_repo_integration_test.go       # Task 14
tests/integration/example_module_test.go (delete)       # Task 14
docs/adr/0007-…md, docs/module-sdk/{lifecycle.md, README.md, creating-a-module.md}, docs/operations/deployment.md, .env.example, docs/prd/PRD-000-…md (Q29)   # Task 16
```

---

### Task 1: Migrasi tabel lifecycle (control) dan `module_migrations` (tenant)

**Files:**
- Create: `migrations/control/0002_module_lifecycle.sql`, `migrations/tenant/0002_module_migrations.sql`, `internal/platform/migrate/lifecycle_tables_integration_test.go`

**Interfaces:**
- Produces (skema):
  - `tenant_state(tenant_id uuid PK, module_state_revision bigint NOT NULL DEFAULT 0 CHECK (>= 0), updated_at timestamptz NOT NULL DEFAULT now())`.
  - `module_operations(id uuid PK, tenant_id uuid NOT NULL, module_key text NOT NULL, operation text NOT NULL CHECK IN ('install','enable','disable','upgrade','uninstall','reconcile'), from_version text NULL, to_version text NULL, status text NOT NULL CHECK IN ('running','succeeded','failed'), steps jsonb NOT NULL DEFAULT '[]', error jsonb NULL, actor text NOT NULL, trace_id text NULL, started_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz NULL)`, indeks `(tenant_id, module_key, started_at DESC)` dan indeks parsial `(tenant_id) WHERE status = 'running'`.
  - `module_installations(id uuid PK, tenant_id uuid NOT NULL, module_key text NOT NULL, installed_version text NULL, status text NOT NULL CHECK IN ('verified','installed','enabled','disabled','upgrade_pending','failed','uninstalled'), previous_status text NULL, last_operation_id uuid NULL REFERENCES module_operations(id), settings jsonb NOT NULL DEFAULT '{}', created_at, updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE (tenant_id, module_key))`. `installed_version` NULL berarti install belum pernah sukses.
  - `module_lifecycle_events(id uuid PK, tenant_id uuid NOT NULL, module_key text NOT NULL, event text NOT NULL, actor text NOT NULL, operation_id uuid NULL, details jsonb NOT NULL DEFAULT '{}', occurred_at timestamptz NOT NULL DEFAULT now())`, indeks `(tenant_id, module_key, occurred_at DESC)`. Trigger `BEFORE UPDATE OR DELETE` (row) dan `BEFORE TRUNCATE` (statement) memanggil fungsi `module_lifecycle_events_append_only()` yang `RAISE EXCEPTION`.
  - Tenant: `module_migrations(id uuid PK, tenant_id uuid NOT NULL, module_key text NOT NULL, module_version text NOT NULL, migration_key text NOT NULL, checksum text NOT NULL, applied_at timestamptz NOT NULL DEFAULT now(), execution_status text NOT NULL CHECK IN ('succeeded','failed'), operation_id uuid NULL)`, indeks `(tenant_id, module_key)`.
  - Setiap file punya blok `-- +goose Down` yang membalik urutan pembuatan.

- [ ] **Step 1: Tulis test integrasi yang gagal** — `lifecycle_tables_integration_test.go` (`//go:build integration`, `pgtest.Start`, `migrate.Up`):
  - `TestLifecycleTables_Exist`: `to_regclass` non-NULL untuk kelima tabel; `migrate.Status` → control `Latest == 2`, tenant `Latest == 2`, `Pending == 0`.
  - `TestLifecycleEvents_AppendOnly`: `INSERT` sukses; `UPDATE … SET actor='x'`, `DELETE`, dan `TRUNCATE` masing-masing error dengan pesan berisi `append-only`.
  - `TestModuleInstallations_UniqueTenantKey`: dua `INSERT` dengan `(tenant_id, module_key)` sama → error unique (`23505`); status `bogus` → error check (`23514`).
  - `TestMigrateUp_Idempotent`: `migrate.Up` kedua kalinya → no-op, versi tetap `2`.
- [ ] **Step 2: Jalankan** `go test -tags integration ./internal/platform/migrate/...` → FAIL (tabel tidak ada).
- [ ] **Step 3: Implementasi** kedua file SQL sesuai Interfaces.
- [ ] **Step 4: Jalankan** test → PASS; `task test:integration` hijau (test M1 `migrate status` diperbarui ke `Latest == 2`).
- [ ] **Step 5: Commit** `feat(migrate): control and tenant tables for module lifecycle`

### Task 2: Runner migrasi modul `goose_module_<slug_snake>`

**Files:**
- Create: `internal/platform/migrate/module.go`, `internal/platform/migrate/module_test.go`, `internal/platform/migrate/module_integration_test.go`, `internal/platform/modules/testdata/migrations/ok/{0001_create_fx_items.sql, 0002_add_fx_items_note.sql}`, `…/failing/{0001_create_fx_items.sql, 0002_broken.sql}`, `…/failing_fixed/{0001_create_fx_items.sql, 0002_broken.sql}` (versi diperbaiki; nama file 0002 sama)

**Interfaces:**
- Consumes: goose Provider (`UpByOne`, `ErrNoNextVersion`), `database.DB`.
- Produces:
  ```go
  func ModuleTable(slug string) string // "goose_module_" + strings.ReplaceAll(slug, "-", "_")
  type AppliedMigration struct { Version int64; Name, Checksum string; Duration time.Duration } // Checksum = sha256 hex isi file
  type MigrationError struct { Version int64; Name, Checksum string; Err error }              // Unwrap() = Err
  // ApplyModule menerapkan migrasi modul yang belum diterapkan, satu file per transaksi (perilaku goose),
  // memanggil after setelah setiap file sukses; berhenti di file pertama yang gagal (*MigrationError).
  func ApplyModule(ctx context.Context, db *database.DB, slug string, fsys fs.FS, after func(AppliedMigration) error) error
  // ModuleStatus read-only (tanpa membuat tabel goose); Target = Target("module:" + slug).
  func ModuleStatus(ctx context.Context, db *database.DB, slug string, fsys fs.FS) (TargetStatus, error)
  // PendingModuleMigrations: nama file yang belum diterapkan, terurut (dipakai Plan).
  func PendingModuleMigrations(ctx context.Context, db *database.DB, slug string, fsys fs.FS) ([]string, error)
  ```
- Provider modul **tanpa** `WithSessionLocker`, karena operasi lifecycle sudah diserialisasi advisory lock tenant (Task 3). Komentar kode menyebut alasannya.

- [ ] **Step 1: Tulis unit test yang gagal** — `module_test.go`: `ModuleTable("example") == "goose_module_example"`, `ModuleTable("example-addon") == "goose_module_example_addon"`, `ModuleTable("a-b-c") == "goose_module_a_b_c"`.
- [ ] **Step 2: Tulis test integrasi yang gagal** — `module_integration_test.go`:
  - `TestApplyModule_AppliesAllAndCallsAfter`: fixture `ok` → `after` dipanggil 2× dengan `Version` 1 lalu 2 dan `Checksum` 64 karakter hex; tabel `goose_module_fx_mod` ada; panggilan kedua → `after` tidak dipanggil (no-op).
  - `TestApplyModule_SecondFileFailsThenResumes` (Review Focus 3): fixture `failing` → `errors.As(err, &*MigrationError)` dengan `Version == 2`; tabel `fx_items` ada (0001 tetap); kolom dari 0002 tidak ada (rollback); `ModuleStatus.Current == 1`. Lalu `ApplyModule` dengan fixture `failing_fixed` → `after` hanya untuk `Version 2`, dan 0001 tidak dijalankan ulang (tabel penghitung di 0001 tetap berisi 1 baris).
  - `TestModuleStatus_ReadOnly`: slug yang belum pernah dimigrasi → `Current 0`, `Pending == jumlah file`, dan `to_regclass('goose_module_fresh')` tetap NULL.
- [ ] **Step 3: Jalankan** `go test ./internal/platform/migrate/...` dan versi `-tags integration` → FAIL.
- [ ] **Step 4: Implementasi.** Loop `p.UpByOne(ctx)` sampai `goose.ErrNoNextVersion`. Checksum dihitung dari `fs.ReadFile(fsys, res.Source.Path)`. `ModuleStatus` dan `PendingModuleMigrations` memakai ulang `appliedVersions` (parameterkan nama tabel) dan `embeddedVersions` versi `fs.FS`.
- [ ] **Step 5: Jalankan** test → PASS; gate hijau.
- [ ] **Step 6: Commit** `feat(migrate): per-module goose runner with goose_module_<slug_snake> tables`

### Task 3: `modules/store` — repository, commit atomik, advisory lock

**Files:**
- Create: `internal/platform/modules/store/{store.go, models.go, lock.go, store_integration_test.go}`; ubah `doc.go`

**Interfaces:**
- Consumes: `database.ControlDB`, `database.TenantConnector`, `tenancy.TenantID`, `clause.Locking`.
- Produces:
  ```go
  type Installation struct { TenantID tenancy.TenantID; ModuleKey, InstalledVersion, Status, PreviousStatus string; LastOperationID uuid.UUID; CreatedAt, UpdatedAt time.Time } // uuid.Nil = belum ada
  type Step struct { Name string `json:"name"`; Status string `json:"status"` /* running|succeeded|failed|skipped */; StartedAt time.Time `json:"startedAt"`; DurationMS int64 `json:"durationMs"`; Error string `json:"error,omitempty"` }
  type OpError struct { Code string `json:"code"`; Message string `json:"message"`; Step string `json:"step,omitempty"`; Details map[string]any `json:"details,omitempty"` }
  type Operation struct { ID uuid.UUID; TenantID tenancy.TenantID; ModuleKey, Operation, FromVersion, ToVersion, Status string; Steps []Step; Error *OpError; Actor, TraceID string; StartedAt time.Time; FinishedAt *time.Time }
  type Event struct { ID uuid.UUID; TenantID tenancy.TenantID; ModuleKey, Event, Actor string; OperationID uuid.UUID; Details map[string]any; OccurredAt time.Time }
  type ModuleMigration struct { TenantID tenancy.TenantID; ModuleKey, ModuleVersion, MigrationKey, Checksum, ExecutionStatus string; OperationID uuid.UUID }
  type Finish struct { OperationID uuid.UUID; Status string; Error *OpError }
  type Commit struct { Installation *Installation; Finish *Finish; Events []Event; BumpRevision bool }
  var ErrLockTimeout = errors.New("store: lock lifecycle tenant tidak diperoleh")
  var ErrNotFound = errors.New("store: tidak ditemukan")
  func New(control database.ControlDB, tenants database.TenantConnector) *Store
  func (s *Store) WithTenantLock(ctx context.Context, t tenancy.TenantID, timeout time.Duration, fn func(ctx context.Context) error) error
  func (s *Store) Installations(ctx context.Context, t tenancy.TenantID) ([]Installation, error)
  func (s *Store) Revision(ctx context.Context, t tenancy.TenantID) (int64, error) // 0 bila belum ada baris
  func (s *Store) StartOperation(ctx context.Context, op Operation) error           // + set installations.last_operation_id (upsert baris bila ada)
  func (s *Store) SaveSteps(ctx context.Context, t tenancy.TenantID, opID uuid.UUID, steps []Step) error
  // Commit: satu transaksi DB — upsert installation (FOR UPDATE), finish operation, insert events,
  // dan bila BumpRevision: upsert tenant_state revision+1. Mengembalikan revision setelah commit.
  func (s *Store) Commit(ctx context.Context, t tenancy.TenantID, c Commit) (int64, error)
  func (s *Store) Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (Operation, error) // ErrNotFound juga bila milik tenant lain
  func (s *Store) Operations(ctx context.Context, t tenancy.TenantID, key string, limit int) ([]Operation, error)
  func (s *Store) RunningOperations(ctx context.Context, t tenancy.TenantID) ([]Operation, error)
  func (s *Store) Events(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]Event, int64, error) // terbaru dulu
  func (s *Store) RecordModuleMigration(ctx context.Context, m ModuleMigration) error // ke tenant DB lewat connector
  ```
- Model GORM (`installationRow`, `operationRow`, `eventRow`, `tenantStateRow`, `moduleMigrationRow`) privat di `models.go` dengan `TableName()`. Mapping eksplisit, ID baru `uuid.NewV7()`, waktu `UTC()`.
- Semua query memfilter `tenant_id`; tidak ada `First(&x, id)` tanpa tenant (FR-119).

- [ ] **Step 1: Tulis test integrasi yang gagal** — `store_integration_test.go` (DB dari `pgtest` + `migrate.Up`, tenant `A` dan `B`):
  - `TestCommit_AtomicWithRevision`: `Commit{Installation: enabled, Finish: succeeded, Events: [enabled], BumpRevision: true}` → revision `1`, baris installation, operasi `succeeded` dengan `finished_at`, dan satu event. `Commit` dengan event yang melanggar NOT NULL → error, dan **tidak ada** perubahan installation maupun revision (rollback).
  - `TestCommit_NoBumpKeepsRevision`: `BumpRevision:false` → revision tetap.
  - `TestOperation_TenantScoped` (IDOR): operasi milik `A` dibaca dengan tenant `B` → `ErrNotFound`; `Events(B, key)` tidak memuat event `A`.
  - `TestSaveSteps_PersistsBeforeAndAfter`: `SaveSteps` dua kali (running lalu succeeded) → `Operation().Steps` sama dengan input terakhir.
  - `TestWithTenantLock_SecondStoreTimesOut`: `Store` kedua dengan **pool terpisah** (`database.Open` dua kali) memanggil `WithTenantLock(A, 200ms)` saat yang pertama memegang lock → `ErrLockTimeout` dalam `< 1s`; tenant `B` tidak terblokir; setelah `fn` pertama selesai, lock kedua berhasil.
  - `TestWithTenantLock_ReleasedOnPanicAndCancel`: `fn` panic (di-recover test) atau ctx dibatalkan → lock tetap dilepas (lock berikutnya langsung berhasil).
  - `TestRecordModuleMigration_TenantDB`: baris tersimpan dengan `tenant_id` dan `checksum`.
- [ ] **Step 2: Jalankan** `go test -tags integration ./internal/platform/modules/store/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces dan keputusan 14. `WithTenantLock`: `s.control.Connection(func(conn *gorm.DB) error { … })`, loop `SELECT pg_try_advisory_lock(hashtextextended(?, 0))` dengan backoff, `defer` unlock memakai `context.WithoutCancel(ctx)`, dan `pg_advisory_unlock_all()` bila unlock gagal. `Commit` memakai `Transaction` + `clause.Locking{Strength: "UPDATE"}` pada installation dan `INSERT … ON CONFLICT (tenant_id) DO UPDATE SET module_state_revision = tenant_state.module_state_revision + 1 RETURNING`.
- [ ] **Step 4: Jalankan** test → PASS; `task arch` hijau (GORM hanya di `store`).
- [ ] **Step 5: Commit** `feat(modules): lifecycle store with atomic commit and tenant advisory lock`

### Task 4: Runtime — `Snapshot.Apply` (CAS per revision) dan `StateNotifier`

**Files:**
- Modify: `internal/platform/modules/runtime/snapshot.go`, `mount.go`, `mount_test.go`
- Create: `internal/platform/modules/runtime/{notifier.go, snapshot_test.go, notifier_test.go}`

**Interfaces:**
- Produces:
  ```go
  // Apply mengganti status tenant hanya bila revision > revision saat ini, atau tenant belum punya state
  // (compare-and-set monoton). Mengembalikan false untuk pembaruan basi.
  func (s *Snapshot) Apply(t tenancy.TenantID, statuses map[string]Status, revision int64) bool
  func (s *Snapshot) Statuses(t tenancy.TenantID) (map[string]Status, int64) // salinan
  // Set dihapus; pemanggil (mount_test, app) beralih ke Apply.
  type StateNotifier interface {
      Publish(ctx context.Context, t tenancy.TenantID, revision int64)
      Subscribe(fn func(ctx context.Context, t tenancy.TenantID, revision int64)) (cancel func())
  }
  func NewLocalNotifier() *LocalNotifier // in-process; Publish memanggil subscriber secara sinkron, panic subscriber di-recover + log
  type MountOption func(*mountConfig)
  func SkipRoute(skip func(registry.RouteInfo) bool) MountOption
  func Mount(m Mounter, reg *registry.Registry, snap *Snapshot, authz Authorizer, log *slog.Logger, opts ...MountOption)
  ```

- [ ] **Step 1: Tulis test yang gagal**:
  - `snapshot_test.go`: `TestApply_RejectsStaleRevision` → `Apply(t, {a: enabled}, 2)` true; `Apply(t, {a: disabled}, 1)` false dan status tetap `enabled`; `Apply(…, 2)` (sama) false; `Apply(…, 3)` true. `TestApply_ConcurrentWritersMonotonic`: 50 goroutine `Apply` dengan revision acak 1..50 → akhir `Revision == 50` dan statusnya milik revision 50. `TestStatus_PlatformCoreAlwaysEnabled` tetap ada.
  - `notifier_test.go`: dua subscriber menerima `(t, 7)`; `cancel()` menghentikan satu; subscriber yang panic tidak menghentikan subscriber lain.
  - `mount_test.go`: `TestMount_SkipRouteNotMounted` → route yang di-skip menjawab `404 not_found` (catch-all) walaupun stub `deny` (bukan 403); route lain tetap terpasang.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/modules/runtime/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces; perbarui pemanggil `Set`.
- [ ] **Step 4: Jalankan** test → PASS; gate hijau.
- [ ] **Step 5: Commit** `feat(modules): snapshot compare-and-set by revision, StateNotifier, mount skip option`

### Task 5: Konteks eksekusi modul — `sdk/authz.AsSystem`, `database.DataProvider`, `runtime.ModuleContext`

**Files:**
- Create: `sdk/authz/{doc.go, system.go, system_test.go}`, `internal/platform/database/provider.go` (+test), `internal/platform/modules/runtime/{modctx.go, modctx_test.go}`
- Modify: `internal/platform/httpserver/middleware.go` (pakai `database.DataProvider`, hapus `connectorProvider`), `internal/platform/observability/slog.go` (atribut `erp.module.key` yang sudah di-bind lewat `With` tidak diduplikasi)

**Interfaces:**
- Produces:
  ```go
  // erpora/sdk/authz
  func AsSystem(ctx context.Context, reason string) context.Context // panic bila reason == ""
  func SystemReason(ctx context.Context) (reason string, ok bool)
  // internal/platform/database
  func DataProvider(conn TenantConnector) data.Provider // DB(ctx) memakai tenancy.FromContext; session WithContext(ctx)
  // internal/platform/modules/runtime
  // ModuleContext menyiapkan ctx + Env untuk hook/command: tenant, data.Provider, erp.module.key (context
  // dan atribut span), AsSystem(reason) bila reason != "", dan Env.Logger = base.With("erp.module.key", key).
  func ModuleContext(ctx context.Context, t tenancy.TenantID, key string, conn database.TenantConnector, base *slog.Logger, systemReason string) (context.Context, module.Env)
  ```
- `sdk/go.mod` tidak bertambah dependency.

- [ ] **Step 1: Tulis test yang gagal**:
  - `system_test.go`: `SystemReason(AsSystem(ctx,"x")) == ("x", true)`; ctx biasa → `ok == false`; `AsSystem(ctx,"")` panic.
  - `provider_test.go`: ctx tanpa tenant → `data.ErrNoTenantContext`; tenant lain → `database.ErrUnknownTenant`.
  - `modctx_test.go`: `TestModuleContext_CarriesTenantProviderModuleAndSystem` → `tenancy.FromContext` ok, `data.DB(ctx)` tanpa error (connector fake), `observability.ModuleFromContext == key`, `authz.SystemReason == ("module.lifecycle:enable", true)`, `Env.TenantID == t.String()`. `TestModuleContext_LoggerSingleModuleKey`: logger JSON ke buffer, `env.Logger.InfoContext(ctx,"x")` → baris log memuat `"erp.module.key"` **tepat satu kali** (carry-over: logger ter-scope tanpa duplikasi oleh `TraceHandler`).
  - Test `httpserver` yang ada tetap lolos (refactor provider).
- [ ] **Step 2: Jalankan** `go test ./sdk/... ./internal/platform/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces. `TraceHandler.WithAttrs` mencatat bila `erp.module.key` sudah di-bind, lalu `Handle` tidak menambahkannya lagi.
- [ ] **Step 4: Jalankan** test → PASS; `GOWORK=off go -C sdk build ./...` → PASS.
- [ ] **Step 5: Commit** `feat(modules): module execution context with scoped logger and authz.AsSystem`

### Task 6: State machine murni — `Transition`, `Check`, `Effective`

**Files:**
- Create: `internal/platform/modules/lifecycle/{transitions.go, check.go, effective.go, errors.go, transitions_test.go, check_test.go, effective_test.go}`

**Interfaces:**
- Consumes: `registry.Registry` (`Entries`, `Entry`, `Graph`), `resolver.Graph` (`RequiredDependencies`, `RequiredDependents`, `Incompatible`, `Node`), `compatibility.Satisfies`, `store.Installation`, `runtime.Status`.
- Produces:
  ```go
  type Op string
  const (OpInstall Op = "install"; OpEnable = "enable"; OpDisable = "disable"; OpUpgrade = "upgrade"; OpUninstall = "uninstall"; OpRetry = "retry")
  type Error struct { Code, Module, Message string; Related []string; Issues []resolver.Issue }
  func (e *Error) Error() string
  func (e *Error) Unwrap() error // *resolver.ResolutionError{Issues} bila Code == module_incompatible, selain itu nil
  const (CodeInvalidTransition = "invalid_transition"; CodeOperationInProgress = "operation_in_progress"; CodeModuleRequired = "module_required"
         CodeDependencyNotReady = "dependency_not_ready"; CodeDependentsActive = "dependents_active"; CodeModuleIncompatible = "module_incompatible"
         CodeDowngradeNotSupported = "downgrade_not_supported"; CodeModuleNotFound = "module_not_found"; CodeOperationNotFound = "operation_not_found"; CodeInterrupted = "interrupted")
  // Transition: from = status tersimpan (available bila tidak ada baris). upgradeTo = status asal untuk upgrade (previous_status).
  func Transition(op Op, from runtime.Status, sameVersion bool) (to runtime.Status, noop bool, err error)
  type View struct { Reg *registry.Registry; Rows map[string]store.Installation }
  func (v View) Stored(key string) runtime.Status // available bila tidak ada baris
  // Check memeriksa prasyarat FR-19.2 (+ keputusan 5, 15) di luar tabel transisi.
  func Check(op Op, key string, v View) error
  // Effective: status untuk snapshot/bootstrap/list. Modul katalog tanpa baris → available; baris tanpa
  // modul di katalog → missing; incompatible transitif (keputusan 5). platform.core tidak dimuat.
  func Effective(reg *registry.Registry, rows []store.Installation) map[string]runtime.Status
  ```
- Tabel transisi (body `Transition`; baris lain → `invalid_transition`):

  | Op | Dari | Ke | No-op bila dari |
  | --- | --- | --- | --- |
  | install | `available`, `uninstalled` | `installed` | `installed`, `enabled`, `disabled` |
  | enable | `installed`, `disabled` | `enabled` | `enabled` |
  | disable | `enabled` | `disabled` | `disabled` |
  | upgrade | `upgrade_pending` | `previous_status` (dipasang pemanggil) | `installed`/`enabled`/`disabled` dengan `sameVersion` |
  | uninstall | `installed`, `disabled` | `uninstalled` | `uninstalled` |
  | retry | `failed` dan semua status lain | ditentukan operasi asal (Task 8) | — (`Transition` menolak `OpRetry`; ditangani `Service.Retry`) |

- `Check` (urutan pemeriksaan; error pertama dikembalikan, kecuali `Issues` yang dikumpulkan):
  1. key tidak ada di registry, atau `platform.core` → `module_not_found`;
  2. `install`/`enable`/`upgrade`: modul atau dependensi wajib transitifnya ada di `Incompatible()` → `module_incompatible` dengan `Issues`;
  3. `disable`/`uninstall` pada manifest `required: true` → `module_required`;
  4. `install`: dependensi wajib langsung dengan status tersimpan ∉ {`installed`,`enabled`,`disabled`} → `dependency_not_ready` (`Related` = key dependensi);
  5. `enable`: dependensi wajib langsung ≠ `enabled` → `dependency_not_ready`;
  6. `disable`: dependen wajib langsung berstatus `enabled` → `dependents_active` (`Related` = key dependen);
  7. `uninstall`: dependen wajib langsung dengan baris ≠ `uninstalled` → `dependents_active`;
  8. `upgrade`: `installed_version` setiap dependensi wajib tidak memenuhi constraint manifest → `dependency_not_ready` dengan `Message` menyebut constraint dan versi.

- [ ] **Step 1: Tulis test tabel yang gagal**:
  - `transitions_test.go`: semua pasangan `(op, from)` dari 5 op × 9 status (termasuk `missing`, `incompatible`) → `to`, `noop`, atau `invalid_transition` sesuai tabel. `TestTransition_RetryNotHandled` → `OpRetry` mengembalikan `invalid_transition`.
  - `check_test.go` (`TestCheck_Prerequisites`, registry dari fake module seperti test registry M2a: `acme.a`, `acme.b` (wajib → a), `acme.c` (opsional → a), `acme.req` (`required: true`), `acme.bad` (`compatibility.platform: "^9.0"`), `acme.uses-bad` (wajib → bad)):
    - enable `b` saat `a` disabled → `dependency_not_ready`, `Related == ["acme.a"]`;
    - disable `a` saat `b` enabled → `dependents_active`, `Related == ["acme.b"]`; saat hanya `c` (opsional) enabled → nil (Review Focus 4);
    - disable/uninstall `req` → `module_required` (Review Focus 5);
    - install `bad` → `module_incompatible` dan `errors.As(err, new(*resolver.ResolutionError))`; install `uses-bad` → `module_incompatible` yang menyebut `acme.bad`; disable `bad` (enabled) → nil;
    - uninstall `a` saat `b` `disabled` → `dependents_active`; saat `b` `uninstalled` → nil;
    - upgrade `b` dengan `a.installed_version = "0.9.0"` dan constraint `^1.0` → `dependency_not_ready`;
    - key `platform.core` atau `acme.nope` → `module_not_found`.
  - `effective_test.go`: tanpa baris → `available`; baris `acme.gone` tanpa modul → `missing`; `bad` enabled → `incompatible`; `uses-bad` enabled → `incompatible`; `uses-bad` uninstalled → `uninstalled`; `failed` tidak ditimpa; hasil tidak memuat `platform.core`.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/modules/lifecycle/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces (fungsi murni, tanpa I/O).
- [ ] **Step 4: Jalankan** → PASS.
- [ ] **Step 5: Commit** `feat(modules): lifecycle state machine, prerequisites, and effective status`

### Task 7: Engine `Service.Execute` — langkah, hook, telemetry, snapshot

**Files:**
- Create: `internal/platform/modules/lifecycle/{service.go, steps.go, telemetry.go, read.go, service_test.go, fixtures_test.go, service_integration_test.go}`

**Interfaces:**
- Consumes: Task 2 (`migrate.ApplyModule`), Task 3 (`*store.Store`), Task 4 (`Snapshot.Apply`, `StateNotifier`), Task 5 (`runtime.ModuleContext`), Task 6.
- Produces:
  ```go
  type Store interface { // dipenuhi *store.Store; fake di unit test
      WithTenantLock(ctx context.Context, t tenancy.TenantID, timeout time.Duration, fn func(context.Context) error) error
      Installations(ctx context.Context, t tenancy.TenantID) ([]store.Installation, error)
      Revision(ctx context.Context, t tenancy.TenantID) (int64, error)
      StartOperation(ctx context.Context, op store.Operation) error
      SaveSteps(ctx context.Context, t tenancy.TenantID, opID uuid.UUID, steps []store.Step) error
      Commit(ctx context.Context, t tenancy.TenantID, c store.Commit) (int64, error)
      Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error)
      Operations(ctx context.Context, t tenancy.TenantID, key string, limit int) ([]store.Operation, error)
      RunningOperations(ctx context.Context, t tenancy.TenantID) ([]store.Operation, error)
      Events(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error)
      RecordModuleMigration(ctx context.Context, m store.ModuleMigration) error
  }
  type Migrator interface {
      Apply(ctx context.Context, t tenancy.TenantID, slug string, fsys fs.FS, after func(migrate.AppliedMigration) error) error
      Pending(ctx context.Context, t tenancy.TenantID, slug string, fsys fs.FS) ([]string, error)
  }
  func NewMigrator(conn database.TenantConnector) Migrator // adaptor ke migrate.ApplyModule/PendingModuleMigrations
  type Deps struct {
      Registry *registry.Registry; Store Store; Migrator Migrator; Tenants database.TenantConnector
      Snapshot *runtime.Snapshot; Notifier runtime.StateNotifier
      Tracer trace.TracerProvider; Meter metric.MeterProvider; Log *slog.Logger
      LockTimeout time.Duration                        // 0 → DefaultLockTimeout (5s)
      AfterStep func(op Op, key, step string)          // seam test (Task 15); nil di produksi
  }
  const DefaultLockTimeout = 5 * time.Second
  type Request struct { Tenant tenancy.TenantID; Op Op; Key string; All, WithDeps, Enable bool; Actor string; OnStarted func(opID uuid.UUID) }
  type Result struct { OperationID uuid.UUID; Module string; Op Op; From, To runtime.Status; Status string /* succeeded|failed|noop */; Steps []store.Step; Error *store.OpError; Revision int64 }
  func New(d Deps) *Service
  func (s *Service) Execute(ctx context.Context, req Request) ([]Result, error) // error = *Error prasyarat/lock; operasi gagal → Result.Status "failed" + error nil
  func (s *Service) Refresh(ctx context.Context, t tenancy.TenantID) (int64, error) // Installations + Revision → Effective → Snapshot.Apply
  // read.go (read model untuk CLI/API/bootstrap)
  type ModuleState struct { Key, Slug, Name, BinaryVersion, InstalledVersion string; Status runtime.Status; Stored string; Required, Compatible bool
      Incompatible []resolver.Issue; Dependencies []module.Dependency; Dependents []string; LastOperation *store.Operation }
  func (s *Service) Modules(ctx context.Context, t tenancy.TenantID) ([]ModuleState, int64, error) // urutan topologis, tanpa platform.core; + baris missing di akhir
  func (s *Service) Module(ctx context.Context, t tenancy.TenantID, key string) (ModuleState, int64, error) // key tak dikenal → *Error module_not_found
  func (s *Service) History(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error)
  func (s *Service) Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error) // store.ErrNotFound → *Error operation_not_found
  ```
- Alur `execute` satu modul (di dalam `WithTenantLock`): tandai `interrupted` (Task 8) → muat `View` → `Check` → `Transition` (no-op → `Result{Status:"noop"}`) → `StartOperation` (status `running`, `trace_id` dari span, `actor`) → `OnStarted(opID)` → langkah-langkah sesuai Global Constraints, masing-masing: `SaveSteps(running)` → jalankan dalam child span `module.<op>.<step>` → `SaveSteps(succeeded|failed)` → `AfterStep` → `Commit` (installation, finish, event `installed|enabled|disabled|upgraded|uninstalled` atau `operation_failed`, `BumpRevision` bila status berubah) → `Refresh` → `Notifier.Publish`.
- Langkah `migrate`: lewati bila `migrations.strategy == "none"` (step `skipped`). Callback `after` → `RecordModuleMigration(succeeded)`. `*migrate.MigrationError` → `RecordModuleMigration(failed)` di transaksi terpisah.
- Hook dipanggil dengan `runtime.ModuleContext(…, "module.lifecycle:"+op)`. Panic hook di-recover menjadi kegagalan langkah (`error.code = hook_panic`, tanpa stack di DB; stack di log).
- `--with-deps`/`--enable`/`--all`: keputusan 16. Urutan dari `Graph().Order()`, melewati `platform.core`.
- Telemetry: span akar `module.<op>` (atribut `erp.module.key`, `erp.tenant.id`, `erp.module.operation.id`), counter `erp.module.operations` (atribut `operation`, `result`), histogram `erp.module.operation.duration` (detik).

- [ ] **Step 1: Tulis unit test yang gagal** — `service_test.go` dengan fake `Store`/`Migrator` dan fixture module (`fixtures_test.go`: `fxModule` dengan hook yang dapat diatur dan merekam urutan panggilan):
  - `TestExecute_EnableStepsAndHooksOrder`: enable dari `installed` → urutan rekaman `OnEnable`, `Health`; `Steps` = `dependencies, hook:enable, health` semuanya `succeeded`; `SaveSteps` dipanggil `2 × jumlah langkah`; `Commit.BumpRevision == true`; `Snapshot.Status == enabled`; notifier menerima revision baru.
  - `TestExecute_HealthFailRevertsToOrigin`: dari `disabled`, `Health` error → `OnDisable` dipanggil, `Result.Status == "failed"`, `Error.Step == "health"`, status tersimpan tetap `disabled`, `BumpRevision == false`, event `operation_failed`.
  - `TestExecute_EnableIdempotent`: enable pada `enabled` → `Status "noop"`, tanpa `StartOperation`, tanpa `Commit`, tanpa event.
  - `TestExecute_PrerequisiteRejectedHasNoOperation`: enable `b` saat `a` disabled → `*Error{Code: dependency_not_ready}`, `StartOperation` tidak dipanggil.
  - `TestExecute_HookPanicBecomesStepFailure`: `Install` panic → `Result.Status "failed"`, `Error.Code == "hook_panic"`, modul `failed`.
  - `TestExecute_HooksGetModuleContext`: hook menerima ctx dengan `authz.SystemReason == "module.lifecycle:install"`, `tenancy.FromContext` ok, dan `Env.Logger` non-nil.
  - `TestExecute_InstallWithDepsEnableOrder`: install `acme.b --with-deps --enable` dari kosong → hasil berurutan `install a, install b, enable a, enable b`; `platform.core` tidak muncul.
  - `TestExecute_AllSkipsPlatformCoreAndPlansFirst`: `install --all` dengan satu modul incompatible → `*Error module_incompatible` dan **tidak ada** `StartOperation`.
  - `TestExecute_Telemetry` (FR-17 AC): `tracetest.SpanRecorder` → span `module.enable` dengan child `module.enable.hook:enable` dan `module.enable.health`; `sdkmetric.ManualReader` → counter `erp.module.operations{operation=enable,result=succeeded} == 1`.
- [ ] **Step 2: Tulis test integrasi yang gagal** — `service_integration_test.go` (Store + Migrator nyata, fixture `fxModule` dengan migrasi `testdata/migrations/ok`):
  - `TestService_InstallEnableDisableUninstallReinstall`: install → `goose_module_fx_mod` versi 2, `module_migrations` 2 baris `succeeded`, status `installed`; enable → revision naik; disable → tabel `fx_items` dan datanya tetap; uninstall → `uninstalled`, data tetap; install ulang → `after` migrasi tidak dipanggil, `module_migrations` tetap 2 baris.
  - `TestService_AllRowsHaveDefaultTenant` (FR-21 AC): setelah skenario di atas, `SELECT count(*) … WHERE tenant_id <> $default` = 0 untuk `module_installations`, `module_operations`, `module_lifecycle_events`, dan `module_migrations`.
  - `TestService_ConcurrentEnableTwoStores` (Review Focus 1): dua `Service` dengan pool terpisah, `OnEnable` fixture menunggu channel, `LockTimeout = 300ms` → satu `succeeded`, satu `*Error{Code: operation_in_progress}`; setelah channel dilepas, `module_lifecycle_events` berisi tepat satu `enabled`, dan revision naik tepat 1.
  - `TestService_AuditEventHasActor` (FR-19 AC disable): event `disabled` dengan `actor == "cli:tester"` dan `operation_id` terisi.
- [ ] **Step 3: Jalankan** unit dan `-tags integration` → FAIL.
- [ ] **Step 4: Implementasi** sesuai Interfaces.
- [ ] **Step 5: Jalankan** → PASS; `task arch` hijau (lifecycle tidak mengimpor GORM/`database/sql`).
- [ ] **Step 6: Commit** `feat(modules): lifecycle engine with steps, hooks, telemetry, and snapshot refresh`

### Task 8: Retry, `interrupted`, reconcile, modul `required`, plan, dan watcher revision

**Files:**
- Create: `internal/platform/modules/lifecycle/{retry.go, reconcile.go, required.go, plan.go, watch.go, retry_test.go, reconcile_test.go, plan_test.go, watch_test.go, recovery_integration_test.go}`

**Interfaces:**
- Produces:
  ```go
  func (s *Service) Retry(ctx context.Context, t tenancy.TenantID, opID uuid.UUID, actor string) (Result, error)
  func (s *Service) RetryModule(ctx context.Context, t tenancy.TenantID, key, actor string) (Result, error) // operasi terakhir modul
  type ReconcileReport struct { UpgradePending, Downgraded, Restored, Missing []string; Interrupted []uuid.UUID; LockSkipped bool }
  func (s *Service) Reconcile(ctx context.Context, t tenancy.TenantID) (ReconcileReport, error) // actor system:reconcile; selalu diakhiri Refresh
  func (s *Service) EnsureRequired(ctx context.Context, t tenancy.TenantID, actor string) ([]Result, error)
  type PlannedOp struct { Module string; Op Op; From, To runtime.Status; NoOp bool; Steps []string; PendingMigrations []string
      RoutesActivated, RoutesDeactivated []string; Blocked *Error }
  type Plan struct { Ops []PlannedOp }
  func (s *Service) Plan(ctx context.Context, req Request) (Plan, error) // read-only: tanpa lock, tanpa tulis
  const DefaultPollInterval = 2 * time.Second
  // Watch mempublikasikan revision DB yang lebih baru dari snapshot ke Notifier sampai ctx selesai.
  func (s *Service) Watch(ctx context.Context, t tenancy.TenantID, interval time.Duration)
  ```
- `markInterrupted` (dipanggil di awal setiap operasi di dalam lock dan di `Reconcile`): setiap operasi `running` → `Commit{Finish: failed {code: interrupted, step: langkah terakhir yang running}, Events: [interrupted]}`. Untuk `install`/`upgrade`, installation menjadi `failed` dengan `previous_status` = status asal (`BumpRevision`).
- `Reconcile` (FR-19.6, keputusan 11 dan 13), per baris installation:
  - versi binary > `installed_version` dan status ∈ {`installed`,`enabled`,`disabled`} → `upgrade_pending` (`previous_status` = status lama), event `reconciled` `{reason: "upgrade_pending"}`;
  - versi binary < `installed_version` → operasi `reconcile` `failed` `downgrade_not_supported`, modul `failed`, `previous_status` disimpan;
  - status `failed`, operasi terakhir `reconcile/downgrade_not_supported`, dan versi binary == ter-install → pulihkan `previous_status`, event `reconciled {reason: "version_restored"}`;
  - baris tanpa modul di binary → `Missing` + log `WARN` (tanpa tulis).

  Lock timeout → `LockSkipped = true`, log `WARN`, lalu `Refresh` saja. Reconcile tidak pernah memigrasi, meng-install, atau meng-enable.
- `Retry`: validasi (operasi ada di tenant ini, `failed`, sama dengan `last_operation_id`, bukan `reconcile`), lalu eksekusi ulang dengan `from = previous_status` bila modul `failed` (keputusan 12). Event `retried {retryOf}`.
- `EnsureRequired`: untuk setiap manifest `required: true` dalam urutan topologis → `Execute{Op: install, WithDeps: true, Enable: true}` (no-op bila sudah enabled).

- [ ] **Step 1: Tulis unit test yang gagal** (fake store):
  - `retry_test.go`: retry operasi yang bukan terakhir → `invalid_transition`; operasi `succeeded` → `invalid_transition`; ID tenant lain → `operation_not_found`; retry install `failed` → langkah `hook:install` dijalankan ulang, langkah `resolve`/`compatibility` yang sudah `succeeded` dilewati, dan `migrate` tetap dipanggil.
  - `reconcile_test.go`: tabel kasus upgrade_pending / downgrade / restore / missing / interrupted; reconcile kedua tanpa perubahan → tidak ada `Commit` (idempoten); `LockSkipped` saat lock fake mengembalikan `ErrLockTimeout`.
  - `plan_test.go`: `Plan(disable acme.a)` saat `b` enabled → `Blocked.Code == dependents_active`; `Plan(install b --with-deps)` → dua `PlannedOp` berurutan dengan `PendingMigrations`; `Plan(enable a)` → `RoutesActivated` berisi pola route modul `a`; tidak ada `Commit`/`StartOperation`.
  - `watch_test.go`: revision DB fake naik dari 3 ke 4 → notifier menerima `4` sekali; revision tetap → tidak ada publish.
- [ ] **Step 2: Tulis test integrasi yang gagal** — `recovery_integration_test.go`:
  - `TestService_FailingMigrationThenRetry` (Review Focus 3, AC "Migration gagal"): fixture migrasi `failing` → install `Result.Status "failed"`, modul `failed`, `goose_module_*` versi 1, `module_migrations` berisi `0002` `failed`. Fixture diganti `failing_fixed` (Migrations FS fixture dapat ditukar), `Retry` → `succeeded`, versi 2, status `installed`, dan 0001 tidak dijalankan ulang.
  - `TestService_InterruptedOperationMarkedUnderLock` (Review Focus 2): `Execute` dijalankan di goroutine; `AfterStep` pada `migrate` memanggil `runtime.Goexit()` (simulasi crash: hanya `defer` unlock yang berjalan, baris `running` disisakan; engine **tidak** boleh memfinalisasi operasi di `defer`) → `Reconcile` → operasi `failed/interrupted`, modul `failed`; `Retry` → `installed` dan `module_migrations` tidak bertambah.
  - `TestService_DowngradeRejectedAndRestored`: fixture versi 1.1.0 terpasang, registry fixture versi 1.0.0 → `Reconcile` → `failed` + `downgrade_not_supported`, dan `Retry` operasi itu → `invalid_transition`; registry kembali 1.1.0 → `Reconcile` → status semula.
  - `TestService_EnsureRequiredIdempotent` (Review Focus 5): fixture `required` → `EnsureRequired` → `enabled`; panggilan kedua → semua `noop`, jumlah event tetap; `Execute(disable)` → `module_required`.
  - `TestService_IncompatibleInstallRejected` (AC "Versi tidak kompatibel"): fixture `compatibility.platform "^9.0"` → `errors.As(err, *resolver.ResolutionError)` dengan issue `platform_incompatible`.
- [ ] **Step 3: Jalankan** → FAIL.
- [ ] **Step 4: Implementasi** sesuai Interfaces.
- [ ] **Step 5: Jalankan** unit dan integrasi → PASS.
- [ ] **Step 6: Commit** `feat(modules): retry, interrupted recovery, reconcile, required modules, and plan`

### Task 9: Composition root — reconcile di `serve`, bootstrap dari snapshot, `migrate up` untuk modul `required`

**Files:**
- Create: `internal/app/assemble.go`, `internal/app/lifecycle.go`, `internal/app/testhooks_default.go`, `internal/app/assemble_integration_test.go`
- Modify: `internal/app/app.go`, `internal/app/registry.go`, `internal/platform/bootstrap/handler.go`, `internal/platform/httpserver/{server.go, routes.go, server_test.go}`, `cmd/erpora/migrate.go`

**Interfaces:**
- Consumes: Task 3–8.
- Produces:
  ```go
  // bootstrap
  type ModuleInput struct { Key, Slug, Version, Status, OfflineMode string }
  func Build(appVersion string, t tenancy.TenantID, mods []ModuleInput, revision int64) Manifest // offline = mode != "online-only"; basePath "/"+slug; Modules/Permissions tidak pernah nil
  // httpserver
  Deps.Bootstrap func(ctx context.Context, t tenancy.TenantID) bootstrap.Manifest // nil → bootstrap.Build(AppVersion, t, nil, 0)
  // app
  type assembly struct { Router *httpserver.Router; Lifecycle *lifecycle.Service; Snapshot *runtime.Snapshot; Tenant tenancy.TenantID }
  func assemble(ctx context.Context, cfg config.Config, log *slog.Logger, db *database.DB, prov observability.Providers, reg *registry.Registry) (*assembly, error)
  // NewLifecycle dipakai CLI: membuka DB, memeriksa migrasi platform, tenant default, membangun Service.
  func NewLifecycle(ctx context.Context, cfg config.Config, log *slog.Logger) (svc *lifecycle.Service, t tenancy.TenantID, cleanup func(), err error)
  func EnsureRequiredModules(ctx context.Context, cfg config.Config, log *slog.Logger) error // dipanggil `migrate up` setelah UpWithTenant
  var lifecycleTestHooks = func(*lifecycle.Deps) {} // default no-op; versi integration di Task 15
  ```
- `Run`: `BuildRegistry` → DB → cek migrasi → `assemble` (`Service.Reconcile` + `Refresh`, `Notifier.Subscribe` → `Refresh` bila revision lebih baru, `go svc.Watch(ctx, tid, lifecycle.DefaultPollInterval)`, `runtime.Mount` dengan snapshot yang sama) → listen. Error reconcile selain lock → startup gagal (exit 1) dengan pesan jelas.
- `Deps.Bootstrap` dirakit dari `reg.Entries()` (tanpa `platform.core`) + `snap.Statuses(t)`. ETag = revision.

- [ ] **Step 1: Tulis test yang gagal**:
  - `bootstrap` unit: `Build` dengan dua modul → `basePath "/example"`, `offline false` untuk `online-only`, `true` untuk `offline-read`, dan `Permissions` = `[]` di JSON.
  - `httpserver/server_test.go`: `Deps.Bootstrap` fake revision 5 → `ETag: "5"`; `If-None-Match: "5"` → 304; `modules[0].status` dari fake.
  - `assemble_integration_test.go`: `TestAssemble_BootstrapReflectsLifecycle` → setelah `migrate.Up`, `GET /api/bootstrap` memuat `acme.example` dan `acme.example-addon` berstatus `available`, `revision 0`; `svc.Execute(install+enable example)` → bootstrap `enabled` dengan revision naik dan ETag berubah tanpa restart. `TestAssemble_CrossProcessChangeVisibleViaWatch` → `Service` kedua (pool terpisah, mensimulasikan CLI) men-disable modul → dalam `≤ 3 × DefaultPollInterval`, `GET /api/example/ping` → `403 module_disabled`. `TestMigrateUp_EnsureRequiredNoRequiredModules` → katalog referensi tanpa modul required → tidak ada baris installation.
- [ ] **Step 2: Jalankan** `go test ./internal/... ` dan `-tags integration ./internal/app/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces; `cmd/erpora/migrate.go` `up` memanggil `app.EnsureRequiredModules`.
- [ ] **Step 4: Jalankan** → PASS; test `tests/integration/serve_test.go` tetap hijau.
- [ ] **Step 5: Commit** `feat(app): reconcile modules on serve, bootstrap from runtime snapshot, required modules on migrate up`

### Task 10: CLI `erpora modules …` dan `migrate status` per modul

**Files:**
- Create: `cmd/erpora/modules.go`, `cmd/erpora/modules_test.go`, `cmd/erpora/testdata/{modules_list.golden, modules_list_json.golden, modules_plan_install.golden, modules_plan_disable_blocked.golden, modules_status.golden}`
- Modify: `cmd/erpora/root.go`, `cmd/erpora/migrate.go` (+ baris `module:<key>`), `cmd/erpora/migrate_test.go`

**Interfaces:**
- Consumes: `app.NewLifecycle`, `lifecycle.Service` (lewat interface di bawah), `migrate.ModuleStatus`.
- Produces:
  ```go
  // modulesBackend dipenuhi *lifecycle.Service; fake di golden test.
  type modulesBackend interface {
      Modules(ctx context.Context, t tenancy.TenantID) ([]lifecycle.ModuleState, int64, error)
      Module(ctx context.Context, t tenancy.TenantID, key string) (lifecycle.ModuleState, int64, error)
      History(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error)
      Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error)
      Execute(ctx context.Context, req lifecycle.Request) ([]lifecycle.Result, error)
      Retry(ctx context.Context, t tenancy.TenantID, id uuid.UUID, actor string) (lifecycle.Result, error)
      Plan(ctx context.Context, req lifecycle.Request) (lifecycle.Plan, error)
  }
  type backendFactory func(ctx context.Context, opts *rootOpts, errOut io.Writer) (modulesBackend, tenancy.TenantID, func(), error)
  func newModulesCmd(opts *rootOpts, open backendFactory) *cobra.Command // GroupID groupRuntime
  func cliActor() string // "cli:" + os/user.Current().Username (fallback USERNAME/USER, lalu "unknown")
  func lifecycleExitCode(err error) int // keputusan exit code; dipakai juga oleh exitCode()
  ```
- Subcommand (FR-20.1): `list [--json]`, `status <key> [--json]`, `install <key> [--with-deps] [--enable]`, `install --all [--enable]`, `enable <key>`, `disable <key>`, `upgrade <key>`, `upgrade --all`, `uninstall <key> [--yes]`, `retry <operation-id>`, `history <key> [--json] [--page N --page-size N]`, `plan <operation> <key>`, `plan <operation> --all` (`--all` hanya untuk `install|enable|upgrade`, selain itu exit 2). Flag persisten `--tenant <uuid>`: selain tenant default → exit 2.
- `list` kolom: `KEY  BINARY  INSTALLED  STATUS  DEPENDENCIES  COMPATIBLE` (`INSTALLED` `-` bila kosong). `status`: detail + operasi terakhir + tabel `STEP  STATUS  DURATION  ERROR`. Hasil operasi: satu baris per `Result` (`acme.example  enable  installed → enabled  succeeded  op=<uuid>`).
- `migrate status`: setelah `control` dan `tenant`, satu baris `module:<key>` untuk setiap modul ber-baris (bukan `available`) dengan `MigrationSource` (FR-07 AC).

- [ ] **Step 1: Tulis test yang gagal** — `modules_test.go` dengan fake backend:
  - golden `list` (teks dan `--json`), `plan install acme.example-addon --with-deps`, `plan disable acme.example` (blocked, exit 3), `status acme.example`; flag `-update` memperbarui golden.
  - exit code tabel: unknown key → 2; `retry not-a-uuid` → 2; `--tenant` lain → 2; `*Error invalid_transition`/`dependents_active`/`module_required`/`operation_in_progress`/`module_incompatible` → 3; `Result.Status "failed"` → 4; no-op → 0 dengan teks `tidak ada perubahan`.
  - `uninstall` tanpa `--yes` dengan stdin kosong → exit 2; stdin `y\n` → `Execute` dipanggil; stdin `n\n` → exit 0, `Execute` tidak dipanggil.
  - `install --all --enable` → `Request{All:true, Enable:true, Actor: "cli:…"}`.
  - `migrate_test.go`: `printStatus` dengan baris `module:acme.example` (golden).
- [ ] **Step 2: Jalankan** `go test ./cmd/erpora/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces; daftarkan `newModulesCmd(opts, defaultBackend)` di root.
- [ ] **Step 4: Jalankan** → PASS; `erpora list` menampilkan `modules` di grup Runtime.
- [ ] **Step 5: Commit** `feat(cli): erpora modules list/status/install/enable/disable/upgrade/uninstall/retry/history/plan`

### Task 11: Command modul `<slug>:<name>` (FR-22.4)

**Files:**
- Create: `cmd/erpora/module_commands.go`, `cmd/erpora/module_commands_test.go`, `internal/app/command.go`, `internal/app/command_integration_test.go`
- Modify: `cmd/erpora/root.go` (grup `module` berjudul "Modul")

**Interfaces:**
- Consumes: `registry.Registry.Commands()`, `runtime.ModuleContext`, `authz.AsSystem`, `lifecycle.Service.Module`.
- Produces:
  ```go
  // cmd/erpora
  func addModuleCommands(root *cobra.Command, opts *rootOpts) // BuildRegistry gagal → tidak menambah apa pun (serve yang melaporkan exit 2)
  // internal/app
  type CommandRun struct { Command registry.CommandInfo; Args []string; Flags map[string]string; Out io.Writer; Actor string }
  // RunModuleCommand: modul harus berstatus efektif enabled (selain itu ExitError{Code:3, "modul <key> berstatus <status>"});
  // ctx = ModuleContext(…, "cli:"+name) + span "command <name>"; log INFO actor, system_reason, durasi.
  func RunModuleCommand(ctx context.Context, cfg config.Config, log *slog.Logger, run CommandRun) error
  ```
- Flag cobra dibangkitkan dari `module.Flag` (string, default `Flag.Default`). `Invocation.Flags` berisi semua flag (termasuk default). `Invocation.Env` dari `ModuleContext`.

- [ ] **Step 1: Tulis test yang gagal**:
  - `module_commands_test.go`: root memuat command `example:recount` di grup `module`; `erpora list` mencetak judul `Modul:` dan `example:recount`; registry fake dengan flag `--limit` (default `5`) → `Invocation.Flags["limit"] == "5"` (runner fake).
  - `command_integration_test.go`: `TestRunModuleCommand_EnabledRuns` → setelah install+enable `example`, output `notes: 3`, ctx command membawa `authz.SystemReason == "cli:example:recount"`; `TestRunModuleCommand_DisabledExit3` → setelah disable, `ExitError.Code == 3` dengan pesan berisi `disabled`; `available` → exit 3 dengan `available`.
- [ ] **Step 2: Jalankan** → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces.
- [ ] **Step 4: Jalankan** unit dan `-tags integration` → PASS.
- [ ] **Step 5: Commit** `feat(cli): run module commands as <slug>:<name> only when module is enabled`

### Task 12: API `/api/platform/modules`, gate admin, dan kontrak platform

**Files:**
- Create: `internal/platform/modules/admin/{api.go, views.go, decode.go, errors.go, api_test.go, api_integration_test.go}`, `contracts/openapi/common.yaml`, `contracts/openapi/platform.yaml`
- Modify: `internal/platform/modules/admin/doc.go`, `internal/app/registry.go` (`Options.Platform` + `PlatformCapabilities`), `internal/app/assemble.go` (`api.Bind`, `SkipRoute`), `internal/platform/config/config.go` (+ `ModulesAdminAPI`), `config_test.go`, `.env.example`, `tests/security/public_routes_test.go` (tetap tanpa route publik baru), `cmd/erpora/testdata/inspect_routes.golden`

**Interfaces:**
- Consumes: `lifecycle.Service` (lewat `Backend`), `httpserver.WriteData/WriteError`, `tenancy.FromContext`.
- Produces:
  ```go
  type Backend interface {
      Modules(ctx context.Context, t tenancy.TenantID) ([]lifecycle.ModuleState, int64, error)
      Module(ctx context.Context, t tenancy.TenantID, key string) (lifecycle.ModuleState, int64, error)
      History(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]store.Event, int64, error)
      Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (store.Operation, error)
      Execute(ctx context.Context, req lifecycle.Request) ([]lifecycle.Result, error)
      RetryModule(ctx context.Context, t tenancy.TenantID, key, actor string) (lifecycle.Result, error)
  }
  type API struct{ /* backend atomic.Pointer, syncWait time.Duration */ }
  func New() *API
  func (a *API) Bind(b Backend)                   // dipanggil assemble; belum di-bind → 503 not_ready
  func (a *API) Register(r module.Registrar) error // dipakai registry.Options.Platform
  func IsMutation(rt registry.RouteInfo) bool      // route POST …/{key}/{operation}
  var Capabilities = []string{"platform.modules.read", "platform.modules.manage"}
  const SyncWait = 30 * time.Second
  // config
  Config.ModulesAdminAPI bool // ERP_MODULES_ADMIN_API: "true"|"false"|"" (default false); nilai lain → ValidationError
  func (c Config) ModulesAdminAPIEnabled() bool // Env == development || ModulesAdminAPI
  ```
- Route (path relatif `platform.core`): `GET /modules` (pagination §6 `page`/`pageSize`, ETag = revision), `GET /modules/{key}` (ETag = revision), `GET /modules/{key}/history` (pagination), `POST /modules/{key}/{operation}` (`platform.modules.manage`), `GET /modules/operations/{id}`.
- View JSON: `ModuleView{key, slug, name, binaryVersion, installedVersion (null), status, required, compatible, incompatibleCodes[], dependencies[{module, version, optional}], dependents[], lastOperationId (null)}`; `OperationView{id, moduleKey, operation, fromVersion, toVersion, status, steps[{name,status,startedAt,durationMs,error}], failure{code,message,step} (null), actor, startedAt, finishedAt}`; `EventView{id, moduleKey, event, actor, operationId, details, occurredAt}`. Respons aksi: `200 {"data":{"module":ModuleView,"operations":[OperationView]}}` (no-op → `operations: []`); melewati `SyncWait` → `202` + `Location` + `{"data":{"operationId","status":"running"}}`. Operasi `failed` tetap `200` dengan `operations[].status == "failed"`.
- Pemetaan error sesuai keputusan 1. `If-Match` sesuai keputusan 19. `{operation}` di luar `install|enable|disable|upgrade|uninstall|retry` → `404 not_found`. `{id}` bukan UUID → `400 bad_request`.

- [ ] **Step 1: Tulis test yang gagal** — `api_test.go` (backend fake, router + `runtime.Mount` nyata dengan registry yang memuat `Options.Platform = api.Register`):
  - `TestList_EnvelopeAndPagination`: `data` berisi array view, `page.total`, dan header `ETag`.
  - `TestAction_ErrorMapping` (tabel): `invalid_transition` → 409 `invalid_state` + `details[0].code == "invalid_transition"`; `operation_in_progress` → 409 `request_in_progress`; `dependents_active` → 422 `business_rule_violated` + detail menyebut `acme.example-addon`; `module_incompatible` dengan 2 issue → 2 detail; key tidak dikenal → 404 `not_found`; operasi `explode` → 404.
  - `TestAction_FailedOperationReturns200`, `TestAction_NoopEmptyOperations`.
  - `TestAction_SlowOperationReturns202`: `syncWait` diperkecil (opsi test), backend memanggil `OnStarted` lalu menunggu → 202, `Location` berisi `operations/<id>`.
  - `TestAction_IfMatchMismatch412`; `TestAction_UnknownBodyField400`; `TestAction_BodyTooLarge400`.
  - `TestPermission_StubDeny403` (FR-20 AC): stub `deny` → 403 `permission_denied` untuk GET dan POST (gate aktif).
  - `TestMutationGate_Production404` (FR-20 AC, keputusan 8): `SkipRoute(admin.IsMutation)` dengan stub `deny` → `POST /api/platform/modules/acme.example/enable` → 404 `not_found`; GET tetap 403/200 sesuai stub.
  - `TestIDOR_OperationOtherTenant`: backend fake mengembalikan `operation_not_found` untuk ID tenant lain → 404 `not_found` (bukan 403).
  - `config_test.go`: `ERP_MODULES_ADMIN_API=yes` → `ValidationError`; `production` + `true` → `ModulesAdminAPIEnabled() == true`; `production` default → false.
- [ ] **Step 2: Tulis test integrasi yang gagal** — `api_integration_test.go` (backend `lifecycle.Service` nyata):
  - `TestAPI_EnableDisableRoundTrip`: `POST …/acme.example/install` lalu `/enable` → 200; `GET /api/example/notes` → 200; `/disable` → `GET /api/example/notes` → 403 `module_disabled`; `GET …/acme.example/history` memuat event `api:dev`.
  - `TestAPI_IDOR_OtherTenantRows` (CLAUDE.md, keputusan 9): sisipkan langsung ke DB operasi dan event dengan `tenant_id` acak → `GET …/operations/{id}` → 404; `POST …/acme.example/retry` tidak memakai operasi itu (→ `409 invalid_state` karena tidak ada operasi `failed` milik tenant); `history` tidak memuat event tenant lain; `GET /modules` tidak memuat key yang hanya ada pada baris tenant lain.
- [ ] **Step 3: Jalankan** → FAIL.
- [ ] **Step 4: Implementasi** sesuai Interfaces. Tulis `contracts/openapi/common.yaml` (komponen `DataEnvelope`, `Error`, `Page`, parameter `page`/`pageSize`, header `If-Match`/`Idempotency-Key`/`ETag`, respons error standar) dan `platform.yaml` (`/api/bootstrap` + kelima endpoint, `x-erp-permission`, contoh request/respons, kode error per operasi, catatan bahwa `Idempotency-Key` tidak disimpan). Perbarui golden `inspect routes`.
- [ ] **Step 5: Jalankan** unit, integrasi, `tests/security` → PASS.
- [ ] **Step 6: Commit** `feat(modules): platform modules management API with admin gate and OpenAPI contract`

### Task 13: FR-07 guardrail 5 — `PrepareStmt` dan deteksi N+1 di development

**Files:**
- Create: `internal/platform/database/{nplusone.go, nplusone_test.go}`, `internal/platform/database/open_integration_test.go`
- Modify: `internal/platform/database/open.go`, `internal/platform/httpserver/{middleware.go, server.go}` (`Deps.NPlusOneThreshold`), `internal/platform/config/config.go` (+`DBNPlusOneThreshold`), `config_test.go`, `internal/app/assemble.go`, `.env.example`

**Interfaces:**
- Produces:
  ```go
  // database
  func WithQueryCounter(ctx context.Context, threshold int) context.Context // threshold <= 0 → ctx apa adanya
  // slogLogger.Trace: bila ctx membawa counter, hitung per bentuk SQL (string SQL tanpa parameter);
  // saat hitungan == threshold+1 → WARN "indikasi N+1" {sql, count, threshold} sekali per bentuk per request.
  // config
  Config.DBNPlusOneThreshold int // ERP_DB_NPLUSONE_THRESHOLD, default 10, >= 0
  // httpserver
  Deps.NPlusOneThreshold int // > 0 hanya dirakit bila ERP_ENV=development; requestScope memasang counter
  ```
- `gorm.Config{PrepareStmt: true}` di `database.Open`.

- [ ] **Step 1: Tulis test yang gagal**:
  - `nplusone_test.go`: `Trace` dipanggil 11× dengan SQL sama pada ctx counter threshold 10 → tepat satu WARN berisi `indikasi N+1` dan `count=11`; 25× → tetap satu WARN; dua bentuk SQL masing-masing 11× → dua WARN; tanpa counter → tidak ada WARN; log tidak memuat nilai parameter.
  - `config_test.go`: default `10`; `-1` atau `abc` → `ValidationError`.
  - `server_test.go`: `Deps{Dev:true, NPlusOneThreshold:2}` + handler yang memicu 3 `Trace` lewat logger → satu WARN; `Dev:false` → tidak ada.
  - `open_integration_test.go`: `db.Config.PrepareStmt == true`.
- [ ] **Step 2: Jalankan** → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces.
- [ ] **Step 4: Jalankan** → PASS.
- [ ] **Step 5: Commit** `feat(database): enable PrepareStmt and warn on N+1 query shapes in development`

### Task 14: Modul referensi lewat platform — upgrade 1.0.0 → 1.1.0, aturan dependensi, dan penghapusan driver (Q28 butir 3)

**Files:**
- Create: `internal/app/reference_modules_integration_test.go`, `internal/app/versioned_module_test.go` (pembungkus khusus test, keputusan 23)
- Delete: `modules/example/internal/infrastructure/notes_repo_integration_test.go`, `tests/integration/example_module_test.go`
- Modify: `modules/example/go.mod`, `modules/example/go.sum` (`go mod tidy` → `gorm.io/driver/postgres` hilang), `tests/integration/README.md`

**Interfaces:**
- Consumes: `assemble` (Task 9), `lifecycle.Service`, `example.New`, `exampleaddon.New`.
- Produces (test only, `internal/app`):
  ```go
  // versioned membungkus modul dengan versi manifest lain (Raw ditulis ulang lewat json round-trip),
  // subset migrasi (nama file yang diizinkan), dan Upgrader perekam opsional.
  func versioned(m module.Module, version string, migrations []string, rec *upgradeRecorder) module.Module
  type upgradeRecorder struct{ calls [][2]string }
  ```

- [ ] **Step 1: Tulis test integrasi yang gagal** — `reference_modules_integration_test.go`:
  - `TestExample_InstallViaPlatformMigrations` (pengganti test repository Q28): install `acme.example` → `goose_module_example` versi `{1,2}`, `module_migrations` 2 baris berchecksum, `Install` menyemai 3 notes; enable → `GET /api/example/notes` 200 dengan 3 item ber-`version: 1` dan `updatedAt`.
  - `TestExample_Upgrade_1_0_0_to_1_1_0` (AC "Upgrade"): registry `versioned(example, "1.0.0", ["0001…"])` → install + enable, sisipkan satu note; registry `versioned(example, "1.1.0", semua, rec)` → `Reconcile` → `upgrade_pending`, route → 503 `module_unavailable` + `Retry-After: 30`; `Execute(upgrade)` → `0002` diterapkan, note lama `version = 1` dan `updated_at = created_at`, `rec.calls == [["1.0.0","1.1.0"]]`, status `enabled`, route 200, revision naik. Request `GET /api/example/notes` setelah upgrade in-process tetap 200 (tidak ada error *cached plan must not change result type* akibat `PrepareStmt`/cache statement pgx; bila gagal, langkah `migrate` mereset cache statement pool sebelum `Commit`).
  - `TestExample_DowngradeRejected` (AC "Downgrade"): DB terpasang 1.1.0, registry 1.0.0 → `Reconcile` → `failed` + `downgrade_not_supported`, route 503.
  - `TestExampleModules_DependencyRules` (Review Focus 4, AC "Dependency tidak tersedia"): enable `acme.example-addon` saat `acme.example` disabled → `dependency_not_ready`, `Related` berisi `acme.example`; dengan keduanya enabled, disable `acme.example` → `dependents_active`; `GET /api/example-addon/summary` 200 dengan `noteCount` = jumlah note.
  - `TestExample_DisableKeepsDataAndAudits` (AC "Modul dinonaktifkan"): disable → 403 `module_disabled`, `SELECT count(*) FROM example_notes` tidak berubah, event `disabled` dengan actor.
  - `TestExample_UninstallReinstall` (AC "Uninstall"): uninstall → tabel dan data tetap; install ulang → `module_migrations` tidak bertambah.
- [ ] **Step 2: Jalankan** `go test -tags integration ./internal/app/...` → FAIL.
- [ ] **Step 3: Implementasi** helper test; hapus kedua test lama; `go -C modules/example mod tidy`.
- [ ] **Step 4: Verifikasi** `go -C modules/example list -m gorm.io/driver/postgres` → error "not a known dependency"; `GOWORK=off go -C modules/example build ./...` → PASS; `task arch check:gowork-off test test:integration` → hijau.
- [ ] **Step 5: Commit** `test(app): drive reference modules through platform lifecycle; drop postgres driver from example (Q28)`

### Task 15: Exit criteria lewat binary — kedua modul dikelola penuh lewat CLI

**Files:**
- Create: `tests/integration/modules_cli_test.go`, `internal/app/testhooks_integration.go`
- Modify: `tests/integration/README.md`

**Interfaces:**
- Produces (`//go:build integration`, hanya di biner ber-tag `integration`, seperti `testroutes_integration.go`):
  ```go
  // lifecycleTestHooks membaca env khusus test:
  // ERP_TEST_CRASH_AFTER_STEP=<step>  → os.Exit(99) setelah langkah itu selesai (simulasi crash).
  // ERP_TEST_STEP_DELAY=<step>:<durasi> → tidur sebelum langkah itu (memaksa kontensi lock).
  var lifecycleTestHooks = func(d *lifecycle.Deps) { … }
  ```

- [ ] **Step 1: Tulis test yang gagal** — `modules_cli_test.go` (memakai `erpBin`, `baseEnv`, `migrateUp`, `startServe` yang sudah ada):
  - `TestCLI_ManageBothModules`: `modules list` → kedua modul `available`; `modules plan install acme.example-addon --with-deps` → memuat `acme.example` sebelum `acme.example-addon`; `install acme.example-addon --with-deps --enable` → exit 0; `serve` berjalan, `GET /api/example-addon/summary` → 200; `disable acme.example` → exit 3 dengan `dependents_active`; `example:recount` → exit 0 dan `notes: 3`; `disable acme.example-addon` → exit 0 dan dalam ≤ 6s `GET /api/example-addon/summary` → 403 (watcher lintas proses); `disable acme.example` → `example:recount` exit 3; `uninstall acme.example --yes` → exit 0; `history acme.example` memuat `uninstalled`; `install acme.example` → exit 0; `enable acme.example` dua kali → kedua exit 0, dan yang kedua mencetak `tidak ada perubahan`; `/api/bootstrap` revision naik dan ETag berubah; `migrate status` memuat `module:acme.example`.
  - `TestCLI_ExitCodes`: `modules enable acme.nope` → 2; `modules retry 00000000-0000-0000-0000-000000000000` → 2; `modules --tenant <uuid lain> list` → 2; `modules uninstall acme.example` tanpa `--yes` (stdin kosong) → 2.
  - `TestCLI_CrashAfterMigrateThenRetry` (Review Focus 2, AC "Crash"): `ERP_TEST_CRASH_AFTER_STEP=migrate modules install acme.example` → exit 99; `serve` dijalankan lalu dihentikan; `modules status acme.example` → `failed`, operasi terakhir `interrupted`; `modules retry <id>` → exit 0; status `installed`; `goose_module_example` versi 2 dan `module_migrations` tidak menggandakan baris.
  - `TestCLI_ConcurrentEnableTwoProcesses` (Review Focus 1, AC "Konkurensi"): dua proses `modules enable acme.example`, salah satunya dengan `ERP_TEST_STEP_DELAY=hook:enable:7s` → satu exit 0, satu exit 3 dengan `operation_in_progress`; status akhir `enabled`, satu event `enabled`.
- [ ] **Step 2: Jalankan** `task test:integration` → FAIL.
- [ ] **Step 3: Implementasi** `testhooks_integration.go`, lalu perbaiki temuan.
- [ ] **Step 4: Jalankan** `task test:integration` → PASS; `TestServe_NormalBinaryHasNoTestRoute` diperluas untuk memastikan biner non-integration mengabaikan `ERP_TEST_CRASH_AFTER_STEP`.
- [ ] **Step 5: Commit** `test(integration): manage both reference modules end-to-end through the CLI`

### Task 16: ADR-007 final dan dokumentasi

**Files:**
- Modify: `docs/adr/0007-semantik-install-enable-disable-uninstall.md` (Status `Accepted`), `docs/module-sdk/README.md`, `docs/module-sdk/creating-a-module.md`, `docs/operations/deployment.md`, `.env.example`, `docs/prd/PRD-000-inisialisasi-project.md` (baris Q29 + penyelarasan teks FR yang terdampak), `docs/superpowers/plans/2026-10-22-m2-overview.md` (catatan M2b selesai)
- Create: `docs/module-sdk/lifecycle.md`

**Interfaces:** tidak ada (dokumentasi).

ADR-007 wajib memuat: status tersimpan vs turunan; tabel transisi + no-op (keputusan 2); langkah per operasi; aturan kegagalan (keputusan 10) dan retry (keputusan 12); advisory lock (keputusan 14); interrupted (keputusan 11); reconcile + downgrade + pemulihan (keputusan 13); `incompatible` transitif (keputusan 5); modul `required` (keputusan 17); revision dan propagasi (keputusan 3 dan 4); uninstall tidak menghapus data; pemicu evaluasi ulang (multi-instance → `LISTEN/NOTIFY`; Jobs PRD → ganti jalur 202; PRD-001 → principal menggantikan `api:dev`/`cli:*`).

`lifecycle.md` (contoh dari `example`/`exampleaddon`): urutan hook per operasi, kewajiban idempotensi beserta contoh (`Install` menyemai hanya bila kosong), ctx hook (tenant, `data.DB`, `authz.SystemReason`, logger ter-scope), aturan migrasi (satu file per transaksi, jangan menulis penanda anotasi goose di komentar, backfill bertahap untuk tabel besar), `Upgrade(from, to)`, `retry`, dan bagaimana menguji hook dengan `moduletest`.

`deployment.md`: `ERP_MODULES_ADMIN_API` tidak lagi 🔶, `ERP_DB_NPLUSONE_THRESHOLD`, exit code CLI, `migrate up` meng-enable modul `required`, latensi propagasi CLI → `serve` (≤ 2s), `modules retry`, perilaku 202.

PRD-000 Q29 mencatat keputusan rencana yang dikonfirmasi owner (1–5, 6, 7, 10, 13, 19), nama env `ERP_DB_NPLUSONE_THRESHOLD` di FR-07 guardrail 5, dan penyimpangan dari FR-19.3 (revision pada install) bila disetujui.

- [ ] **Step 1:** Tulis dokumen.
- [ ] **Step 2:** Pastikan setiap identifier yang disebut ada di kode (`git grep -n <identifier>` per nama), dan tautan relatif valid.
- [ ] **Step 3: Commit** `docs: accept ADR-007 and document module lifecycle, CLI, and API`

---

## Verifikasi akhir M2b (exit criteria overview: "Semua AC FR-19 di integration test; kedua modul dapat dikelola penuh lewat CLI")

| AC FR-19 / FR-20 / FR-22.4 | Test |
| --- | --- |
| Modul diaktifkan: route, bootstrap, revision | `TestAssemble_BootstrapReflectsLifecycle` (T9), `TestCLI_ManageBothModules` (T15) |
| Modul dinonaktifkan: 403, data tetap, audit dengan actor | `TestExample_DisableKeepsDataAndAudits` (T14), `TestService_AuditEventHasActor` (T7) |
| Dependency tidak tersedia (dua arah) | `TestExampleModules_DependencyRules` (T14) |
| Manifest tidak valid → startup gagal | `run_catalog_test.go` (M2a, tetap hijau) |
| Versi tidak kompatibel → `ResolutionError` | `TestService_IncompatibleInstallRejected` (T8) |
| Migration gagal + retry dari migrasi ke-2 | `TestService_FailingMigrationThenRetry` (T8), `TestApplyModule_SecondFileFailsThenResumes` (T2) |
| Health check gagal saat enable | `TestExecute_HealthFailRevertsToOrigin` (T7) |
| Upgrade 1.0.0 → 1.1.0 | `TestExample_Upgrade_1_0_0_to_1_1_0` (T14) |
| Downgrade ditolak | `TestExample_DowngradeRejected` (T14), `TestService_DowngradeRejectedAndRestored` (T8) |
| Uninstall + install ulang | `TestExample_UninstallReinstall` (T14) |
| Konkurensi | `TestService_ConcurrentEnableTwoStores` (T7), `TestCLI_ConcurrentEnableTwoProcesses` (T15) |
| Crash di tengah operasi | `TestService_InterruptedOperationMarkedUnderLock` (T8), `TestCLI_CrashAfterMigrateThenRetry` (T15) |
| Idempotensi enable | `TestExecute_EnableIdempotent` (T7), `TestCLI_ManageBothModules` (T15) |
| FR-21: semua baris `tenant_id` default | `TestService_AllRowsHaveDefaultTenant` (T7) |
| FR-17: span `module.enable` + child, metrik | `TestExecute_Telemetry` (T7) |
| FR-20: golden `list`/`plan`, exit code | `cmd/erpora/modules_test.go` (T10), `TestCLI_ExitCodes` (T15) |
| FR-20: stub deny → 403; mutasi production → 404 | `TestPermission_StubDeny403`, `TestMutationGate_Production404` (T12) |
| FR-22.4: `example:recount` enabled jalan, disabled exit 3 | `command_integration_test.go` (T11), `TestCLI_ManageBothModules` (T15) |
| IDOR (operasi/event tenant lain) | `TestOperation_TenantScoped` (T3), `TestAPI_IDOR_OtherTenantRows` (T12) |

- [ ] `task ci` exit 0 (termasuk `test:integration`).
- [ ] Manual: `task erpora -- migrate up`, `task erpora -- modules install --all --enable`, `task erpora -- modules list`, `task erpora -- serve`, lalu `curl -s localhost:8080/api/example/ping` → 200 dan `/api/bootstrap` memuat kedua modul `enabled`.
- [ ] Ledger `.superpowers/sdd/<tanggal>-m2b/progress.md` diperbarui; tidak ada push tanpa permintaan user.

## Di luar cakupan M2b (sengaja)

- FR-20.3 UI `/settings/modules` dan E2E Playwright → M3.
- FR-22.1/22.5 penolakan command tulis oleh binary dev terhadap DB berpenanda `production`, `db:seed` (pemanggil `DevSeeder`), dan `db:fresh` → M2d (developer tools + penanda lingkungan).
- Linter Spectral, `contracts:generate`, tipe TS `packages/platform-contracts`, dan test kesesuaian route ↔ kontrak → M2d (FR-23).
- `StateNotifier` berbasis `LISTEN/NOTIFY`, penyimpanan `Idempotency-Key`, dan antrean Jobs → PRD terkait.
- Principal nyata, audit PRD-001 FR-130, dan analyzer `AsSystem` → PRD-001.

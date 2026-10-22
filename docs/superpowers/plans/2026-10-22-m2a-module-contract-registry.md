# M2a — Kontrak Modul & Registry Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Binary `erpora` memuat modul referensi `example` dan `exampleaddon` lewat kontrak SDK. Katalog modul divalidasi saat startup (schema, kompatibilitas, dependensi, capability, route), lalu route modul dipasang di `/api/{slug}` dengan rantai middleware lengkap dan module status guard.

**Architecture:** `sdk/module` mendefinisikan kontrak publik (`Module`, `Registrar`, `Manifest`, `Command`, hook lifecycle). `contracts/module.schema.json` menjadi sumber validasi manifest. Di platform, `modules/manifest` memvalidasi, `modules/compatibility` dan `modules/resolver` memeriksa SemVer dan graf, `modules/registry` merekam registrasi dan menolak kesalahan secara agregat, lalu `modules/runtime` memegang snapshot status dan memasang route ke `httpserver.Router` dengan guard dan permission stub. `internal/app` merakit modul secara eksplisit di `modules.go`. Lifecycle dan persistensi status **belum** ada (M2b), jadi semua modul berstatus `available` dan route-nya membalas `404 module_not_found`.

**Tech Stack:** Go 1.27.3 · `github.com/Masterminds/semver/v3` · `github.com/santhosh-tekuri/jsonschema/v6` (JSON Schema 2020-12) · GORM (hanya di infrastructure modul) · goose (file migrasi modul di-embed; dijalankan di M2b) · cobra · OpenTelemetry

**Spec:** `docs/prd/PRD-000-inisialisasi-project.md` (FR-05, FR-06, FR-15, FR-18, FR-16/ADR-006), `docs/prd/PRD-001-core.md` FR-110 (field manifest opsional), `docs/architecture/api-guidelines.md`, pembagian M2: `docs/superpowers/plans/2026-10-22-m2-overview.md`

## Global Constraints

- Go module path: root `erpora`, SDK `erpora/sdk`, modul `erpora/modules/<name>`. Semua `go.mod`: `go 1.27`, `toolchain go1.27.3`.
- Modul hanya bergantung pada `net/http` stdlib; tidak menerima router (FR-05). Route modul dipasang di `/api/{slug}{path}`.
- Urutan rantai middleware route modul (FR-05): `otel → recover → request-id/log → authN (stub) → tenant context → module status guard → permission check (stub) → handler`. (`otel` di luar `recover` sesuai perbaikan M1: panic tercatat di span aktif.)
- Route `Public` melewati authN dan permission, tetapi tetap melewati otel, recover, log, tenant context, dan module status guard.
- Pseudo-modul platform: key `platform.core`, slug `platform`, versi = `buildinfo.PlatformVersion`, selalu `enabled`.
- Respons guard (FR-19.7): `enabled` → handler; `installed`/`disabled` → `403 module_disabled`; `available`/`uninstalled`/`missing` → `404 module_not_found`; `upgrade_pending`/`failed`/`incompatible` → `503 module_unavailable` (+ `Retry-After: 30` hanya untuk `upgrade_pending`). Permission stub menolak → `403 permission_denied`. Semua kode sudah ada di katalog api-guidelines §8.
- SemVer memakai `github.com/Masterminds/semver/v3`; constraint `^`, `~`, `>=`, `<`, rentang, `||`; pre-release hanya cocok bila constraint menyebutnya (perilaku bawaan library).
- Topological order: Kahn + urutan alfabet untuk tie-break; kebalikannya untuk disable/uninstall.
- GORM hanya di `modules/*/internal/infrastructure/**` (D3b). `AutoMigrate` dilarang. Tabel modul **tanpa** kolom `tenant_id` (database-per-tenant, ADR-004); tabel platform tenant-scoped tetap punya `tenant_id`.
- Semua kesalahan validasi katalog dikumpulkan dan dilaporkan sekaligus, menyebut modul dan field.
- Error validasi katalog saat `erpora serve` = startup gagal dengan exit code 2.
- Migrasi tidak pernah berjalan saat `serve`/HTTP request.
- Tidak ada perubahan breaking pada API yang sudah ada (`/healthz`, `/readyz`, `/api/bootstrap`).
- Taskfile tanpa `rm`/`cp`/`sed`; perintah lintas OS.
- TDD; satu commit per task; pesan commit konvensional diakhiri `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Gate sebelum setiap commit: `task lint arch check:gowork-off test`.

## Keputusan rencana (perlu disetujui pada review rencana)

1. **`buildinfo.PlatformVersion = "0.1.0"`** (konstanta di source, setara `sdk/module.SDKVersion`). `buildinfo.Version` dari `-ldflags` tetap label rilis (bisa `dev`), sehingga tidak dipakai untuk pencocokan SemVer. Workflow tag rilis (M2d) memeriksa tag root `vX.Y.Z` == `PlatformVersion`.
2. **Constraint `platform.core` pada modul referensi = `^0.1`**, bukan `^1.0` seperti contoh tabel FR-15, karena platform masih pre-1.0. Versi modul referensi tetap `1.0.0`, dan `exampleaddon` bergantung pada `acme.example ^1.0` sesuai FR-15.
3. **Permission stub** dikonfigurasi lewat `ERP_AUTHZ_STUB=allow|deny`. Default `allow` bila `ERP_ENV=development`, selain itu `deny`. Setiap keputusan stub dicatat di log level `DEBUG` (allow) atau `WARN` (deny). AuthN stub meneruskan request apa adanya dengan log `DEBUG`.
4. **Slug tercadang** ditolak registry: `platform`, `api`, `assets`, `settings`, `bootstrap`, `healthz`, `readyz`, `sw`, `make`, `db`, `module`, `modules`, `contracts`, `route`, `permission`, `migrate`, `serve`, `inspect`, `version`, `list`, `doctor`, `completion`, `help`. Alasannya: slug dipakai di `/api/{slug}`, `basePath` frontend `/{slug}`, dan nama command `<slug>:<name>`.
5. **`trust`**: schema menerima enum `builtin | verified | untrusted` agar format tidak breaking di fase 06, sedangkan validasi manifest menolak selain `builtin` dengan kode `trust_not_supported`.
6. `StateNotifier`, persistensi status, dan `module_installations` ditunda ke M2b (YAGNI); M2a cukup `runtime.Snapshot` in-memory.

## Review Focus

1. **Slug yang bentrok dengan path atau command platform** (`settings`, `assets`, `db`, `make`, …) → registry menolak dengan kode `reserved_slug`. Test di Task 5.
2. **Path route modul yang mencoba keluar dari namespace**: `"x"` (tanpa `/` awal), `"/../x"`, `"//x"`, `"/a/../b"`, path berisi host (`"example.com/x"`), atau path kosong → ditolak `invalid_route`. Test di Task 5.
3. **Capability yang hanya mirip prefix**: modul `example` mendeklarasikan `example-addon.summary.read` → ditolak (prefix harus tepat `slug + "."`). Test di Task 5.
4. **Dependensi ke diri sendiri** (`acme.a` bergantung pada `acme.a`) → dilaporkan sebagai siklus `acme.a → acme.a`. Test di Task 3.
5. **Trailing slash dan method salah pada route modul**: `GET /api/example/ping/` → `404 not_found` (tidak masuk handler), `POST /api/example/ping` → `405` + `Allow: GET, HEAD`. Test di Task 6.

---

## File Structure (M2a)

```
contracts/
├── module.schema.json                  # Task 1
└── embed.go                            # package contracts: ModuleSchema []byte (Task 1)
sdk/module/
├── manifest.go                         # Manifest + ParseManifest (Task 1)
├── module.go                           # Module, lifecycle interfaces, Env (Task 2)
├── registrar.go                        # Registrar, Command, Flag, Invocation (Task 2)
└── version.go                          # SDKVersion (sudah ada)
internal/platform/buildinfo/buildinfo.go   # + PlatformVersion (Task 3)
internal/platform/modules/
├── manifest/{validate.go, validate_test.go}             # Task 1
├── compatibility/{compat.go, compat_test.go}            # Task 3
├── resolver/{graph.go, errors.go, graph_test.go}        # Task 3
├── registry/{registry.go, recorder.go, errors.go, reserved.go, registry_test.go}   # Task 4, 5
├── runtime/{status.go, snapshot.go, mount.go, mount_test.go}   # Task 6
└── testdata/manifests/*.json           # fixture manifest (Task 1)
internal/platform/authorization/stub.go # permission stub (Task 6)
internal/platform/httpserver/server.go  # + Deps.Authenticate, RouteOption Public() (Task 6)
internal/platform/config/config.go      # + ERP_AUTHZ_STUB (Task 6)
modules/example/                        # Task 7
├── module.go, module.json, contract/notes.go
├── migrations/0001_create_example_notes.sql
└── internal/{domain/note.go, application/list_notes.go, infrastructure/notes_repo.go, presentation/http/handlers.go}
modules/exampleaddon/                   # Task 8 (Go module baru)
├── go.mod, module.go, module.json
└── internal/presentation/http/summary.go
internal/app/{modules.go, registry.go}  # Task 9
cmd/erpora/inspect.go                   # + routes|modules|permissions, alias route:list, permission:list (Task 9)
tests/security/{public_routes.txt, public_routes_test.go}   # Task 9
docs/adr/0006-module-manifest-sdk-semver.md                 # Task 10
docs/module-sdk/creating-a-module.md                        # Task 10
```

---

### Task 1: Manifest schema, tipe SDK `Manifest`, dan validasi

**Files:**
- Create: `contracts/module.schema.json`, `contracts/embed.go`, `sdk/module/manifest.go`, `sdk/module/manifest_test.go`, `internal/platform/modules/manifest/validate.go`, `internal/platform/modules/manifest/validate_test.go`, `internal/platform/modules/testdata/manifests/{valid_full.json, bad_version.json, bad_key.json, bad_constraint.json, trust_untrusted.json, missing_slug.json, unknown_field.json}`
- Modify: `go.mod` (require `github.com/santhosh-tekuri/jsonschema/v6`, `github.com/Masterminds/semver/v3`)

**Interfaces:**
- Produces (SDK, `erpora/sdk/module`):
  ```go
  type Manifest struct {
      SchemaVersion string        `json:"schema_version"`
      Key           string        `json:"key"`
      Slug          string        `json:"slug"`
      Name          string        `json:"name"`
      Version       string        `json:"version"`
      Publisher     Publisher     `json:"publisher"`
      Trust         string        `json:"trust"`
      Required      bool          `json:"required,omitempty"`
      Compatibility Compatibility `json:"compatibility"`
      Dependencies  []Dependency  `json:"dependencies"`
      Capabilities  []string      `json:"capabilities"`
      Provides      []string      `json:"provides,omitempty"`
      Migrations    Migrations    `json:"migrations"`
      Offline       Offline       `json:"offline"`
      Raw           []byte        `json:"-"` // byte module.json asli; divalidasi platform terhadap schema
  }
  type Publisher struct{ ID, Name string }            // json: id, name
  type Compatibility struct{ Platform, SDK string }   // json: platform, sdk
  type Dependency struct{ Module, Version string; Optional bool } // json: module, version, optional
  type Migrations struct{ Strategy string }           // "versioned" | "none"
  type Offline struct{ Mode string }                  // online-only | offline-read | offline-draft | offline-write
  func ParseManifest(b []byte) (Manifest, error)      // json.Unmarshal + salin b ke Raw
  func MustParseManifest(b []byte) Manifest           // panic bila error; dipakai modul dengan //go:embed module.json
  ```
- Produces (root): `contracts.ModuleSchema []byte`; `manifest.Validate(m module.Manifest) []manifest.Problem` dengan `type Problem struct{ Module, Field, Code, Message string }`. Kode: `schema_invalid`, `trust_not_supported`, `invalid_version`, `invalid_constraint`, `raw_missing`.

Schema (JSON Schema 2020-12, `additionalProperties: false` di level atas):
- Wajib: `schema_version` (const `"1"`), `key` (pattern `^[a-z][a-z0-9]*(-[a-z0-9]+)*\.[a-z][a-z0-9]*(-[a-z0-9]+)*$`), `slug` (pattern `^[a-z][a-z0-9]*(-[a-z0-9]+)*$`), `name` (string 1–100), `version` (pattern SemVer 2.0.0 resmi), `publisher {id, name}`, `trust` (enum `builtin|verified|untrusted`), `compatibility {platform, sdk}` (string non-kosong), `dependencies[] {module, version, optional}`, `capabilities[]` (pattern `^[a-z][a-z0-9-]*(\.[a-z][a-z0-9_]*)+$`, `uniqueItems`), `migrations {strategy}` (enum `versioned|none`), `offline {mode}` (enum di atas).
- Opsional: `provides[]`, `required` (boolean), field marketplace `artifact`, `checksum`, `signature`, `requested_permissions`, `license`; field PRD-001 `resources`, `scope_dimensions`, `default_roles`, `settings` (bertipe `array`/`object`, isi bebas untuk saat ini, `description` merujuk PRD-001 FR-110/FR-120).

- [ ] **Step 1: Tulis test SDK yang gagal** — `sdk/module/manifest_test.go`:
  - `TestParseManifest_FillsFieldsAndRaw`: parse JSON minimal valid → `Key == "acme.example"`, `Dependencies[0].Module == "platform.core"`, `bytes.Equal(m.Raw, input)`.
  - `TestParseManifest_InvalidJSON`: input `{` → error non-nil.
- [ ] **Step 2: Jalankan** `go test ./sdk/module/...` (dari root, workspace) → FAIL: `undefined: ParseManifest`.
- [ ] **Step 3: Implementasi** `sdk/module/manifest.go` sesuai Interfaces.
- [ ] **Step 4: Tulis test validasi yang gagal** — `internal/platform/modules/manifest/validate_test.go`, tabel kasus memakai fixture `testdata/manifests`:
  | Fixture | Problem yang diharapkan (`Code`, `Field`) |
  | --- | --- |
  | `valid_full.json` (memuat semua field opsional termasuk `resources`, `settings`) | tidak ada |
  | `bad_version.json` (`"version": "1.0"`) | `schema_invalid`, `/version` |
  | `bad_key.json` (`"key": "example"`) | `schema_invalid`, `/key` |
  | `bad_constraint.json` (`dependencies[0].version: "^^1"`) | `invalid_constraint`, `/dependencies/0/version` |
  | `trust_untrusted.json` | `trust_not_supported`, `/trust` |
  | `missing_slug.json` | `schema_invalid`, `/slug` |
  | `unknown_field.json` (`"routes": []`) | `schema_invalid`, `/routes` |
  - `TestValidate_RawMissing`: `module.Manifest{Key: "acme.x"}` tanpa `Raw` → `raw_missing`.
  - `TestValidate_ReportsAllProblems`: satu manifest dengan `version` salah **dan** `trust: "untrusted"` → dua problem sekaligus.
- [ ] **Step 5: Jalankan** `go test ./internal/platform/modules/manifest/...` → FAIL.
- [ ] **Step 6: Implementasi** `contracts/embed.go` (`//go:embed module.schema.json`), `contracts/module.schema.json`, dan `manifest.Validate`. Kompilasi schema sekali (`sync.OnceValues`). Validasi `Raw` terhadap schema, lalu cek tambahan Go: `trust == "builtin"`, `semver.NewVersion(version)`, `semver.NewConstraint` untuk `compatibility.*` dan setiap `dependencies[].version`. `Field` memakai JSON pointer dari error schema.
- [ ] **Step 7: Jalankan** test kedua paket → PASS; `task lint arch check:gowork-off test` → hijau.
- [ ] **Step 8: Commit** `feat(modules): module manifest schema, SDK Manifest type, and validation`

### Task 2: Kontrak SDK `Module`, `Registrar`, `Command`, dan hook lifecycle

**Files:**
- Create: `sdk/module/module.go`, `sdk/module/registrar.go`, `sdk/module/contract_test.go`
- Modify: `sdk/module/doc.go` (ringkasan kontrak)

**Interfaces:**
- Produces (`erpora/sdk/module`), persis seperti PRD-000 FR-05:
  ```go
  type Module interface { Manifest() Manifest; Register(r Registrar) error }
  type Registrar interface {
      Handle(method, path, permission string, h http.Handler)
      Public(method, path string, h http.Handler)
      Command(c Command)
  }
  type MigrationSource interface{ Migrations() fs.FS }
  type Installer interface{ Install(ctx context.Context, env Env) error }
  type Upgrader interface{ Upgrade(ctx context.Context, env Env, from, to string) error }
  type EnableHook interface{ OnEnable(ctx context.Context, env Env) error }
  type DisableHook interface{ OnDisable(ctx context.Context, env Env) error }
  type HealthChecker interface{ Health(ctx context.Context, env Env) error }
  type DevSeeder interface{ SeedDev(ctx context.Context, env Env) error }
  // Env: konteks terbatas untuk hook & command. Akses data lewat data.DB(ctx); platform menjamin
  // ctx yang diberikan ke hook/command membawa data.Provider tenant yang sama dengan Env.TenantID.
  type Env struct { TenantID string; Logger *slog.Logger }
  type Flag struct{ Name, Usage, Default string }
  type Invocation struct { Args []string; Flags map[string]string; Out io.Writer; Env Env }
  type Command struct { Name, Short string; Flags []Flag; Run func(ctx context.Context, inv Invocation) error }
  ```
- Tidak ada perilaku: hanya tipe. `Registrar` diimplementasi platform (Task 4).

- [ ] **Step 1: Tulis test kompilasi** `sdk/module/contract_test.go`: tipe `fakeModule` yang mengimplementasi `Module` dan **semua** interface lifecycle, dengan assertion compile-time `var _ module.Installer = fakeModule{}` dst., plus `TestCommand_ZeroValueRunNil` yang memastikan `Command{}.Run == nil` (dokumentasi bahwa registry wajib menolak `Run` nil).
- [ ] **Step 2: Jalankan** `go test ./sdk/...` → FAIL (tipe belum ada).
- [ ] **Step 3: Implementasi** `module.go` dan `registrar.go` sesuai Interfaces, dengan komentar Go doc berbahasa Indonesia per tipe (idempotensi hook wajib — FR-19.5).
- [ ] **Step 4: Jalankan** `go test ./sdk/...` dan `GOWORK=off go -C sdk build ./...` → PASS. Pastikan `sdk/go.mod` tidak bertambah dependency selain stdlib dan GORM yang sudah ada.
- [ ] **Step 5: Commit** `feat(sdk): module runtime and lifecycle contract`

### Task 3: Compatibility & dependency resolver

**Files:**
- Create: `internal/platform/modules/compatibility/compat.go`, `compat_test.go`, `internal/platform/modules/resolver/graph.go`, `errors.go`, `graph_test.go`
- Modify: `internal/platform/buildinfo/buildinfo.go` (+ `const PlatformVersion = "0.1.0"`)

**Interfaces:**
- Produces:
  ```go
  // compatibility
  func Satisfies(version, constraint string) (bool, error) // error bila version/constraint tidak valid
  // resolver
  type Dep struct{ Key, Constraint string; Optional bool }
  type Node struct{ Key, Version, PlatformConstraint, SDKConstraint string; Deps []Dep }
  type Versions struct{ Platform, SDK string }
  type Issue struct{ Code, Module, Dependency, Constraint, Found string; Path []string }
  type ResolutionError struct{ Issues []Issue }   // Error() menggabungkan semua issue, satu per baris
  type Graph struct{ /* privat */ }
  func Resolve(nodes []Node, v Versions) (*Graph, error) // error = *ResolutionError untuk masalah struktural
  func (g *Graph) Order() []string                        // topologis, deterministik
  func (g *Graph) Incompatible() map[string][]Issue       // platform/sdk tidak cocok (bukan error startup)
  func (g *Graph) RequiredDependencies(key string) []string // transitif, urutan topologis
  func (g *Graph) RequiredDependents(key string) []string   // transitif, urutan topologis terbalik
  func (g *Graph) Node(key string) (Node, bool)
  ```
- Kode `Issue`: `missing_dependency`, `version_mismatch`, `optional_version_mismatch`, `cycle` (`Path` = jalur lengkap, mis. `[a b c a]`), `invalid_constraint`, `platform_incompatible`, `sdk_incompatible`.
- Klasifikasi: `missing_dependency`, `version_mismatch`, `optional_version_mismatch`, `cycle`, `invalid_constraint` → `*ResolutionError` (katalog rusak = bug build, FR-19.6). `platform_incompatible`/`sdk_incompatible` → `Incompatible()`; modul itu tidak bisa di-install/enable (M2b membungkusnya menjadi `ResolutionError` saat operasi).
- Dependensi opsional yang tidak ada: tidak ada issue (cukup diabaikan). Edge ke dependensi opsional yang ada tetap ikut menentukan urutan.

- [ ] **Step 1: Tulis test yang gagal** — `compat_test.go`: `Satisfies("1.2.3","^1.0")` true; `("2.0.0","^1.0")` false; `("1.3.0-rc.1","^1.0")` false; `("1.3.0-rc.1",">=1.3.0-rc.0")` true; `("1.2.3","~1.2")` true; `("1.5.0",">=1.2 <2.0")` true; `("3.0.0","^1.0 || ^3.0")` true; `("x","^1")` error; `("1.0.0","^^1")` error.
- [ ] **Step 2: Tulis test resolver yang gagal** — `graph_test.go` (tabel, `Versions{Platform:"0.1.0", SDK:"0.1.0"}`):
  - dependency hilang → `missing_dependency` dengan `Module`, `Dependency`;
  - versi dependency tidak memenuhi → `version_mismatch` dengan `Constraint` dan `Found`;
  - siklus 2 node (`a→b→a`) dan 3 node (`a→b→c→a`) → `cycle` dengan `Path` lengkap berawal dan berakhir di node yang sama;
  - self-loop `a→a` → `cycle` dengan `Path == [a a]` (Review Focus 4);
  - opsional hilang → lolos; opsional salah versi → `optional_version_mismatch`;
  - platform tidak kompatibel (`PlatformConstraint "^1.0"`) → tidak error, `Incompatible()["acme.a"][0].Code == "platform_incompatible"`; idem SDK;
  - `TestResolve_ReportsAllIssuesTogether`: katalog dengan dependency hilang + siklus → keduanya ada di `Issues`;
  - `TestOrder_StableUnderShuffle`: 6 node dengan beberapa sumber; acak urutan input 100 kali (`rand.New(rand.NewPCG(1,2))`) → `Order()` selalu sama dan `platform.core` di posisi pertama;
  - `TestRequiredDependents`: `example-addon` bergantung wajib pada `example` → `RequiredDependents("acme.example") == ["acme.example-addon"]`; dependen opsional tidak ikut.
- [ ] **Step 3: Jalankan** `go test ./internal/platform/modules/...` → FAIL.
- [ ] **Step 4: Implementasi.** Deteksi siklus memakai DFS tiga warna dan merekonstruksi jalur dari stack; Kahn dengan antrean terurut alfabet (`slices.Sort` pada himpunan node siap di setiap langkah).
- [ ] **Step 5: Jalankan** test → PASS; gate lokal hijau.
- [ ] **Step 6: Commit** `feat(modules): SemVer compatibility and dependency resolver`

### Task 4: Registry — rekam registrasi, route table, command table

**Files:**
- Create: `internal/platform/modules/registry/{registry.go, recorder.go, errors.go, reserved.go, registry_test.go}`

**Interfaces:**
- Consumes: `manifest.Validate` (Task 1), `module.*` (Task 2), `resolver.Resolve` (Task 3), `buildinfo.PlatformVersion`, `module.SDKVersion`.
- Produces:
  ```go
  type Options struct {
      PlatformVersion, SDKVersion string
      // Platform mendaftarkan route/command milik platform.core (dipakai M2b untuk /api/platform/modules).
      Platform func(r module.Registrar) error
  }
  type RouteInfo struct {
      Module, Method, Pattern, Permission string // Pattern = path penuh "/api/{slug}{path}"
      Public bool
      Handler http.Handler `json:"-"`
  }
  type CommandInfo struct{ Module, Name string; Command module.Command } // Name = "<slug>:<name>"
  type Entry struct{ Manifest module.Manifest; Module module.Module } // Module nil untuk platform.core
  type Problem struct{ Module, Field, Code, Message string }
  type Error struct{ Problems []Problem } // Error(): satu baris per problem, terurut (Module, Field, Code)
  type Registry struct{ /* privat */ }
  func Build(opts Options, mods ...module.Module) (*Registry, error) // error = *Error (berisi juga ResolutionError issue sebagai Problem kode resolver)
  func (r *Registry) Routes() []RouteInfo         // terurut (Pattern, Method)
  func (r *Registry) Commands() []CommandInfo     // terurut Name
  func (r *Registry) Entries() []Entry            // urutan topologis graph
  func (r *Registry) Entry(key string) (Entry, bool)
  func (r *Registry) Graph() *resolver.Graph
  func (r *Registry) Capabilities() []string      // semua capability, terurut
  const PlatformKey = "platform.core"; const PlatformSlug = "platform"
  ```
- `platform.core` dibuat registry sendiri: `Manifest{Key: PlatformKey, Slug: PlatformSlug, Version: opts.PlatformVersion, Trust: "builtin", Capabilities: nil}` (M2b menambah capability lewat `Options.Platform`); manifest ini tidak melewati validasi schema.

- [ ] **Step 1: Tulis test yang gagal** — `registry_test.go` memakai `fakeModule` (manifest dari JSON inline lewat `module.MustParseManifest`, fungsi `register` yang dapat diatur):
  - `TestBuild_RoutesPrefixedBySlug`: modul `acme.notes` slug `notes` mendaftarkan `Handle("GET","/items","notes.items.read",h)` → `Routes()` berisi `{Module:"acme.notes", Method:"GET", Pattern:"/api/notes/items", Permission:"notes.items.read"}`.
  - `TestBuild_PublicRoute`: `Public("GET","/status",h)` → `Public:true`, `Permission:""`.
  - `TestBuild_CommandsPrefixed`: `Command{Name:"recount", Run: f}` → `Commands()[0].Name == "notes:recount"`.
  - `TestBuild_PlatformCoreAlwaysPresent`: tanpa modul → `Entry("platform.core")` ada dengan versi `opts.PlatformVersion`; `Entries()[0].Manifest.Key == "platform.core"`.
  - `TestBuild_RegisterErrorReported`: `Register` mengembalikan error → `Problem{Code:"register_failed"}` dengan nama modul.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/modules/registry/...` → FAIL.
- [ ] **Step 3: Implementasi** `Build`: validasi manifest (Task 1) untuk setiap modul → resolve graph (Task 3) dengan node dari manifest + node `platform.core` → panggil `Register` per modul **dalam urutan topologis** dengan `recorder` (implementasi `module.Registrar` per modul yang hanya merekam). Semua problem dikumpulkan; `Build` tidak berhenti di error pertama.
- [ ] **Step 4: Jalankan** test → PASS.
- [ ] **Step 5: Commit** `feat(modules): registry records module routes and commands`

### Task 5: Registry — tujuh aturan validasi FR-05 + aturan tambahan

**Files:**
- Modify: `internal/platform/modules/registry/{registry.go, recorder.go, reserved.go}`
- Test: `internal/platform/modules/registry/rules_test.go`

**Interfaces:**
- Kode `Problem` yang ditambahkan: `duplicate_key`, `duplicate_slug`, `reserved_slug`, `duplicate_capability`, `capability_prefix`, `permission_missing`, `permission_undeclared`, `invalid_route`, `route_conflict`, `invalid_command`, `command_conflict`, ditambah kode schema/resolver yang diteruskan dari Task 1 dan Task 3.

- [ ] **Step 1: Tulis test tabel yang gagal** — `rules_test.go`, setiap kasus memeriksa `Code` dan `Module` pada problem:
  1. dua modul dengan `key` sama → `duplicate_key`; dua modul `slug` sama → `duplicate_slug`;
  2. manifest invalid (versi `1.0`) → problem `schema_invalid` dengan `Field:"/version"`;
  3. capability sama di dua modul → `duplicate_capability` (menyebut kedua modul di `Message`); modul `example` dengan capability `example-addon.summary.read` → `capability_prefix` (Review Focus 3); capability `other.x.read` → `capability_prefix`;
  4. `Handle` dengan permission `""` → `permission_missing`; permission yang tidak ada di `capabilities` → `permission_undeclared`;
  5. dua `Handle("GET","/items",…)` di modul yang sama, dan pola yang dianggap konflik oleh ServeMux (`"/{a}/x"` vs `"/x/{b}"`) → `route_conflict` dengan `Message` menyebut kedua pendaftaran (modul + pola); tidak boleh panic;
  6. `Routes()` read-only: mengubah slice hasil tidak mengubah isi registry;
  7. `TestBuild_ReportsAllProblemsAtOnce`: katalog dengan `duplicate_slug` + `permission_missing` + `missing_dependency` → ketiganya muncul dalam satu `*Error`;
  8. slug tercadang (`settings`, `db`, `assets`) → `reserved_slug` (Review Focus 1);
  9. path route tidak valid → `invalid_route` untuk masing-masing: `"x"`, `""`, `"/../x"`, `"/a/../b"`, `"//x"`, `"example.com/x"`, method `"FETCH"` (Review Focus 2). Method yang diizinkan: `GET, POST, PUT, PATCH, DELETE`;
  10. command dengan `Name` tidak cocok `^[a-z][a-z0-9-]*$` atau `Run == nil` → `invalid_command`; dua command bernama sama di satu modul → `command_conflict`.
- [ ] **Step 2: Jalankan** → FAIL.
- [ ] **Step 3: Implementasi.** Validasi path: harus diawali `/`, `path.Clean(p) == p` (kecuali `"/"`), tidak mengandung `//`, tidak memuat host. Deteksi konflik ServeMux: daftarkan setiap pola penuh ke `http.NewServeMux()` sementara di dalam `func` dengan `recover()`, lalu ubah panic menjadi `route_conflict` dengan pemilik kedua pola (peta pola → modul).
- [ ] **Step 4: Jalankan** test registry → PASS; gate lokal hijau.
- [ ] **Step 5: Commit** `feat(modules): registry validation rules (FR-05)`

### Task 6: Runtime snapshot, module status guard, permission stub, dan pemasangan route

**Files:**
- Create: `internal/platform/modules/runtime/{status.go, snapshot.go, mount.go, mount_test.go}`, `internal/platform/authorization/stub.go`, `internal/platform/authorization/stub_test.go`
- Modify: `internal/platform/httpserver/server.go` (`Deps.Authenticate`, opsi `Public()`), `internal/platform/httpserver/server_test.go`, `internal/platform/config/config.go` (+`AuthzStub`), `config_test.go`, `.env.example`

**Interfaces:**
- Consumes: `registry.Registry.Routes()` (Task 4), `httpserver.Router.Handle`, `httpserver.WriteError`, `tenancy.FromContext`.
- Produces:
  ```go
  // runtime
  type Status string
  const (StatusAvailable Status = "available"; StatusInstalled = "installed"; StatusEnabled = "enabled"
         StatusDisabled = "disabled"; StatusUpgradePending = "upgrade_pending"; StatusFailed = "failed"
         StatusUninstalled = "uninstalled"; StatusMissing = "missing"; StatusIncompatible = "incompatible")
  type Snapshot struct{ /* sync.RWMutex + map[tenancy.TenantID]state */ }
  func NewSnapshot() *Snapshot
  func (s *Snapshot) Set(t tenancy.TenantID, statuses map[string]Status, revision int64)
  func (s *Snapshot) Status(t tenancy.TenantID, key string) Status // platform.core selalu enabled; tak dikenal → available
  func (s *Snapshot) Revision(t tenancy.TenantID) int64
  type Authorizer interface{ Allow(ctx context.Context, permission string) bool }
  type Mounter interface{ Handle(pattern string, h http.Handler, opts ...httpserver.RouteOption) }
  func Mount(m Mounter, reg *registry.Registry, snap *Snapshot, authz Authorizer)
  // authorization
  type StubMode string // "allow" | "deny"
  type Stub struct{ Mode StubMode; Log *slog.Logger } // implementasi runtime.Authorizer
  // httpserver
  type Deps struct{ …; Authenticate func(http.Handler) http.Handler } // nil = pass-through
  func Public() RouteOption // lewati Authenticate
  // config
  Config.AuthzStub string // ERP_AUTHZ_STUB: allow|deny; default allow bila ERP_ENV=development, selain itu deny
  ```
- `Router.Handle` urutan baru: `otel → recover → requestScope → Authenticate (kecuali Public) → tenantContext → h`. `Mount` membungkus `h` dengan `guard → permission (kecuali Public) → handler`, mendaftarkan route `Public` dengan opsi `httpserver.Public()`, memberi atribut span `erp.module.id` (key modul) dan atribut log `module`.
- Pola yang didaftarkan ke router: `"<METHOD> /api/{slug}{path}"`. ServeMux otomatis menangani `HEAD` untuk `GET`.

- [ ] **Step 1: Tulis test yang gagal** — `mount_test.go` (registry dari `fakeModule`, router dari `httpserver.NewRouter` dengan `tracetest.SpanRecorder`, tenant tetap):
  - tabel guard: snapshot `enabled` → 200 dari handler; `installed`, `disabled` → 403 `module_disabled`; `available`, `uninstalled`, `missing` → 404 `module_not_found`; `failed`, `incompatible` → 503 `module_unavailable` tanpa `Retry-After`; `upgrade_pending` → 503 dengan `Retry-After: 30`;
  - guard berlaku untuk route `Public` (status `disabled` → 403);
  - permission stub `deny` → 403 `permission_denied` untuk route terproteksi, sedangkan route `Public` tetap 200;
  - `TestMount_MiddlewareOrder`: handler memeriksa `observability.RequestID(ctx) != ""`, `trace.SpanContextFromContext(ctx).IsValid()`, `tenancy.FromContext(ctx)` ok; span yang direkam memiliki atribut `erp.module.id == "acme.notes"`; `Authenticate` dipanggil sebelum `tenantContext` (catat urutan lewat slice di test);
  - `TestMount_TrailingSlashAndWrongMethod` (Review Focus 5): `GET /api/notes/items/` → 404 `not_found` tanpa memanggil handler; `POST /api/notes/items` → 405 dengan `Allow` berisi `GET, HEAD`.
  - `stub_test.go`: `allow` → true, `deny` → false; keduanya menulis log dengan `permission`.
  - `config_test.go`: tanpa `ERP_AUTHZ_STUB` → `allow` di development dan `deny` di production; nilai `maybe` → `ValidationError`.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces. `Snapshot` aman untuk konkuren (`sync.RWMutex`). `Mount` tidak memasang route platform M1 yang sudah ada.
- [ ] **Step 4: Jalankan** test → PASS; jalankan juga seluruh test `httpserver` (urutan rantai baru tidak merusak test M1).
- [ ] **Step 5: Commit** `feat(modules): runtime snapshot, module status guard, and route mounting`

### Task 7: Modul referensi `example` (backend)

**Files:**
- Create: `modules/example/module.go`, `modules/example/contract/notes.go`, `modules/example/migrations/0001_create_example_notes.sql`, `modules/example/internal/domain/note.go` (+test), `modules/example/internal/application/list_notes.go` (+test), `modules/example/internal/infrastructure/notes_repo.go`, `modules/example/internal/presentation/http/handlers.go` (+test), `modules/example/module_test.go`
- Modify: `modules/example/module.json`, `modules/example/go.mod` (require `erpora/sdk`, `gorm.io/gorm`, `github.com/google/uuid`), hapus `doc.go` stub yang digantikan file nyata
- Delete: `modules/example/doc.go` (isi dipindah ke komentar package di `module.go`)

**Interfaces:**
- Consumes: `module.*`, `data.DB(ctx)`.
- Produces:
  ```go
  package example
  func New() *Module                    // implementasi module.Module, MigrationSource, Installer, HealthChecker, DevSeeder
  func (m *Module) Notes() contract.Notes
  package contract
  type Notes interface{ Count(ctx context.Context) (int64, error) }
  ```
- `module.json`: key `acme.example`, slug `example`, name `Example`, version `1.0.0`, publisher `{id:"acme", name:"ACME"}`, trust `builtin`, compatibility `{platform:"^0.1", sdk:"^0.1"}`, dependencies `[{module:"platform.core", version:"^0.1", optional:false}]`, capabilities `["example.ping.read","example.notes.read"]`, migrations `{strategy:"versioned"}`, offline `{mode:"online-only"}`.
- Route: `Handle("GET","/ping","example.ping.read",…)` → `200 {"data":{"pong":true}}`; `Handle("GET","/notes","example.notes.read",…)` → daftar notes dengan pagination halaman api-guidelines §6 (`page` mulai 1, `pageSize` default 25 maks. 200, respons `"page": {"page","pageSize","total","hasMore"}`; parameter tidak valid → `400 bad_request`). Field JSON note: `id` (UUID string), `title`, `createdAt` (RFC 3339 UTC).
- Command: `recount` → menulis `notes: <n>` ke `inv.Out`.
- Migrasi `0001`: tabel `example_notes(id uuid primary key, title text not null check (length(title) between 1 and 200), created_at timestamptz not null default now())` + indeks `created_at`. Tanpa `tenant_id` (ADR-004).
- `Install`: seed 3 notes **hanya bila tabel kosong** (idempoten). `SeedDev`: tambah 20 notes contoh bila jumlah < 20. `Health`: `SELECT 1 FROM example_notes LIMIT 1` lewat repository.
- `contract.Notes.Count`: tidak menerima ID dan hanya mengembalikan agregat tenant aktif dari context. Komentar Go doc menyatakan bahwa pemeriksaan permission di dalam kontrak ditambahkan saat `sdk/authz` tersedia (PRD-001 FR-119).

- [ ] **Step 1: Tulis test yang gagal**:
  - `module_test.go`: `TestManifest_ValidAgainstPlatformRules` tidak bisa memakai paket platform (D9), jadi cukup `TestManifest_Parses` (`New().Manifest().Key == "acme.example"`, `Raw` tidak kosong) dan `TestMigrations_Embedded` (`fs.Glob(New().Migrations(), "*.sql")` berisi `0001_create_example_notes.sql`). Validasi schema modul ini dilakukan di Task 9 (test registry nyata).
  - `domain/note_test.go`: `NewNote("")` dan judul 201 karakter → error; judul 1–200 karakter → ok.
  - `presentation/http/handlers_test.go` (dengan fake use case): `GET /notes?page=0` → 400 `bad_request`; `pageSize=201` → 400; default → `page.pageSize == 25`; respons memakai amplop `data` + `page`; `GET /ping` → `{"data":{"pong":true}}`.
- [ ] **Step 2: Jalankan** `go test ./modules/example/...` → FAIL.
- [ ] **Step 3: Implementasi** domain → application (`ListNotes(ctx, page, pageSize) (items []domain.Note, total int64, err error)`) → infrastructure (model GORM `noteRow` terpisah dari entity, mapping eksplisit, query lewat `data.DB(ctx)`) → presentation → `module.go` (`//go:embed module.json`, `//go:embed migrations/*.sql`).
- [ ] **Step 4: Jalankan** `go test ./modules/example/...`, `GOWORK=off go -C modules/example build ./...`, `task arch` → PASS.
- [ ] **Step 5: Commit** `feat(example): reference module backend (routes, migration, hooks, contract, command)`

### Task 8: Modul referensi `exampleaddon` (Go module baru)

**Files:**
- Create: `modules/exampleaddon/{go.mod, module.go, module.json, module_test.go}`, `modules/exampleaddon/internal/presentation/http/{summary.go, summary_test.go}`
- Modify: `go.work` (+`use ./modules/exampleaddon`), `go.work.sum` (bila berubah), root `go.mod` (require + replace `erpora/modules/exampleaddon`)

**Interfaces:**
- Consumes: `erpora/modules/example/contract.Notes` (Task 7).
- Produces: `package exampleaddon; func New(notes contract.Notes) *Module` — implementasi `module.Module`, `module.EnableHook`, `module.DisableHook`.
- `module.json`: key `acme.example-addon`, slug `example-addon`, name `Example Add-on`, version `1.0.0`, dependencies `platform.core ^0.1` dan `acme.example ^1.0` (keduanya `optional:false`), capabilities `["example-addon.summary.read"]`, migrations `{strategy:"none"}`, offline `online-only`, compatibility sama dengan `example`.
- `go.mod`: `module erpora/modules/exampleaddon`; require `erpora/sdk` dan `erpora/modules/example`; `replace` relatif ke `../../sdk` dan `../example` (D8: require == dependencies manifest).
- Route: `Handle("GET","/summary","example-addon.summary.read",…)` → `200 {"data":{"noteCount":<int>}}`; error dari kontrak → `500 internal_error` tanpa detail internal.
- `OnEnable`/`OnDisable`: menulis log `INFO` lewat `env.Logger` dengan atribut `module`.

- [ ] **Step 1: Tulis test yang gagal** — `summary_test.go` dengan fake `contract.Notes` (Count=3 → `noteCount:3`; error → 500 envelope `internal_error`); `module_test.go`: hook enable/disable menulis log berisi `acme.example-addon` (logger ke `bytes.Buffer`).
- [ ] **Step 2: Jalankan** `go test ./modules/exampleaddon/...` → FAIL.
- [ ] **Step 3: Implementasi** dan daftarkan modul di `go.work` serta root `go.mod`. Pastikan loop Taskfile/CI per modul (`go list -m` di workspace) otomatis mencakup modul baru; bila ada daftar modul yang ditulis manual, tambahkan.
- [ ] **Step 4: Jalankan** `go test ./modules/exampleaddon/...`, `GOWORK=off go -C modules/exampleaddon build ./...`, `task arch check:gowork-off` → PASS (D8: require ↔ dependencies cocok).
- [ ] **Step 5: Commit** `feat(exampleaddon): reference add-on module depending on example contract`

### Task 9: Composition root, startup validation, inspect, dan security test route publik

**Files:**
- Create: `internal/app/modules.go`, `internal/app/registry.go`, `internal/app/registry_test.go`, `tests/security/public_routes.txt`, `tests/security/public_routes_test.go`
- Modify: `internal/app/app.go`, `cmd/erpora/inspect.go`, `cmd/erpora/cli_test.go` (+ golden `cmd/erpora/testdata/inspect_routes.golden`), `cmd/erpora/root.go` (alias), `tests/security/README.md`

**Interfaces:**
- Consumes: `registry.Build`, `runtime.Mount`, `runtime.NewSnapshot`, `authorization.Stub`, `example.New`, `exampleaddon.New`.
- Produces:
  ```go
  package app
  // compiledModules: modul yang dikompilasi ke binary. Baris baru ditambahkan di atas penanda (FR-22.3).
  func compiledModules() []module.Module
  // BuildRegistry memvalidasi katalog modul terkompilasi; dipakai serve, inspect, dan security test.
  func BuildRegistry() (*registry.Registry, error)
  ```
- `compiledModules` (wiring dependensi kontrak eksplisit):
  ```go
  ex := example.New()
  return []module.Module{
      ex,
      exampleaddon.New(ex.Notes()),
      // erp:generated:modules
  }
  ```
- `app.Run`: `BuildRegistry()` dipanggil **sebelum** membuka DB; error → `&ExitError{Code: 2, Err: …}` dengan pesan berisi semua problem. Setelah router dibuat: `runtime.Mount(router, reg, runtime.NewSnapshot(), authorization.Stub{Mode: cfg.AuthzStub, Log: log})`, dan `Deps.Authenticate` = pass-through dengan log `DEBUG "authN stub"`.
- CLI (tanpa DB):
  - `erpora inspect routes [--json]`: kolom `MODULE  METHOD  PATTERN  PERMISSION  PUBLIC`, terurut seperti `Routes()`.
  - `erpora inspect modules [--json]`: `KEY  SLUG  VERSION  DEPENDENCIES  COMPATIBLE` (urutan topologis; `COMPATIBLE` = `no (<kode>)` bila ada di `Incompatible()`).
  - `erpora inspect permissions [--json]`: `MODULE  PERMISSION`.
  - Alias `erpora route:list` = `inspect routes`, `erpora permission:list` = `inspect permissions`; keduanya tersedia di flavor release (FR-22.2), grup `inspect`.
- `tests/security/public_routes.txt`: satu pola `METHOD /path` per baris, `#` untuk komentar; awalnya hanya header komentar (modul referensi tidak punya route publik).

- [ ] **Step 1: Tulis test yang gagal**:
  - `internal/app/registry_test.go`: `TestBuildRegistry_ReferenceModulesValid` → tanpa error; `Entries()` berurutan `platform.core, acme.example, acme.example-addon`; `Routes()` memuat `GET /api/example/ping`, `GET /api/example/notes`, `GET /api/example-addon/summary`; `Commands()` memuat `example:recount`.
  - `tests/security/public_routes_test.go`: `TestEveryRouteHasPermissionOrIsPublic` dan `TestPublicRoutesMatchAllowlist` (selisih ke dua arah dilaporkan dengan daftar pola).
  - `cmd/erpora/cli_test.go`: golden test `inspect routes` dan `route:list` menghasilkan output identik; `inspect modules --json` dapat di-decode dan berisi 3 entri.
- [ ] **Step 2: Jalankan** `go test ./internal/app/... ./tests/security/... ./cmd/erpora/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces. Golden file diperbarui lewat flag `-update` pada test.
- [ ] **Step 4: Verifikasi end-to-end lokal**: `task test`, `task test:integration` (Docker), lalu `task erpora -- inspect routes` menampilkan tiga route; `task erpora -- serve` (setelah `migrate up`) dan `curl -s localhost:8080/api/example/ping` → `404` dengan `"code":"module_not_found"`.
- [ ] **Step 5: Commit** `feat(app): wire reference modules, validate catalog at startup, inspect routes/modules/permissions`

### Task 10: ADR-006 final & dokumentasi SDK

**Files:**
- Modify: `docs/adr/0006-module-manifest-sdk-semver.md` (Status `Accepted`), `docs/module-sdk/README.md`
- Create: `docs/module-sdk/creating-a-module.md`

**Interfaces:** tidak ada (dokumentasi).

ADR-006 wajib mencatat:
- manifest sebagai satu-satunya sumber deklarasi capability; schema 2020-12 dengan `additionalProperties: false` dan perluasan hanya aditif (`schema_version` tetap `1`);
- `Registrar` `Handle`/`Public`/`Command` (modul tidak menerima router; menyimpang dari contoh arsitektur awal);
- SemVer (Masterminds), `PlatformVersion` sebagai konstanta (keputusan rencana 1), constraint `^0.1` selama pre-1.0 (keputusan 2);
- multi Go module + `go.work`, aturan tag `modules/<name>/vX.Y.Z` == `version` manifest, MAJOR ≥ 2 → path `/v2`;
- dependency berbasis `provides` di luar cakupan (FR-18), dicatat sebagai pertimbangan;
- path modul bare `erpora/...` harus pindah ke domain vanity sebelum SDK dipublikasikan (Q24, ADR-001);
- slug tercadang (keputusan 4) dan alasan `trust` hanya `builtin` di fase ini (keputusan 5);
- pemicu evaluasi ulang: modul pihak ketiga (fase 06), dependency berbasis `provides`.

`creating-a-module.md` memakai `example`/`exampleaddon` sebagai contoh: struktur folder (§4.1), `module.json`, `Register`, hook lifecycle (idempoten), kontrak antarmodul (`contract/` + wiring di `internal/app/modules.go`), aturan D1–D11 yang relevan bagi penulis modul, dan langkah menyalin `example` → modul baru.

- [ ] **Step 1:** Tulis kedua dokumen.
- [ ] **Step 2:** Pastikan setiap nama tipe/fungsi yang disebut dokumen ada di kode (`git grep -n` per identifier).
- [ ] **Step 3: Commit** `docs: accept ADR-006 and add module SDK guide`

---

## Verifikasi akhir M2a

- [ ] `task ci` exit 0 (termasuk `test:integration`).
- [ ] `task erpora -- inspect modules` menampilkan `platform.core`, `acme.example`, `acme.example-addon` dalam urutan itu.
- [ ] Mengubah sementara `exampleaddon/module.json` dependency menjadi `acme.example ^2.0` → `erpora serve` gagal dengan exit code 2 dan pesan `version_mismatch` yang menyebut `acme.example-addon` dan `acme.example` (dikembalikan setelah dicek).
- [ ] Ledger `.superpowers/sdd/<tanggal>-m2a/progress.md` diperbarui; tidak ada push tanpa permintaan user.

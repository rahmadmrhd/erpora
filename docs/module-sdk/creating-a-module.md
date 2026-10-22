# Membuat modul baru

Panduan ini menjelaskan cara membuat modul ERPora memakai dua modul referensi sebagai contoh:

- `modules/example` (`acme.example`): modul dengan route, migrasi, hook, command, dan kontrak untuk modul lain.
- `modules/exampleaddon` (`acme.example-addon`): add-on yang bergantung pada kontrak `acme.example` dan tidak punya tabel sendiri.

Kontrak dan aturan versi dijelaskan di ADR-006. Aturan dependensi D1–D11 dijelaskan di ADR-002.

## 1. Struktur folder

Struktur `modules/example`:

```
modules/example/
├── go.mod                      # Go module sendiri: erpora/modules/example
├── module.json                 # manifest (satu-satunya sumber capability)
├── module.go                   # New, Manifest, Register, hook lifecycle, Notes()
├── module_test.go              # assertion antarmuka dan tes manifest
├── contract/notes.go           # port publik untuk modul lain (tanpa internal/)
├── migrations/0001_create_example_notes.sql
└── internal/
    ├── domain/                 # entity dan aturan bisnis, tanpa I/O (D3)
    ├── application/            # use case (Service)
    ├── infrastructure/         # repository; satu-satunya tempat GORM (D3b)
    └── presentation/http/      # handler HTTP
```

`exampleaddon` memakai bagian yang sama, tetapi tanpa `contract/`, `migrations/`, dan `infrastructure/`, karena tidak memiliki data.

## 2. `module.json`

Manifest divalidasi terhadap `contracts/module.schema.json` saat startup. Objek di setiap tingkat memakai `additionalProperties: false`, sehingga field yang salah ketik akan ditolak. Contoh `modules/example/module.json`:

```json
{
  "schema_version": "1",
  "key": "acme.example",
  "slug": "example",
  "name": "Example",
  "version": "1.0.0",
  "publisher": { "id": "acme", "name": "ACME" },
  "trust": "builtin",
  "compatibility": { "platform": "^0.1", "sdk": "^0.1" },
  "dependencies": [
    { "module": "platform.core", "version": "^0.1", "optional": false }
  ],
  "capabilities": ["example.ping.read", "example.notes.read"],
  "migrations": { "strategy": "versioned" },
  "offline": { "mode": "online-only" }
}
```

Aturan yang diperiksa registry:

| Field | Aturan |
| --- | --- |
| `schema_version` | Harus `"1"`. |
| `key` | Pola `nama.modul`, huruf kecil dengan tanda hubung. Harus unik. |
| `slug` | Pola kebab-case. Harus unik dan tidak boleh tercadang (lihat ADR-006 keputusan 7). Dipakai sebagai `/api/{slug}` dan prefix nama command. |
| `version` | SemVer. Harus sama dengan tag `modules/<name>/vX.Y.Z` (diperiksa CI rilis, M2d). |
| `trust` | Hanya `builtin` yang diterima di fase ini. |
| `compatibility.platform`, `compatibility.sdk`, `dependencies[].version` | Constraint SemVer. Selama 0.x, gunakan `^0.1`. |
| `dependencies` | Setiap modul yang diimpor harus tercantum di sini (D8). `platform.core` selalu tersedia. |
| `capabilities` | Harus berawalan `<slug>.` dan unik antarmodul. Setiap permission di `Handle` harus ada di sini. |
| `migrations.strategy` | `versioned` atau `none`. |
| `offline.mode` | `online-only`, `offline-read`, `offline-draft`, atau `offline-write`. |

Field yang sudah dicadangkan untuk marketplace (`artifact`, `checksum`, `signature`, `requested_permissions`, `license`) dan PRD-001 (`resources`, `scope_dimensions`, `default_roles`, `settings`) boleh dibiarkan kosong.

## 3. Memuat manifest dan menyatakan modul

Manifest di-embed dengan `//go:embed` dan diurai saat paket diinisialisasi:

```go
//go:embed module.json
var manifestJSON []byte

func New() *Module {
    return &Module{manifest: module.MustParseManifest(manifestJSON), /* ... */}
}

func (m *Module) Manifest() module.Manifest { return m.manifest }
```

`MustParseManifest` menyimpan byte asli di `Manifest.Raw`. Registry memakai byte asli ini untuk validasi schema.

## 4. `Register`: route dan command

Modul tidak menerima router. Modul memanggil `Registrar` dan platform yang memasang route:

```go
func (m *Module) Register(r module.Registrar) error {
    r.Handle(http.MethodGet, "/ping", "example.ping.read", http.HandlerFunc(m.handlers.Ping))
    r.Handle(http.MethodGet, "/notes", "example.notes.read", http.HandlerFunc(m.handlers.ListNotes))
    r.Command(module.Command{
        Name:  "recount",
        Short: "Hitung jumlah notes",
        Run: func(ctx context.Context, inv module.Invocation) error {
            n, err := m.svc.Count(ctx)
            if err != nil {
                return err
            }
            _, err = fmt.Fprintf(inv.Out, "notes: %d\n", n)
            return err
        },
    })
    return nil
}
```

- `Handle(method, path, permission, h)` memasang route terproteksi. `permission` wajib ada di `capabilities`.
- `Public(method, path, h)` memasang route tanpa permission. Gunakan hanya bila memang publik, karena setiap route publik harus ditinjau.
- `Command(c)` mendaftarkan perintah CLI. `Name` tanpa prefix slug. Registry menyimpannya sebagai `<slug>:<name>`, misalnya `example:recount`.
- `path` relatif terhadap prefix modul. `/ping` dipasang sebagai `/api/example/ping`.

Pelanggaran dikumpulkan dan dilaporkan sekaligus saat startup dengan kode problem, misalnya `permission_undeclared`, `invalid_route`, `route_conflict`, `invalid_command`, dan `command_conflict`. Tabel lengkap ada di `internal/platform/modules/registry/errors.go`.

Urutan middleware pada setiap route adalah `recover`, `otel`, log, authN (kecuali `Public`), tenant context, guard status modul, permission, lalu handler. Modul tidak perlu menulis middleware sendiri.

Catatan M2a: command tercatat di `Registry.Commands()`, tetapi belum dihubungkan ke CLI cobra. Menjalankan `erpora example:recount` belum tersedia.

## 5. Hook lifecycle

Hook bersifat opsional. Modul cukup mengimplementasikan interface yang dibutuhkan, dan platform mendeteksinya dengan type assertion. Interface ada di `sdk/module/module.go`:

| Interface | Method | Kapan dipanggil (rencana) |
| --- | --- | --- |
| `MigrationSource` | `Migrations() fs.FS` | Migrasi modul dijalankan. |
| `Installer` | `Install(ctx, env)` | Saat modul pertama kali dipasang. |
| `Upgrader` | `Upgrade(ctx, env, from, to)` | Saat versi modul naik. |
| `EnableHook` | `OnEnable(ctx, env)` | Setelah modul diaktifkan. |
| `DisableHook` | `OnDisable(ctx, env)` | Sebelum modul dinonaktifkan. |
| `HealthChecker` | `Health(ctx, env)` | Pemeriksaan kesehatan. Error berarti tidak sehat. |
| `DevSeeder` | `SeedDev(ctx, env)` | Data contoh untuk development saja. |

Semua hook wajib **idempoten**, karena platform dapat memanggilnya ulang setelah kegagalan sebagian. Contoh di `example`:

```go
// Install menyemai 3 notes hanya bila tabel kosong (idempoten).
func (m *Module) Install(ctx context.Context, _ module.Env) error { return m.svc.SeedInitial(ctx) }
```

Di `exampleaddon`, `OnEnable` dan `OnDisable` hanya mencatat log, sehingga aman dipanggil berulang.

Data diakses lewat `data.DB(ctx)` dari `sdk/data`, bukan dengan memegang pool database. `module.Env` hanya berisi `TenantID` dan `Logger`.

Catatan M2a: definisi hook sudah ada, tetapi runtime belum memanggilnya. Pemanggilan hook dan status modul per tenant termasuk cakupan M2b.

## 6. Kontrak antarmodul

Modul lain hanya boleh mengimpor paket `contract/` modul ini (D1). Di `example`, kontraknya adalah port baca:

```go
// modules/example/contract/notes.go
type Notes interface {
    Count(ctx context.Context) (int64, error)
}
```

Implementasinya berada di `module.go`. Ia dikembalikan oleh `Notes()` dan hanya membungkus `application.Service`:

```go
func (m *Module) Notes() contract.Notes { return notesContract{svc: m.svc} }
```

Aturan kontrak:

- Parameter kontrak tidak boleh membawa `tenantId` atau `userId`. Tenant diambil dari `context`.
- Kontrak harus memeriksa otorisasi sendiri terhadap principal di `context`. Pemeriksaan permission di dalam kontrak belum ada di M2a dan menunggu `sdk/authz` (PRD-001 FR-119).

Pemanggil mendapatkan kontrak lewat wiring di `internal/app/modules.go`, bukan lewat import langsung:

```go
func compiledModules() []module.Module {
    ex := example.New()
    return []module.Module{
        ex,
        exampleaddon.New(ex.Notes()),
        // erp:generated:modules
    }
}
```

`exampleaddon` menerima `contract.Notes` lewat konstruktor `New(notes)`, sehingga ia tidak mengimpor `internal/` milik `example`.

Dependensi kontrak wajib dinyatakan di tiga tempat, dan ketiganya harus konsisten:

1. `dependencies` di `module.json` (`{"module": "acme.example", "version": "^1.0", ...}`).
2. `require` dan `replace` di `go.mod` modul (`erpora/modules/example => ../example`).
3. Wiring di `internal/app/modules.go`.

## 7. Aturan dependensi yang relevan bagi penulis modul

| Aturan | Isi |
| --- | --- |
| D1 | Modul lain hanya boleh mengimpor `modules/<x>/contract`. `internal/**` tertutup oleh compiler. |
| D3 | `internal/domain` tidak boleh mengimpor `net/http`, GORM, `pgx`, `database/sql`, `infrastructure`, atau `presentation`. |
| D3b | GORM hanya di `internal/infrastructure`, `internal/platform/database`, `internal/platform/modules/store`, dan `sdk/data`. |
| D4 | `internal/application` tidak boleh mengimpor `presentation` atau `infrastructure`. |
| D8 | Modul hanya boleh mengimpor modul yang tercantum di `dependencies`. `require` di `go.mod` harus sama dengan `dependencies`. |
| D9 | `modules/**` tidak boleh mengimpor root module. Akses platform hanya lewat `sdk/`. |
| D10 | Modul memakai `net/http` dari stdlib, tanpa library router pihak ketiga. |
| D11 | Dependensi antar-Go-module tidak boleh siklik. |

Penegak aturan ini adalah `internal/archcheck` dan `task arch`. Build `GOWORK=off` per modul dijalankan oleh `task check:gowork-off`.

Selain itu, setiap route yang menerima ID wajib memiliki test IDOR (user A terhadap user B), dan record di luar scope harus mengembalikan 404.

## 8. Menyalin `example` menjadi modul baru

Contoh berikut membuat modul `inventory` dengan key `acme.inventory`.

1. **Salin folder** `modules/example` ke `modules/inventory`. Hapus `migrations/0001_create_example_notes.sql` beserta isinya, lalu tulis migrasi milik modul baru dengan format goose (`-- +goose Up` / `-- +goose Down`).

2. **Ubah module path** di `modules/inventory/go.mod`:
   ```
   module erpora/modules/inventory
   ```
   Biarkan `replace erpora/sdk => ../../sdk` tetap ada.

3. **Ubah import** dengan mengganti `erpora/modules/example` menjadi `erpora/modules/inventory` pada seluruh file `.go`. Cari dengan `git grep -n "erpora/modules/example"`.

4. **Ubah `module.json`**: `key`, `slug`, `name`, `publisher`, `version` (mulai `1.0.0`), `capabilities` (berawalan `inventory.`), `dependencies` (hanya modul yang benar-benar diimpor), dan `migrations`. Ganti semua `Handle` agar memakai permission yang ada di `capabilities`.

5. **Daftarkan modul** di tiga tempat:
   - `go.work`: tambahkan `./modules/inventory` pada blok `use`.
   - `go.mod` root: tambahkan `erpora/modules/inventory v0.0.0` pada blok `require`, dan `erpora/modules/inventory => ./modules/inventory` pada blok `replace`.
   - `internal/app/modules.go`: tambahkan `inventory.New(),` di atas penanda `// erp:generated:modules`. Jika ada dependensi kontrak, teruskan kontraknya ke konstruktor.

6. **Verifikasi**:
   ```
   task lint
   task arch
   task check:gowork-off
   task test
   task erpora -- inspect modules
   task erpora -- inspect routes
   ```
   `inspect modules` harus menampilkan modul baru beserta urutan topologisnya.

Setelah langkah ini, modul baru harus lolos semua gate di atas. Jika salah satu gagal, pesan `registry.Error` atau arch test menyebut aturan dan modul yang bermasalah.

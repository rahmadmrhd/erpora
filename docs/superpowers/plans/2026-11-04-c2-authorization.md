# PRD-001 C2 Authorization: Rencana Implementasi

> **Untuk agentic worker:** WAJIB memakai `superpowers:subagent-driven-development` (disarankan) atau `superpowers:executing-plans`. Langkah memakai kotak centang (`- [ ]`). Alur sprint dan pemilihan model subagent mengikuti `~/.claude/CLAUDE.md`: implementasi tanpa review per task (Sonnet untuk logika inti, Haiku untuk pekerjaan mekanis), satu review akhir per wave oleh agent Opus independen (kesesuaian spek + `security-audit` terfokus pada perubahan wave dan permukaan yang disentuhnya), lalu putaran perbaikan oleh implementer yang sama lewat `SendMessage`. Maksimal 3 subagent bersamaan; subagent tidak men-spawn subagent; koordinator yang commit; tanpa push atau PR kecuali diminta.

**Goal:** mengganti stub otorisasi PRD-000 (`ERP_AUTHZ_STUB`, `NewStubFieldMasker`) dengan mesin otorisasi tiga lapis (aksi, record/scope, field) PRD-001 FR-110 sampai FR-119, lengkap dengan `sdk/authz` penuh, pencegahan IDOR yang ditegakkan otomatis, dan UI administrasi hak akses di modul `core`; Wave 1 sengaja menjadi gerbang pembuka T4 spec `resource-metadata` (grup 19 sampai 25).

**Architecture:** deklarasi resource/aksi/field group/dimensi/role bawaan di manifest (divalidasi schema, registry, dan `module:check`); **mesin evaluasi** di `internal/platform/authorization` (kompilasi hak efektif per `(tenant, user, permission_revision, revisi snapshot modul)`, cache LRU di memori, gagal tertutup) dengan GORM terkurung di `internal/platform/authorization/store`; tabel otorisasi milik platform di database tenant; modul memakai `sdk/authz` (dependency inversion D5/D9) dan modul `core` mengelola hak akses lewat `authz.Admin` di context (seperti `identity.Admin`). Penegakan berlapis: middleware route (lapis aksi, `TypedAuthorizer` yang sudah ada), helper `Filter`/`Check`/`MaskFields`/`CheckWrite` di handler dan implementasi kontrak, analyzer `go vet` + aturan arsitektur, generator, dan security test penanda IDOR.

**Tech Stack:** Go 1.27 stdlib, GORM (hanya `authorization/store`, dan `sdk/authz` untuk tipe scope bila K-C2.7 disetujui), goose (migrasi tenant), `golang.org/x/tools/go/analysis` (modul `golang.org/x/tools` sudah dependensi langsung root lewat `go/packages`; tanpa modul dependensi baru), React/TS (`modules/core/web`, `packages/platform-web`), komponen `@erpora/ui` yang sudah terpasang saja.

**Spec:** `docs/prd/PRD-001-core.md` §3, §4, §6 (FR-110 sampai FR-119), §9 (NFR-101, NFR-102, NFR-106), §10 (exit gate C2), §11 (Q1, Q2, Q6, Q11, Q13, Q14, Q15 K-C1.3/K-C1.7/K-C1.9, Q18, Q29); `docs/adr/0013-autentikasi-otorisasi-audit.md` §4, 7a butir 5 dan 6; `docs/architecture/api-guidelines.md` §5, §7, §8, §12 sampai §15; `docs/security/security-guidelines.md` §2, §3.1, §6, §7; `docs/specs/resource-metadata/{requirements,design,tasks}.md` (D-12, D-22, K35, Req 15 dan 16.7, grup 19 sampai 26); rencana `2026-11-02-c-overview.md` dan `2026-11-02-c1-identity-lokal.md`.

## Prasyarat masuk

- **C1 ditutup lebih dulu:** C1.15 Step 3 (review akhir independen + review atas diff perbaikan), Step 4 (verifikasi browser), dan butir M3(b) yang menunggu pemilik produk. Task C2.0 (dokumen) boleh berjalan paralel; kode C2 dimulai setelah C1 di-merge.
- **Butir konfirmasi K-C2.7 (allowlist `sdk/authz`) dijawab sebelum C2.0/C2.3; K-C2.1 (permission warisan) dan K-C2.5 (label) sebelum C2.1; K-C2.8 (header revisi) sebelum C2.5;** K-C2.2 sampai K-C2.4 dan K-C2.6 sebelum Wave 3. Daftar ada di akhir dokumen.
- **Catatan terbuka sebelum C2.3/C2.4 (temuan C2.0, ADR-0013 §10.10):** `authz.Engine.Filter` bertipe `func(*gorm.DB) *gorm.DB` membuat paket mesin `internal/platform/authorization` ikut mengimpor GORM, padahal D3b hanya mengizinkan `authorization/store` dan `sdk/authz`. Rekomendasi (dalam spek, cukup keputusan koordinator): penerjemahan ke GORM seluruhnya di `sdk/authz` (`Engine.Filter` mengembalikan deskripsi netral `authz.RowFilter`). Alternatif yang butuh persetujuan pemilik produk: tambahkan `internal/platform/authorization` ke allowlist D3b.

## Global Constraints

- Hak efektif = **union** semua baris semua role pengguna; **tidak ada deny**. Union dihitung per baris: sebuah aksi pada record diizinkan bila ada **satu baris yang memuat aksi itu dan scope-nya cocok**; scope dari baris lain (mis. baris `read` ber-scope `all`) tidak pernah meminjamkan cakupannya ke aksi lain.
- Scope baris berupa daftar kondisi yang digabung **AND**: `all`, `owner`, `assigned:<dimensi>`, `custom:<key>`; antarbaris dan antarrole **OR**. `all` dinyatakan eksplisit (`["all"]`); daftar kosong tidak sah (CHECK + validasi).
- `owner` = `owner_field = principal.user_id` (default `created_by`); nilai `NULL` tidak pernah cocok. `assigned:<dimensi>` dengan pengguna tanpa nilai penugasan = **FALSE** (bukan klausa yang dilewati). Untuk `create`, scope diperiksa terhadap data yang akan dibuat.
- Field group per role dan resource: `none`/`read`/`write`. Grup `default` mengikuti aksi (`read` → minimal read, `update` → write). Field yang tidak tercantum di grup mana pun masuk grup `default`. Field `none` **dihapus** dari respons (bukan dikosongkan); tulis ke field tanpa `write` → **403 `field_forbidden`** dengan daftar field; filter/sort pada field `none` → **403 `field_forbidden`**.
- Record di luar scope → **404 `not_found`**; ID referensi di body di luar scope/tak terbaca → **422 `validation_failed`** dengan `details[].code = not_found` pada field itu; tanpa permission aksi → **403 `permission_denied`**.
- Kontrak antarmodul: implementasi penyedia menegakkan `authz.Check`/`authz.Filter` terhadap principal di context; parameter kontrak tidak boleh `tenantId`/`userId`/penanda hak; hak sistem hanya `authz.AsSystem(ctx, reason)` dengan `reason` konstanta tidak kosong, **hanya di paket `internal/application` modul**.
- **Gagal tertutup** (NFR-106): galat evaluasi, cache rusak, scope provider panic atau galat, nilai record tak terbaca, principal tidak ada → ditolak (`Filter` menambah `WHERE false`).
- Role sistem `administrator` (semua izin termasuk modul baru, matriks tidak dapat diubah) dan `authenticated` (hak dasar semua pengguna login, matriksnya dapat diubah, berlaku implisit tanpa baris `user_roles`). Administrator aktif terakhir tidak dapat dicabut role-nya atau dinonaktifkan.
- `permission_revision` (tabel tenant `authz_state`) naik **dalam transaksi yang sama** dengan setiap perubahan role, matriks, field access, `user_roles`, penugasan scope, unit organisasi, dan sinkronisasi `default_roles`. Keputusan server **tidak pernah** memakai klaim `perm_rev` JWT; klaim hanya informasi bagi klien.
- Performa (NFR-102/FR-116): `Require` ≤ **20 µs p99** pada cache hit (benchmark); `Filter` menghasilkan klausa `WHERE` berparameter yang dapat memakai indeks.
- Arsitektur: modul tidak mengimpor platform (D9); `internal/platform/**` tidak mengimpor `modules/**` (D2); GORM hanya di daftar D3b (ditambah `internal/platform/authorization/store`, dan `sdk/authz` bila K-C2.7 disetujui; task C2.0); tanpa `AutoMigrate`; migrasi tenant tanpa `tenant_id` (Q18); tidak ada breaking change API (bootstrap, kontrak modul) maupun breaking change manifest (`schema_version` tetap `"1"`).
- Teks UI lewat i18n; hanya komponen `@erpora/ui` yang sudah terpasang (komponen shadcn baru butuh konfirmasi pemilik produk); `dangerouslySetInnerHTML` dilarang; anggaran bundle 210 KB gzip (halaman admin dimuat lazy).
- TDD; satu commit per unit kerja; pesan konvensional; tanpa push atau PR.

## Review Focus

Lima kelas masukan/kegagalan yang paling mungkin menggigit pengguna dan tidak dicakup tes fungsional biasa; tesnya ditambahkan ke task pemilik dan dijalankan ulang di C2.15:

1. **Scope gagal terbuka:** pengguna tanpa nilai penugasan dimensi, scope tersimpan rusak atau kosong, `ScopeProvider` yang panic/galat, `owner_field` bernilai `NULL` (baris `example` lama dengan `createdBy` null, K19/K22), record yang tidak memuat field scope, principal hilang karena pemanggil kontrak memakai `context.Background()`: semuanya **ditolak**, tidak pernah "lihat semua". Tes: `TestScopeEmptyAssignmentIsFalse`, `TestScopeInvalidStoredJSONDenies`, `TestCustomScopePanicDenies`, `TestOwnerNullNeverMatches`, `TestCheckRecordMissingScopeFieldDenies`, `TestNoPrincipalDenies` (C2.4); `TestContractWithBackgroundCtxNotFound` (C2.6).
2. **Kebocoran field lewat nama dan jalur samping:** DTO memakai camelCase (`costPrice`) sedangkan manifest snake_case (`cost_price`); field yang tak tercocokkan jatuh ke grup `default` dan bocor. Juga filter, sort, `fields=`, `expand`, ekspor, dan isi `details` galat `412`/`422`. Tes: `TestMaskFieldsCamelCaseMatchesSnakeGroup`, `TestCheckWriteCamelCase`, `TestReadableRejectsNoneForFilterAndSort` (C2.3/C2.4); `TestExampleListFilterOnNoneField403`, `TestExampleExportOmitsNoneGroup` (C2.6).
3. **Cache hak basi atau lintas pengguna:** perubahan role harus terlihat tanpa login ulang; cache tidak boleh bocor antarpengguna atau antar-tenant; modul yang di-disable menghentikan barisnya; sinkronisasi `default_roles` saat install/upgrade dan pemberian role `administrator` oleh `core:create-admin` ikut menaikkan revisi. Tes: `TestCacheKeyIncludesUserAndTenant`, `TestRevisionBumpInvalidatesWithoutRelogin`, `TestJWTPermRevClaimIgnored`, `TestDisabledModuleRowsIgnored`, `TestRevisionVisibleToOtherInstanceViaNotifierAndPoll` (C2.4); `TestCreateAdminGrantBumpsRevision` (C2.2); `TestDefaultRolesSyncBumpsRevision` (C2.5).
4. **IDOR lewat kontrak (confused deputy) dan hak sistem:** `exampleaddon` memanggil `example/contract` dengan ID record milik pengguna lain; `AsSystem` di handler atau dengan alasan dari input; parameter kontrak `userId`. Tes: `TestAddonContractGetOtherUsersNoteNotFound` (C2.6); fixture analyzer `asys_outside_application`, `asys_nonconst_reason`, `contract_param_userid`, `repo_first_without_filter` (C2.7).
5. **Union salah dan role Administrator terakhir:** role A `read`+`all` dan role B `update`+`owner` tidak boleh menghasilkan `update` atas semua record; dua admin yang saling mencabut role serentak tidak boleh menyisakan nol administrator. Tes: `TestUnionDoesNotCrossCombineScopes` (C2.4); `TestLastAdministratorConcurrentRevokeOneFails` (C2.10).

## Struktur berkas

| Berkas/paket | Tanggung jawab |
| --- | --- |
| `contracts/module.schema.json`, `sdk/module/manifest.go` | Bentuk `resources`, `scope_dimensions`, `default_roles` (aditif) |
| `internal/platform/modules/{manifest,registry}`, `internal/devtools/modcheck` | Validasi semantik manifest dan permission route (satu fungsi validasi dipakai keduanya) |
| `sdk/authz/` | API modul: `Require`, `Filter`, `Check`, `Fields`, `MaskFields`, `CheckWrite`, `ScopeProvider`, `Record`, galat, `Admin`; `authztest` |
| `sdk/module/registrar.go`, `moduletest` | `Registrar.Scope` |
| `sdk/httpx` | `httpx.WriteAuthzError` (pemetaan galat authz ke respons) |
| `migrations/tenant/0005_authz.sql`, `0006_org_units.sql` | Skema otorisasi dan unit organisasi |
| `internal/platform/authorization/` | Mesin: katalog, kompilasi, evaluasi scope, field, cache, revisi, `Admin`, penjelas akses; stub dihapus |
| `internal/platform/authorization/store/` | GORM store tabel otorisasi |
| `internal/archcheck/authzcheck/` | Analyzer `go vet` (FR-115, FR-119 butir 3) |
| `tests/security/idor_markers_test.go` | Penanda tes IDOR per route dan metode kontrak |
| `internal/devtools/templates/{handler,repository,contract}` | Kode bangkitan memanggil helper authz + tes IDOR |
| `modules/example`, `modules/exampleaddon` | Resource, field group, scope kustom, endpoint ber-ID, kontrak ber-ID, tes IDOR |
| `modules/core/` | API dan UI administrasi hak akses |
| `packages/platform-web/src/permissions/` | Ringkasan permission, `usePermission`, `useFieldAccess`, muat ulang saat revisi berubah |

## Peta ketergantungan dan gelombang

| Wave | Task | Catatan |
| --- | --- | --- |
| 0 | C2.0 dokumen dan allowlist | paralel dengan penutupan C1 |
| 1a | C2.1 manifest, C2.2 migrasi dan store, C2.3 `sdk/authz` | independen; 3 agent |
| 1b | C2.4 mesin | bergantung 1a |
| 1c | C2.5 perakitan dan pensiun stub, C2.6 `example`/`exampleaddon` + IDOR | C2.6 memakai perakitan C2.5 untuk tes integrasi; berkas terpisah |
| **Checkpoint W1** | review independen; **T4 resource-metadata terbuka** | lihat bagian Checkpoint |
| 2 | C2.7 analyzer dan aturan arsitektur, C2.8 security test penanda IDOR, C2.9 generator | independen |
| 3 | C2.10 `authz.Admin` dan unit organisasi, lalu C2.11 API modul `core` (+ operasi pengguna K-C2.2), C2.12 gerbang API admin (K-C2.4) | C2.11 bergantung C2.10 |
| 4 | C2.13 frontend platform, C2.14 UI administrasi `core` | bergantung C2.11 |
| 5 | C2.15 gerbang, dokumen, review akhir, verifikasi browser | seluruh C2 |

Berkas yang dimiliki satu task tidak disentuh task lain dalam gelombang yang sama.

---

### Task C2.0: Dokumen keputusan dan allowlist D3b

**Kelas:** Haiku (mekanis: allowlist empat tempat) lalu Sonnet (draf keputusan); pemilik produk menjawab K-C2.x.

**Files:** Modify `internal/archcheck/rules.go` (`GormAllowedPaths` + `internal/platform/authorization/store` (K-C1.7) dan, menurut jawaban K-C2.7, `sdk/authz`), `internal/archcheck/rules_test.go`, `CLAUDE.md` dan `AGENTS.md` (baris D3b, identik), `docs/prd/PRD-000-inisialisasi-project.md` §4.2 dan tabel keputusan **Q44** (nomor PRD-000 berikutnya; Q43 terakhir), `docs/prd/PRD-001-core.md` §11 (**Q35** dan seterusnya), `docs/adr/0013-autentikasi-otorisasi-audit.md` (bagian baru §10 Otorisasi), `docs/superpowers/plans/2026-11-02-c-overview.md` (baris C2).

**Interfaces:** Produces: daftar D3b identik di empat tempat (ditegakkan `TestD3bAllowlistDocsMatchRules`); Q35 berisi "keputusan yang saya ambil" (bagian akhir dokumen ini) sebagai keputusan implementer dan jawaban K-C2.x pemilik produk dengan tanggal absolut.

- [x] **Step 1: Test.** Tambah kasus `internal/platform/authorization/store` dan `sdk/authz` pada tes allowlist; jalankan `go test ./internal/archcheck/... ./tests/architecture/...`, harapkan gagal.
- [x] **Step 2:** perbarui empat tempat sekaligus; tes hijau; `task lint arch` (4 Nov 2026: `go test ./internal/archcheck/... ./tests/architecture/...` hijau, `golangci-lint run ./internal/archcheck/...` 0 isu; `task lint` penuh dijalankan koordinator di gerbang)
- [x] **Step 3:** tulis ADR-0013 §10 (mesin, union per baris, cache dan revisi, mekanisme revisi klien menurut K-C2.8, log `authz.system_call` sementara sebelum audit C3, pensiun stub) dan Q35; catat jawaban K-C2.x saat diterima (ubah juga butir spek yang bertentangan, tutup pertanyaan terbuka).
- [ ] **Step 4: Commit** `docs(core): keputusan C2 dan allowlist D3b authorization/store, sdk/authz`.

### Task C2.1: Deklarasi manifest `resources`, dimensi, role bawaan (FR-110)

**Kelas:** Sonnet. **Bergantung:** jawaban K-C2.1 dan K-C2.5.

**Files:** Modify `contracts/module.schema.json` (perketat `resources`, isi `scope_dimensions`, `default_roles`), `sdk/module/manifest.go` (+ tes), `internal/platform/modules/manifest/validate.go`, `internal/platform/modules/registry/{registry.go,errors.go}`, `internal/devtools/modcheck/*`, `internal/app/registry.go` (resource platform dari `admin.Capabilities`, `i18nadmin.Capabilities`, `desks.Capabilities`), `modules/core/module.json` (resource `core.user`, `core.role`, `core.org_unit`; dimensi `core.org_unit`).

**Interfaces:**
- Produces (Go, `sdk/module`):

```go
type Resource struct {
    Key           string            `json:"key"`            // <slug>.<nama_snake>
    Label         string            `json:"label"`          // lihat K-C2.5
    Actions       []string          `json:"actions"`        // aksi standar FR-110
    CustomActions []CustomAction    `json:"custom_actions,omitempty"`
    OwnerField    string            `json:"owner_field,omitempty"` // default "created_by"
    ScopeFields   map[string]string `json:"scope_fields,omitempty"` // dimensi -> field
    FieldGroups   []FieldGroup      `json:"field_groups"`
    Audit         *ResourceAudit    `json:"audit,omitempty"`        // FR-131, hanya disimpan
}
type CustomAction struct{ Key, Label string }
type FieldGroup struct {
    Key, Label, Classification string
    Fields  []string `json:"fields,omitempty"` // opsional bila ada metadata (PRD-000 Q41)
    Default bool     `json:"default,omitempty"`
    Sensitive bool   `json:"sensitive,omitempty"` // FR-131
}
type ScopeDimension struct{ Key, Label, Source string }
type DefaultRole struct {
    Key, Label  string
    Permissions []RoleRow          `json:"permissions"`
    FieldAccess []RoleFieldAccess  `json:"field_access,omitempty"`
}
type RoleRow struct { Resource string; Actions []string; Scope []string }
type RoleFieldAccess struct { Resource, FieldGroup, Access string }
// Manifest: Resources []Resource; ScopeDimensions []ScopeDimension; DefaultRoles []DefaultRole
// Registry: (*Registry).Resources() []ResolvedResource; Permissions() []string (turunan resource×aksi)
```

- Aturan validasi (galat registry dan `module:check` memakai fungsi yang sama, kode galat baru `invalid_resource`): kunci resource berawalan `<slug>.`, unik lintas modul; aksi ⊂ aksi standar FR-110; aksi kustom `^[a-z_]+$`, tidak bentrok aksi standar, label wajib; tepat satu grup `default`; satu field paling banyak di satu grup; `scope_fields` hanya merujuk dimensi terdaftar (milik modul sendiri, `core.org_unit`, atau dependensi manifest); nama field `^[a-z][a-z0-9_]*$`; `default_roles[].key` berawalan `<slug>.`, baris merujuk resource modul sendiri dengan scope token sah; setiap `Registrar.Handle(..., permission)` (selain `module.Authenticated()`) wajib = `<resource>.<aksi>` terdeklarasi (menggantikan cek `capabilities`); `capabilities` diturunkan otomatis (perlakuan capability warisan mengikuti jawaban K-C2.1); menu Desk `permission` wajib ada di himpunan turunan.

- [ ] **Step 1: Test** AC FR-110: `TestManifestRejectsCustomActionClash`, `TestManifestRejectsUnknownScopeDimension`, `TestManifestRejectsResourceWithoutDefaultGroup`, `TestManifestRejectsTwoDefaultGroups`, `TestManifestRejectsFieldInTwoGroups`, `TestRouteMustReferenceDeclaredResourceAction`, `TestCapabilitiesDerivedFromResources`, `TestScopeFieldsMayReferenceDependencyDimension` (dimensi modul non-dependensi ditolak), `TestDefaultRoleRowValidated`; `modcheck` memberi galat yang sama untuk berkas `module.json` yang sama (tes paritas); manifest tanpa `resources` lama tetap valid (aditif).
- [ ] **Step 2:** jalankan `go test ./sdk/module/... ./internal/platform/modules/... ./internal/devtools/modcheck/...`, harapkan gagal.
- [ ] **Step 3:** implementasi; pesan galat menyebut modul, path JSON, dan nilai.
- [ ] **Step 4:** hijau; `task erpora -- module:check`; `task contracts:check`. **Step 5: Commit** `feat(platform): deklarasi resource, dimensi scope, dan role bawaan di manifest`.

### Task C2.2: Migrasi tenant dan `authorization/store`

**Kelas:** Sonnet. **Bergantung:** C2.0 (allowlist).

**Files:** Create `migrations/tenant/0005_authz.sql`, `internal/platform/authorization/store/{doc.go,models.go,store.go,store_test.go,store_integration_test.go}`; Modify `internal/platform/identity/store/roles.go` (pemberian/pencabutan `user_roles` sudah berjalan di DB tenant lewat `tenantDB`; dibungkus satu transaksi tenant yang juga menaikkan `authz_state`, karena `authz_state` dan `user_roles` sama-sama tabel tenant).

**Interfaces:**
- Skema (tanpa `tenant_id`, Q18; CHECK pada enum):

| Tabel | Kolom penting |
| --- | --- |
| `role_permissions` | `id uuid PK`, `role_id → roles`, `resource text`, `actions text[]` (tidak kosong), `scope jsonb` (larik string tidak kosong; CHECK `jsonb_typeof = 'array' AND jsonb_array_length >= 1`); indeks `(role_id)` |
| `role_field_access` | PK `(role_id, resource, field_group)`, `access` ∈ `none/read/write` |
| `user_scope_assignments` | PK `(user_id, dimension)`, `values text[]`, `include_descendants boolean NOT NULL DEFAULT false` (mode FR-118; aditif atas §4) |
| `authz_state` | `id smallint PK CHECK (id = 1)`, `permission_revision bigint NOT NULL`; benih `(1, 1)` |
| `roles` | benih role sistem `authenticated` |

- `store.Store`: `Revision(ctx) (int64, error)`; `BumpRevision(tx) (int64, error)` (`UPDATE ... SET permission_revision = permission_revision + 1 RETURNING`); `LoadUserGrants(ctx, userID) (Grants, error)` (role, baris, field access, penugasan; satu kueri per tabel); metode tulis per entitas untuk C2.10 yang **selalu** memanggil `BumpRevision` pada transaksi yang sama (nama ditetapkan task ini di `doc.go`).

- [ ] **Step 1: Test integrasi** (`pgtest`, tag `integration`): migrasi naik/turun bersih; CHECK menolak `scope = '[]'`, `access = 'all'`; `BumpRevision` dua transaksi serentak menghasilkan dua nilai berbeda; setiap metode tulis menaikkan revisi tepat satu kali dan rollback tidak menaikkannya; `TestCreateAdminGrantBumpsRevision` (pemberian `administrator` oleh store identitas); tanpa `AutoMigrate`.
- [ ] **Step 2:** jalankan lewat tunnel VPS (`ERP_TEST_DATABASE_URL=postgres://erp:erp@127.0.0.1:15432/postgres?sslmode=disable GOFLAGS=-p=2 go test -tags integration ./internal/platform/authorization/store/...`), harapkan gagal.
- [ ] **Step 3:** implementasi (struct `row` + `TableName()` seperti `desks/store`). **Step 4:** hijau. **Step 5: Commit** `feat(authz): skema otorisasi tenant dan store`.

### Task C2.3: `sdk/authz` penuh, `Registrar.Scope`, pemetaan galat

**Kelas:** Sonnet.

**Files:** Create `sdk/authz/{authz.go,errors.go,fields_access.go,filter.go,record.go,scope.go,admin.go,names.go,*_test.go}`; Modify `sdk/authz/{doc.go,fields.go}`, `sdk/authz/authztest/authztest.go` (mesin uji dengan grant deklaratif), `sdk/module/registrar.go` (+ `Scope`), implementer `Registrar`: `internal/platform/modules/registry/recorder.go`, `sdk/module/moduletest/moduletest.go`, fake di `sdk/module/contract_test.go`, `internal/platform/desks/api_test.go`, `internal/platform/i18n/admin/api_test.go`; `sdk/httpx` (+ `WriteAuthzError`).

**Interfaces (Produces, dipakai C2.4 sampai C2.14 dan T4):**

```go
package authz

var (
    ErrNotFound        = errors.New("authz: not found")        // record di luar scope / tak ada -> 404 not_found
    ErrForbidden       = errors.New("authz: forbidden")        // tanpa permission aksi -> 403 permission_denied
    ErrUnauthenticated = errors.New("authz: unauthenticated")  // tanpa principal dan tanpa hak sistem -> 401
)
type FieldForbiddenError struct{ Resource string; Fields []string } // 403 field_forbidden, details per field

type Access uint8 // AccessNone, AccessRead, AccessWrite
type FieldAccess interface {
    Group(key string) Access
    Field(name string) Access                 // nama camelCase atau snake_case (dinormalisasi CanonicalField)
    Readable(fields ...string) error          // untuk filter/sort/group/fields=; *FieldForbiddenError
}
type Record interface{ AuthzValue(field string) (value any, ok bool) } // field = nama manifest snake_case

func Require(ctx context.Context, permission string) error
func Filter(ctx context.Context, resource, action string) func(*gorm.DB) *gorm.DB // galat: WHERE false
func Check(ctx context.Context, permission string, record any) error              // Record | map[string]any; lainnya ditolak
func Fields(ctx context.Context, resource string) FieldAccess
func MaskFields(ctx context.Context, resource string, dto any) any                // sudah ada; perilaku tetap
func CheckWrite(ctx context.Context, resource string, changedFields []string) error
func CanonicalField(name string) string // camelCase -> snake_case, identik dengan metadatatools/naming

type ScopeProvider struct {
    Key, Label string // Key berawalan <slug>.
    Filter func(ctx context.Context, db *gorm.DB) *gorm.DB
    Match  func(ctx context.Context, rec Record) (bool, error)
}
// Engine diimplementasi platform (C2.4); fungsi paket di atas hanya mendelegasikan ke Engine di context.
type Engine interface {
    Require(ctx context.Context, permission string) error
    Filter(ctx context.Context, resource, action string) func(*gorm.DB) *gorm.DB
    Check(ctx context.Context, permission string, rec Record) error
    Fields(ctx context.Context, resource string) FieldAccess
    MaskFields(ctx context.Context, resource string, fields map[string]any) map[string]any // = FieldMasker
    CheckWrite(ctx context.Context, resource string, changedFields []string) error
}
func WithEngine(ctx context.Context, e Engine) context.Context // HANYA platform (aturan IDENT, C2.7)
// Admin: deklarasi antarmuka di task ini, daftar metode persis = Interfaces C2.10 (didefinisikan dan diimplementasi C2.10).
func WithAdmin(ctx context.Context, a Admin) context.Context; func AdminFrom(ctx context.Context) (Admin, bool)
```

**Tanpa principal dan tanpa hak sistem:** `Require` → `ErrUnauthenticated`; `Check` → `ErrNotFound` (bukan 401, sehingga implementasi kontrak yang dipanggil dengan `context.Background()` menjawab "tidak ditemukan" sesuai FR-119; di handler HTTP kasus ini tidak terjadi karena middleware sudah menjawab 401); `Filter` → `WHERE false`; `Fields` → semua `none`; `CheckWrite` → `*FieldForbiddenError` untuk semua field.

`Registrar.Scope(p authz.ScopeProvider)` (registry menolak key tanpa awalan slug atau duplikat: `invalid_scope_provider`). `httpx.WriteAuthzError(w, r, err) bool` memetakan galat di atas (respons 404 tidak membedakan tidak ada vs di luar scope). Tanpa `Engine` di context semua fungsi gagal tertutup (`Filter` → `WHERE false`, `Fields` → semua `none`, `MaskFields` → `{}` seperti sekarang). `authz.AsSystem` tetap; `Engine` memperlakukannya sebagai izin penuh dan mencatat `authz.system_call` (C2.4).

- [ ] **Step 1: Test** `TestNoEngineFailsClosed` (keenam fungsi), `TestCanonicalFieldParityWithNaming` (korpus `metadatatools/naming`, termasuk `address2Line`), `TestMaskFieldsCamelCaseMatchesSnakeGroup` (dengan `authztest`), `TestCheckWriteCamelCase`, `TestReadableRejectsNoneForFilterAndSort`, `TestCheckRejectsUnsupportedRecordType`, `TestWriteAuthzErrorMapping` (404/403/403 `field_forbidden` + `details`), `TestRegistrarScopeValidated`; arsitektur: `sdk/authz` hanya mengimpor stdlib dan `gorm.io/gorm`.
- [ ] **Step 2:** gagal. **Step 3:** implementasi (fungsi paket mendelegasikan ke `Engine` dari context). **Step 4:** hijau; `task arch`. **Step 5: Commit** `feat(sdk): authz Require, Filter, Check, Fields, CheckWrite, dan Registrar.Scope`.

### Task C2.4: Mesin otorisasi, cache, dan revisi

**Kelas:** Sonnet. **Bergantung:** C2.1, C2.2, C2.3.

**Files:** Create `internal/platform/authorization/{catalog.go,compile.go,scope.go,fieldaccess.go,engine.go,cache.go,revision.go,*_test.go,*_integration_test.go,bench_test.go}` (nama `fieldaccess.go` karena `fields.go` stub masih ada; stub dihapus di C2.5, bukan di sini).

**Interfaces:**
- Consumes: `registry.Resources()`, `ScopeProvider` terdaftar, `store.Store`, `runtime.Snapshot` (status modul), `identity.FromContext`, `authz.SystemReason`.
- Produces: `authorization.New(Deps) (*Engine, error)`; `*Engine` memenuhi `authz.Engine`, `runtime.Authorizer` (`Allow`) untuk `TypedAuthorizer`, dan `authz.FieldMasker`; `(*Engine).Summary(ctx) (Summary, error)` untuk bootstrap (aksi per resource, penanda `conditional`, akses per grup); `(*Engine).Revision(ctx) int64`; `revision.Watch(ctx, notifier, interval)` (kanal `authz.revision_changed` + muat ulang berkala 1 menit, pola `session.RevocationList`).
- Kunci cache: `(tenantID, userID, permission_revision, revisi snapshot modul)`; LRU berbatas (default 10.000 entri); entri berisi grant terkompilasi (`map[permission][]compiledRow`, field access per resource).
- Evaluasi `Filter`: OR atas baris yang memuat aksi; per baris AND kondisi: `owner` → `clause.Eq{Column{Table: CurrentTable, Name: owner_field}, userID}`; `assigned` → `IN` nilai (FALSE bila kosong; `core.org_unit` dengan `include_descendants` diperluas lewat `path` dari C2.10, sampai saat itu tanpa turunan); `custom` → `ScopeProvider.Filter`; `all` → TRUE. Nama kolom dari manifest tervalidasi, tidak pernah dari input.
- Administrator: izin penuh untuk resource modul berstatus enabled; `authenticated` digabung untuk setiap principal manusia.

- [ ] **Step 1: Test** tabel uji AC FR-111 (unit + integrasi PostgreSQL): owner saja; assigned saja; owner AND assigned; dua role scope berbeda (union); scope kustom; `create` di luar scope ditolak; tanpa assignment tidak melihat apa pun; FR-112: grup `none` dihapus, `read` menolak tulis, grup default mengikuti aksi; Review Focus 1, 3, 5: `TestScopeEmptyAssignmentIsFalse`, `TestScopeInvalidStoredJSONDenies`, `TestCustomScopePanicDenies`, `TestOwnerNullNeverMatches`, `TestCheckRecordMissingScopeFieldDenies`, `TestNoPrincipalDenies`, `TestUnionDoesNotCrossCombineScopes`, `TestCacheKeyIncludesUserAndTenant`, `TestRevisionBumpInvalidatesWithoutRelogin`, `TestJWTPermRevClaimIgnored`, `TestDisabledModuleRowsIgnored`, `TestRevisionVisibleToOtherInstanceViaNotifierAndPoll`; `TestAdministratorAllowsNewModuleResources`; `TestAsSystemAllowsAndLogsSystemCall` (log memuat principal asli dan reason, tanpa nilai record); `TestFilterSQLParameterized` (nilai assignment `x' OR 1=1` tetap parameter); `BenchmarkRequireCacheHit` (p99 ≤ 20 µs, diukur lewat `testing.B` + persentil sampel).
- [ ] **Step 2:** gagal. **Step 3:** implementasi; panic `ScopeProvider` di-`recover` dan dicatat; setiap galat → tolak. **Step 4:** hijau (unit lokal, integrasi VPS). **Step 5: Commit** `feat(authz): mesin otorisasi tiga lapis dengan cache perm_rev`.

### Task C2.5: Perakitan, pensiun `ERP_AUTHZ_STUB`, `perm_rev`, bootstrap, sinkronisasi role bawaan

**Kelas:** Sonnet. **Bergantung:** C2.4.

**Files:** Modify `internal/app/assemble.go` (`TypedAuthorizer(engine, ...)`, `apis.Desks.Bind(... Authz: engine)`, `platformServices` memasang `engine` sebagai masker dan `authz.WithEngine`), `internal/platform/modules/runtime/{services.go,modctx.go}` (engine di route, hook, command, seed), `internal/platform/modules/lifecycle` (install/upgrade: sinkronisasi `default_roles` + `BumpRevision` dalam satu transaksi; role `customized` tidak disentuh), `internal/platform/identity/{service.go,token}` (klaim `perm_rev` diisi revisi saat terbit), `internal/app/bootstrap_user.go`, `internal/platform/bootstrap/handler.go`, `contracts/openapi/platform.yaml` (aditif), `internal/platform/httpserver` (header `X-Erp-Perm-Rev` pada respons request terautentikasi, bila K-C2.8 disetujui), `internal/platform/config/config.go` + tes, `cmd/erpora/{inspect.go,cli_test.go}`, `.env.example`, `docs/operations/deployment.md` (tabel env); Delete `internal/platform/authorization/{stub.go,stub_test.go,fields.go,fields_test.go}`.

**Interfaces:**
- Bootstrap (aditif; `permissions` tetap, kini terisi):

```jsonc
"permissions": ["example.note.read", ...],          // aksi dimiliki, termasuk yang bersyarat
"permRev": 7,                                        // revisi saat ini (bukan 0 lagi)
"access": { "example.note": { "actions": ["read","update"], "conditional": ["update"],
                              "fields": { "general": "write", "detail": "read" } } }
```

- `ERP_AUTHZ_STUB`: dihapus dari `config.Config`; bila masih diset, `serve` mencatat WARN sekali "diabaikan sejak C2" (tidak gagal; mesin selalu gagal tertutup). `ERP_MODULES_ADMIN_API` **tidak** disentuh di task ini (C2.12).

- [ ] **Step 1: Test** route modul dengan pengguna tanpa role 403 `permission_denied`, dengan role sesuai 200; aturan MFA dan `module.Authenticated()` tetap seperti C1 (`authenticated_test.go` hijau tanpa stub); menu Desk tersaring mesin; `TestDefaultRolesSyncBumpsRevision`, `TestCustomizedRoleNotOverwrittenOnUpgrade`, `TestDisableModuleKeepsRowsButInactive`; bootstrap memuat `access` dan `permRev` > 0; ETag bootstrap berubah saat revisi naik; header `X-Erp-Perm-Rev` hanya pada respons terautentikasi; `TestAuthzStubEnvIgnoredWithWarning`; `contracts:breaking` hijau (perubahan aditif).
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau; `task test:integration` paket tersentuh; `task contracts:check`. **Step 5: Commit** `feat(platform): mesin otorisasi menggantikan stub, perm_rev, dan ringkasan permission bootstrap`.

### Task C2.6: `example` dan `exampleaddon` memakai resource, scope kustom, field group, dan lulus IDOR

**Kelas:** Sonnet. **Bergantung:** C2.3 (API), tes integrasi memakai C2.5.

**Files:** Modify `modules/example/{module.json,openapi.yaml,module.go,contract/notes.go,internal/**,metadata/**,i18n/*}`, `modules/exampleaddon/{module.json,openapi.yaml,internal/**}`; Create tes integrasi IDOR `modules/example/internal/presentation/http/idor_integration_test.go`, `modules/exampleaddon/.../contract_idor_integration_test.go`.

**Interfaces:**
- `example.note`: aksi `read`, `create`, `update`; grup `general` (default) dan satu grup non-default (mis. `detail`); `owner_field: created_by`; scope kustom `example.recent` (`Registrar.Scope`: catatan dengan `created_at` dalam 30 hari terakhir; tanpa kolom baru).
- Titik masuk ID referensi di body (AC FR-119): `POST /api/example-addon/note-summaries` dengan body `{ "noteId": "<uuid>" }` (tanpa tabel; memanggil `example/contract` `Notes.Get` dengan ctx asli; rujukan tak terbaca → 422 `validation_failed`, `details[].field = noteId`, `code = not_found`).
- Endpoint aditif: `GET /api/example/notes/{id}`, `PATCH /api/example/notes/{id}` (`If-Match`, `CheckWrite`); daftar memakai `Filter`, `MaskFields`, dan `Fields(...).Readable` untuk `filter`/`sort`.
- Kontrak: `Notes.Get(ctx context.Context, id string) (NoteDTO, error)` (aditif) dengan `authz.Check`; `Count` kini ber-`Filter` (menghitung hanya record terlihat pemanggil). `exampleaddon` memanggil keduanya dengan ctx asli.
- Permission rute lama mengikuti jawaban K-C2.1.

- [ ] **Step 1: Test** (dua pengguna fixture A dan B lewat `core` store, scope berbeda): B membaca/mengubah record A → 404; B memakai ID A sebagai `noteId` pada `note-summaries` → 422 `not_found`; `TestExampleRecentCustomScopeEndToEnd` (role dengan scope `custom:example.recent` hanya melihat dan mengubah catatan 30 hari terakhir lewat HTTP dan kontrak); `exampleaddon` memanggil kontrak dengan ID A sebagai B → `ErrNotFound`; `TestContractWithBackgroundCtxNotFound`; daftar B tidak memuat record A termasuk lewat filter dan `expand`; `TestExampleListFilterOnNoneField403`; `TestExampleExportOmitsNoneGroup`; `PATCH` field grup `read` → 403 `field_forbidden` dengan daftar; penanda `// erp:idor-test <resource>.<operasi>` pada tiap tes (format C2.8).
- [ ] **Step 2:** gagal. **Step 3:** implementasi; `metadata:generate` dijalankan bila metadata berubah; `contracts:generate`. **Step 4:** hijau; `task contracts:check`, `task metadata:check`, `task erpora -- module:check`. **Step 5: Commit** `feat(example): resource, field group, scope kustom, dan tes IDOR`.

### Checkpoint Wave 1 (gerbang T4)

- [ ] Gerbang: `task lint arch check:gowork-off test`; integrasi VPS paket tersentuh; `task contracts:check`; `task erpora -- module:check`.
- [ ] Review akhir wave oleh agent **Opus independen** (diberi diff dan spek, tanpa kesimpulan implementer): kesesuaian FR-110 sampai FR-113, FR-115 sampai FR-117, FR-119 butir 1 dan 2, plus `security-audit` terfokus pada mesin, cache, `sdk/authz`, perakitan, dan permukaan pemanggilnya (otorisasi, IDOR, mass assignment, injeksi lewat nama kolom/nilai scope, race revisi). Putaran perbaikan oleh implementer yang sama; review atas diff perbaikan.
- [ ] **Kriteria buka T4** (catat hasilnya di `docs/specs/resource-metadata/tasks.md` 19.1 dan "Masih menunggu" butir 12): keenam fungsi FR-115 mengevaluasi baris `role_permissions`/`role_field_access`/`user_scope_assignments` nyata; owner, assigned, custom, dan union lulus; cache revisi berjalan; manifest `resources`/`field_groups`/`scope_dimensions`/`default_roles` tervalidasi di schema, registry, dan `module:check`; `example` dan `exampleaddon` lulus IDOR A vs B termasuk lewat kontrak. Yang terbuka setelah W1 hanya T4 19.1 dan grup 20 sampai 23; **T4 19.2 dan grup 24 menunggu C2.7 dan C2.8** (analyzer dan format penanda IDOR dibangun C2, T4 hanya memperluasnya; jangan membangun analyzer kedua).

### Task C2.7: Analyzer `go vet` dan aturan arsitektur authz (FR-115, FR-119 butir 2-3)

**Kelas:** Sonnet. **Bergantung:** Checkpoint W1.

**Files:** Create `internal/archcheck/authzcheck/{analyzer.go,analyzer_test.go}`, fixture `internal/archcheck/testdata/authz/**`; Modify `internal/archcheck/{typed.go,rules.go}`, `Taskfile.yml` (`task arch` menjalankan analyzer atas `modules/*`), `tests/architecture`.

**Interfaces:** Produces aturan (galat, bukan peringatan):
- `AUTHZQ`: di `modules/*/internal/infrastructure`, setiap pemanggilan terminal GORM (`First`, `Take`, `Last`, `Find`, `Pluck`, `Count`, `Scan`, `Row(s)`, `Update*`, `Delete`) wajib memiliki `Scopes(authz.Filter(...))` pada rantai yang sama, atau fungsi beranotasi `// erp:authz-exempt <alasan>` (alasan wajib; dicetak sebagai INFO).
- `ASYS`: `authz.AsSystem` di kode modul hanya di paket `internal/application` dan argumen `reason` wajib konstanta string tidak kosong; kontrak yang dipanggil dengan hak sistem memuat `// erp:system-call-allowed` pada metodenya.
- `CONTRACTID`: metode antarmuka di `modules/*/contract` tidak boleh punya parameter bernama `tenantId`, `tenantID`, `userId`, `userID` (atau tipe `tenancy.TenantID`).
- `IDENT` diperluas: kode non-test modul dilarang memanggil `authz.WithEngine`, `authz.WithFieldMasker`, `authz.WithAdmin`; `authz.AdminFrom` hanya di `modules/core` (pola keputusan ADR-0013 7a butir 5); impor `erpora/sdk/authz/authztest` dari kode non-test modul dilarang (sejajar larangan `identitytest`). Fixture tambahan: `module_imports_authztest`.

- [ ] **Step 1: Test** fixture melanggar gagal dengan aturan yang tepat: `repo_first_without_filter`, `repo_exempt_without_reason`, `asys_outside_application`, `asys_nonconst_reason`, `contract_param_userid`, `module_calls_withengine`, `noncore_authz_adminfrom`; fixture patuh lolos; seluruh repo (termasuk `example` hasil C2.6) lolos tanpa pengecualian baru.
- [ ] **Step 2:** gagal. **Step 3:** implementasi di atas `go/analysis` dengan pemuatan bertipe yang sudah ada (varian build seperti `IDENT`). **Step 4:** hijau; `task arch`. **Step 5: Commit** `feat(archcheck): analyzer authz, AsSystem, dan parameter kontrak`.

### Task C2.8: Security test penanda IDOR (FR-119 butir 3)

**Kelas:** Sonnet. **Bergantung:** Checkpoint W1.

**Files:** Create `tests/security/idor_markers_test.go`, `tests/security/idor_exempt.txt` (kosong; tiap baris butuh alasan); Modify `tests/security/README.md`.

**Interfaces:** format penanda mengikuti resource-metadata D-22 dan diperluas (superset): `// erp:idor-test <resource>.<operationId>` untuk route, `// erp:idor-test <resource>.contract.<Antarmuka>.<Metode>` untuk metode kontrak. Titik masuk ID = operasi kontrak OpenAPI modul dengan parameter path, atau properti body/query ber-`format: uuid` atau bernama `id`/`*Id`; metode antarmuka `contract/` dengan parameter `id`/`ids`/`*ID`. Penanda dicari di berkas `_test.go` bertag `integration`.

- [ ] **Step 1: Test** fixture registry dengan route ber-ID tanpa penanda gagal; dengan penanda lolos; metode kontrak ber-ID tanpa penanda gagal; penanda yang menunjuk operasi tak ada gagal (penanda basi).
- [ ] **Step 2:** gagal. **Step 3:** implementasi (memakai `apicontract.Parse` dan `go/parser`). **Step 4:** hijau untuk repo nyata (`example`, `exampleaddon`, `core`). **Step 5: Commit** `test(security): setiap titik masuk ID wajib punya tes IDOR`.

### Task C2.9: Generator memanggil helper authz dan membangkitkan tes IDOR; indeks scope (FR-115, FR-116, FR-119)

**Kelas:** Sonnet. **Bergantung:** C2.7, C2.8 (aturan dan format penanda).

**Files:** Modify `internal/devtools/templates/{handler,repository,contract}/*.tmpl`, `internal/devtools/gen/**` dan golden `internal/devtools/gen/testdata/**`, `internal/devtools/templates/migration/module.sql.tmpl` atau `internal/devtools/metadatatools/dbmodel` (indeks untuk `owner_field` dan setiap `scope_fields` resource yang ber-metadata), `docs/module-sdk/generators.md`.

**Interfaces:** kode bangkitan: repository memakai `Scopes(authz.Filter(ctx, res, aksi))`, use case memanggil `authz.Check` untuk record tunggal dan `CheckWrite` untuk update, handler memakai `MaskFields`, `Fields(...).Readable` untuk filter/sort, dan `httpx.WriteAuthzError`; implementasi kontrak bangkitan memanggil `Check`/`Filter`; tes IDOR bangkitan (integration, dua principal sungguhan) dengan penanda C2.8 menggantikan kerangka "principal uji lewat context".

- [ ] **Step 1: Test** golden diperbarui; `TestMakeHandler_AllKindsCompileAndPassTests` dan uji generator lain hijau; kode bangkitan lolos `AUTHZQ`; tes IDOR bangkitan **gagal** bila `Scopes(authz.Filter(...))` dibuang dari salinan sementara (bukan hijau karena stub); migrasi bangkitan memuat indeks untuk `owner_field` dan `scope_fields`.
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau; `task test:generator` di VPS (±12 menit, Docker + Bun). **Step 5: Commit** `feat(devtools): generator handler, repository, dan kontrak memakai authz dengan tes IDOR`.

### Checkpoint Wave 2

- [ ] Gerbang seperti W1 + `task test:generator`. Review Opus independen atas diff W2 (analyzer dapat dilewati? false negative pada rantai GORM lewat variabel, alias paket, method value; kualitas tes IDOR bangkitan) + `security-audit` terfokus. Putaran perbaikan.

### Task C2.10: `authz.Admin`, unit organisasi, hak efektif, penjelas akses, perbandingan role (FR-113, FR-114 backend, FR-118)

**Kelas:** Sonnet. **Bergantung:** Checkpoint W1; jawaban K-C2.3 untuk sumber nilai dimensi.

**Files:** Create `migrations/tenant/0006_org_units.sql`, `internal/platform/authorization/{admin.go,orgunits.go,explain.go,*_test.go,*_integration_test.go}`; Modify `authorization/store`, `sdk/authz/admin.go`, `internal/app/assemble.go` (`authz.WithAdmin` hanya untuk context modul `core`, seperti `installIdentity`); permukaan registrasi baru dimiliki task ini: `sdk/module/registrar.go` (opsi `Registrar.ScopeValues(resource, loader)` untuk pemuat nilai scope D-C2.6, dan pendaftaran `authz.DimensionSource` bila K-C2.3 disetujui) beserta seluruh implementer `Registrar` yang didaftar di C2.3.

**Interfaces:**
- `org_units`: `id uuid`, `parent_id` (nullable, FK diri), `code` (unik), `name`, `kind` (`company`/`branch`/`department` atau kunci jenis kustom), `path text` (materialized path id), `active`; indeks `path text_pattern_ops`. Pindah node memperbarui `path` seluruh subpohon dalam satu transaksi dan menolak siklus; node yang masih dirujuk tidak dihapus, hanya dinonaktifkan (penghapusan tidak disediakan).
- `authz.Admin` (setiap metode menegakkan permission sendiri lewat `Require` terhadap principal di context, menaikkan revisi pada tulis): role (`ListRoles`, `GetRole`, `CreateRole`, `DuplicateRole`, `UpdateRole` dengan `version`, `DeleteRole` hanya `custom` tanpa pemegang); matriks dan field access per role; `AssignRoles(userID, roleIDs)`; `SetScopeAssignments(userID, []Assignment)`; unit organisasi (`ListOrgUnits`, `CreateOrgUnit`, `MoveOrgUnit`, `SetOrgUnitActive`); `EffectiveAccess(userID)` (matriks gabungan + role pemberi tiap izin); `Explain(userID, permission, ref *RecordRef)` (jawaban per lapis aksi/scope/field + saran role pemberi; evaluasi record hanya bila modul pemilik resource mendaftarkan pemuat nilai scope, lihat keputusan D-C2.6); `CompareRoles(a, b)`; `Catalog()` (resource, aksi, grup beserta field, dimensi, scope kustom, status modul, penanda "izin baru belum diberikan" untuk role `customized`); `DimensionValues(dimension, q)` (mekanisme menurut K-C2.3).
- Permission (deklarasi `modules/core/module.json`, C2.1): `core.role.{read,create,update,delete}`, `core.user.{read,update}` (penugasan role dan scope, cabut sesi), `core.org_unit.{read,create,update}`; `EffectiveAccess`/`Explain` butuh `core.user.read` **dan** `core.role.read`.

- [ ] **Step 1: Test** AC FR-118: penugasan ke induk dengan `include_descendants` melihat semua cabang di bawahnya; memindahkan cabang ke induk lain langsung mengubah hasil scope (revisi naik); unit nonaktif tetap terlihat di data historis; siklus ditolak; AC FR-113: role `administrator` tidak dapat diubah; `TestLastAdministratorConcurrentRevokeOneFails` (serialisasi lewat kunci advisory transaksi pada pemegang `administrator`); tes pencegahan eskalasi menurut jawaban K-C2.6 (rekomendasi: `TestNonAdminCannotGrantAdministrator`, `TestCannotGrantBeyondOwnEffectiveAccess`); role bawaan diubah menjadi `customized`; duplikasi role menyalin baris dan field access; setiap tulis tanpa permission → `ErrForbidden`; IDOR: `userID`/`roleID` dari pengguna tanpa `core.user.read` → `ErrNotFound`/`ErrForbidden` sesuai FR-119; `Explain` tidak membocorkan nilai field record selain nilai kolom scope yang dievaluasi.
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau (VPS). **Step 5: Commit** `feat(authz): Admin, unit organisasi, hak efektif, dan penjelas akses`.

### Task C2.11: API administrasi modul `core` (FR-114 backend) dan operasi pengguna

**Kelas:** Sonnet. **Bergantung:** C2.10; jawaban K-C2.2.

**Files:** Modify `modules/core/{module.json,openapi.yaml,module.go,i18n/*}`, Create `modules/core/internal/presentation/http/{roles.go,users.go,orgunits.go,access.go,sessions.go,*_test.go,*_idor_integration_test.go}`; Modify `sdk/identity` dan `internal/platform/identity` (aditif: `Admin.ListUsers`, `GetUser`, `ListSessions(userID)`, `RevokeSession(userID, sessionID)`, ditambah operasi K-C2.2 bila disetujui; otorisasi lewat `authz.Require(ctx, "core.user.*")`, bukan hanya role administrator), `tests/security/*`.

**Interfaces (endpoint; semua `x-erp-permission`, amplop `data`, `If-Match` pada `PATCH`/`DELETE`/aksi, `Idempotency-Key` pada `POST`):**

| Method dan path | Permission |
| --- | --- |
| `GET /api/core/access-catalog` | `core.role.read` |
| `GET/POST /api/core/roles`, `GET/PATCH/DELETE /api/core/roles/{id}` (body memuat `permissions[]`, `fieldAccess[]`) | `core.role.read/create/update/delete` |
| `POST /api/core/roles/{id}/duplicate` | `core.role.create` |
| `GET /api/core/role-comparisons?a=&b=` | `core.role.read` |
| `GET /api/core/users`, `GET /api/core/users/{id}` | `core.user.read` |
| `POST /api/core/users/{id}/assign-roles`, `POST /api/core/users/{id}/set-scope-assignments` | `core.user.update` |
| `GET /api/core/users/{id}/effective-access`, `POST /api/core/access-explanations` | `core.user.read` (+ `core.role.read` di implementasi) |
| `GET /api/core/sessions?userId=`, `POST /api/core/sessions/{id}/revoke` | `core.user.read` / `core.user.update` |
| `GET/POST /api/core/org-units`, `PATCH /api/core/org-units/{id}`, `POST /api/core/org-units/{id}/move` | `core.org_unit.read/create/update` |
| `GET /api/core/scope-dimensions/{key}/values?q=` | `core.user.update` |
| (K-C2.2) `POST /api/core/users`, `POST /api/core/users/{id}/disable`, `.../enable` | `core.user.create` / `core.user.update` |

Kode galat baru dalam katalog api-guidelines §8 bila diperlukan: `last_administrator` (422 `business_rule_violated`, `details[].code`), `role_immutable` (409 `invalid_state`).

- [ ] **Step 1: Test** kesesuaian kontrak (route, permission, klasifikasi); tiap endpoint ber-ID punya tes IDOR A vs B dengan penanda C2.8; `RevokeSession` hanya sesi milik `userID` di path (sesi pengguna lain → 404); `disable` mencabut seluruh sesi (`OnUserDisabled`, menutup butir terbuka security-guidelines §6 V7); respons tanpa `password_hash`, secret, atau hash token; `task contracts:check` hijau.
- [ ] **Step 2:** gagal. **Step 3:** implementasi (handler tipis memanggil `authz.AdminFrom`/`identity.AdminFrom`). **Step 4:** hijau; `task contracts:check`; `module:check`. **Step 5: Commit** `feat(core): API administrasi role, pengguna, unit organisasi, dan penjelas akses`.

### Task C2.12: Gerbang API mutasi admin `ERP_MODULES_ADMIN_API` (K-C2.4)

**Kelas:** Haiku (bila opsi rekomendasi disetujui; perubahan mekanis berspesifikasi lengkap). **Bergantung:** C2.5, jawaban K-C2.4.

**Files (opsi rekomendasi):** Modify `internal/app/assemble.go` (hapus `SkipRoute` mutasi), `internal/platform/modules/admin/api.go` dan `internal/platform/i18n/admin/overrides.go` (route mutasi `module.RequireMFA()`), `internal/platform/config/config.go` (flag diabaikan + WARN; `ModulesAdminAPIEnabled` dihapus), `bootstrapFrom` (`features.modulesAdminApi` selalu `true`, field tetap di kontrak), `contracts/openapi/platform.yaml` (deskripsi), `cmd/erpora/inspect.go`, `.env.example`, `docs/operations/deployment.md` (tabel env dan checklist produksi), `docs/module-sdk/i18n.md`, PRD-000 FR-20.2 (catatan keputusan).

- [ ] **Step 1: Test** mutasi modul/i18n di production: tanpa `platform.modules.manage` 403, dengan permission tetapi sesi tanpa `otp` 403 `mfa_required`, administrator ber-MFA 200; tidak ada lagi 404 karena flag; `TestModulesAdminAPIEnvIgnoredWithWarning`; `contracts:breaking` hijau.
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau. **Step 5: Commit** `feat(platform): API mutasi admin dijaga permission dan MFA, flag sementara dipensiunkan`.

### Checkpoint Wave 3

- [ ] Gerbang seperti W1 + integrasi penuh paket `authorization`, `identity`, `modules/core`. Review Opus independen + `security-audit` terfokus pada API admin (eskalasi hak menurut aturan jawaban K-C2.6; BFLA, IDOR sesi/pengguna, race administrator terakhir, kebocoran penjelas akses). Putaran perbaikan.

### Task C2.13: Frontend platform: ringkasan permission dan pembaruan tanpa login ulang (FR-117)

**Kelas:** Sonnet. **Bergantung:** C2.5.

**Files:** Modify `packages/platform-web/src/permissions/index.ts` (+ tes), `packages/platform-web/src/api/client.ts` (membaca `X-Erp-Perm-Rev`), `packages/platform-web/src/bootstrap/{fetcher.ts,provider.tsx}`, tipe bangkitan `platform.gen.ts` (`contracts:generate`), `docs/module-sdk/frontend.md`.

**Interfaces:** `usePermission(permission: string): boolean` (bila `access` ada di bootstrap: hanya permission tercantum; `permissions` kosong berarti tidak berhak; perilaku optimistis hanya bila `access` tidak ada); `usePermissionConditional(permission): boolean`; `useFieldAccess(resource: string): (group: string) => "none" | "read" | "write"`; klien memuat ulang bootstrap (single-flight) saat `X-Erp-Perm-Rev` lebih besar dari `permRev` tersimpan, dan membersihkan cache query terkait.

- [ ] **Step 1: Test** `bun test`: ringkasan kosong dengan `access` = tidak berhak; revisi lebih besar memicu tepat satu muat ulang bootstrap untuk tiga respons serentak; revisi sama/lebih kecil tidak memicu; header tidak sah diabaikan; tidak ada penyimpanan di `localStorage`.
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau; `task web:check`. **Step 5: Commit** `feat(web): ringkasan permission dan muat ulang saat perm_rev berubah`.

### Task C2.14: UI administrasi hak akses modul `core` (FR-114)

**Kelas:** Sonnet. **Bergantung:** C2.11, C2.13.

**Files:** Create `modules/core/web/src/pages/{roles,role-editor,role-compare,users,user-detail,effective-access,access-explainer,org-units}/**`, i18n `modules/core/i18n/{id,en}.json`; Modify `modules/core/web/src/{routes.tsx,module.ts}`, `modules/core/module.json` (menu Desk ber-permission).

**Interfaces:** rute `/core/roles`, `/core/roles/:id`, `/core/roles/compare`, `/core/users`, `/core/users/:id`, `/core/org-units`; komponen hanya dari `@erpora/ui` terpasang (Table, Checkbox, Select, Tabs, Sheet, Dialog, Collapsible, Command, Popover, RadioGroup, Badge, Alert, Button, LoadingButton); halaman dimuat lazy. Matriks: baris resource dikelompokkan per modul, kolom aksi yang dideklarasikan resource, kolom Scope (pemilih tipe), panel Grup field (daftar field); resource modul nonaktif tampil abu-abu; banner "izin baru belum diberikan" untuk role `customized`. Halaman pengguna: role, penugasan dimensi (termasuk mode "beserta turunan"), sesi aktif dengan tombol cabut (PAT menyusul C4).

- [ ] **Step 1: Test** `bun test`: matriks merender aksi per resource; perubahan scope dan field group mengirim payload benar dengan `If-Match`; konflik `412` ditampilkan; tombol tersembunyi sesuai `usePermission`; penjelas menampilkan tiga lapis; semua teks lewat i18n; tanpa `dangerouslySetInnerHTML`.
- [ ] **Step 2:** gagal. **Step 3:** implementasi. **Step 4:** hijau; `task web:check` dalam anggaran 210 KB. **Step 5: Commit** `feat(core): UI matriks role, pengguna, unit organisasi, dan penjelas akses`.

### Task C2.15: Gerbang, dokumen, review akhir, verifikasi browser (checkpoint C2)

**Kelas:** Sonnet (gerbang dan dokumen; Haiku untuk dokumen mekanis), review akhir Opus independen.

- [ ] **Step 1: Gerbang** `task lint arch check:gowork-off test`; `ERP_TEST_DATABASE_URL=postgres://erp:erp@127.0.0.1:15432/postgres?sslmode=disable GOFLAGS=-p=2 task test:integration` (tunnel `ssh -N -L 15432:127.0.0.1:5432 <host>`); `task contracts:check`; `task erpora -- module:check`; `task metadata:check`; `ERP_REQUIRE_BUN=1 task web:check`; `task test:generator` (VPS); `govulncheck ./...`; jalankan ulang semua tes Review Focus 1 sampai 5 dan `BenchmarkRequireCacheHit`.
- [ ] **Step 2: Dokumen**: PRD-001 §10 baris C2 (status), §4 (aditif `include_descendants`, benih `authenticated`, `authz_state`), §11 Q35 dan seterusnya; ADR-0013 §10; `docs/architecture/api-guidelines.md` §8 (kode galat baru), §12 (`X-Erp-Perm-Rev`), §15; `docs/security/security-guidelines.md` §3.1 (penegak nyata), §6 (checklist V8 otorisasi), §7 (baris Authorization **C2 ✅**); `docs/module-sdk/security-primitives.md` (bagian baru `sdk/authz`), `creating-a-module.md` (deklarasi resource), `contracts.md` (penanda IDOR), `frontend.md`, `generators.md`; `contracts/module.schema.json` (aditif, sudah di C2.1); `docs/specs/resource-metadata/tasks.md` (T4 terbuka); rencana ini (centang).
- [ ] **Step 3: Review akhir independen** (Opus, tanpa konteks implementer, diberi diff seluruh C2 dan spek): kesesuaian FR-110 sampai FR-119 dan exit gate §10, plus `security-audit` terfokus pada permukaan C2. Temuan sesuai spek diperbaiki implementer yang sama; di luar spek ditanyakan; satu review atas diff perbaikan.
- [ ] **Step 4: Verifikasi browser** (aplikasi penuh, DB sementara di VPS lewat tunnel): AC E2E FR-114: admin membuat role dari duplikat, mengubah scope ke "milik sendiri", menyembunyikan grup non-default, meng-assign ke pengguna kedua; pengguna kedua (sesi terbuka di tab lain) **tanpa login ulang** melihat perubahan setelah `perm_rev` berubah (menu, kolom, tombol, data); penjelas akses dan perbandingan role; pohon unit organisasi dan scope "beserta turunan"; konsol bersih; DB sementara dihapus.
- [ ] **Step 5: Commit** dan laporkan terpisah: yang dikerjakan, yang menunggu konfirmasi, yang sengaja tidak dikerjakan.

---

## Keputusan yang saya ambil sesuai PRD (dicatat sebagai keputusan implementer di PRD-001 Q35, bukan keputusan pemilik produk)

| # | Keputusan | Dasar |
| --- | --- | --- |
| D-C2.1 | Tabel `org_units` milik platform (migrasi tenant, tanpa awalan slug); modul `core` hanya API/UI dan `modules/core/contract` untuk modul lain | §3 (platform "pemilik tabel"; `core` tanpa tabel identitas/authz sendiri), §4 |
| D-C2.2 | `ERP_AUTHZ_STUB` dan masker stub dihapus di C2; nilai sisa diabaikan dengan WARN | PRD-000 FR-05 dan PRD-001 FR-106 (stub diganti), K-C1.9 |
| D-C2.3 | Server selalu memakai revisi terkini (memori, notifier + muat ulang 1 menit); klaim JWT `perm_rev` hanya informasi (cara klien mengetahui perubahan: K-C2.8) | FR-116, ADR-0013 "alternatif ditolak: permission di JWT" |
| D-C2.4 | Bootstrap: `permissions` diisi daftar datar, ringkasan per resource di properti aditif `access` | FR-117, api-guidelines §14.1 |
| D-C2.5 | Sebelum audit C3, pemakaian `AsSystem` dicatat log terstruktur `authz.system_call` (principal asli + reason); C3 memindahkannya ke `audit_log` | FR-119 butir 2, FR-130 milik C3 |
| D-C2.6 | Penjelas akses untuk record tertentu memakai pemuat nilai scope opsional yang didaftarkan modul pemilik (hanya kolom `owner_field`/`scope_fields`); tanpa pemuat, penjelas menjawab per lapis tanpa evaluasi record | FR-114 butir 4, D2 (platform tidak mengimpor modul) |
| D-C2.7 | `internal/platform/authorization/store` masuk allowlist D3b (sudah disetujui sebagai K-C1.7); `sdk/authz` disetujui lewat K-C2.7 (4 Nov 2026, PRD-000 Q44); keduanya sudah di `rules.go` (C2.0) | K-C1.7, PRD-001 Q15, Q42 |
| D-C2.13 | Operasi `identity.Admin` yang ada (`CreateAdmin`, `ResetMFA`, `UnlockAccount`, `LogoutAllDevices`, `RotateSigningKeys`) **tetap** memakai aturan administrator ber-MFA atau hak sistem `cli:core:` (Q30 butir 1, tidak dilonggarkan); hanya operasi baru C2.11 (`ListUsers`, `GetUser`, `ListSessions`, `RevokeSession`) memakai `authz.Require("core.user.*")` | Q30 butir 1, FR-114 |
| D-C2.8 | Mode penugasan FR-118 disimpan sebagai kolom aditif `user_scope_assignments.include_descendants` | FR-118 (dua mode), §4 aditif |
| D-C2.9 | Kode `scope` tersimpan sebagai larik token PRD (`all`, `owner`, `assigned:<dim>`, `custom:<key>`), `all` eksplisit | FR-111; mencegah gagal terbuka pada larik kosong |
| D-C2.10 | Nama permission modul `core`: `core.role.*`, `core.user.*`, `core.org_unit.*` | FR-114 (PRD tidak menamai) |
| D-C2.11 | Pemberitahuan "izin baru dari modul X" ditampilkan sebagai banner yang dihitung saat dibaca, bukan sistem notifikasi | FR-113; PRD Notifications belum ada |
| D-C2.12 | Format penanda IDOR = format resource-metadata D-22 diperluas untuk metode kontrak | FR-119 butir 3, konsistensi dengan T4 24.1 |

**Sengaja tidak dikerjakan di C2:** konflik SoD kombinasi role (`transaction-data-standards.md` §9; tidak ada FR PRD-001), audit append-only dan penolakan 403 teraudit (C3, FR-130), PAT di halaman pengguna (C4), event outbox yang membawa principal (belum ada infrastruktur outbox), masking baris `embedded` bersarang (T4 Req 6.11).

## KEPUTUSAN YANG BUTUH KONFIRMASI PEMILIK PRODUK

> **STATUS: seluruh butir K-C2.1 sampai K-C2.8 DISETUJUI pemilik produk pada 4 Nov 2026 sesuai rekomendasi (opsi pertama/(a) tiap butir).** Task C2.0 mencatatnya di PRD-001 (Q35 dst.), PRD-000 (Q44 untuk K-C2.7), ADR-0013, api-guidelines §12 (K-C2.8), dan merevisi butir K35 (K-C2.1) serta contoh FR-110 (K-C2.5), mengubah butir spek yang bertentangan dan menutup pertanyaan terkait, dengan tanggal absolut dan "disetujui pemilik produk". Tabel di bawah dipertahankan sebagai konteks keputusan.

| # | Pertanyaan | Mengapa ambigu | Opsi dan rekomendasi |
| --- | --- | --- | --- |
| **K-C2.1** | Permission rute yang tidak berbentuk `<resource>.<aksi>` terdeklarasi: `example.notes.read` (K35: "tidak diganti"), juga `example.ping.read`, `example-addon.summary.read`, dan `platform.*` | FR-110 mewajibkan setiap permission rute merujuk resource terdeklarasi; tanpa itu permission tidak dapat diberikan di matriks dan hanya Administrator yang bisa mengakses rutenya. K35 (2 Nov 2026) menahan `example.notes.read` padahal resource metadata-nya `example.note` | **Rekomendasi (a):** ganti `example.notes.read` → `example.note.read` (merevisi butir K35 itu; aman karena belum ada `role_permissions` tersimpan dan modul belum bertag rilis), sedangkan capability lain yang sudah berbentuk `<resource>.<aksi>` dideklarasikan sebagai resource apa adanya (`example.ping`, `example-addon.summary`, `platform.modules`, `platform.i18n`, `platform.desks`). (b) Pertahankan K35: registry menurunkan resource implisit dari capability warisan (hanya grup `default`), sehingga matriks memuat `example.notes` dan `example.note` berdampingan |
| **K-C2.2** | Jalur membuat dan menonaktifkan pengguna non-admin | AC E2E FR-114 meng-assign role ke pengguna lain, tetapi satu-satunya jalur pembuatan akun adalah `core:create-admin`; seed tidak boleh memanggil `identity.Admin` (Q30 butir 1); hook `OnUserDisabled` belum punya pemanggil (security-guidelines §6 V7) | **Rekomendasi:** `POST /api/core/users` (email, nama; akun aktif tanpa sandi, email tautan atur-sandi lewat alur reset 30 menit yang sudah ada, dapat dikirim ulang) dan `POST .../disable`/`enable` (disable mencabut semua sesi), permission `core.user.create`/`core.user.update`. Alternatif: command `core:create-user` saja (CLI), UI tanpa pembuatan pengguna |
| **K-C2.3** | Sumber nilai dimensi scope untuk pemilih penugasan | FR-111 menyebut "lookup ke modul pemilik lewat `contract/`", tetapi `core` tidak dapat bergantung pada semua modul (D1, D8) | **Rekomendasi:** modul pemilik dimensi mendaftarkan `authz.DimensionSource{Key, Search, Labels}` lewat `Registrar` (implementasi menegakkan otorisasi terhadap principal admin, seperti kontrak); `core` memanggilnya lewat `authz.Admin.DimensionValues`. Dimensi `core.org_unit` disediakan platform. Alternatif: nilai diketik bebas tanpa validasi (tidak disarankan) |
| **K-C2.4** | Nasib `ERP_MODULES_ADMIN_API` | K-C1.9 "dipertahankan sampai C2", tetapi PRD-000 FR-20.2 "production memakai CLI sampai PRD Identity & Authorization selesai" (PRD-001 baru selesai di C4); i18n D-5 menyebut ganti nama flag saat PRD-001 | **Rekomendasi:** pensiunkan di C2 (task C2.12): mutasi dijaga `platform.modules.manage`/`platform.i18n.manage` + `module.RequireMFA()`, flag diabaikan dengan WARN, `features.modulesAdminApi` tetap ada dan bernilai `true`. Alternatif: pertahankan flag (ganti nama `ERP_ADMIN_MUTATION_API`) sebagai sakelar tambahan sampai C4 |
| **K-C2.5** | `label` resource, aksi kustom, field group, dimensi, dan role bawaan: teks literal atau kunci i18n | Contoh FR-110 memakai teks literal ("Barang"), padahal aturan proyek mewajibkan teks UI lewat i18n dan `desk` memakai kunci i18n (PRD-000 Q40) | **Rekomendasi:** kunci i18n namespace modul (`$defs/messageKey`), diterjemahkan di klien seperti Desk. Alternatif: literal satu bahasa |
| **K-C2.6** | Pencegahan eskalasi hak oleh admin terdelegasi | PRD tidak mengatur siapa yang boleh memberi role `administrator` atau menyusun role yang lebih luas dari haknya sendiri; tanpa aturan, pemegang `core.user.update` dapat menjadikan dirinya administrator | **Rekomendasi:** hanya pemegang role `administrator` yang dapat memberi/mencabut role `administrator`; pengguna lain hanya dapat memberi role dan menulis baris matriks/field access yang tidak melampaui hak efektifnya sendiri (422 `business_rule_violated`, `details[].code = privilege_escalation`). Alternatif: hanya aturan pertama (lebih sederhana; delegasi parsial tetap berisiko) |
| **K-C2.7** | `sdk/authz` masuk allowlist GORM D3b | Tanda tangan FR-115 `authz.Filter(...) func(*gorm.DB) *gorm.DB` membuat `sdk/authz` mengimpor GORM, sedangkan D3b hanya mengizinkan `sdk/data`; setiap penambahan D3b sebelumnya disetujui pemilik produk (PRD-000 Q35 D-1, K-C1.7, Q24) | **Rekomendasi:** tambahkan `sdk/authz` ke allowlist (dibatasi praktis pada `filter.go`, `scope.go`, `authz.go`; SDK sudah bergantung GORM lewat `sdk/data`), dicatat PRD-000 Q44. Alternatif: tipe opaque `authz.Scope` di `sdk/authz` dan penerjemah GORM di `sdk/data` (`data.Apply(db, scope)`), menyimpang dari tanda tangan FR-115 |
| **K-C2.8** | Cara klien mengetahui `perm_rev` berubah tanpa login ulang | AC FR-114 mewajibkan perubahan terlihat tanpa login ulang, tetapi klaim JWT hanya diperbarui saat refresh (≤ 10 menit) dan PRD tidak menetapkan mekanismenya; menambah header berarti mengubah api-guidelines §12 yang mengikat (preseden Q25, Q29) | **Rekomendasi:** header respons `X-Erp-Perm-Rev` pada setiap respons request terautentikasi; klien memuat ulang bootstrap bila lebih besar (C2.5, C2.13). Alternatif: klien memuat ulang bootstrap tiap N detik dan saat menerima 403 (lebih lambat, tanpa perubahan pedoman) |

Butir yang memblokir: K-C2.7 → C2.0/C2.3 (Wave 1a); K-C2.1, K-C2.5 → C2.1; K-C2.8 → C2.5/C2.13; K-C2.2 → C2.11; K-C2.3 → C2.10; K-C2.4 → C2.12; K-C2.6 → C2.10/C2.11.

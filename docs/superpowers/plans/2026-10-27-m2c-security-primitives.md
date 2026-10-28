# M2c — Primitif Keamanan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Modul dan platform memiliki primitif keamanan bersama sebelum konsumen pertamanya ditulis (generator M2d, OIDC PRD-001). Primitif itu adalah: helper respons/error `sdk/httpx` dan decoder ketat `httpx.DecodeJSON`; opsi per-route variadik (batas body, klasifikasi) yang menghasilkan `Cache-Control: no-store` untuk data `confidential`/`highly_confidential`; HTTP client keluar anti-SSRF `sdk/egress`; helper ekspor `sdk/export` (CSV/XLSX, anti formula injection, masking field, audit); `statement_timeout` untuk koneksi database `serve`; serta arch test yang melarang HTTP client mentah dan decode ke tipe infrastructure di `modules/**`.

**Architecture:** `sdk/module` memperoleh tipe `Classification` dan `RouteOption`. `Registrar.Handle`/`Public` menerima `opts ...RouteOption` (Q26 butir 13), lalu registry menyimpan hasilnya di `RouteInfo`. `runtime.Mount` membungkus setiap handler modul dengan middleware batas request: `http.MaxBytesReader`, batas body di context, `no-store` berbasis klasifikasi, serta pemasangan `authz.FieldMasker` dan `export.Auditor` stub platform. `sdk/httpx` menjadi satu-satunya implementasi amplop `data`/`error` (`httpserver.WriteError` mendelegasikan ke sana). `sdk/egress` memvalidasi skema/host sebelum request, memvalidasi IP setelah resolve di `DialContext`, lalu dial ke IP tervalidasi. `sdk/export` menulis CSV/XLSX secara streaming lewat `iter.Seq2`. `internal/archcheck` memperoleh pemeriksa bertipe (`golang.org/x/tools/go/packages`) untuk aturan `EGRESS` dan `DECODE`. `database.OpenWith` memasang `statement_timeout` sebagai runtime parameter pgx pada pool `serve`.

**Tech Stack:** Go 1.27.3 · `net/http`, `net/netip`, `encoding/json`, `encoding/csv`, `archive/zip`, `iter` · OpenTelemetry trace API (`go.opentelemetry.io/otel/trace` v1.47.0, kini juga di `sdk`) · pgx v5 (`pgx.ParseConfig`, `stdlib.OpenDB`) · `golang.org/x/tools/go/packages` v0.52.0 · testcontainers-go (integration)

**Spec:** `docs/prd/PRD-000-inisialisasi-project.md` FR-05 (Registrar, rantai middleware), FR-06 (schema, perluasan aditif), FR-24.2 (+ AC), Q22, Q25 (helper error `sdk/httpx` di M2c), Q26 butir 7 (`statement_timeout`) dan butir 13 (opsi per-route variadik), Q28; `docs/security/security-guidelines.md` §2, §3.1, §3.2, §3.3, §3.4, §3.8, §3.10, §6, §7; `docs/architecture/api-guidelines.md` §4.1, §4.2, §7.3, §8, §10, §12; `docs/prd/PRD-001-core.md` FR-110, FR-112, FR-115 (seam `MaskFields`); pembagian M2: `docs/superpowers/plans/2026-10-22-m2-overview.md` (baris M2c).

## Global Constraints

- Go module path: root `erpora`, SDK `erpora/sdk`, modul `erpora/modules/<name>`. Semua `go.mod`: `go 1.27`, `toolchain go1.27.3`. Paket baru SDK: `erpora/sdk/httpx`, `erpora/sdk/egress`, `erpora/sdk/export`. SDK hanya bergantung pada stdlib, `gorm.io/gorm` (sudah ada), dan `go.opentelemetry.io/otel/trace` v1.47.0 (baru, sama dengan root).
- Klasifikasi (urutan naik): `public` < `internal` < `confidential` < `highly_confidential`. `no-store` berlaku untuk `confidential` ke atas. Kolom ekspor `highly_confidential` ditolak.
- Batas body request: default `1 << 20` byte (1 MiB, `module.DefaultMaxBodyBytes`). Opsi per route `module.MaxBodyBytes(n)` hanya menerima `1 ≤ n ≤ 10 << 20` (`module.MaxRouteBodyBytes`). Platform memasang `http.MaxBytesReader` pada **setiap** route modul sebelum handler.
- `DecodeJSON`: kedalaman maksimal `32` (objek akar = 1), panjang array maksimal `1000` elemen, DTO wajib `struct`, `Content-Type` wajib `application/json` (parameter `charset` hanya `utf-8`), satu nilai JSON objek, tanpa data sesudahnya.
- Pemetaan error decode (katalog api-guidelines §8, kode spesifik di `error.details[].code`):

  | Kondisi | Status | `error.code` | `details[].code` | `details[].field` |
  | --- | --- | --- | --- | --- |
  | `Content-Type` bukan `application/json`/charset bukan utf-8/hilang padahal body ada | 400 | `bad_request` | `unsupported_media_type` | — |
  | Body melebihi batas | 400 | `bad_request` | `body_too_large` | — |
  | Body kosong (tanpa `AllowEmptyBody`) | 400 | `bad_request` | `body_required` | — |
  | JSON rusak | 400 | `bad_request` | `malformed_json` | — |
  | Nilai akar bukan objek (`[]`, `null`, `1`) | 400 | `bad_request` | `expected_object` | — |
  | Ada data sesudah nilai akar | 400 | `bad_request` | `trailing_data` | — |
  | Kedalaman > batas | 400 | `bad_request` | `too_deep` | path |
  | Array > batas | 400 | `bad_request` | `array_too_long` | path |
  | Kunci ganda dalam satu objek | 400 | `bad_request` | `duplicate_key` | path |
  | Field tidak dikenal (termasuk beda huruf besar/kecil) | 422 | `validation_failed` | `unknown_field` | path |
  | Tipe nilai salah | 422 | `validation_failed` | `invalid_type` | path |

  Path memakai format api-guidelines §8: `lines[2].itemId`, `meta.k`.
- Egress: skema `https`; `http` hanya untuk host di `Config.AllowHTTPHosts`. Timeout koneksi `5s`, total `30s`, respons maksimal `10 << 20` byte, redirect maksimal `3`, TLS minimal 1.2, proxy dari environment **tidak** dipakai. Alamat yang diblokir (setelah `netip.Addr.Unmap()`): `0.0.0.0/8`, `10.0.0.0/8`, `100.64.0.0/10`, `127.0.0.0/8`, `169.254.0.0/16`, `172.16.0.0/12`, `192.0.0.0/24`, `192.168.0.0/16`, `198.18.0.0/15`, `224.0.0.0/4`, `240.0.0.0/4` (termasuk `255.255.255.255`), `::/128`, `::1/128`, `fc00::/7`, `fe80::/10`, `fec0::/10`, `ff00::/8`, serta IPv4 yang tertanam di `64:ff9b::/96` (NAT64) dan `2002::/16` (6to4) yang jatuh di rentang IPv4 terblokir. Host numerik non-kanonik (`2130706433`, `0x7f000001`, `0177.0.0.1`, `127.1`) ditolak sebagai `numeric_host`.
- Ekspor: `DefaultMaxRows = 10000`. CSV: diawali BOM UTF-8, akhir baris CRLF, `Content-Type: text/csv; charset=utf-8`. XLSX: `application/vnd.openxmlformats-officedocument.spreadsheetml.sheet`, semua sel `t="inlineStr"`. `Content-Disposition: attachment; filename="<base>.<ext>"; filename*=UTF-8''<base>.<ext>`, dengan nama dasar cocok `^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`. Sel CSV yang diawali `=`, `+`, `-`, `@`, tab, atau CR diberi awalan `'`, kecuali string desimal murni `^-?[0-9]+(\.[0-9]+)?$` (keputusan 7).
- `statement_timeout`: env `ERP_DB_STATEMENT_TIMEOUT_MS`, default `30000`, rentang valid `1000`–`600000` (di luar itu `ValidationError`, exit 2). Hanya pool `erpora serve` yang memakainya.
- Kode problem registry baru: `invalid_route_option`. Aturan arch baru: `EGRESS`, `DECODE`.
- Rantai middleware route modul: `otel → recover → request-id/log → authN → tenant context → module status guard → permission check → batas request (body limit, no-store, masker, auditor) → handler`.
- Tidak ada breaking change API HTTP. Amplop error tetap byte-identik dengan M2b, kecuali endpoint aksi admin yang kini menjawab `422 validation_failed` untuk field tak dikenal (sudah terdokumentasi `422` di `platform.yaml`).
- Windows: `-race` dilewati; Taskfile tanpa `rm`/`cp`/`sed`.
- TDD; satu commit per task; pesan konvensional diakhiri baris kosong + `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Gate sebelum commit: `task lint arch check:gowork-off test`, ditambah `task test:integration` untuk task yang menyentuh DB (Task 11). Setelah mengubah dependensi `sdk`, jalankan `task tidy` agar `go.sum` modul ikut diperbarui.

## Keputusan rencana (perlu disetujui pada review rencana)

1. **Error decode memakai katalog api-guidelines §8 apa adanya** (tabel di Global Constraints): kondisi struktur/format → `400 bad_request`, field tak dikenal dan tipe salah → `422 validation_failed` (§7.3), dengan kode spesifik di `details[].code`. Pola ini sama dengan keputusan 1 M2b. **(perlu konfirmasi owner)** — semantik HTTP yang tepat untuk body terlalu besar dan media type salah adalah `413`/`415`, tetapi katalog belum memuat `payload_too_large`/`unsupported_media_type`. Rekomendasi: tetap `400` sekarang. Bila owner memilih `413`/`415`, kedua kode ditambahkan ke katalog §8 dan `common.yaml` di Task 13, lalu tabel decode di Task 4 disesuaikan.
2. **Keketatan decode melebihi teks FR-24.2.** Selain batas ukuran, kedalaman, field tak dikenal, dan DTO (yang diwajibkan spec), `DecodeJSON` juga menolak `Content-Type` selain `application/json`, kunci ganda, kunci yang hanya cocok bila huruf besar/kecil diabaikan (`encoding/json` mencocokkan tanpa memedulikan huruf, sehingga `DisallowUnknownFields` meloloskan `{"Title":…}`), dan data sesudah objek akar. Batas panjang array (`1000`) berasal dari security-guidelines §3.8. **(perlu konfirmasi owner)** — keempat penolakan pertama adalah perilaku baru. Alasannya: kunci ganda dan kunci beda huruf membuka *parser differential* (proxy/validator membaca nilai yang berbeda dari handler), dan pemeriksaan `Content-Type` menutup jalur CSRF *simple request*. Rekomendasi: setujui.
3. **Sumber `no-store` di M2c adalah opsi route `module.Classify(c)`, bukan kontrak.** FR-24.2 menyebut "berdasarkan klasifikasi di kontrak", tetapi kontrak OpenAPI per modul, `x-erp-classification`, dan test kesesuaian route ↔ kontrak baru dibangun di M2d (FR-23). M2c menyimpan klasifikasi di `RouteInfo.Classification`. M2d kemudian menambah assertion pada test kesesuaian FR-23.2: `RouteInfo.Classification` wajib sama dengan klasifikasi tertinggi (rekursif, termasuk properti bersarang) skema respons operasi itu di kontrak. Dengan begitu kontrak tetap menjadi sumber kebenaran dan kode tidak dapat menyimpang. Route tanpa `Classify` tidak mendapat header (diperlakukan setara `internal`). **(perlu konfirmasi owner)** — urutan milestone membuat teks spec tidak dapat dipenuhi secara harfiah di M2c.
4. **`statement_timeout` dipasang sebagai runtime parameter koneksi pool `serve`**, bukan `SET` per request. `database.OpenWith(…, OpenOptions{StatementTimeout})` memakai `pgx.ParseConfig` + `RuntimeParams["statement_timeout"]` + `stdlib.OpenDB`, sehingga setiap koneksi di pool membawa nilai yang sama sejak dibuat. `SET` level sesi tidak pernah dipakai karena nilainya akan terbawa ke request berikutnya yang memakai koneksi pool yang sama. `SET LOCAL` di dalam transaksi tetap aman karena berakhir bersama transaksinya. Nilai default `30000` ms lewat `ERP_DB_STATEMENT_TIMEOUT_MS`. CLI (`migrate`, `modules`, command modul) tidak memakai batas ini agar migrasi panjang tidak terputus. Konsekuensinya, operasi lifecycle lewat API admin dev di dalam `serve` ikut dibatasi `30s` per statement. Pembatalan saat klien putus tetap datang dari ctx request (`WithContext`). **(perlu konfirmasi owner)** — Q26 butir 7 tidak menyebut nilai maupun mekanisme.
5. **Seam `authz.MaskFields` dan auditor ekspor sebelum PRD-001.** `sdk/authz` memperoleh `MaskFields(ctx, resource, dto) any` dengan signature PRD-001 FR-115, ditopang `FieldMasker` yang dipasang platform di context (`authz.WithFieldMasker`). Tanpa masker, hasilnya objek kosong (gagal tertutup). `sdk/export` memperoleh `Auditor` di context (`export.WithAuditor`). Platform memasang `authorization.StubFieldMasker` (mode mengikuti `ERP_AUTHZ_STUB`: `allow` = tanpa masking dan tercatat di log, `deny` = semua field dibuang) dan `audit.ExportLog` (log terstruktur tanpa nilai sel) di setiap route modul dan `ModuleContext`. PRD-001 mengganti keduanya tanpa mengubah modul. **(perlu konfirmasi owner)** — keputusan ini mendefinisikan API PRD-001 lebih awal. Tanpa seam ini, frasa FR-24.2 "menghormati `authz.MaskFields`" dan "mencatat ekspor ke audit" tidak dapat ditegakkan oleh helper.
6. **XLSX ditulis dengan stdlib** (`archive/zip` + XML minimal, satu sheet `Data`). Semua sel bertipe `inlineStr`, dan elemen `<f>` tidak pernah ditulis, sehingga formula injection mustahil secara konstruksi. Awalan `'` tidak dipakai di XLSX karena akan tampil literal. Library `excelize` ditolak karena menambah dependensi besar ke setiap modul lewat `sdk`.
7. **Pengecualian awalan `'` untuk string desimal murni di CSV.** Security-guidelines §3.2 mewajibkan awalan untuk sel yang diawali `-`, padahal uang/kuantitas di sistem ini adalah string desimal (api-guidelines §4.1), sehingga nilai negatif `-1250.00` akan tampil sebagai `'-1250.00` di spreadsheet. String yang cocok `^-?[0-9]+(\.[0-9]+)?$` tidak dapat menjadi formula, jadi tidak diberi awalan. String lain yang diawali `-` (mis. `-1+2`) tetap diberi awalan. **(perlu konfirmasi owner)** — menyimpang dari teks harfiah §3.2. Rekomendasi: setujui, lalu teks §3.2 diselaraskan di Task 13.
8. **Opsi per-route variadik** (Q26 butir 13): `Handle(method, path, permission string, h http.Handler, opts ...RouteOption)` dan `Public(method, path string, h http.Handler, opts ...RouteOption)`. Pemanggil modul tidak perlu berubah. Implementer `Registrar` hanya ada di platform/SDK (`registry.recorder`, `moduletest.Recorder`, `fakeRegistrar` di `contract_test.go`), dan semuanya diperbarui di Task 1. Opsi yang tidak valid menjadi problem registry `invalid_route_option`, dikumpulkan bersama problem lain (FR-05 aturan 7). `SDKVersion` tetap `0.1.0`: SDK belum pernah di-tag (Q24), sehingga aturan 0.x ADR-006 mulai berlaku pada tag pertama (workflow tag M2d). Catatan ini ditambahkan ke ADR-006 di Task 13.
9. **Batas request ditegakkan platform, bukan hanya oleh `DecodeJSON`.** Middleware batas request di `runtime.Mount` memasang `http.MaxBytesReader` (sehingga handler yang membaca body sendiri pun terbatas) dan `httpx.WithBodyLimit(ctx, n)`. `DecodeJSON` membaca batas dari context (default 1 MiB di luar platform, mis. di unit test). Langkah ini berada di slot handler pada rantai FR-05, setelah permission. FR-05 diberi catatan di Task 13.
10. **`sdk/httpx` adalah satu-satunya implementasi amplop.** `httpserver.WriteError`/`WriteData`/`WritePage` mendelegasikan ke `httpx`, dan `httpserver.PageInfo` menjadi alias `httpx.Page`. `traceId` dibaca dari span context (sebabnya `sdk` bergantung pada `otel/trace`, yang hanya berupa API). Handler `example`/`exampleaddon` dan API admin dimigrasikan ke `httpx`, lalu helper lokal `writeJSON`/`writeError`/`decodeAction` dihapus, karena keduanya menjadi acuan template `make:module` (M2d).
11. **Egress gagal tertutup pada jawaban DNS campuran**: bila salah satu IP hasil resolve terblokir, seluruh request ditolak. Dial hanya ke IP tervalidasi, berurutan, tanpa resolve ulang. Daftar rentang terblokir lebih lengkap daripada contoh §3.3 (rentang khusus IANA, NAT64/6to4) karena §3.3 mewajibkan "blokir alamat internal". `Config.Resolver` boleh diganti (validasi tetap berjalan sesudah resolve). Pengecualian loopback hanya ada lewat hook di `export_test.go`, sehingga tidak dapat dipakai dari luar paket.
12. **Aturan arch bertipe.** `EGRESS` dan `DECODE` memakai `go/packages` + `go/types`, karena deteksi "decode ke tipe infrastructure" butuh tipe target, termasuk slice, map, dan struct pembungkus. `DECODE` melarang target yang memuat tipe bernama dari paket bersegmen `internal/infrastructure`, kecuali `json.Unmarshal`/`Decoder.Decode` **di dalam** paket infrastructure itu sendiri (payload jsonb/egress milik adapter). `httpx.DecodeJSON` di paket infrastructure selalu ditolak. Cakupan keduanya adalah `modules/**` (sesuai FR-24.2). Pemakaian HTTP client di platform (exporter OTLP ke endpoint operator) bukan koneksi yang dipengaruhi data, sehingga tetap lewat review.
13. **Schema manifest**: `resources[].field_groups[].classification` didefinisikan sebagai enum empat kelas (opsional di M2c, aditif; `schema_version` tetap `1`). Isi `resources` lainnya tetap longgar sampai PRD-001. Linter kontrak yang mewajibkan klasifikasi untuk field sensitif tetap di M2d.
14. **Ekspor sinkron dibatasi `MaxRows`** (default 10 000), karena api-guidelines §10 melarang endpoint proses panjang sebelum infrastruktur Jobs ada. Kesalahan yang terjadi sebelum byte pertama ditulis dikembalikan sebagai error (handler → `500 internal_error`). Kesalahan di tengah stream mencatat audit dengan `Err`, lalu `panic(http.ErrAbortHandler)`, sehingga klien tidak menerima file terpotong yang tampak utuh. Nilai kolom wajib skalar: nilai bersarang (map/slice/struct) ditolak `ErrNonScalarValue`, agar field bersarang tidak bocor di bawah klasifikasi kolom yang lebih rendah.

## Review Focus

1. **Bypass SSRF**: hostname yang resolve ke IP privat, jawaban DNS campuran publik+privat, DNS rebinding (resolve kedua berbeda), redirect ke IP privat/metadata, IPv4-mapped IPv6 (`[::ffff:127.0.0.1]`), host desimal/heksadesimal/oktal/singkat (`2130706433`, `0x7f000001`, `0177.0.0.1`, `127.1`), dan NAT64/6to4 yang menanam IP privat. Test: `TestBlockedReason_Table`, `TestClient_RejectsFR24Table`, `TestClient_DialsValidatedIP`, `TestClient_Redirects` (Task 7).
2. **Keketatan decode**: kunci ganda (termasuk di map), field tak dikenal bersarang dan beda huruf, data sesudah objek, body raksasa (termasuk handler yang membaca body tanpa `DecodeJSON`), `Content-Type` salah, dan kedalaman/array berlebih. Test: `TestDecodeJSON_Table` (Task 4), `TestMount_BodyLimitEnforcedWithoutDecode` (Task 5), `TestAction_BodyStrict` (Task 6).
3. **Formula injection CSV dan kebocoran lewat ekspor**: `=1+1` → `'=1+1`, tab/CR, nilai bersarang, kolom yang di-mask, dan kolom `highly_confidential`. Test: `TestSanitizeCell_Table`, `TestWrite_CSV_Golden`, `TestWrite_NestedValueRejected`, `TestWrite_MaskedColumnsEmpty`, `TestValidate_HighlyConfidentialColumnRejected` (Task 9).
4. **Klasifikasi → `no-store`**: `confidential` dan `highly_confidential` membawa `no-store`, termasuk pada respons error handler, sedangkan `public`/`internal`/tanpa opsi tidak. `Classify` berulang mengambil kelas tertinggi. Test: `TestResolveRouteOptions` (Task 1), `TestMount_NoStoreByClassification` (Task 5), `TestServe_Headers` (Task 9).
5. **`statement_timeout` dan connection pooling**: setiap koneksi pool `serve` membawa batas yang sama, `SET LOCAL` di transaksi tidak bocor ke pemakai koneksi berikutnya, statement yang melebihi batas gagal dengan SQLSTATE `57014`, dan pool CLI tidak dibatasi. Test: `TestStatementTimeout_AllPooledConnections`, `TestOpen_NoStatementTimeoutByDefault` (Task 11).

---

## File Structure (M2c)

```
sdk/module/{classification.go, route.go, registrar.go, classification_test.go, route_test.go, contract_test.go}   # Task 1
sdk/module/moduletest/{moduletest.go, moduletest_test.go}                                   # Task 1
internal/platform/modules/registry/{recorder.go, registry.go, errors.go, route_options_test.go}  # Task 1
contracts/module.schema.json; internal/platform/modules/manifest/validate_test.go;
  internal/platform/modules/testdata/manifests/{valid_full.json, bad_field_group_classification.json}  # Task 2
sdk/go.mod, sdk/go.sum; sdk/httpx/{doc.go, respond.go, respond_test.go}                     # Task 3
internal/platform/httpserver/errors.go                                                       # Task 3
sdk/httpx/{limits.go, decode.go, decode_walk.go, decode_test.go}                             # Task 4
internal/platform/modules/runtime/{limits.go, mount.go, limits_test.go}                      # Task 5, 10
internal/platform/modules/admin/{api.go, api_test.go}; delete decode.go; contracts/openapi/platform.yaml  # Task 6
modules/example/internal/presentation/http/{handlers.go, handlers_test.go}; modules/exampleaddon/internal/presentation/http/{summary.go, summary_test.go}; modules/*/go.{mod,sum}  # Task 6
sdk/egress/{doc.go, egress.go, addr.go, dial.go, body.go, export_test.go, addr_test.go, egress_test.go}  # Task 7
sdk/authz/{fields.go, fields_test.go}; sdk/authz/authztest/authztest.go                      # Task 8
sdk/export/{doc.go, export.go, cell.go, csv.go, xlsx.go, serve.go, *_test.go}                # Task 9
internal/platform/authorization/{fields.go, fields_test.go}; internal/platform/audit/{export.go, export_test.go};
  internal/platform/modules/runtime/{modctx.go, modctx_test.go}; internal/app/{assemble.go, lifecycle.go, command.go}  # Task 10
internal/platform/config/{config.go, config_test.go}; internal/platform/database/{open.go, open_test.go,
  statement_timeout_integration_test.go}; internal/app/app.go                                 # Task 11
internal/archcheck/{typed.go, typed_test.go}; internal/archcheck/testdata/secmod/**; tests/architecture/arch_test.go; go.mod  # Task 12
docs/module-sdk/{security-primitives.md, README.md, creating-a-module.md}; docs/security/security-guidelines.md;
  docs/prd/PRD-000-inisialisasi-project.md (Q30, FR-05, FR-24.2); docs/adr/0006-…md; docs/operations/deployment.md;
  .env.example; docs/superpowers/plans/2026-10-22-m2-overview.md                             # Task 13
```

---

### Task 1: `Classification`, `RouteOption`, dan `Registrar` variadik

**Files:**
- Create: `sdk/module/classification.go`, `sdk/module/route.go`, `sdk/module/classification_test.go`, `sdk/module/route_test.go`, `internal/platform/modules/registry/route_options_test.go`
- Modify: `sdk/module/registrar.go`, `sdk/module/contract_test.go` (`fakeRegistrar`), `sdk/module/moduletest/moduletest.go`, `sdk/module/moduletest/moduletest_test.go`, `internal/platform/modules/registry/{recorder.go, registry.go, errors.go}`

**Interfaces:**
- Produces (`sdk/module`):
  ```go
  type Classification string
  const (
      ClassPublic             Classification = "public"
      ClassInternal           Classification = "internal"
      ClassConfidential       Classification = "confidential"
      ClassHighlyConfidential Classification = "highly_confidential"
  )
  func ParseClassification(s string) (Classification, error) // selain empat nilai → error
  func (c Classification) Valid() bool
  func (c Classification) Rank() int                          // 0..3; tidak valid/"" → -1
  func (c Classification) AtLeast(o Classification) bool      // c.Rank() >= o.Rank() && c.Valid()
  func MaxClassification(cs ...Classification) Classification // "" bila tidak ada yang valid

  const (
      DefaultMaxBodyBytes int64 = 1 << 20
      MaxRouteBodyBytes   int64 = 10 << 20
  )
  type RouteConfig struct {
      MaxBodyBytes   int64          // 0 = default platform
      Classification Classification // "" = tidak dinyatakan
  }
  type RouteOption func(*RouteConfig)
  func MaxBodyBytes(n int64) RouteOption
  // Classify: dipanggil berulang → MaxClassification. Nilai tidak valid disimpan apa adanya dan tidak
  // tergantikan oleh panggilan berikutnya, agar registry melaporkannya sebagai invalid_route_option.
  func Classify(c Classification) RouteOption
  func ResolveRouteOptions(opts ...RouteOption) RouteConfig // opsi nil diabaikan

  type Registrar interface {
      Handle(method, path, permission string, h http.Handler, opts ...RouteOption)
      Public(method, path string, h http.Handler, opts ...RouteOption)
      Command(c Command)
  }
  ```
- Produces (`moduletest`): `Route` memperoleh field `Options module.RouteConfig` (hasil `ResolveRouteOptions`, tanpa default).
- Produces (`registry`): `RouteInfo` memperoleh `MaxBodyBytes int64` (sudah diberi default: `0` → `module.DefaultMaxBodyBytes`) dan `Classification module.Classification`. `CodeInvalidRouteOption = "invalid_route_option"`.

- [ ] **Step 1: Tulis test yang gagal**:
  - `TestClassification_Order`: `ClassHighlyConfidential.AtLeast(ClassConfidential) == true`; `ClassInternal.AtLeast(ClassConfidential) == false`; `Classification("").AtLeast(ClassPublic) == false`; `MaxClassification(ClassInternal, ClassConfidential, ClassPublic) == ClassConfidential`; `MaxClassification() == ""`; `ParseClassification("secret")` → error; `ParseClassification("highly_confidential") == ClassHighlyConfidential`.
  - `TestResolveRouteOptions`: `ResolveRouteOptions() == RouteConfig{}`; `ResolveRouteOptions(MaxBodyBytes(4096), Classify(ClassInternal), nil, Classify(ClassConfidential), Classify(ClassPublic)) == RouteConfig{MaxBodyBytes: 4096, Classification: ClassConfidential}`.
  - `TestRecorder_RecordsRouteOptions` (`moduletest`): `reg.Handle("POST", "/notes", "x.notes.create", h, module.MaxBodyBytes(2048), module.Classify(module.ClassConfidential))` → `rt.Options == module.RouteConfig{MaxBodyBytes: 2048, Classification: "confidential"}`; `reg.Public("GET", "/status", h)` → `rt.Options == module.RouteConfig{}`. Test lama (tanpa opsi) tetap terkompilasi tanpa perubahan.
  - `route_options_test.go` (`registry`): `TestRegister_RouteOptionsDefaultsAndValues` → route tanpa opsi: `MaxBodyBytes == 1<<20`, `Classification == ""`; dengan `MaxBodyBytes(4<<10)` dan `Classify(ClassConfidential)` → nilai tersimpan. `TestRegister_InvalidRouteOptionsCollected` → satu modul dengan `MaxBodyBytes(-1)`, `MaxBodyBytes(10<<20 + 1)`, dan `Classify("secret")` → `*Error` berisi tepat tiga problem `invalid_route_option` yang menyebut key modul dan `METHOD path`; route itu tidak masuk `Routes()`.
- [ ] **Step 2: Jalankan** `go test ./sdk/... ./internal/platform/modules/registry/...` → FAIL (tipe/opsi belum ada).
- [ ] **Step 3: Implementasi** sesuai Interfaces. Recorder registry: `cfg := module.ResolveRouteOptions(opts...)`; validasi `cfg.MaxBodyBytes < 0 || cfg.MaxBodyBytes > module.MaxRouteBodyBytes`, lalu `cfg.Classification != "" && !cfg.Classification.Valid()` (keduanya dicatat sebelum `return`); `MaxBodyBytes == 0` → default. Komentar `registrar.go` diperbarui (opsi kini ada, rujuk Q26 butir 13).
- [ ] **Step 4: Jalankan** → PASS; `go build ./...` di root, `modules/example`, dan `modules/exampleaddon` sukses tanpa mengubah pemanggil.
- [ ] **Step 5: Commit** `feat(sdk): variadic per-route options with data classification and body limit`

### Task 2: Klasifikasi field group di schema manifest

**Files:**
- Modify: `contracts/module.schema.json`, `internal/platform/modules/testdata/manifests/valid_full.json`, `internal/platform/modules/manifest/validate_test.go`
- Create: `internal/platform/modules/testdata/manifests/bad_field_group_classification.json`

**Interfaces:**
- Produces (schema, aditif): `resources.items` = `{"type":"object","additionalProperties":true,"properties":{"field_groups":{"type":"array","items":{"type":"object","additionalProperties":true,"required":["key"],"properties":{"key":{"type":"string","pattern":"^[a-z][a-z0-9_]*$"},"fields":{"type":"array","items":{"type":"string"}},"classification":{"enum":["public","internal","confidential","highly_confidential"]}}}}}}`. `description` merujuk security-guidelines §2 dan PRD-001 FR-110.

- [ ] **Step 1: Tulis test yang gagal** — `TestValidate_FieldGroupClassification`: `valid_full.json` (ditambah `resources: [{"key":"x.note","field_groups":[{"key":"general","fields":["title"],"classification":"internal"},{"key":"pricing","fields":["cost"],"classification":"confidential"}]}]`) → lolos; subtest per kelas (4 nilai) → lolos; `bad_field_group_classification.json` (`"classification": "secret"`) → error yang menyebut `/resources/0/field_groups/0/classification`.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/modules/manifest/...` → FAIL (kasus `secret` lolos).
- [ ] **Step 3: Implementasi** perubahan schema.
- [ ] **Step 4: Jalankan** → PASS; `TestExampleManifestsValid` (M2a) tetap hijau.
- [ ] **Step 5: Commit** `feat(contracts): data classification enum for manifest field groups`

### Task 3: `sdk/httpx` — amplop respons dan error (Q25)

**Files:**
- Create: `sdk/httpx/doc.go`, `sdk/httpx/respond.go`, `sdk/httpx/respond_test.go`
- Modify: `sdk/go.mod`, `sdk/go.sum` (+ `go.opentelemetry.io/otel/trace v1.47.0`), `internal/platform/httpserver/errors.go`, `go.sum` modul (`task tidy`)

**Interfaces:**
- Produces:
  ```go
  const HeaderRequestID = "X-Request-Id"
  type Detail struct {
      Field   string `json:"field,omitempty"`
      Code    string `json:"code"`
      Message string `json:"message"`
  }
  // Error adalah error HTTP terstruktur; Err (opsional) hanya untuk log, tidak pernah dikirim.
  type Error struct { Status int; Code, Message string; Details []Detail; Err error }
  func (e *Error) Error() string
  func (e *Error) Unwrap() error
  func NewError(status int, code, message string, details ...Detail) *Error
  // WriteError menulis {"error":{code,message,details,requestId,traceId}}; requestId dari header respons
  // X-Request-Id, traceId dari span context. Pesan 5xx tidak boleh memuat detail internal.
  func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string, details any)
  // RespondError: *Error (errors.As) → WriteError sesuai isinya; error lain → slog.ErrorContext + 500 internal_error
  // "terjadi kesalahan pada server" tanpa detail.
  func RespondError(w http.ResponseWriter, r *http.Request, err error)
  func WriteData(w http.ResponseWriter, status int, v any)
  type Page struct { Page int `json:"page"`; PageSize int `json:"pageSize"`; Total int64 `json:"total"`; HasMore bool `json:"hasMore"` }
  func WritePage(w http.ResponseWriter, status int, data any, page Page)
  ```
- `httpserver`: `WriteError`, `WriteData`, `WritePage` memanggil fungsi `httpx`, dan `type PageInfo = httpx.Page`. Tipe lokal `errorBody`/`errorEnvelope` dihapus.

- [ ] **Step 1: Tulis test yang gagal** — `respond_test.go`:
  - `TestWriteError_Envelope`: header `X-Request-Id: req-1` sudah diset, ctx membawa span context dengan trace ID `4bf92f3577b34da6a3ce929d0e0e4736`; `WriteError(w, r, 422, "validation_failed", "Data tidak valid.", []Detail{{Field: "title", Code: "required", Message: "wajib"}})` → status `422`, `Content-Type: application/json; charset=utf-8`, body persis `{"error":{"code":"validation_failed","message":"Data tidak valid.","details":[{"field":"title","code":"required","message":"wajib"}],"requestId":"req-1","traceId":"4bf92f3577b34da6a3ce929d0e0e4736"}}` + `"\n"`. Tanpa header/span → kunci `requestId`/`traceId` tidak ada.
  - `TestRespondError_MapsHttpxError`: `fmt.Errorf("wrap: %w", NewError(409, "invalid_state", "x"))` → `409`, `code == "invalid_state"`.
  - `TestRespondError_UnknownErrorIs500Generic`: `errors.New("pq: secret detail")` → `500`, `code == "internal_error"`, body tidak memuat `secret`; record slog level `ERROR` memuat `secret detail` (handler buffer lewat `slog.SetDefault`, dipulihkan dengan `t.Cleanup`).
  - `TestWritePage_Envelope`: body persis `{"data":[1,2],"page":{"page":1,"pageSize":25,"total":2,"hasMore":false}}` + `"\n"`.
- [ ] **Step 2: Jalankan** `go test ./sdk/httpx/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces, delegasikan `httpserver`, lalu `task tidy`.
- [ ] **Step 4: Jalankan** `go test ./sdk/... ./internal/platform/...` → PASS (seluruh test amplop `httpserver`, `runtime`, dan `admin` tetap hijau tanpa perubahan); `task check:gowork-off` hijau.
- [ ] **Step 5: Commit** `feat(sdk): httpx response and error envelope helpers shared with the platform`

### Task 4: `httpx.DecodeJSON`

**Files:**
- Create: `sdk/httpx/limits.go`, `sdk/httpx/decode.go`, `sdk/httpx/decode_walk.go`, `sdk/httpx/decode_test.go`

**Interfaces:**
- Produces:
  ```go
  const (
      DefaultMaxBodyBytes = module.DefaultMaxBodyBytes
      DefaultMaxDepth     = 32
      DefaultMaxArrayLen  = 1000
  )
  func WithBodyLimit(ctx context.Context, n int64) context.Context // dipanggil platform (Task 5)
  func BodyLimit(ctx context.Context) (int64, bool)
  type DecodeOption func(*decodeConfig)
  func AllowEmptyBody() DecodeOption // body kosong → zero T; Content-Type boleh tidak ada
  func MaxDepth(n int) DecodeOption
  func MaxArrayLen(n int) DecodeOption
  // DecodeJSON membaca body (batas = BodyLimit(ctx) atau DefaultMaxBodyBytes) ke DTO T. T wajib struct;
  // selain itu panic "httpx: DecodeJSON hanya untuk DTO struct" (kesalahan pemrograman). Semua kegagalan
  // input adalah *Error sesuai tabel Global Constraints.
  func DecodeJSON[T any](w http.ResponseWriter, r *http.Request, opts ...DecodeOption) (T, error)
  ```
- Algoritme (`decode_walk.go`):
  1. Periksa `Content-Type` dengan `mime.ParseMediaType`.
  2. Baca `io.ReadAll(http.MaxBytesReader(w, r.Body, limit))`. `*http.MaxBytesError` → `body_too_large`.
  3. Lakukan satu lintasan token (`json.Decoder.Token`, `UseNumber`) yang dipandu `reflect.Type` T untuk menghitung kedalaman, panjang array, dan kunci ganda per objek. Pada objek struct, kunci harus **sama persis** dengan nama JSON field menurut aturan `encoding/json` (tag, `-` diabaikan, embedded struct diratakan). Kesesuaian jenis token skalar diperiksa terhadap kind field. Tipe yang mengimplementasi `json.Unmarshaler`/`encoding.TextUnmarshaler`, `json.RawMessage`, dan `any` tidak diperiksa isinya, tetapi kedalaman/array/kunci ganda tetap diperiksa. Field tak dikenal dikumpulkan (maksimal 20 detail). Kesalahan struktur (400) berhenti di temuan pertama.
  4. Sesudah nilai akar, `Token()` harus `io.EOF` (spasi diperbolehkan).
  5. `json.NewDecoder` + `DisallowUnknownFields` + `Decode(&v)` sebagai lapis kedua; `*json.UnmarshalTypeError` yang tersisa → `invalid_type` dengan `Field`.

- [ ] **Step 1: Tulis test yang gagal** — `decode_test.go` dengan DTO:
  ```go
  type lineDTO struct { ItemID string `json:"itemId"`; Quantity string `json:"quantity"` }
  type createDTO struct {
      Title string `json:"title"`; Tags []string `json:"tags"`; Lines []lineDTO `json:"lines"`
      Meta map[string]string `json:"meta"`; Any map[string]any `json:"any"`; Internal string `json:"-"`
  }
  ```
  `TestDecodeJSON_Table` (Review Focus 2), kolom `(name, contentType, body, ctxLimit, wantStatus, wantCode, wantDetail, wantField)`. Assertion: `errors.As(err, &he)`, `he.Status`, `he.Code`, `he.Details[0].Code`, `he.Details[0].Field`.
  - `ok`: `application/json`, `{"title":"a","lines":[{"itemId":"x","quantity":"1.5"}]}` → `err == nil`, `got.Title == "a"`, `got.Lines[0].Quantity == "1.5"`.
  - `ok_charset`: `application/json; charset=UTF-8` → nil. `ok_trailing_ws`: `{"title":"a"}` + `"\n\t "` → nil.
  - `ct_text`: `text/plain` → `400 bad_request unsupported_media_type`. `ct_missing` (body ada) → sama. `ct_latin1`: `application/json; charset=iso-8859-1` → sama.
  - `empty`: body `""` → `400 body_required`. `empty_ws`: `"  "` → `400 body_required`.
  - `malformed`: `{"title":` → `400 malformed_json`.
  - `not_object`: `[1]` dan `null` → `400 expected_object`.
  - `trailing_object`: `{"title":"a"} {"title":"b"}` → `400 trailing_data`. `trailing_garbage`: `{"title":"a"}x` → `400 trailing_data`.
  - `unknown`: `{"title":"a","admin":true}` → `422 validation_failed unknown_field` field `admin`.
  - `case_variant`: `{"Title":"a"}` → `422 unknown_field` field `Title`.
  - `dash_tag`: `{"Internal":"x"}` → `422 unknown_field` field `Internal`.
  - `nested_unknown`: `{"lines":[{"itemId":"x"},{"itemId":"y","price":"1"}]}` → `422 unknown_field` field `lines[1].price`.
  - `dup`: `{"title":"a","title":"b"}` → `400 duplicate_key` field `title`. `dup_map`: `{"meta":{"k":"1","k":"2"}}` → `400 duplicate_key` field `meta.k`.
  - `type`: `{"title":1}` → `422 invalid_type` field `title`. `type_nested`: `{"lines":[{"quantity":1.5}]}` → `422 invalid_type` field `lines[0].quantity`.
  - `depth_ok`: `{"any":` + `strings.Repeat("{\"a\":", 31)` + `1` + `strings.Repeat("}", 31)` + `}` (objek akar = 1, total kedalaman 32) → nil. `depth_bad`: 32 lapis (kedalaman 33) → `400 too_deep`.
  - `array_ok`: `tags` 1000 elemen → nil. `array_bad`: 1001 elemen → `400 array_too_long` field `tags`.
  - `too_large_ctx`: `ctxLimit = 64`, body JSON valid 65 byte → `400 body_too_large`. `too_large_default`: tanpa ctx, `{"title":"` + 1 MiB `a` + `"}` → `400 body_too_large`. `limit_exact`: body valid tepat `1<<20` byte (dipadatkan spasi) → nil.
  - `TestDecodeJSON_AllowEmptyBody`: body kosong tanpa `Content-Type` + `AllowEmptyBody()` → `got == createDTO{}`, nil; body `{}` tanpa `Content-Type` + `AllowEmptyBody()` → `400 unsupported_media_type`.
  - `TestDecodeJSON_NonStructPanics`: `DecodeJSON[map[string]any]` → panic dengan pesan berisi `DTO struct`.
- [ ] **Step 2: Jalankan** `go test ./sdk/httpx/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai algoritme di atas.
- [ ] **Step 4: Jalankan** → PASS; `go vet ./sdk/...` bersih.
- [ ] **Step 5: Commit** `feat(sdk): strict httpx.DecodeJSON with size, depth, array, duplicate and unknown-field limits`

### Task 5: Middleware batas request di `runtime.Mount` (body limit + `no-store`)

**Files:**
- Create: `internal/platform/modules/runtime/limits.go`, `internal/platform/modules/runtime/limits_test.go`
- Modify: `internal/platform/modules/runtime/mount.go`

**Interfaces:**
- Consumes: `registry.RouteInfo.MaxBodyBytes`, `.Classification`; `httpx.WithBodyLimit`.
- Produces:
  ```go
  // requestLimitsMW: r.Body = http.MaxBytesReader(w, r.Body, rt.MaxBodyBytes); ctx = httpx.WithBodyLimit(ctx, rt.MaxBodyBytes);
  // rt.Classification.AtLeast(module.ClassConfidential) → w.Header().Set("Cache-Control", "no-store") sebelum next.
  func requestLimitsMW(rt registry.RouteInfo, next http.Handler) http.Handler
  ```
  Urutan di `Mount`: `h = requestLimitsMW(rt, rt.Handler)` → `permissionMW` (kecuali Public) → `guardMW`. Komentar rantai di `Mount` diperbarui.

- [ ] **Step 1: Tulis test yang gagal** — `limits_test.go` (router + snapshot `enabled`, pola yang sama dengan `mount_test.go`):
  - `TestMount_NoStoreByClassification` (Review Focus 4): tabel `""`, `public`, `internal` → `Cache-Control == ""`; `confidential`, `highly_confidential` → `"no-store"`. Berlaku untuk `Handle` dan `Public`.
  - `TestMount_NoStoreAlsoOnHandlerError`: route `confidential` yang handler-nya menjawab `httpx.WriteError(…, 422, …)` → `Cache-Control == "no-store"`.
  - `TestMount_BodyLimitEnforcedWithoutDecode` (Review Focus 2): route `MaxBodyBytes(16)`, handler `io.ReadAll(r.Body)` dengan body 17 byte → handler menerima error `*http.MaxBytesError`.
  - `TestMount_DecodeUsesRouteLimit`: route `MaxBodyBytes(2 << 20)` + `DecodeJSON` → body valid 1.5 MiB diterima (`200`); route tanpa opsi → body `1<<20 + 1` → `400` dengan `details[0].code == "body_too_large"`.
  - `TestMount_GuardBeforeLimits`: modul `disabled` dengan route `confidential` → `403 module_disabled`, handler tidak terpanggil.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/modules/runtime/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces.
- [ ] **Step 4: Jalankan** → PASS; `TestMount_*` lama tetap hijau.
- [ ] **Step 5: Commit** `feat(modules): enforce per-route body limit and classification-based no-store`

### Task 6: Migrasi konsumen — API admin dan modul referensi

**Files:**
- Delete: `internal/platform/modules/admin/decode.go`
- Modify: `internal/platform/modules/admin/{api.go, api_test.go}`, `contracts/openapi/platform.yaml`, `modules/example/internal/presentation/http/{handlers.go, handlers_test.go}`, `modules/exampleaddon/internal/presentation/http/{summary.go, summary_test.go}`, `modules/example/go.{mod,sum}`, `modules/exampleaddon/go.{mod,sum}` (lewat `task tidy`)

**Interfaces:**
- Admin: `r.Handle(http.MethodPost, "/modules/{key}/{operation}", permManage, a.wrap(a.action), module.MaxBodyBytes(4 << 10))`. `action` memakai `body, err := httpx.DecodeJSON[actionBody](w, r, httpx.AllowEmptyBody())`, dan error dari sana → `httpx.RespondError(w, r, err)`. Konstanta `maxBodyBytes` dihapus.
- `platform.yaml`: deskripsi respons `400` aksi menyebut `details[].code` `unsupported_media_type`, `body_too_large`, `malformed_json`, `trailing_data`; deskripsi `422` menambah `unknown_field`, `invalid_type`.
- `example`: `Ping` → `httpx.WriteData(w, 200, map[string]bool{"pong": true})`; `ListNotes` → `httpx.WritePage(…, httpx.Page{…})`; parameter tidak valid → `httpx.WriteError(w, r, 400, "bad_request", msg, []httpx.Detail{{Field: "page"|"pageSize", Code: "invalid", Message: msg}})`; 5xx → `httpx.RespondError(w, r, err)`. Tipe lokal `pageDTO` serta fungsi `writeJSON`/`writeError` dihapus.
- `exampleaddon`: `Summary` → `httpx.WriteData` / `httpx.RespondError`. `writeJSON` dihapus.

- [ ] **Step 1: Tulis test yang gagal**:
  - `api_test.go` `TestAction_BodyStrict` (Review Focus 2): `{"withDeps":true,"x":1}` → `422 validation_failed`, `details[0] == {field:"x", code:"unknown_field"}`; body 5 KiB → `400`, `details[0].code == "body_too_large"`; `Content-Type: text/plain` dengan body `{}` → `400 unsupported_media_type`; `{"enable":true}{"enable":false}` → `400 trailing_data`; body kosong tanpa `Content-Type` → diteruskan ke backend (status sama seperti test M2b yang ada).
  - `handlers_test.go`: `TestListNotes_BadPageEnvelope` → header `X-Request-Id: req-1` diset sebelum handler → `error.requestId == "req-1"`, `details[0].field == "page"`; `TestPing_BodyUnchanged` → body persis `{"data":{"pong":true}}` + `"\n"`; `TestListNotes_RepoErrorIs500Generic` → `500 internal_error` tanpa teks error repo.
  - `summary_test.go`: jalur 500 → `error.code == "internal_error"` dan `error.requestId == "req-1"`.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/modules/admin/... ./modules/example/... ./modules/exampleaddon/...` → FAIL (`details` lama `bad_request`/`invalid`; `requestId`/`details` example belum sesuai).
- [ ] **Step 3: Implementasi** sesuai Interfaces, lalu `task tidy`.
- [ ] **Step 4: Jalankan** → PASS; `git grep -n -e "func writeError" -e "func writeJSON" -e "decodeAction" -- modules internal` kosong; `task check:gowork-off` hijau.
- [ ] **Step 5: Commit** `refactor: migrate admin API and reference modules to sdk/httpx`

### Task 7: `sdk/egress` — HTTP client keluar anti-SSRF

**Files:**
- Create: `sdk/egress/{doc.go, egress.go, addr.go, dial.go, body.go, export_test.go, addr_test.go, egress_test.go}`

**Interfaces:**
- Produces:
  ```go
  type Resolver interface {
      LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error) // *net.Resolver memenuhinya
  }
  type Config struct {
      AllowHosts          []string      // kosong = semua host publik; isi = exact host atau "*.example.com" (subdomain saja)
      AllowHTTPHosts      []string      // host (exact) yang boleh memakai skema http
      AllowedContentTypes []string      // media type tanpa parameter; kosong = semua
      ConnectTimeout      time.Duration // 0 → 5s
      Timeout             time.Duration // 0 → 30s (total, termasuk redirect dan membaca header)
      MaxResponseBytes    int64         // 0 → 10<<20
      MaxRedirects        int           // 0 → 3; > 3 atau < 0 ditolak New
      RootCAs             *x509.CertPool // nil = CA sistem
      Resolver            Resolver      // nil = net.DefaultResolver
  }
  const (
      DefaultConnectTimeout         = 5 * time.Second
      DefaultTimeout                = 30 * time.Second
      DefaultMaxResponseBytes int64 = 10 << 20
      DefaultMaxRedirects           = 3
  )
  var (
      ErrSchemeNotAllowed, ErrHostNotAllowed, ErrBlockedAddress, ErrTooManyRedirects,
      ErrResponseTooLarge, ErrUnexpectedContentType error // errors.New dengan pesan Bahasa Indonesia
  )
  type BlockedError struct { Host string; Addr netip.Addr; Reason string } // Reason: loopback|private|link_local|cgnat|unspecified|multicast|reserved|unique_local|nat64|6to4|numeric_host|...
  func (e *BlockedError) Error() string
  func (e *BlockedError) Is(target error) bool // target == ErrBlockedAddress
  type Client struct{ /* *http.Client privat; tidak diekspos */ }
  func New(cfg Config) (*Client, error)
  func (c *Client) Do(req *http.Request) (*http.Response, error)
  func (c *Client) Get(ctx context.Context, rawURL string) (*http.Response, error)
  // addr.go (privat)
  func blockedReason(a netip.Addr) (reason string, blocked bool)
  ```
- Perilaku: `Do` memvalidasi URL lebih dulu (skema, allowlist host, host numerik non-kanonik, IP literal terblokir). `Transport.Proxy = nil`, `DialContext` = resolve → semua IP harus lolos `blockedReason` → dial berurutan ke `netip.AddrPortFrom(ip, port)` dengan `ConnectTimeout`. `TLSClientConfig{MinVersion: tls.VersionTLS12, RootCAs}`. `CheckRedirect` menghitung hop (`> MaxRedirects` → `ErrTooManyRedirects`) dan memvalidasi URL hop seperti `Do`. `Content-Length > MaxResponseBytes` → `ErrResponseTooLarge` (body ditutup). `Body` dibungkus pembaca yang mengembalikan `ErrResponseTooLarge` saat melewati batas, tanpa memotong diam-diam. `AllowedContentTypes` tidak cocok → body ditutup + `ErrUnexpectedContentType`.
- `export_test.go` (hanya test): `setDialHook(func(ctx context.Context, network, addr string) (net.Conn, error))`, `setAllowAddrForTest(func(netip.AddrPort) bool)`, `configOf(*Client) Config`, `proxyOf(*Client) func(*http.Request) (*url.URL, error)`.

- [ ] **Step 1: Tulis test yang gagal**:
  - `TestBlockedReason_Table` (Review Focus 1): terblokir → `127.0.0.1`, `127.255.255.254`, `::1`, `10.0.0.1`, `172.16.0.1`, `172.31.255.255`, `192.168.1.1`, `169.254.169.254`, `fe80::1`, `fc00::1`, `fd12::1`, `100.64.0.1`, `0.0.0.0`, `224.0.0.1`, `ff02::1`, `::ffff:127.0.0.1`, `::ffff:10.0.0.1`, `::`, `255.255.255.255`, `240.0.0.1`, `198.18.0.1`, `192.0.0.1`, `64:ff9b::a00:1`, `2002:a00:1::`; diizinkan → `93.184.216.34`, `1.1.1.1`, `2606:4700:4700::1111`, `172.32.0.1`, `100.128.0.1`, `64:ff9b::5db8:d822`.
  - `TestClient_RejectsFR24Table` (AC FR-24.2): `Config{AllowHTTPHosts: {"127.0.0.1","169.254.169.254","::1","10.0.0.1","internal.example.test","mixed.example.test","2130706433","0x7f000001","0177.0.0.1","127.1"}, Resolver: stub{"internal.example.test": {10.1.2.3}, "mixed.example.test": {93.184.216.34, 10.0.0.1}}}`; dial hook menggagalkan test bila terpanggil. Untuk `http://127.0.0.1/`, `http://169.254.169.254/latest/meta-data/`, `http://[::1]/`, `http://10.0.0.1/`, `http://internal.example.test/`, `http://mixed.example.test/`, `http://[::ffff:127.0.0.1]/`, `http://2130706433/`, `http://0x7f000001/`, `http://0177.0.0.1/`, `http://127.1/` → `errors.Is(err, ErrBlockedAddress)`; jumlah panggilan dial hook `0`. (AllowHTTPHosts memastikan penolakan berasal dari alamat, bukan skema.)
  - `TestClient_DialsValidatedIP` (DNS rebinding): resolver mengembalikan `93.184.216.34` pada panggilan pertama dan `127.0.0.1` sesudahnya; dial hook merekam `addr` lalu mengembalikan `errStop`. `Get(ctx, "https://rebind.example.test/")` → `errors.Is(err, errStop)`, rekaman `== []string{"93.184.216.34:443"}`, jumlah panggilan resolver `== 1`.
  - `TestClient_SchemeAndHostAllowlist`: `ftp://a.example.test/` → `ErrSchemeNotAllowed`; `http://public.example.test/` (tidak di AllowHTTPHosts) → `ErrSchemeNotAllowed`. Dengan `AllowHosts {"api.example.test","*.svc.example.test"}`: `https://evil.example.test/` → `ErrHostNotAllowed`; `https://svc.example.test/` → `ErrHostNotAllowed`; `https://a.svc.example.test/` → mencapai dial hook.
  - `TestClient_Redirects` (`httptest.NewTLSServer`, `RootCAs` dari sertifikat server, `setAllowAddrForTest` hanya untuk alamat server): `/hop3` (3 redirect) → `200`; `/hop4` → `ErrTooManyRedirects`; `/to-private` (`Location: https://10.0.0.1/x`) → `ErrBlockedAddress`; `/to-metadata` (`Location: http://169.254.169.254/latest/meta-data/`, host itu ada di AllowHTTPHosts) → `ErrBlockedAddress`; `/to-http` (`Location: http://example.test/`) → `ErrSchemeNotAllowed`.
  - `TestClient_ResponseLimits`: `/big` dengan `Content-Length` `10<<20 + 1` → `Get` error `ErrResponseTooLarge`; `/big-chunked` tanpa `Content-Length` → `io.ReadAll(resp.Body)` error `ErrResponseTooLarge`; tepat `10<<20` byte → sukses; `AllowedContentTypes {"application/json"}` dengan server `text/html` → `ErrUnexpectedContentType`.
  - `TestClient_Timeout`: `Config{Timeout: 200 * time.Millisecond}`, server tidur 1s → error timeout (`net.Error.Timeout()` atau `context.DeadlineExceeded`) dalam waktu kurang dari 1s.
  - `TestNew_DefaultsAndValidation`: `configOf(New(Config{}))` → `ConnectTimeout == 5s`, `Timeout == 30s`, `MaxResponseBytes == 10<<20`, `MaxRedirects == 3`; `MaxRedirects: 4` → error; `MaxResponseBytes: -1` → error; `AllowHosts: {"*."}` → error.
  - `TestClient_IgnoresProxyEnv`: `t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")` → `proxyOf(c) == nil`.
- [ ] **Step 2: Jalankan** `go test ./sdk/egress/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces. Host numerik non-kanonik dikenali bila host bukan IP literal valid (`netip.ParseAddr` gagal) tetapi seluruhnya terdiri dari digit/titik/`x`/heksadesimal dengan pola `^(0x[0-9a-f]+|[0-9]+)(\.(0x[0-9a-f]+|[0-9]+)){0,3}$` (case-insensitive) → `BlockedError{Reason: "numeric_host"}`.
- [ ] **Step 4: Jalankan** → PASS; `go vet ./sdk/egress/...` dan `gosec` (lewat `task lint`) bersih.
- [ ] **Step 5: Commit** `feat(sdk): egress client with SSRF protections (post-resolve IP checks, validated dial, redirect and size limits)`

### Task 8: Seam `authz.MaskFields`

**Files:**
- Create: `sdk/authz/fields.go`, `sdk/authz/fields_test.go`, `sdk/authz/authztest/authztest.go`

**Interfaces:**
- Produces:
  ```go
  // FieldMasker diimplementasi platform (stub sebelum PRD-001, field-level access di PRD-001 FR-112).
  type FieldMasker interface {
      MaskFields(ctx context.Context, resource string, fields map[string]any) map[string]any
  }
  func WithFieldMasker(ctx context.Context, m FieldMasker) context.Context // dipanggil platform
  func HasFieldMasker(ctx context.Context) bool
  // MaskFields (signature PRD-001 FR-115): dto berupa map[string]any (disalin) atau struct/pointer struct
  // (dikonversi lewat JSON, UseNumber). Tanpa masker di ctx → map[string]any{} (gagal tertutup).
  // dto yang bukan objek → map[string]any{}.
  func MaskFields(ctx context.Context, resource string, dto any) any
  // authztest
  var AllowAllFields authz.FieldMasker // mengembalikan salinan apa adanya; hanya untuk test
  ```

- [ ] **Step 1: Tulis test yang gagal** — `fields_test.go`:
  - `TestMaskFields_NoMaskerFailsClosed`: `MaskFields(ctx, "x.note", map[string]any{"title": "a"})` → `map[string]any{}` (panjang 0); `HasFieldMasker(ctx) == false`.
  - `TestMaskFields_UsesMaskerOnCopy`: masker membuang `"cost"` dan merekam `resource` → hasil `map{"title":"a"}`, input asli tetap memuat `cost`, resource terekam `"x.note"`.
  - `TestMaskFields_StructDTO`: `struct{Title string `json:"title"`; Cost string `json:"cost"`}{"a","9"}` dengan masker yang membuang `cost` → `map[string]any{"title": "a"}`.
  - `TestMaskFields_NonObject`: `[]int{1}` → `map[string]any{}`.
- [ ] **Step 2: Jalankan** `go test ./sdk/authz/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces; `doc.go` diperbarui.
- [ ] **Step 4: Jalankan** → PASS.
- [ ] **Step 5: Commit** `feat(sdk): authz.MaskFields seam backed by a platform-installed field masker`

### Task 9: `sdk/export` — CSV/XLSX aman

**Files:**
- Create: `sdk/export/{doc.go, export.go, cell.go, csv.go, xlsx.go, serve.go, cell_test.go, export_test.go, xlsx_test.go, serve_test.go}`

**Interfaces:**
- Consumes: `module.Classification`, `authz.MaskFields`, `authz.HasFieldMasker`.
- Produces:
  ```go
  type Format string
  const ( CSV Format = "csv"; XLSX Format = "xlsx" )
  type Column struct { Key, Header string; Classification module.Classification } // Classification wajib diisi
  type Spec struct {
      Resource string // resource authz, mis. "example.note"
      Format   Format
      Filename string // nama dasar tanpa ekstensi; ekstensi dari Format
      Columns  []Column
      MaxRows  int    // 0 → DefaultMaxRows
  }
  type Row = map[string]any
  const DefaultMaxRows = 10000
  var (
      ErrInvalidSpec, ErrForbiddenClassification, ErrNoAuditor, ErrNoFieldMasker,
      ErrTooManyRows, ErrNonScalarValue error
  )
  type Event struct {
      Resource, Filename string; Format Format; Columns []string; Rows int
      Classification module.Classification // tertinggi di antara kolom
      Err error
  }
  type Auditor interface { RecordExport(ctx context.Context, e Event) error }
  func WithAuditor(ctx context.Context, a Auditor) context.Context // dipanggil platform
  func HasAuditor(ctx context.Context) bool
  func SanitizeCell(s string) string
  func Validate(ctx context.Context, spec Spec) error // spec, klasifikasi, masker, auditor; dipanggil Write/Serve sebelum byte pertama
  // Write menulis header + baris (setiap baris lewat authz.MaskFields(ctx, spec.Resource, row)); mengembalikan jumlah baris.
  // Auditor (bila ada) dipanggil sekali setelah selesai, baik sukses maupun gagal.
  func Write(ctx context.Context, w io.Writer, spec Spec, rows iter.Seq2[Row, error]) (int, error)
  // Serve: Validate → header (Content-Type, Content-Disposition, Cache-Control no-store bila ≥ confidential) → Write.
  // Error sebelum byte pertama dikembalikan (handler → httpx.RespondError); error di tengah stream → log + panic(http.ErrAbortHandler).
  func Serve(w http.ResponseWriter, r *http.Request, spec Spec, rows iter.Seq2[Row, error]) error
  ```
- Aturan `Validate`: `Resource` tidak kosong; `Format` dikenal; `Filename` cocok `^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`; minimal satu kolom; `Key` unik dan tidak kosong; setiap `Classification` valid (selain itu `ErrInvalidSpec`); kolom `highly_confidential` → `ErrForbiddenClassification`; `!authz.HasFieldMasker(ctx)` → `ErrNoFieldMasker`; kolom ≥ `confidential` tanpa auditor → `ErrNoAuditor`.
- Format sel (`cell.go`): `string`; `bool`; integer/unsigned (`strconv`); float (`'f'`, `-1`); `json.Number`; `time.Time` → UTC RFC 3339; `fmt.Stringer` (mis. tipe desimal) → `String()`; `nil` → `""`; map/slice/array/struct lain → `ErrNonScalarValue` (menyebut key kolom, tidak menyebut nilai). Header dan sel CSV melewati `SanitizeCell`. Sel XLSX di-escape XML, karakter XML tidak valid dibuang, dan tidak diberi awalan.

- [ ] **Step 1: Tulis test yang gagal** (ctx test = `authz.WithFieldMasker(ctx, authztest.AllowAllFields)` + auditor perekam kecuali disebut lain):
  - `TestSanitizeCell_Table` (Review Focus 3, AC FR-24.2): `"=1+1"→"'=1+1"`, `"+1"→"'+1"`, `"-1+2"→"'-1+2"`, `"@SUM(A1)"→"'@SUM(A1)"`, `"\t=1"→"'\t=1"`, `"\r=1"→"'\r=1"`, `"-1250.00"→"-1250.00"`, `"-5"→"-5"`, `"-5."→"'-5."`, `"1+1"→"1+1"`, `""→""`, `"Kopi"→"Kopi"`.
  - `TestWrite_CSV_Golden`: spec `{Resource:"x.note", Format:CSV, Filename:"notes", Columns: {{"id","ID",internal},{"title","Judul",internal},{"amount","Jumlah",internal}}}`, baris `{id:"1", title:"=1+1", amount:"-10.50"}`, `{id:"2", title:"Kopi, \"susu\"", amount:nil}` → output persis `"﻿ID,Judul,Jumlah\r\n1,'=1+1,-10.50\r\n2,\"Kopi, \"\"susu\"\"\",\r\n"`, `n == 2`, satu event auditor `{Resource:"x.note", Format:"csv", Filename:"notes", Columns:["id","title","amount"], Rows:2, Classification:"internal", Err:nil}`.
  - `TestWrite_MaskedColumnsEmpty`: kolom `{"id","ID",internal}`, `{"cost","Biaya",confidential}`, masker membuang `cost`, baris `{id:"1", cost:"9"}` → output persis `"﻿ID,Biaya\r\n1,\r\n"` (tidak memuat `9`).
  - `TestWrite_NestedValueRejected`: `{title: map[string]any{"secret":"x"}}` → `errors.Is(err, ErrNonScalarValue)`, output tidak memuat `secret`, event auditor `Err != nil`.
  - `TestValidate_HighlyConfidentialColumnRejected`: kolom `highly_confidential` → `ErrForbiddenClassification`, buffer kosong, auditor tidak dipanggil.
  - `TestValidate_ConfidentialRequiresAuditor`: tanpa auditor → `ErrNoAuditor`, buffer kosong; dengan auditor → event `Classification == "confidential"`.
  - `TestValidate_NoMaskerFailsClosed`: ctx tanpa masker → `ErrNoFieldMasker`.
  - `TestValidate_InvalidSpec`: tabel `Filename` `"a\r\nb"`, `"../x"`, `""`, `".x"`; kolom duplikat; `Classification` `""`; `Format` `"pdf"` → `ErrInvalidSpec`.
  - `TestWrite_MaxRows`: `MaxRows: 2`, 3 baris → `ErrTooManyRows`, event `Rows == 2`, `Err` = `ErrTooManyRows`.
  - `TestWrite_XLSX`: `zip.NewReader` memuat tepat `[Content_Types].xml`, `_rels/.rels`, `xl/workbook.xml`, `xl/_rels/workbook.xml.rels`, `xl/worksheets/sheet1.xml`; `sheet1.xml` memuat `t="inlineStr"` dan `<t xml:space="preserve">=1+1</t>`, dan tidak memuat `<f>`; nilai `"a<b&c"` ter-escape `a&lt;b&amp;c`.
  - `TestServe_Headers`: kolom `internal` → `Content-Type: text/csv; charset=utf-8`, `Content-Disposition: attachment; filename="notes.csv"; filename*=UTF-8''notes.csv`, tanpa `Cache-Control`; kolom `confidential` → `Cache-Control: no-store`; XLSX → `filename="notes.xlsx"` dan content type spreadsheet; `Filename: "a\r\nb"` → `ErrInvalidSpec`, `rec.Body.Len() == 0`, `Content-Disposition` kosong.
  - `TestServe_MidStreamErrorAborts`: iterator menghasilkan satu baris lalu error → `Serve` panic dengan nilai `http.ErrAbortHandler` (ditangkap `recover`), event auditor `Err != nil`.
- [ ] **Step 2: Jalankan** `go test ./sdk/export/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces (`encoding/csv` dengan `UseCRLF = true`).
- [ ] **Step 4: Jalankan** → PASS.
- [ ] **Step 5: Commit** `feat(sdk): export helper for CSV/XLSX with formula-injection guard, field masking, and audit hook`

### Task 10: Platform memasang masker dan auditor stub

**Files:**
- Create: `internal/platform/authorization/fields.go`, `internal/platform/authorization/fields_test.go`, `internal/platform/audit/export.go`, `internal/platform/audit/export_test.go`
- Modify: `internal/platform/modules/runtime/{limits.go, limits_test.go, mount.go, modctx.go, modctx_test.go}`, `internal/app/{assemble.go, lifecycle.go, command.go}` (pemanggil `Mount`/`ModuleContext`)

**Interfaces:**
- Produces:
  ```go
  // authorization
  // StubFieldMasker: mode allow → salinan apa adanya + log INFO sekali per resource ("masking field stub: allow");
  // mode deny atau tidak dikenal → map kosong + log WARN (gagal tertutup).
  func NewStubFieldMasker(mode StubMode, log *slog.Logger) authz.FieldMasker
  // audit
  // ExportLog mencatat INFO "ekspor data" dengan resource, format, filename, rows, columns, classification,
  // error (bila ada), erp.module.key; tidak pernah mencatat nilai sel.
  func NewExportLog(log *slog.Logger) export.Auditor
  // runtime
  type Services struct { Masker authz.FieldMasker; Auditor export.Auditor }
  func WithServices(s Services) MountOption
  // requestLimitsMW juga memasang authz.WithFieldMasker dan export.WithAuditor bila tidak nil.
  func ModuleContext(ctx context.Context, t tenancy.TenantID, key string, conn database.TenantConnector,
      base *slog.Logger, systemReason string, svc Services) (context.Context, module.Env)
  ```
- `assemble` membangun `svc := runtime.Services{Masker: authorization.NewStubFieldMasker(authorization.StubMode(cfg.AuthzStub), log), Auditor: audit.NewExportLog(log)}`, lalu meneruskannya ke `Mount` (`runtime.WithServices(svc)`) dan ke setiap pemanggil `ModuleContext`.

- [ ] **Step 1: Tulis test yang gagal**:
  - `fields_test.go`: `allow` → `MaskFields(ctx, "x.note", {"a":1}) == {"a":1}` dan dua panggilan hanya menghasilkan satu log INFO; `deny` → `{}` + WARN; mode `"bogus"` → `{}`.
  - `audit/export_test.go`: `RecordExport(ctx, Event{Resource:"x.note", Format:"csv", Filename:"notes", Columns:["id"], Rows:3, Classification:"confidential"})` dengan ctx `observability.WithModule(ctx, "acme.x")` → satu record JSON dengan `msg == "ekspor data"`, `rows == 3`, `classification == "confidential"`, `erp.module.key == "acme.x"`.
  - `limits_test.go` `TestMount_InstallsExportServices`: dengan `WithServices`, handler melihat `authz.HasFieldMasker(r.Context()) && export.HasAuditor(r.Context())`; tanpa opsi keduanya `false`.
  - `modctx_test.go` `TestModuleContext_InstallsServices`: ctx hasil `ModuleContext(…, Services{…})` membawa keduanya.
  - `TestExport_EndToEndThroughMount`: route `confidential` yang memanggil `export.Serve` dengan masker stub `allow` → `200`, `Cache-Control: no-store`, body CSV, satu record log `ekspor data`.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/... ./internal/app/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces; perbarui semua pemanggil `ModuleContext`.
- [ ] **Step 4: Jalankan** → PASS; `task test:integration` (karena `internal/app` tersentuh) hijau.
- [ ] **Step 5: Commit** `feat(platform): install stub field masker and export audit log in module request and command contexts`

### Task 11: `statement_timeout` pool `serve` (Q26 butir 7)

**Files:**
- Modify: `internal/platform/config/{config.go, config_test.go}`, `internal/platform/database/{open.go, open_test.go}`, `internal/app/app.go`
- Create: `internal/platform/database/statement_timeout_integration_test.go`, `internal/app/serve_options_test.go`

**Interfaces:**
- Produces:
  ```go
  // config
  DBStatementTimeout time.Duration // ERP_DB_STATEMENT_TIMEOUT_MS, default 30000, valid 1000..600000
  // database
  type OpenOptions struct { StatementTimeout time.Duration } // 0 = tanpa batas
  // OpenWith: bila StatementTimeout > 0 → pgx.ParseConfig(DSN) (error dibungkus connError), RuntimeParams["statement_timeout"]
  // = milidetik, stdlib.OpenDB, gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), …) dengan konfigurasi GORM yang sama.
  func OpenWith(ctx context.Context, cfg config.Config, tp trace.TracerProvider, log *slog.Logger, o OpenOptions) (*gorm.DB, error)
  func Open(ctx context.Context, cfg config.Config, tp trace.TracerProvider, log *slog.Logger) (*gorm.DB, error) // = OpenWith(…, OpenOptions{})
  // internal/app
  func serveOpenOptions(cfg config.Config) database.OpenOptions // {StatementTimeout: cfg.DBStatementTimeout}
  ```
- `app.go` (serve) memakai `database.OpenWith(ctx, cfg, prov.Tracer, log, serveOpenOptions(cfg))`. `migrate.go`, `lifecycle.go`, dan test tetap memakai `Open`.

- [ ] **Step 1: Tulis test yang gagal**:
  - `config_test.go`: default → `DBStatementTimeout == 30*time.Second`; `"600000"` → `10m`; `"999"`, `"600001"`, `"abc"`, `"-1"` → `ValidationError` dengan `Field == "ERP_DB_STATEMENT_TIMEOUT_MS"`.
  - `open_test.go` `TestOpenWith_DSNErrorRedacted`: `DatabaseURL "postgres://u:supersecret@[bad"` + `StatementTimeout 1s` → error tidak memuat `supersecret`.
  - `serve_options_test.go`: `serveOpenOptions(config.Config{DBStatementTimeout: 7 * time.Second}).StatementTimeout == 7*time.Second`.
  - `statement_timeout_integration_test.go` (`//go:build integration`, `pgtest.Start`), Review Focus 5:
    - `TestStatementTimeout_AllPooledConnections`: `OpenWith(…, OpenOptions{StatementTimeout: 300 * time.Millisecond})`, `SetMaxOpenConns(4)`. Ambil 4 `*sql.Conn` bersamaan, dan masing-masing `SHOW statement_timeout` → `"300ms"`. Di conn 1 jalankan `BEGIN; SET LOCAL statement_timeout = '5s'; COMMIT`. Di conn 2 jalankan `SELECT pg_sleep(1)` → `*pgconn.PgError` dengan `Code == "57014"`. Lepas semua koneksi, ambil 4 lagi → semuanya `"300ms"`.
    - `TestOpen_NoStatementTimeoutByDefault`: `Open` → `SHOW statement_timeout == "0"`.
- [ ] **Step 2: Jalankan** `go test ./internal/platform/config/... ./internal/platform/database/... ./internal/app/...` dan `go test -tags integration ./internal/platform/database/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces.
- [ ] **Step 4: Jalankan** → PASS; `task test:integration` hijau (`serve` dan `migrate` integration tidak berubah).
- [ ] **Step 5: Commit** `feat(database): per-statement timeout for the serve connection pool via runtime parameter`

### Task 12: Arch test `EGRESS` dan `DECODE`

**Files:**
- Create: `internal/archcheck/typed.go`, `internal/archcheck/typed_test.go`, `internal/archcheck/testdata/secmod/` (`go.mod`: `module erpora/modules/secfixture` + `replace erpora/sdk => ./stubsdk`; `stubsdk/go.mod` `module erpora/sdk`; `stubsdk/httpx/httpx.go` dengan `func DecodeJSON[T any](w http.ResponseWriter, r *http.Request, opts ...any) (T, error)`; `stubsdk/egress/egress.go`; `egress_bad.go`, `egress_ok.go`, `internal/infrastructure/{model.go, jsonb.go, handler_bad.go}`, `internal/presentation/http/{decode_bad.go, decode_ok.go}`)
- Modify: `tests/architecture/arch_test.go`, `go.mod` (`golang.org/x/tools v0.52.0` menjadi require langsung)

**Interfaces:**
- Produces:
  ```go
  type TypedPackage struct {
      ImportPath, Module string
      Fset  *token.FileSet
      Files []*ast.File
      Info  *types.Info
      Types *types.Package
  }
  // LoadTyped: packages.Load(NeedName|NeedFiles|NeedSyntax|NeedTypes|NeedTypesInfo|NeedModule, Tests:false, Dir: dir, Env: os.Environ()+env) pola "./...".
  func LoadTyped(dir string, env ...string) ([]TypedPackage, error)
  // LoadTypedRepo: LoadTyped untuk setiap direktori modules/*/go.mod di bawah root (mode workspace).
  func LoadTypedRepo(root string) ([]TypedPackage, error)
  // CheckSecurity: aturan EGRESS dan DECODE untuk paket dengan Module berprefiks "erpora/modules/".
  // Detail: "<file>:<line>: <uraian>".
  func CheckSecurity(pkgs []TypedPackage) []Violation
  ```
- `EGRESS`: pemakaian (`Info.Uses`) objek `net/http` bernama `DefaultClient`, `DefaultTransport`, `Get`, `Post`, `Head`, `PostForm`, serta composite literal atau `new(T)` dengan `T` (setelah deref pointer) `net/http.Client` atau `net/http.Transport`.
- `DECODE`: panggilan ke `encoding/json.Unmarshal` (target `args[1]`), method `(*encoding/json.Decoder).Decode` (target `args[0]`), dan instansiasi `erpora/sdk/httpx.DecodeJSON` (target = `Info.Instances[ident].TypeArgs.At(0)`). Tipe target ditelusuri (pointer, slice, array, map key/elem, field struct termasuk embedded, underlying named; dengan visited set). Pelanggaran terjadi bila ada tipe bernama dari paket bersegmen `internal/infrastructure`, kecuali `json.Unmarshal`/`Decode` yang call site-nya berada di paket infrastructure. `httpx.DecodeJSON` di paket infrastructure selalu dilanggar.

- [ ] **Step 1: Tulis test yang gagal** — `typed_test.go` memuat fixture dengan `LoadTyped("testdata/secmod", "GOWORK=off", "GOFLAGS=-mod=mod", "GOPROXY=off")`:
  - `TestEgress_FixtureViolations` (AC FR-24.2 "arch test menolak fixture modul yang memakai `http.Get`"): `egress_bad.go` memuat `http.Get`, `http.Post`, `http.Head`, `http.PostForm`, `http.DefaultClient.Do`, `&http.Client{}`, `new(http.Client)`, `http.Transport{}`, `http.DefaultTransport`, dan `nh.Get` (impor beralias `nh "net/http"`) → tepat 10 pelanggaran `EGRESS`, semuanya dengan detail berawalan `egress_bad.go:`. `egress_ok.go` (`egress.New`, `http.NewRequestWithContext`, `http.Header{}`) → 0.
  - `TestDecode_FixtureViolations`: `decode_bad.go` memuat `httpx.DecodeJSON[infrastructure.NoteModel]`, `json.Unmarshal(b, &m)` dengan `var m infrastructure.NoteModel`, `json.NewDecoder(r.Body).Decode(&[]infrastructure.NoteModel{})`, dan `httpx.DecodeJSON[wrapDTO]` (`wrapDTO struct{ N infrastructure.NoteModel }`). Ditambah `handler_bad.go` (`httpx.DecodeJSON[localDTO]` di paket infrastructure), totalnya tepat 5 pelanggaran `DECODE`. `decode_ok.go` (`httpx.DecodeJSON[createNoteDTO]`) dan `jsonb.go` (`json.Unmarshal(raw, &m.Meta)` di infrastructure) → 0.
  - `arch_test.go` `TestRepositorySecurityRules`: `CheckSecurity(LoadTypedRepo("../.."))` → tidak ada pelanggaran.
- [ ] **Step 2: Jalankan** `go test ./internal/archcheck/... ./tests/architecture/...` → FAIL.
- [ ] **Step 3: Implementasi** sesuai Interfaces; `go mod tidy` root.
- [ ] **Step 4: Jalankan** `task arch` → PASS. Bukti negatif manual: tambah sementara `http.Get("https://x")` di `modules/example/module.go` → `task arch` gagal dengan `EGRESS`, lalu kembalikan perubahan itu.
- [ ] **Step 5: Commit** `feat(archcheck): typed EGRESS and DECODE rules for modules`

### Task 13: Dokumentasi dan penyelarasan spec

**Files:**
- Create: `docs/module-sdk/security-primitives.md`
- Modify: `docs/module-sdk/README.md`, `docs/module-sdk/creating-a-module.md`, `docs/security/security-guidelines.md` (§3.2 sesuai keputusan 7, §7), `docs/prd/PRD-000-inisialisasi-project.md` (baris Q berikutnya: **Q30** bila Q29 sudah dipakai M2b; catatan rantai FR-05; catatan FR-24.2), `docs/adr/0006-module-manifest-sdk-semver.md` (opsi route variadik + `SDKVersion` tetap `0.1.0` sampai tag pertama), `docs/operations/deployment.md` (`ERP_DB_STATEMENT_TIMEOUT_MS`), `.env.example`, `docs/superpowers/plans/2026-10-22-m2-overview.md` (catatan M2c selesai). Bila owner memilih `413`/`415` pada keputusan 1: `docs/architecture/api-guidelines.md` §8 dan `contracts/openapi/common.yaml`.

**Interfaces:** tidak ada (dokumentasi).

`security-primitives.md` (contoh dari `example`/fixture) memuat:
1. Respons dan error (`httpx.WriteData`/`WritePage`/`WriteError`/`RespondError`; 5xx tanpa detail).
2. Decode request: pola `DecodeJSON[createNoteDTO]` dengan DTO khusus per endpoint, tabel error, field read-only tidak pernah diambil dari body, `AllowEmptyBody`, serta larangan arch `DECODE`.
3. Opsi route: `MaxBodyBytes`, `Classify`; aturan memilih klasifikasi tertinggi termasuk field bersarang (M2d memeriksa kesesuaiannya dengan `x-erp-classification`).
4. Koneksi keluar: `egress.New`, allowlist host per integrasi, error tujuan tidak diteruskan mentah, larangan arch `EGRESS`.
5. Ekspor: `export.Serve` dengan `iter.Seq2`, klasifikasi kolom wajib, permission `<resource>.export` untuk data `confidential`, dan batas `MaxRows` (proses panjang menunggu Jobs).
6. Timeout database (`statement_timeout` 30s di `serve`; query panjang → job).

`security-guidelines.md` §7 diubah sebagai berikut. Baris `statement_timeout` → **M2c ✅**. Baris FR-24.2 dipecah menjadi `sdk/egress`, `DecodeJSON`, `no-store`, `sdk/export`, klasifikasi manifest → **M2c ✅**, sedangkan linter `x-erp-classification` dan kesesuaian klasifikasi route ↔ kontrak → **M2d**.

Baris Q30 PRD-000 mencatat keputusan 1–7 sesuai jawaban owner, ditambah 8, 9, 12, dan 14 sebagai keputusan rencana yang mengikat.

- [ ] **Step 1:** Tulis dokumen.
- [ ] **Step 2:** Pastikan setiap identifier yang disebut ada di kode (`git grep -n <identifier>` per nama), dan tautan relatif valid.
- [ ] **Step 3: Commit** `docs: document M2c security primitives and align security guidelines and PRD-000`

---

## Verifikasi akhir M2c (exit criteria overview: "Test tabel SSRF, decode, ekspor lolos")

| Butir (FR-24.2 AC / overview / Q) | Test |
| --- | --- |
| Egress menolak `http://127.0.0.1`, `http://169.254.169.254`, `http://[::1]`, `http://10.0.0.1`, hostname yang resolve ke alamat privat | `TestClient_RejectsFR24Table`, `TestBlockedReason_Table` (T7) |
| Egress menolak redirect ke alamat privat | `TestClient_Redirects` (T7) |
| Anti DNS rebinding (dial ke IP tervalidasi), timeout, batas ukuran, content type | `TestClient_DialsValidatedIP`, `TestClient_Timeout`, `TestClient_ResponseLimits` (T7) |
| Arch test menolak fixture modul yang memakai `http.Get` | `TestEgress_FixtureViolations` (T12) |
| `DecodeJSON` menolak body > batas, field tak dikenal, kedalaman berlebih | `TestDecodeJSON_Table` (T4), `TestMount_DecodeUsesRouteLimit` (T5) |
| Batas body dapat ditimpa per route | `TestRegister_RouteOptionsDefaultsAndValues` (T1), `TestMount_DecodeUsesRouteLimit` (T5) |
| Arch test: decode ke tipe `internal/infrastructure` ditolak | `TestDecode_FixtureViolations` (T12) |
| `no-store` untuk `confidential`/`highly_confidential` | `TestMount_NoStoreByClassification` (T5), `TestServe_Headers` (T9) |
| Ekspor CSV mengawali `=1+1` dengan `'` | `TestSanitizeCell_Table`, `TestWrite_CSV_Golden` (T9) |
| Ekspor menghormati `MaskFields` dan mencatat audit | `TestWrite_MaskedColumnsEmpty`, `TestValidate_ConfidentialRequiresAuditor` (T9), `TestExport_EndToEndThroughMount` (T10) |
| Klasifikasi `field_groups` di manifest | `TestValidate_FieldGroupClassification` (T2) |
| Q25: helper error `sdk/httpx` | `TestWriteError_Envelope`, `TestRespondError_*` (T3); konsumen dimigrasikan (T6) |
| Q26 butir 7: `statement_timeout` | `TestStatementTimeout_AllPooledConnections` (T11) |
| Q26 butir 13: opsi per-route variadik tanpa mengubah pemanggil | `TestRecorder_RecordsRouteOptions` (T1); build modul tanpa perubahan (T1 Step 4) |
| Repo nyata lolos `EGRESS`/`DECODE` | `TestRepositorySecurityRules` (T12) |

- [ ] `task ci` exit 0 (termasuk `test:integration`).
- [ ] Manual: `task erpora -- serve`, lalu `curl -s -X POST -H "Content-Type: text/plain" -d "{}" localhost:8080/api/platform/modules/acme.example/enable` (dengan `ERP_ENV=development`) → `400` dengan `details[0].code == "unsupported_media_type"`; `curl -sI localhost:8080/api/example/notes` → tanpa `Cache-Control` (klasifikasi `internal`).
- [ ] Ledger `.superpowers/sdd/<tanggal>-m2c/progress.md` diperbarui; tidak ada push tanpa permintaan user.

## Di luar cakupan M2c (sengaja)

- Linter kontrak Spectral untuk `x-erp-classification` (menolak `password` di respons, token di query, field sensitif tanpa klasifikasi) dan assertion kesesuaian `RouteInfo.Classification` ↔ kontrak → **M2d** (FR-23; bagian AC FR-24.2 tentang linter dinilai di sana).
- Template `make:module`/`make:handler` yang memakai `httpx.DecodeJSON`, `Classify`, dan `export` → **M2d** (FR-22).
- Field-level access nyata di balik `MaskFields`, audit log append-only, dan pemeriksaan permission `export` per field group → **PRD-001** (FR-112, FR-130).
- Endpoint ekspor asinkron (Jobs) dan rate limit ekspor/impor (security-guidelines §3.10) → PRD Events/Jobs dan PRD-001.
- Instrumentasi OTel untuk request egress (span client) dan dukungan proxy keluar → ditambahkan saat konsumen pertama (OIDC PRD-001) membutuhkannya.
- `SafeHtml`, ZAP baseline → M3 (FR-24.3). Pipeline upload → PRD Files.
- Penambahan `413`/`415` ke katalog api-guidelines → hanya bila owner memilihnya pada keputusan 1.

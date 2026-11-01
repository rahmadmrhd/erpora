# Metadata resource di frontend: kontrak konsumsi untuk M3c dan M3d

Panduan ini menjelaskan paket metadata frontend `@erpora/platform-web/metadata` (spec `docs/specs/resource-metadata`, Requirement 18, tahap T3): apa yang disediakan, bentuk metadata yang dipancarkan pembangkit, aturan evaluasi, titik perluasan, dan batas keamanannya. Pembaca utamanya adalah pembuat komponen generik (form M3c, daftar dan filter M3d) dan modul yang mendaftarkan widget sendiri.

Komponen perender generik (form, tabel, FilterBuilder) **bukan** bagian paket ini. Paket ini hanya menyediakan data, evaluator, validator, dan kontrak editor sebagai tipe TypeScript. `@erpora/ui` tetap daun (F4) dan tidak mengimpor paket ini.

## Isi dan cara mengimpor

```ts
import {
  validateRecord, evaluateCondition, evaluateExpr, fieldTypeRegistry, resourceMetaLoader,
  formatDecimalString, labelRef, type ResourceMeta, type FieldMeta,
} from "@erpora/platform-web/metadata";
```

- Hanya lewat **subpath** `@erpora/platform-web/metadata`. Paket ini **tidak** diekspor dari barrel utama `@erpora/platform-web`, supaya tidak masuk bundel awal (anggaran 200 KB gzip, `task web:check`). Muat malas dengan `import()` dari kode yang memerlukannya (halaman form atau daftar).
- Isi: tipe metadata, evaluator kondisi dan ekspresi, templat label, desimal berskala, pencocokan `pattern`, validator record, registri tipe field, pemuat malas per resource, dan format tampilan desimal.

## Bentuk metadata (`ResourceMeta`)

Setiap resource dibangkitkan menjadi satu berkas `modules/<dir>/web/src/metadata/<nama>.meta.gen.ts` dengan **satu ekspor bernama `resourceMeta`** bertipe `ResourceMeta`. Bentuknya mencerminkan model datar hasil resolver (penyertaan, timpaan, tipe domain, dan blok sudah diselesaikan; tidak ada jejak asal). Berkas bangkitan tidak diedit tangan.

| Properti `ResourceMeta` | Isi |
| --- | --- |
| `key` | Kunci resource `<slug>.<nama>` |
| `labelTemplate?` | Templat label, placeholder `{field}` |
| `defaultGroup` | Kunci grup field default modul pemilik |
| `fields` | `FieldMeta[]` berurutan menurut deklarasi |
| `relations` | `RelationMeta[]` |
| `rules` | `RuleMeta[]`, hanya aturan `scope: both` |
| `sections` | `SectionMeta[]` (tata letak dari blok) |
| `lockWhen?` | Kondisi penguncian dokumen (hanya bila boleh dipublikasikan) |

`FieldMeta` memuat: `name`, `type` (tipe dasar: `string`, `text`, `integer`, `decimal`, `boolean`, `date`, `datetime`, `enum`, `uuid`), `label` (kunci i18n), `nullable`, `readOnly`, `default?`, `fieldGroup`, `classification`, penanda `filterable`/`sortable`/`groupable`/`aggregatable`, batasan (`minLength`, `maxLength`, `pattern`, `format`, `precision`, `scale`, `currency`, `minimum`, `maximum`, `values`, `extensible`), `widget?`/`display?`/`operators?`/`domainType?` dari tipe domain, `visibleWhen?`/`requiredWhen?`/`readonlyWhen?`, `deprecated?`, dan `accessor?`. `RelationMeta` memuat `type`, `to`/`of`/`types`, `onDelete`, `required`, `embedded`, `filter` (kondisi pembatas pilihan), `snapshot`, dan `pivotFields`. Definisi tipe lengkap ada di `packages/platform-web/src/metadata/meta.ts`.

Aturan penting bentuk ini:

- Kunci memakai camelCase seperti berkas metadata; nilai `enum` tetap snake_case.
- `decimal` selalu string desimal (`"12.50"`), `integer` bilangan bulat aman (|n| ≤ 2^53−1), `datetime` UTC berakhiran `Z`.
- `minimum` dan `maximum` untuk `decimal` berupa string desimal agar tidak kehilangan presisi.
- Kondisi memakai bentuk JSON yang sama dengan berkas metadata: `{all:[...]}`, `{any:[...]}`, `{not:{...}}`, daun `{field, op, value}` atau `{field, op, ref}`. Operator: `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`, `contains`, `isnull` (selaras api-guidelines §5.1).
- Kunci i18n (`label`, `RuleMeta.message`) berada pada katalog namespace **modul pemilik resource**: namespace = slug pada `key` (`sales` untuk `sales.invoice`), kunci = string apa adanya. Gunakan `labelRef(meta, key)` untuk mendapatkan `MessageRef { ns, key }` yang cocok dengan `useTranslation(ns).t(key)`. Tidak ada teks UI bebas di metadata.

### Yang TIDAK ada di metadata klien (chunk bersifat publik)

Chunk metadata disajikan sebagai aset SPA statis tanpa autentikasi (Requirement 18.8; design D-8 dan D-21). Karena itu pembangkit tidak memancarkan:

- field yang ditandai `clientVisible: false` (beserta `clientVisibleReason`); halaman yang memakainya ditulis tangan;
- aturan berpenanda `scope: server`, yaitu aturan yang merujuk field berklasifikasi `confidential` ke atas atau field di luar grup `default` (Requirement 4.6), beserta nilai ambangnya.

Tipe `ResourceMeta` sengaja tidak punya tempat bagi `clientVisible`, `clientVisibleReason`, atau `scope`. Pemuat (lihat di bawah) menolak chunk yang tetap memuatnya (`checkClientMeta`). `checkClientMeta` juga menelusuri **setiap** kondisi dan ekspresi di chunk: `visibleWhen`/`requiredWhen`/`readonlyWhen` field, `rules`, `relation.filter` (hanya `ref`), `lockWhen`, `accessor.expr` (termasuk kondisi dan cabang `if` di dalamnya, juga untuk `stored`), dan semua itu pada `relation.pivotFields[]`. Rujukan ke field yang tidak ada di metadata klien (mis. yang `clientVisible: false`), field `confidential` ke atas, atau field di luar grup default ditolak, karena yang demikian harus `scope: server` atau tidak dipancarkan (pembangkit tidak memancarkan `expr` untuk `stored`, dan memancarkan `expr` untuk accessor hanya bila semua masukannya berklasifikasi paling tinggi `internal` di grup default). Pemindaian yang melampaui batas (200.000 simpul) dianggap pelanggaran (gagal tertutup), dan chunk tak lengkap (koleksi `fields`, `relations`, `rules`, `sections` atau `defaultGroup` hilang) ditolak sebagai `invalid_module`, bukan `TypeError`. Klien tidak pernah menyimpulkan akses grup field dari metadata: akses datang dari respons API.

### Field tidak ada di respons = "tidak tersedia", bukan "kosong"

Field dari grup field yang tidak boleh dilihat principal **dihilangkan dari JSON** (api-guidelines §4.4). Konsekuensinya bagi konsumen:

- `isFieldAvailable(record, name)` dan `fieldAvailability(meta, record)` membedakan kunci yang ada (nilai boleh `null` = kosong) dari kunci yang tidak ada atau `undefined` (= tidak tersedia).
- Jangan menampilkan field tidak tersedia sebagai kolom kosong; sembunyikan atau tandai tidak tersedia.
- Evaluator memperlakukan field tidak tersedia sebagai hasil tak tentu, bukan nol atau kosong (lihat aturan evaluasi).
- `validateRecord` mode `update` melewati field yang tidak tersedia; `writablePayload(meta, record)` hanya menyertakan field yang tersedia dan dapat ditulis.

## Aturan evaluasi

Evaluator Go (`sdk/metadata`) dan TypeScript berperilaku identik; paritas dibuktikan korpus vektor bersama `contracts/metadata-vectors/` dan property test dua sisi. Seluruh evaluasi adalah **fungsi total**: tidak melempar untuk input apa pun dan berbiaya linear terhadap jumlah simpul.

**Tipe nilai.** Tipe field menentukan penafsiran nilai: `decimal` hanya string kanonik `-?(0|[1-9][0-9]*)(\.[0-9]+)?` (maksimum 64 digit), `integer` bilangan bulat aman, `date` `YYYY-MM-DD`, `datetime` `YYYY-MM-DDTHH:MM:SS[.d{1,9}]Z`, `uuid` huruf kecil kanonik. Nilai yang tidak cocok bukan galat: hasilnya tak tentu (di bawah).

**Hasil tak tentu.** Tipe tak cocok, field tidak ada di metadata (`unknown_field`), field tidak tersedia (`field_unavailable`), operator tak didukung (`unsupported_op`), nilai terlalu panjang, dan sejenisnya membuat hasil kondisi **tak tentu**. `evaluateCondition` mengembalikan `{ value: false, reasons: [...] }`; `reasons` kosong berarti hasilnya pasti. Logika tiga-nilai dipakai pada grup: `all`/`any` tetap pasti bila simpul lain menentukan, dan `not` tidak pernah mengubah tak tentu menjadi `true`. Konsumen yang perlu membedakan "salah" dari "tidak diketahui" memeriksa `reasons`.

**Null.** `null` tidak pernah sama dengan nilai bukan-null; `eq` hanya benar bila kedua sisi `null`, `ne` adalah negasi `eq`, operator urutan, `contains`, dan `in` bernilai `false`. Gunakan `isnull` untuk menguji kosong.

**Teks.** Dibandingkan menurut code point (identik dengan urutan byte UTF-8, bukan lokal). `contains` tidak peka huruf besar-kecil hanya untuk huruf ASCII.

**Mode pembulatan.** Hanya tujuh mode di atas yang sah. Mode tak dikenal atau skala negatif **ditolak** (`roundDecimal` dan `divDecimal` di TS mengembalikan `null`; `Decimal.Round` di Go mengembalikan `ok=false`; ekspresi bangunan sendiri menjadi hasil tak tentu `invalid_expression`); tidak pernah berperilaku diam-diam sebagai mode lain.

**Pattern.** Pola harus lolos validator subset (`ValidatePattern`, dijalankan `metadata:check`): subset RE2 yang sama di Go dan JS, tinggi bintang paling banyak 1 (kuantor variabel di dalam operand pengulang, termasuk `{n}` tepat dengan n > 1, ditolak: `(?:\d+){5}`), alternasi tak ambigu di bawah kuantor berulang, dan **tidak ada dua pengulangan lebar (tak terbatas atau batas atas > 32) yang himpunan karakternya beririsan dan hanya dipisahkan elemen opsional** (`\d*\d*x`, `.*.*=.*`, `\d+-?\d+` ditolak; `\d+-\d+`, `.+@.+`, `\d+(\.\d+)?` sah). Risiko sisa: pemisah wajib yang beririsan dengan pengulang di sekitarnya (`.*=.*=.*=.*`) tetap polinomial; di klien dibatasi panjang input 1024 dan hanya mempengaruhi tab pengguna sendiri.

**Batas** (Requirement 1.3, 19.1): kondisi paling banyak 32 simpul dan kedalaman 4, `in` paling banyak 100 nilai, nilai teks paling banyak 65.536 satuan UTF-16, ekspresi paling banyak 32 simpul dan kedalaman 8. Struktur yang melampaui batas ditolak tanpa dieksekusi.

**Ekspresi** (accessor `expr`): `lit`, `dec`, `field`, `concat`, `coalesce`, aritmetika `add`/`sub`/`mul`/`div` dengan `scale` (0 sampai 30) dan `round` (`half_up`, `half_down`, `half_even`, `up`, `down`, `floor`, `ceiling`) yang **selalu eksplisit**, dan `if`/`then`/`else` (hanya cabang terpilih yang dievaluasi). `null` merambat melalui aritmetika; pembagian nol membuat hasil tak tentu.

**Templat label.** `parseLabelTemplate` dan `renderLabel` merender teks biasa (bukan HTML). Placeholder bernilai null atau tidak tersedia menjadi teks kosong; sebab yang bukan null tercatat di `reasons`. Rujukan yang tidak dapat dibaca ditampilkan sebagai label placeholder terjemahan oleh komponen (Requirement 6.9), bukan oleh paket ini.

## Validator: `validateRecord`

```ts
const issues = validateRecord(meta, draft, { mode: "create" | "update" });
```

Memvalidasi draf form terhadap metadata: tipe, wajib dan `nullable`, panjang (dalam code point), rentang, `pattern`, `format`, enum, presisi dan skala desimal, `requiredWhen`, dan `rules`. Hanya kenyamanan: **server tetap otoritatif** dan batasan klien tidak pernah lebih ketat daripada server.

- Mode `create` (bawaan): field yang tidak ada dianggap kosong (wajib bila non-nullable tanpa `default`). Mode `update`: field yang tidak ada atau tidak tersedia dilewati; `null` pada non-nullable tetap ditolak.
- Dilewati: field `readOnly` dan accessor; field yang pasti tersembunyi (`visibleWhen` pasti salah); field yang sedang terkunci (`readonlyWhen` pasti benar). `requiredWhen` yang pasti benar menambah kewajiban.
- `rules` yang hasilnya tak tentu dilewati; keputusan ada di server.
- String kosong adalah nilai, bukan kosong; komponen form sebaiknya mengubah masukan kosong menjadi `null`. `emptyRecord(meta)` menghasilkan draf awal untuk form baru.
- `pattern` diuji di klien hanya untuk input sampai 1024 satuan UTF-16 (mesin JavaScript dapat melakukan backtracking eksponensial); input lebih panjang tidak diuji dan diserahkan ke server.
- Nilai `decimal` tidak pernah dikonversi ke `Number`: perbandingan, penjumlahan, dan format memakai utilitas desimal berbasis `BigInt`.

**Hasil.** Setiap isu `ValidationIssue { field, code, params?, ruleId?, message? }` hanya membawa kode, parameter, dan rujukan pesan, tidak pernah teks bebas atau nilai field:

| `code` | `params` | Arti |
| --- | --- | --- |
| `required` | | Wajib diisi |
| `invalid_type` | | Tipe atau bentuk nilai salah |
| `too_short`, `too_long` | `min` atau `max` | Panjang di luar batas (code point) |
| `pattern`, `format` | | Tidak cocok pola atau format (`email`, `url`) |
| `below_minimum`, `above_maximum` | `min` atau `max` (string desimal) | Di luar rentang inklusif |
| `not_in_values` | | Bukan nilai enum yang sah |
| `decimal_precision`, `decimal_scale` | `precision`, `scale` | Melebihi `numeric(p,s)` |
| `rule` | | Aturan lintas field dilanggar; `ruleId` dan `message` (`MessageRef` ke katalog modul) terisi |

**Teks UI lewat i18n.** Komponen memetakan `code` dan `params` ke pesan ICU di katalog (`useTranslation`; argumen mis. `{min}` dan `{max}`), dan merender isu `rule` dari `message` (`MessageRef`). Usulan pemetaan: kunci `validation.<code>` pada katalog namespace `platform`; kunci itu belum ada di katalog saat ini dan ditambahkan M3c bersama komponen formnya. Pesan galat dari server (`422 validation_failed`) tetap diterjemahkan oleh `errors.<code>`.

## Registri tipe field

`fieldTypeRegistry` memetakan tiap tipe ke (1) operator filter yang sah, (2) jenis editor nilai untuk form, dan (3) jenis editor nilai untuk filter. Seluruhnya hanya kontrak; komponen editor dibuat di M3c dan M3d.

Operator sah per tipe dasar (selaras api-guidelines §5.1; selalu himpunan bagian dari yang didukung evaluator):

| Tipe | Operator |
| --- | --- |
| `string`, `text` | `eq`, `ne`, `in`, `contains`, `isnull` |
| `integer`, `decimal`, `date`, `datetime` | `eq`, `ne`, `gt`, `gte`, `lt`, `lte`, `in`, `isnull` |
| `boolean` | `eq`, `ne`, `isnull` |
| `enum`, `uuid` | `eq`, `ne`, `in`, `isnull` |

- `operatorsFor(field)`: kosong bila field tidak `filterable`; selain itu irisan tabel tipe dasar, `field.operators` (tipe domain), dan operator widget terdaftar. Hanya mempersempit, tidak pernah melebarkan.
- `formEditorKind(field)`: jenis editor form (`text`, `textarea`, `integer`, `decimal`, `boolean`, `date`, `datetime`, `enum`). M3c memetakannya ke komponen shadcn.
- `filterEditor(field, op)`: `{ kind, multiple }` untuk editor nilai filter, atau `null` bila operator tidak sah. `isnull` memakai editor `boolean`; `in` menerima banyak nilai.
- Kontrak komponen: `FieldEditorProps` (form: `field`, `id`, `value`, `onChange`, `disabled`, `readOnly`, `invalid`) dan `FilterEditorProps` (filter: `field`, `operator`, `id`, `value`, `onChange`). `value === undefined` berarti field tidak tersedia, berbeda dari `null`.
- `resolve(field)`: `{ operators, formKind, widget? }` dalam satu panggilan.

### Widget kustom modul (Requirement 3.13)

Tipe domain membawa kunci `widget` (mis. `email`, `rupiah`). Modul mendaftarkan editornya di registri tanpa mengubah `@erpora/ui`:

```ts
fieldTypeRegistry.registerWidget("rupiah", {
  form: () => import("./widgets/rupiah-editor").then((m) => m.RupiahEditor), // komponen atau pemuat malas
  filter: RupiahFilterEditor,
  operators: ["eq", "gte", "lte"], // opsional: hanya mempersempit
});
```

Kunci widget cocok `^[a-z][A-Za-z0-9]*$`; pendaftaran ganda ditolak. Field tanpa widget terdaftar jatuh ke editor bawaan tipenya. Daftarkan saat registrasi modul, bukan saat modul dievaluasi di entry awal.

## Pemuat malas per resource

```ts
resourceMetaLoader.register("sales.invoice", () => import("./metadata/invoice.meta.gen"));
const meta = await resourceMetaLoader.load("sales.invoice"); // chunk terpisah, dimuat sekali
```

- Satu importer (satu chunk) per resource; tidak ada metadata di bundel awal. Permintaan bersamaan dipadukan, hasil di-cache, kegagalan impor tidak di-cache (dapat diulang). `peek(key)` mengembalikan metadata yang sudah termuat.
- Chunk divalidasi: ekspor `resourceMeta` harus lengkap bentuknya (`invalid_module`), `key` harus cocok, dan `checkClientMeta` harus bersih (lihat "Yang tidak ada di metadata klien"). Pelanggaran memunculkan `MetadataLoadError` (`code`: `not_registered`, `import_failed`, `invalid_module`, `key_mismatch`, `unsafe_for_client`); chunk yang bocor tidak pernah dipakai.

## Catatan untuk penerjemah SQL (T2: `stored`, CHECK, pemicu `lockWhen`)

Semantik evaluator di bawah ini adalah acuan yang harus dipertahankan saat kondisi dan ekspresi diterjemahkan ke SQL (kolom `stored`, CHECK, pemicu penguncian; Requirement 5.3, 11.5, 13.2):

- `eq`/`ne` aman-null: `null` hanya sama dengan `null` (`a IS NOT DISTINCT FROM b`); `ne` adalah negasinya (`a IS DISTINCT FROM b`), jadi `ne` atas null bernilai **benar** (bukan NULL seperti `<>` SQL).
- `gt`, `gte`, `lt`, `lte`, `contains`, dan `in` atas null bernilai **salah pasti**, bukan NULL: bungkus dengan `COALESCE(..., false)` atau `IS NOT NULL AND ...`.
- `isnull` tanpa `value` berarti `value: true`.
- `not` atas hasil pasti membalik biasa; kondisi yang diterjemahkan tidak boleh menghasilkan NULL pada akar (CHECK memperlakukan NULL sebagai lolos, yang berbeda dari "salah").
- `contains` tidak peka huruf besar-kecil hanya untuk huruf ASCII (`lower()` SQL melipat huruf non-ASCII dan bergantung kolasi; terjemahkan ke pelipatan ASCII atau tandai tak dapat diterjemahkan).
- Teks dibandingkan menurut code point (kolasi `"C"`), bukan kolasi bahasa.
- Aritmetika `add`/`sub`/`mul`/`div` merambatkan null; `div` dengan pembagi nol adalah hasil tak tentu (di SQL: tolak atau jaga dengan `CASE`, jangan menjalankan `/` mentah); pembulatan hanya lewat `round` dengan skala dan mode eksplisit yang diterjemahkan secara eksak (tujuh mode di atas), tidak ke `ROUND()` bawaan yang hanya setara `half_up`.
- Field tidak tersedia dan tipe tak cocok tidak ada di SQL (kolom selalu ada dan bertipe); kasus itu hanya hidup di evaluator.

## Format tampilan desimal

`formatDecimalString(value, locale, { scale?, currency? })` memformat string desimal untuk tampilan tanpa konversi `Number`: digit berasal dari utilitas desimal, `Intl` hanya memberi pola bahasa (pemisah, simbol mata uang, posisi tanda). Skala tampilan di-pad ke skala field tanpa memotong digit nilai. Masukan yang bukan desimal kanonik menghasilkan `null`; selain itu fungsi ini total: bahasa, mata uang (`currency: "rupiah"`), atau skala yang rusak tidak pernah melempar (mata uang tak sah diabaikan, bahasa tak sah jatuh ke `en`, skala dibatasi 0 sampai 64). Pengelompokan ribuan tiga digit. Jangan memformat nilai `decimal` dengan `Number(...)`, `parseFloat`, atau `toFixed`.

## Titik perluasan dan checklist konsumen

- **Widget kustom:** `fieldTypeRegistry.registerWidget` (di atas).
- **Pustaka baru tidak diperlukan:** evaluator, validator, dan desimal adalah kode paket ini; jangan menambah pustaka desimal atau regex.
- **Form (M3c):** muat metadata lewat `resourceMetaLoader`, mulai dari `emptyRecord`, render field dari `fields` menurut `sections`, evaluasi `visibleWhen`/`readonlyWhen`/`requiredWhen` dengan `evaluateCondition(cond, fieldTypes(meta), draft)`, validasi dengan `validateRecord`, kirim `writablePayload`. Tampilkan galat server (`422`) berdampingan; server tetap menegakkan semuanya.
- **Daftar dan filter (M3d):** hanya field `filterable`/`sortable`; operator dari `operatorsFor`; editor dari `filterEditor`. Field yang tidak ada pada baris respons ditampilkan "tidak tersedia".
- **Teks:** semua teks UI lewat i18n (`useTranslation`, `msg()`, `crumb()`); `labelRef` untuk label dan pesan aturan. `dangerouslySetInnerHTML` dilarang; label dirender sebagai teks.

## Keamanan

- Chunk metadata publik: tidak ada nilai atau ambang rahasia; `clientVisible: false` dan aturan `scope: server` tidak pernah dipancarkan (dijaga `checkClientMeta` di pemuat).
- Regex di klien hanya untuk pola yang lolos validator subset (`metadata:check`), dengan batas panjang input; `.` diterjemahkan agar sama dengan RE2.
- Desimal tidak pernah lewat `Number`; nilai di luar jangkauan float tetap eksak.
- Label dan pesan dirender sebagai teks biasa; evaluator tidak pernah menjalankan teks dari metadata.
- Semua validasi klien hanya kenyamanan; server adalah otoritas (otorisasi, IDOR, mass assignment, validasi).

# Primitif keamanan untuk modul

Panduan memakai primitif keamanan SDK (M2c, PRD-000 FR-24.2). Primitif ini wajib dipakai oleh modul: HTTP client mentah dan decode ke tipe `internal/infrastructure` ditolak oleh arch test (aturan `EGRESS` dan `DECODE`). Rujukan: `docs/security/security-guidelines.md` §2, §3.1, §3.2, §3.3, §3.4, §3.8, §6, dan `docs/architecture/api-guidelines.md` §4, §8.

## 1. Respons dan error (`erpora/sdk/httpx`)

`httpx` adalah satu-satunya implementasi amplop `data`/`error`. Platform (`httpserver.WriteError`) mendelegasikan ke paket yang sama.

```go
httpx.WriteData(w, http.StatusOK, dto)                       // {"data": ...}
httpx.WritePage(w, http.StatusOK, items, httpx.Page{...})    // {"data": [...], "page": {...}}
httpx.WriteError(w, r, 400, "bad_request", "pesan", []httpx.Detail{{Field: "page", Code: "invalid", Message: "..."}})
httpx.RespondError(w, r, err)                                // *httpx.Error -> sesuai isinya; error lain -> 500 generik
```

- `error.requestId` diambil dari header respons `X-Request-Id` dan `error.traceId` dari span context; modul tidak mengisinya.
- Error `5xx` tidak pernah memuat detail internal. `RespondError` mencatat error asli di log lalu menjawab `500 internal_error` generik. Untuk error terstruktur gunakan `httpx.NewError(status, code, message, details...)`; `Err` hanya untuk log.
- Contoh nyata: `modules/example/internal/presentation/http/handlers.go`.

## 2. Decode request (`httpx.DecodeJSON`)

Gunakan DTO khusus per endpoint. Field read-only (`id`, `version`, `createdAt`, field milik server) tidak ada di DTO sehingga tidak pernah dapat diisi dari body (mass assignment).

```go
type createNoteDTO struct {
    Title string `json:"title"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
    in, err := httpx.DecodeJSON[createNoteDTO](w, r)
    if err != nil {
        httpx.RespondError(w, r, err)
        return
    }
    // validasi bisnis lalu use case
}
```

`DecodeJSON[T]` hanya menerima `struct` (selain itu panic: kesalahan pemrograman). Batas bawaan: body 1 MiB (dapat ditimpa per route, lihat bagian 3), kedalaman 32 (objek akar = 1), array 1000 elemen (`httpx.MaxDepth`, `httpx.MaxArrayLen` untuk mengubahnya per panggilan).

Urutan pemeriksaan: baca body (batas ukuran), body kosong, `Content-Type`, lintasan token (struktur, kunci ganda, field tak dikenal, tipe), data sesudah objek akar, lalu decode. Karena itu body kosong dengan `Content-Type` salah menjawab `body_required`, bukan `unsupported_media_type`.

| Kondisi | Status | `error.code` | `details[].code` |
| --- | --- | --- | --- |
| `Content-Type` bukan `application/json`, charset bukan utf-8, atau tidak ada padahal body ada | 400 | `bad_request` | `unsupported_media_type` |
| Body melebihi batas | 400 | `bad_request` | `body_too_large` |
| Body kosong (tanpa `AllowEmptyBody`) | 400 | `bad_request` | `body_required` |
| JSON rusak | 400 | `bad_request` | `malformed_json` |
| Nilai akar bukan objek | 400 | `bad_request` | `expected_object` |
| Data sesudah objek akar | 400 | `bad_request` | `trailing_data` |
| Kedalaman atau panjang array melebihi batas | 400 | `bad_request` | `too_deep`, `array_too_long` |
| Kunci ganda dalam satu objek | 400 | `bad_request` | `duplicate_key` |
| Field tak dikenal (termasuk beda huruf besar/kecil) | 422 | `validation_failed` | `unknown_field` |
| Tipe nilai salah (juga kegagalan decode lapis kedua, mis. overflow atau unmarshaler kustom) | 422 | `validation_failed` | `invalid_type` |

`details[].field` memakai format api-guidelines §8, mis. `lines[2].itemId`. `httpx.AllowEmptyBody()` membuat body kosong menghasilkan DTO kosong dan tidak mewajibkan `Content-Type` (dipakai endpoint aksi dengan body opsional); body tidak kosong tetap wajib `application/json`.

Arch test `DECODE` menolak `httpx.DecodeJSON`, `json.Unmarshal`, dan `(*json.Decoder).Decode` ke tipe yang memuat tipe bernama dari paket `internal/infrastructure` (termasuk lewat slice, map, atau struct pembungkus). Pengecualian hanya `json.Unmarshal`/`Decode` yang dipanggil di dalam paket infrastructure sendiri (payload jsonb milik adapter); `httpx.DecodeJSON` di paket infrastructure selalu ditolak.

## 3. Opsi route (`module.MaxBodyBytes`, `module.Classify`)

`Registrar.Handle` dan `Registrar.Public` menerima opsi variadik; pemanggil lama tidak berubah.

```go
r.Handle(http.MethodPost, "/notes", "example.notes.create", http.HandlerFunc(h.Create),
    module.MaxBodyBytes(4<<10),                 // 1 <= n <= 10 MiB; default 1 MiB
    module.Classify(module.ClassConfidential))  // public | internal | confidential | highly_confidential
```

- Platform memasang `http.MaxBytesReader` pada setiap route modul sebelum handler (handler yang membaca body sendiri pun terbatas), dan meneruskan batas itu ke `DecodeJSON`.
- `Classify` dipanggil berulang mengambil kelas tertinggi. `confidential` dan `highly_confidential` membuat respons membawa `Cache-Control: no-store`, termasuk respons error handler. Route tanpa `Classify` diperlakukan setara `internal`.
- Pilih klasifikasi tertinggi di antara semua properti respons, termasuk properti bersarang. Kontrak OpenAPI (`x-erp-classification`) tetap menjadi sumber kebenaran; M2d menambahkan pemeriksaan bahwa klasifikasi route sama dengan klasifikasi tertinggi skema respons di kontrak.
- Opsi tidak valid (batas di luar rentang, klasifikasi tidak dikenal) membuat registry gagal dengan problem `invalid_route_option`.
- Klasifikasi field group dideklarasikan juga di manifest: `resources[].field_groups[].classification`.

## 4. Koneksi keluar (`erpora/sdk/egress`)

Satu-satunya HTTP client keluar yang boleh dipakai modul.

```go
c, err := egress.New(egress.Config{
    AllowHosts:          []string{"api.partner.example"},
    AllowedContentTypes: []string{"application/json"},
})
resp, err := c.Get(ctx, "https://api.partner.example/v1/rates")
```

- Skema `https`; `http` hanya untuk host di `AllowHTTPHosts`. Allowlist host per integrasi (`AllowHosts`: host persis atau `*.example.com`).
- IP divalidasi sesudah resolve DNS dan dial hanya ke IP tervalidasi (anti DNS rebinding); jawaban DNS campuran publik+privat ditolak; host numerik non-kanonik (`2130706433`, `0x7f000001`, `0177.0.0.1`, `127.1`) ditolak; redirect divalidasi per hop (maks 3).
- Timeout koneksi 5 detik, total 30 detik, respons maksimal 10 MiB (melewati batas → `egress.ErrResponseTooLarge`, tidak dipotong diam-diam), TLS minimal 1.2, proxy dari environment tidak dipakai.
- Error tujuan (`*egress.BlockedError`, `ErrBlockedAddress`, dst.) tidak diteruskan mentah ke klien API: petakan ke error generik (mis. `502`/`503`) dan catat detailnya di log.
- Arch test `EGRESS` melarang `http.DefaultClient`, `http.DefaultTransport`, `http.Get/Post/Head/PostForm`, serta `http.Client{}`/`http.Transport{}` di `modules/**`.

## 5. Ekspor (`erpora/sdk/export`)

```go
spec := export.Spec{
    Resource: "example.note", Format: export.CSV, Filename: "notes",
    Columns: []export.Column{
        {Key: "id", Header: "ID", Classification: module.ClassInternal},
        {Key: "title", Header: "Judul", Classification: module.ClassInternal},
    },
}
err := export.Serve(w, r, spec, rows) // rows: iter.Seq2[export.Row, error]
if err != nil {
    httpx.RespondError(w, r, err)
}
```

- Klasifikasi setiap kolom wajib diisi; kolom `highly_confidential` ditolak (`ErrForbiddenClassification`). Kolom `confidential` mewajibkan auditor terpasang dan menambah `Cache-Control: no-store`.
- Setiap baris melewati `authz.MaskFields`; kolom yang di-mask tampil kosong. Tanpa masker di context, ekspor ditolak (`ErrNoFieldMasker`).
- CSV: BOM UTF-8, CRLF, sel yang diawali `=`, `+`, `-`, `@`, tab, atau CR diberi awalan `'` (string desimal murni seperti `-1250.00` dikecualikan). XLSX: semua sel teks tanpa formula. Nilai sel wajib skalar; nilai bersarang ditolak (`ErrNonScalarValue`).
- Maksimal `DefaultMaxRows` (10 000) baris per ekspor sinkron; ekspor lebih besar menunggu infrastruktur Jobs. Nama berkas harus cocok `^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`.
- Kesalahan sebelum byte pertama (spec tidak valid dan sejenisnya) dikembalikan sebagai error. Preamble (BOM + header, atau berkas XLSX awal) langsung dikirim, sehingga kegagalan sesudahnya (iterator error, `ErrTooManyRows`, nilai bersarang) dicatat di audit lalu koneksi diputus (`panic(http.ErrAbortHandler)`) agar klien tidak menerima berkas terpotong yang tampak utuh.
- Permission: ekspor data `confidential` memakai permission `<resource>.export` yang dideklarasikan di manifest dan dicek oleh route (`Handle`).
- Platform memasang masker dan auditor stub (`authorization.NewStubFieldMasker`, `audit.NewExportLog`) di setiap route modul, hook lifecycle, dan command; PRD-001 menggantinya tanpa mengubah modul. Di test modul gunakan `authztest.AllowAllFields`.

## 6. Timeout database

Pool `erpora serve` memasang `statement_timeout` (default 30 detik, env `ERP_DB_STATEMENT_TIMEOUT_MS`, rentang 1000–600000) pada setiap koneksi. Statement yang melewatinya dibatalkan dengan SQLSTATE `57014`. CLI (`migrate`, `modules`, command modul) tidak dibatasi. Query yang memang panjang adalah pekerjaan job, bukan request HTTP.

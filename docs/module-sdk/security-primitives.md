# Primitif keamanan untuk modul

Panduan memakai primitif keamanan SDK (M2c, PRD-000 FR-24.2). Primitif ini wajib dipakai oleh modul: HTTP client mentah dan decode ke tipe `internal/infrastructure` ditolak oleh arch test (aturan `EGRESS` dan `DECODE`), begitu pula pemasangan identitas atau pengirim email di context oleh modul (aturan `IDENT`, bagian 7). Rujukan: `docs/security/security-guidelines.md` §2, §3.1, §3.2, §3.3, §3.4, §3.8, §6, dan `docs/architecture/api-guidelines.md` §4, §8.

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
n**Keterbatasan (Q32 M-8):** aturan `DECODE` memeriksa tipe target pada titik panggilan. Pembungkus generik yang menerima type parameter (mis. `func Bind[T any](r *http.Request) T { ... httpx.DecodeJSON[T](...) }`) tidak ditelusuri: tipe yang memuat model infrastructure yang diteruskan lewat type parameter baru terlihat di situs pemanggilan pembungkus, bukan di dalamnya, dan pembungkus yang menyembunyikan tipe di balik interface atau `any` lolos. Penelusuran instansiasi generik ditunda (belum ada konsumen); sampai saat itu, jangan membungkus `httpx.DecodeJSON` dengan fungsi generik buatan sendiri dan tinjau pembungkus semacam itu secara manual di code review.

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
- Rentang yang ditolak (security-guidelines §3.3, Q32 M-1) mencakup juga IPv4-compatible, SIIT, NAT64 (global dan local-use), 6to4, Teredo, discard-only, dan rentang dokumentasi.
- **Keterbatasan: redirect tidak dapat dimatikan** (Q32 M-10). Client selalu mengikuti redirect sampai 3 hop, dengan validasi alamat dan skema per hop. Opsi untuk mematikan redirect (atau membatasinya ke host tertentu) **ditunda sampai konsumen pertama** yang membutuhkannya (OIDC/OAuth di PRD-001); sampai saat itu modul yang tidak boleh mengikuti redirect harus memeriksa `resp.Request.URL` pada respons dan menolak bila host berubah.
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
- Maksimal `DefaultMaxRows` (10 000) baris per ekspor sinkron; ekspor lebih besar menunggu infrastruktur Jobs. Nama berkas harus cocok `^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`. XLSX ditolak sebagai `ErrInvalidSpec` bila `MaxRows` melebihi 1.048.575 baris data (header ikut dihitung) atau kolom melebihi 16.384 (dimensi lembar).
- Kesalahan sebelum byte pertama (spec tidak valid dan sejenisnya) dikembalikan sebagai error. Preamble (BOM + header, atau berkas XLSX awal) langsung dikirim, sehingga kegagalan sesudahnya (iterator error, `ErrTooManyRows`, nilai bersarang) dicatat di audit lalu koneksi diputus (`panic(http.ErrAbortHandler)`) agar klien tidak menerima berkas terpotong yang tampak utuh.
- Permission: ekspor data `confidential` memakai permission `<resource>.export` yang dideklarasikan di manifest dan dicek oleh route (`Handle`).
- Platform memasang masker dan auditor stub (`authorization.NewStubFieldMasker`, `audit.NewExportLog`) di setiap route modul, hook lifecycle, dan command; PRD-001 menggantinya tanpa mengubah modul. Di test modul gunakan `authztest.AllowAllFields`.

## 6. Timeout database

Pool `erpora serve` memasang `statement_timeout` (default 30 detik, env `ERP_DB_STATEMENT_TIMEOUT_MS`, rentang 1000–600000) pada setiap koneksi. Statement yang melewatinya dibatalkan dengan SQLSTATE `57014`. CLI (`migrate`, `modules`, command modul) tidak dibatasi. Query yang memang panjang adalah pekerjaan job, bukan request HTTP.

## 7. Identitas dan email (`erpora/sdk/identity`, `erpora/sdk/mail`)

Modul **membaca** identitas dan **mengirim** email; platform yang **memasangnya** di context.

- Boleh di semua modul: `identity.FromContext(ctx)` (Principal pemanggil), `mail.Send(ctx, msg)`, `mail.From(ctx)`. **Hanya modul `core`** yang boleh membaca `identity.ServiceFrom(ctx)` (layanan identitas) dan `identity.AdminFrom(ctx)` (operasi administratif); keduanya hanya dipasang pada context modul `core` dan alasan sistem `cli:core:*` (keputusan pemilik produk 3 Nov 2026). Modul lain yang butuh data identitas memakai kontrak modul `core`. Principal bukan sumber otorisasi tunggal: otorisasi tetap lewat `authz` (PRD-001 FR-119), dan parameter kontrak antarmodul tidak membawa `userId`/`tenantId`.
- **Dilarang di kode non-test modul** (aturan arch test `IDENT`, PRD-000 Q42 butir 5; `CheckSecurity`): `identity.WithPrincipal`, `identity.WithService`, `identity.WithAdmin`, dan `mail.WithSender`, baik dipanggil maupun sekadar direferensikan; selain `modules/core`, juga `identity.AdminFrom` dan `identity.ServiceFrom`; dan mengimpor `erpora/sdk/identity/identitytest` serta `testing` dari berkas non-test. Juga dilarang di kode non-test modul: mengimpor `unsafe` (termasuk `import _ "unsafe"`), direktif `//go:linkname`, dan **setiap ekspresi bertipe `unsafe.Pointer`** (juga yang didapat lewat refleksi tanpa impor, mis. `reflect.NewAt` dengan `Value.UnsafePointer()`), karena semuanya dapat memintas deteksi pemanggilan atau memalsukan nilai context (mis. `//go:linkname forge erpora/sdk/identity.WithPrincipal`). Hak core dikunci ke path Go module `erpora/modules/core` **dan** direktori persis `<root>/modules/core` (aturan `MODPATH`: path go.mod harus `erpora/modules/<direktori>`). Satu `go.mod` per modul (aturan `MODNEST`): `go.mod` bersarang dilarang dan `replace` lokal hanya `erpora/sdk => ../../sdk` serta `erpora/modules/<x> => ../<x>`. Pemindaian memeriksa varian build: bawaan (host), tag `devtools`, dan tiap target matriks rilis (`.github/workflows/ci.yml`, job `build`: linux/amd64, linux/arm64, windows/amd64) selain target host dengan `CGO_ENABLED=0` (rilis lintas target dibangun tanpa cgo, sedangkan gerbang CI Linux berjalan dengan cgo aktif); varian dimuat bersamaan (`TestRepositorySecurityRules` sekitar 9 detik, dibanding 13,8 detik semula dengan 2 varian berurutan); batasan: tag selain `devtools`, GOARCH di luar matriks rilis, dan GOAMD64/GOARM tidak dicakup. Hak memasang itu milik platform (`Authenticate`, composition root); modul yang memilikinya dapat memalsukan identitas atau mengambil hak admin.
- **Batas jaminan:** aturan ini **mempersulit dan mendeteksi**, bukan menjamin. Kotak tertutup di `sdk/identity` hanya **mempersulit pemalsuan murni-refleksi** dan menyerahkan kontrol ke aturan `IDENT`, yang menandai setiap ekspresi bertipe `unsafe.Pointer` (termasuk yang didapat lewat `reflect.NewAt`/`Value.UnsafePointer` tanpa `import "unsafe"`). Risiko residual yang diterima: `unsafe.Pointer` yang diperoleh lewat cgo atau assembly (berkas bantu `_cgo_*` tidak dipindai) dan dependensi pihak ketiga yang memanggil API tanpa dipindai arch test; modul pihak ketiga dianggap kode yang di-review sebelum dipasang.
- Di tes modul, pasang Principal lewat `identitytest` (`sdk/identity/identitytest`); berkas `_test.go` tidak diperiksa aturan `IDENT`.
- Mekanisme autentikasi (token, sesi, MFA, rate limit): ADR-0013.

### 7.1 Principal dan galat domain (`erpora/sdk/identity`)

- `identity.Principal` adalah pelaku yang sudah terautentikasi: `UserID`, `Kind` (`KindHuman` atau `KindService`), `SessionID`, `AuthMethod`, `AMR` (faktor yang sudah diverifikasi: `AMRPassword`, `AMROTP`), dan `TokenScope`. Principal tidak memuat tenant; tenant selalu diambil dari `tenancy.FromContext`.
- `identity.FromContext(ctx)` mengembalikan salinan Principal dan `ok`. `identity.RequirePrincipal(ctx)` juga memastikan `UserID` tidak kosong, dan mengembalikan `ErrUnauthenticated` bila tidak ada. `p.MFAVerified()` benar bila `AMR` memuat `otp`.
- Principal dipasang platform (`WithPrincipal`, oleh middleware `Authenticate`). Modul hanya membacanya. Di test, Principal dipasang lewat `sdk/identity/identitytest`.
- Galat domain (`errors.go`) dipetakan handler modul `core` ke kode galat api-guidelines (`modules/core/internal/presentation/http/errors.go`):

| Galat | Respons |
| --- | --- |
| `ErrInvalidCredentials` | 401 `invalid_credentials` |
| `ErrUnauthenticated`, `ErrInvalidRefresh` | 401 `unauthenticated` |
| `ErrForbidden`, `ErrOriginNotAllowed` | 403 `permission_denied` |
| `ErrMFARequired` | 403 `mfa_required` |
| `ErrAccountLocked` | 403 `account_locked` |
| `ErrWrongCurrentPassword`, `ErrInvalidResetToken`, `ErrInvalidMFACode` | 422 `validation_failed` dengan `details[].code` `wrong_current_password`, `invalid_token`, atau `invalid_code` |
| `*PasswordPolicyError` | 422 `password_policy_violation` |
| `*RateLimitedError` | 429 `rate_limited` dengan `Retry-After` |
| `ErrMFAAlreadyEnrolled`, `ErrMFANotEnrolled` | 409 (`conflict`, `invalid_state`) |

- Login gagal menjawab `ErrInvalidCredentials` untuk semua sebabnya, termasuk akun `locked` dan akun nonaktif (anti-enumerasi). `ErrAccountLocked` hanya dikembalikan oleh operasi yang sudah membuktikan sandi: ganti sandi dan verifikasi MFA.

### 7.2 Layanan `Service` dan `Admin` (`erpora/sdk/identity`)

- `identity.Service` (dipasang `WithService`, dibaca `ServiceFrom`) memuat login, refresh, logout, lupa dan reset sandi, ganti sandi, pendaftaran dan verifikasi MFA, serta pembuatan ulang recovery code. Metode yang bekerja atas sesi atau akun tidak menerima ID pengguna atau sesi dari pemanggil; keduanya diturunkan dari Principal di context (`RequirePrincipal`).
- `LoginResult` yang sampai ke klien berisi `accessToken`, `expiresIn`, dan `mfaState` (`MFANone`, `MFAEnrollRequired`, `MFAVerifyRequired`). `RefreshToken` dan `RefreshExpiresAt` bertag `json:"-"`; refresh token hanya sampai ke klien sebagai cookie `HttpOnly`.
- `identity.Admin` (dipasang `WithAdmin`, dibaca `AdminFrom`) memuat `CreateAdmin`, `RotateSigningKeys`, `ResetMFA`, `UnlockAccount`, `LogoutAllDevices`, dan `SetPublicURL`. Target operasi dinyatakan eksplisit (email atau userID), sehingga implementasi menegakkan otorisasinya sendiri: principal harus memegang role `administrator` dengan sesi yang terverifikasi MFA, atau pemanggil memakai hak sistem `cli:core:*` yang teraudit (PRD-001 Q30 butir 1).
- Modul selain `core` tidak pernah menerima `Service` atau `Admin` (aturan `IDENT`, lihat bagian 7 di atas). Modul yang butuh data identitas memakai kontrak modul `core` (`modules/core/contract`; direktori ini belum ada di C1, jadi kontrak antarmodul pertama menyusul bersama kebutuhan modul lain).

### 7.3 Pengiriman email (`erpora/sdk/mail`)

- `mail.Message` berisi `To`, `Subject`, `Text`, dan `HTML`. Modul memanggil `mail.Send(ctx, msg)`: pesan divalidasi (`mail.Validate`) lalu dikirim lewat `Sender` di context. Tanpa `Sender`, `Send` mengembalikan `ErrNoSender`; email tidak pernah dianggap terkirim diam-diam.
- `Validate` menolak penerima kosong atau lebih dari 50; alamat selain alamat polos ASCII (nama tampilan, domain literal, IDN, encoded-word, atau karakter kontrol); subjek kosong, lebih dari 998 byte, atau berisi karakter kontrol, pemisah baris, dan karakter format; UTF-8 tidak valid; serta isi kosong atau lebih dari 1 MiB. Galatnya membungkus `ErrInvalidMessage`.
- Driver berada di `internal/platform/mail` dan dipasang platform; modul tidak mengimpornya.
  - `smtp`: mode `starttls` (bawaan; STARTTLS wajib, tanpa downgrade), `tls` (implisit), dan `none` (hanya development, tanpa autentikasi). AUTH PLAIN atau LOGIN hanya di atas TLS yang sudah terverifikasi. Timeout sambung 10 detik dan timeout operasi 30 detik; percobaan maksimal 3 kali, hanya untuk galat sementara. Sandi SMTP bertipe `config.Secret` yang tersamarkan pada log, galat, dan `inspect config`.
  - `log`: tidak membuka koneksi. Bawaan hanya mencatat penerima tersamar (`b***@domain`), subjek, dan ukuran isi. Penerima dan isi penuh hanya dengan `WithBody`, yang dipasang perakit hanya bila `ERP_ENV=development`.
- Di produksi, `serve` gagal start bila `ERP_MAIL_DRIVER` tidak diisi (`Config.RequireMailDriver`, PRD-001 Q21). Konfigurasi SMTP masih lewat env `ERP_MAIL_DRIVER` dan `ERP_SMTP_*` sampai Settings (C3) (PRD-001 K-C1.6).

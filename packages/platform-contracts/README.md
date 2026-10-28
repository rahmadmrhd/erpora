# platform-contracts

Tipe TypeScript hasil generate dari `contracts/openapi/` (`common.gen.ts` dan `platform.gen.ts`, JANGAN DIEDIT) dan client fetch
bertipe `createClient<Paths>()` (`src/client.ts`). Frontend modul memakai client ini, bukan library pihak ketiga (aturan F2).

Perbarui lewat `erpora contracts:generate`; `erpora contracts:generate --check` (bagian `task contracts:check`) menggagalkan CI
bila file hasil generate tidak sesuai dengan kontrak.

Opsi klien: `baseUrl`, `fetch`, `headers()` (header per request), dan `getLocale()` (menyisipkan `Accept-Language`; dibaca ulang tiap
request; header eksplisit menang). Hasil sukses memuat `etag`; status `304` menjadi `{ ok: false, status: 304, error.code:
"not_modified" }`. Mutasi tanpa body tidak menyetel `Content-Type` (mutasi platform wajib `body: {}`).

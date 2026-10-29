# platform-web

Layanan frontend platform (`@erpora/platform-web`) yang dipakai bersama app shell dan modul. Tidak mengimpor `@erpora/ui`
(komponen dijembatani di shell, mis. `UiLabelsBridge`) dan tidak mengimpor paket modul. Spesifikasi: `.kiro/specs/m3a-frontend-foundation-shell/`
(design §5) dan `.kiro/specs/i18n-platform/`.

## Isi

- **`src/i18n`** (spec i18n-platform): format pesan ICU MessageFormat (`format.ts`, `intl-messageformat` dipin; sama dengan Go lewat
  `sdk/i18n/testdata/conformance.json`) dan runtime React. `I18nProvider({ client })` membaca `tenantId` dan `bootstrap.i18n` dari
  `BootstrapProvider`, memilih bahasa (`localStorage["erpora.locale"]` yang terdaftar, lalu `navigator.languages`, lalu
  `defaultLocale`), memperbarui `<html lang>`, dan memuat katalog per namespace dari `GET /api/platform/i18n/{locale}/{ns}`
  (`useSuspenseQuery`, key `[tenantId, "i18n", locale, ns]`, `staleTime: Infinity`, `If-None-Match`/ETag, muat ulang bila
  `bootstrap.i18n.revisions[ns]` berubah). `useTranslation(ns)` mengembalikan `t` (teks polos), `rich` (tag rich sebagai elemen
  React), `locale`, `setLocale`; kunci mentah tidak pernah tampil selama memuat (Suspense) dan pesan rusak mengembalikan kunci.
  Tipe kunci dan argumen berasal dari `I18nCatalogs` yang diperluas `keys.gen.ts` lewat
  `declare module "@erpora/platform-web" { interface I18nCatalogs { ... } }`; tag rich ditulis sebagai properti bertipe `RichTag`.
  `createTestI18n({ ns: { "kunci.lengkap": "pesan" } })` menyediakan provider uji tanpa jaringan. `getActiveLocale` dipakai
  `createPlatformClient({ getLocale: getActiveLocale })` untuk `Accept-Language`.
- **`prefs`**: `readPref`/`writePref` membungkus `localStorage` dengan `try/catch`; hanya tiga kunci preferensi UI
  (`erpora.theme`, `erpora.sidebar`, `erpora.locale`). Tidak untuk rahasia atau data pengguna.
- **`bootstrap`**: `createBootstrapFetcher` (ber-`ETag`, `304` mengembalikan referensi objek yang sama), `BootstrapProvider`
  (loading, galat dengan `retry`, refetch saat focus), `useBootstrap`, `useTenantId`, `useFeature`, `BOOTSTRAP_KEY`
  (satu-satunya query key tanpa awalan tenant), `StaticBootstrapProvider` dan `makeBootstrap` untuk test.
- **`tenancy`**: `scopedKey(tenantId, ...)` dan `useScopedKey(...)`; semua query key selain bootstrap berawalan tenant (diuji
  pemindai statis di `tenancy/query-keys.scan.test.ts`).
- **`registry`**: `validateModules` (key/slug ganda), `resolveModules` (peringatan `not_in_bootstrap`, `version_mismatch`,
  `offline_mismatch`, `slug_mismatch`), `selectNavigation` (hanya modul `enabled`, `to` absolut), `moduleStatus`,
  `ModuleRegistryProvider` dan `useModuleRegistry`.
- **`api`**: `createPlatformClient` (klien terketik + `Accept-Language`) dan `describeApiError` (kode galat ke kunci pesan
  `errors.<code>`, tidak pernah menyalin `details` mentah).
- **`msg`**: `msg(ns, key)` dan `crumb(ns, key)` terketik (`createMsgHelpers<Catalogs>()` untuk katalog lokal), `Message`,
  `useMessage`, `MessageTranslatorProvider`.
- **`theme`**: `ThemeProvider`, `useTheme`, `parseThemePreference`, `resolveTheme`, `applyTheme`; konsisten dengan
  `web/src/theme-init.ts` (skrip eksternal sinkron sebelum paint, tanpa `<script>` inline).
- **`page`**: `usePageActions`/`PageActions`/`usePageLayout`/`PageChromeProvider`/`usePageChrome`: aksi halaman yang dirender
  navbar; `onSelect` terbaru tanpa render ulang konsumen; dibersihkan saat unmount.
- **`sources`**: antarmuka `NotificationSource` dan `CurrentUserSource` (+ `emptyNotificationSource`, `anonymousUserSource`),
  `useNotifications`, `useCurrentUser`, `safeInternalHref`. Data notifikasi dan pengguna (PII) hanya di cache memori.
- **`format`**: `formatRelativeTime`, `formatDateTime`.
- **`permissions`**: `usePermission` (petunjuk UX; `permissions` kosong berarti optimistis `true`; backend tetap otoritas).
- **`db`**: `loadDb`/`useDb` memuat TanStack DB lewat `import()` dinamis (chunk terpisah; DB tidak boleh ada di bundle awal).

## Catatan integrasi

1. Shell memasang `<QueryClientProvider><BootstrapProvider><I18nProvider client={...}><Suspense>...`; klien dibuat sekali dengan
   `createPlatformClient({ getLocale: getActiveLocale })`. `<Message>`/`useMessage` tersambung otomatis ke `useTranslation`.
2. Kunci galat `errors.*` (`api/errors.ts`, `API_ERROR_KEY_PREFIX`) harus ada di katalog `platform` (termasuk `errors.unknown`).
3. Generator `keys.gen.ts` harus menulis tag rich sebagai properti `RichTag` dan bentuk argumen sesuai `ArgsOf`.

## Pengujian

`bun test` (dari root atau paket) memakai preload `web/test` (DOM happy-dom). Test yang memerlukan bootstrap memakai
`StaticBootstrapProvider` dan `makeBootstrap`.

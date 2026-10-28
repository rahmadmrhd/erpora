# platform-web

Layanan frontend platform (`@erpora/platform-web`) yang dipakai bersama app shell dan modul. Tidak mengimpor `@erpora/ui`
(komponen dijembatani di shell, mis. `UiLabelsBridge`) dan tidak mengimpor paket modul. Spesifikasi: `.kiro/specs/m3a-frontend-foundation-shell/`
(design §5) dan `.kiro/specs/i18n-platform/`.

## Isi

- **`src/i18n`** (spec i18n-platform): format pesan ICU MessageFormat di atas `intl-messageformat` (versi dipin), dibuktikan sama
  dengan sisi Go lewat fixture konformansi `sdk/i18n/testdata/conformance.json`. Provider dan hook React (`I18nProvider`,
  `useTranslation`) menyusul (spec i18n-platform, tahap runtime frontend).
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
  `error.<code>`, tidak pernah menyalin `details` mentah).
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

## Titik integrasi yang menunggu runtime i18n dan API katalog

1. `I18nCatalogs` sementara dideklarasikan di `src/msg/catalogs.ts`; bila `src/i18n` mendefinisikannya, hapus dan impor dari sana.
2. `Message`/`useMessage` membaca terjemahan lewat `MessageTranslatorProvider useTranslate={(ns) => useTranslation(ns).t}`.
3. Kunci galat `error.*` (`api/errors.ts`, `API_ERROR_KEY_PREFIX`) harus ada di katalog `platform`.
4. `createPlatformClient({ getLocale })` mengirim `Accept-Language` lewat `headers`; dapat dialihkan ke opsi `getLocale` klien
   kontrak bila spec i18n menambahkannya.
5. `I18nProvider` harus membaca `tenantId` dan `bootstrap.i18n` dari `BootstrapProvider` (M3a task 9.4).

## Pengujian

`bun test` (dari root atau paket) memakai preload `web/test` (DOM happy-dom). Test yang memerlukan bootstrap memakai
`StaticBootstrapProvider` dan `makeBootstrap`.

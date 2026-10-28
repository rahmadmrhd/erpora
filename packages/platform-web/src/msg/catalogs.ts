/**
 * Bentuk katalog terjemahan per namespace (`{ [namespace]: { [kunci]: pesan } }`), diperluas oleh `keys.gen.ts` tiap
 * paket lewat deklarasi `interface` yang digabung (design spec i18n-platform §9). Sampai runtime i18n
 * (`I18nProvider`/`keys.gen.ts`) dipasang kosong, sehingga `msg()` belum menerima kunci apa pun.
 *
 * TITIK INTEGRASI: bila runtime i18n mendefinisikan `I18nCatalogs` di `src/i18n`, hapus deklarasi ini dan impor yang
 * itu di `helpers.ts` agar hanya ada satu sumber.
 */
// biome-ignore lint/suspicious/noEmptyInterface: sengaja kosong, diperluas lewat penggabungan interface
export interface I18nCatalogs {}

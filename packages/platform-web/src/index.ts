// ===== i18n (spec i18n-platform): format pesan ICU =====

// ===== Lapisan layanan frontend (M3a grup 9) =====
// Titik integrasi i18n (menunggu `I18nProvider`/`useTranslation`/`keys.gen.ts`): `msg/catalogs.ts` (I18nCatalogs),
// `msg/message.tsx` (MessageTranslatorProvider) dan `api/errors.ts` (API_ERROR_KEY_PREFIX); lihat README paket.
export * from "./api";
export * from "./bootstrap";
export * from "./db";
export * from "./format";
export type { Args, ArgValue, RichTags } from "./i18n/format";
export { Compiled, compile, format, formatRich, I18nFormatError } from "./i18n/format";
export * from "./msg";
export * from "./page";
export * from "./permissions";
export * from "./prefs";
export * from "./registry";
export * from "./sources";
export * from "./tenancy";
export * from "./theme";

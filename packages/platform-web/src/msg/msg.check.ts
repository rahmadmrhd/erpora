// Pemeriksaan tipe saja (dijalankan lewat `typecheck`; M3a 9.6): `msg()`/`crumb()` terketik menolak kunci dan
// namespace yang salah ketik. Memakai pembangun `createMsgHelpers` dengan katalog lokal sehingga tidak bergantung
// pada augmentasi `I18nCatalogs` global dari `keys.gen.ts`. Tidak diekspor dan tidak dijalankan.
import { createMsgHelpers } from "./helpers";

type Catalogs = {
  example: { "nav.notes": string; "page.title": string };
  platform: { "app.name": string };
};
const { msg, crumb } = createMsgHelpers<Catalogs>();

export const ok = [msg("example", "nav.notes"), crumb("platform", "app.name")];

// @ts-expect-error kunci salah ketik
export const badKey = msg("example", "tidak.ada");

// @ts-expect-error kunci milik namespace lain
export const crossNs = msg("platform", "nav.notes");

// @ts-expect-error namespace tidak ada
export const badNs = msg("lain", "nav.notes");

// @ts-expect-error crumb juga terketik
export const badCrumb = crumb("example", "tidak.ada");

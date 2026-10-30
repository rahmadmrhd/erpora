// Pemeriksaan tipe saja (dijalankan lewat `typecheck`; i18n 12.5/12.7): `t`/`rich` terketik menolak kunci salah,
// argumen hilang, dan argumen salah tipe. Memakai `Translator<K>` dengan bentuk katalog lokal sehingga tidak
// bergantung pada augmentasi `I18nCatalogs` global. Tidak diekspor dan tidak dijalankan.
import type { RichTag, Translator } from "./types";

type Cat = {
  "note.title": Record<never, never>;
  "note.count": { count: number };
  "note.status": { state: "draft" | "approved" | "other" };
  "note.rich": { name: string; b: RichTag };
};
declare const tr: Translator<Cat>;

export const ok = [
  tr.t("note.title"),
  tr.t("note.count", { count: 2 }),
  tr.t("note.status", { state: "draft" }),
  tr.rich("note.rich", { name: "x", b: (c) => c }),
];

// @ts-expect-error kunci salah ketik
export const badKey = tr.t("note.tidak.ada");

// @ts-expect-error argumen wajib hilang
export const missingArg = tr.t("note.count");

// @ts-expect-error argumen salah tipe (string bukan number)
export const wrongType = tr.t("note.count", { count: "dua" });

// @ts-expect-error nilai di luar gabungan literal select
export const wrongSelect = tr.t("note.status", { state: "lain" });

// @ts-expect-error argumen tak dikenal
export const extraArg = tr.t("note.count", { count: 1, extra: 2 });

// @ts-expect-error tag rich tidak boleh dikirim lewat t() (hanya rich)
export const tagInPlain = tr.t("note.rich", { name: "x", b: (c: unknown) => c });

// @ts-expect-error rich wajib menyertakan fungsi tag
export const missingTag = tr.rich("note.rich", { name: "x" });

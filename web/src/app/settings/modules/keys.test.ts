import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { API_ERROR_KEYS } from "@erpora/platform-web";
import { modulesKeyList } from "./keys";

// Dibaca dari berkas (bukan `import`) agar aturan F2 (tanpa deep import ke packages/*) tidak terpicu.
const repoRoot = join(import.meta.dir, "../../../../..");
const read = (locale: string): Record<string, unknown> =>
  JSON.parse(
    readFileSync(join(repoRoot, "internal/platform/i18n/catalog", `${locale}.json`), "utf8"),
  );

function flatten(obj: unknown, prefix = ""): Record<string, string> {
  if (typeof obj === "string") return { [prefix]: obj };
  if (typeof obj !== "object" || obj === null) return {};
  return Object.assign(
    {},
    ...Object.entries(obj).map(([k, v]) => flatten(v, prefix === "" ? k : `${prefix}.${k}`)),
  );
}

/** Nama argumen tingkat atas pesan ICU: `{nama}` atau `{nama, plural, ...}`; isi cabang tidak dihitung. */
const params = (s: string): string[] =>
  [...s.matchAll(/\{(\w+)\s*[,}]/g)].map((m) => m[1] as string).sort();

describe("kunci platform:modules.* terhadap katalog JSON (id dan en)", () => {
  for (const locale of ["id", "en"]) {
    const flat = flatten(read(locale));
    const modules = Object.keys(flat).filter((k) => k.startsWith("modules."));

    test(`${locale}: himpunan kunci modules.* sama persis dengan keys.ts`, () => {
      expect(modules.sort()).toEqual([...modulesKeyList].sort());
    });

    test(`${locale}: tanpa HTML dan tanpa pesan kosong`, () => {
      for (const k of modules) {
        expect(flat[k], k).toBeTruthy();
        expect(flat[k], k).not.toMatch(/<[a-z/]/i);
      }
    });
  }

  test("id dan en memiliki argumen yang sama untuk setiap kunci", () => {
    const id = flatten(read("id"));
    const en = flatten(read("en"));
    for (const k of modulesKeyList) {
      expect(params(en[k] ?? ""), k).toEqual(params(id[k] ?? ""));
    }
  });

  test("kunci galat umum yang dipetakan modul ada di katalog platform", () => {
    const id = flatten(read("id"));
    for (const k of API_ERROR_KEYS) expect(id[k], k).toBeTruthy();
  });
});

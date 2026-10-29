import { afterEach, describe, expect, test } from "bun:test";
import fc from "fast-check";
import { PREF_KEYS } from "../prefs";
import { chooseLocale, getActiveLocale, matchLocale, setActiveLocale } from "./locale";

const info = {
  defaultLocale: "id",
  locales: [{ code: "id" }, { code: "en" }, { code: "en-GB" }, { code: "pt-BR" }],
};
const codes = info.locales.map((l) => l.code);

describe("matchLocale", () => {
  test("kecocokan persis (tidak peka huruf besar-kecil), lalu bahasa dasar", () => {
    expect(matchLocale("en-GB", codes)).toBe("en-GB");
    expect(matchLocale("EN-gb", codes)).toBe("en-GB");
    expect(matchLocale("en-US", codes)).toBe("en");
    expect(matchLocale("id-ID", codes)).toBe("id");
    expect(matchLocale("pt", codes)).toBe("pt-BR");
    expect(matchLocale("pt-PT", codes)).toBe("pt-BR");
  });

  test("tidak ada yang cocok: undefined", () => {
    expect(matchLocale("fr", codes)).toBeUndefined();
    expect(matchLocale("", codes)).toBeUndefined();
    expect(matchLocale("*", codes)).toBeUndefined();
  });
});

describe("chooseLocale", () => {
  test("urutan: tersimpan yang valid, lalu navigator.languages, lalu bahasa bawaan", () => {
    expect(chooseLocale({ ...info, stored: "en", languages: ["id"] })).toBe("en");
    expect(chooseLocale({ ...info, stored: null, languages: ["fr", "en-US", "id"] })).toBe("en");
    expect(chooseLocale({ ...info, stored: null, languages: ["fr", "de"] })).toBe("id");
    expect(chooseLocale({ ...info, stored: null, languages: [] })).toBe("id");
  });

  test("tersimpan tidak terdaftar (atau berupa tag mirip) diabaikan", () => {
    expect(chooseLocale({ ...info, stored: "fr", languages: ["en"] })).toBe("en");
    expect(chooseLocale({ ...info, stored: "en-US", languages: ["id"] })).toBe("id");
    expect(chooseLocale({ ...info, stored: "<script>", languages: [] })).toBe("id");
  });

  test("daftar bahasa kosong: bahasa bawaan", () => {
    expect(
      chooseLocale({ defaultLocale: "id", locales: [], stored: "en", languages: ["en"] }),
    ).toBe("id");
  });

  test("properti: hasil selalu anggota daftar atau bahasa bawaan", () => {
    fc.assert(
      fc.property(
        fc.option(fc.string(), { nil: null }),
        fc.array(fc.string(), { maxLength: 4 }),
        fc.uniqueArray(fc.constantFrom("id", "en", "en-GB", "pt-BR", "fr"), { maxLength: 5 }),
        (stored, languages, list) => {
          const out = chooseLocale({
            defaultLocale: "id",
            locales: list.map((code) => ({ code })),
            stored,
            languages,
          });
          expect((list as string[]).includes(out) || out === "id").toBe(true);
        },
      ),
      { numRuns: 300 },
    );
  });
});

describe("bahasa aktif untuk header Accept-Language", () => {
  afterEach(() => {
    setActiveLocale(undefined);
    localStorage.clear();
  });

  test("memakai bahasa yang diset provider; tanpa itu preferensi tersimpan; selain itu undefined", () => {
    expect(getActiveLocale()).toBeUndefined();
    localStorage.setItem(PREF_KEYS.locale, "en");
    expect(getActiveLocale()).toBe("en");
    setActiveLocale("id");
    expect(getActiveLocale()).toBe("id");
  });

  test("preferensi tersimpan yang bukan kode bahasa wajar diabaikan", () => {
    localStorage.setItem(PREF_KEYS.locale, "<script>alert(1)</script>");
    expect(getActiveLocale()).toBeUndefined();
  });
});

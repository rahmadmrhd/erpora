import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import fc from "fast-check";
import {
  applyTheme,
  parseThemePreference,
  resolveTheme,
  THEME_STORAGE_KEY,
  type ThemePreference,
} from "./theme";

const PREFS: readonly ThemePreference[] = ["light", "dark", "system"];

describe("parseThemePreference", () => {
  test.each(PREFS)("%s dipertahankan", (p) => {
    expect(parseThemePreference(p)).toBe(p);
  });

  test("null, undefined, string asing, dan non-string menjadi system", () => {
    for (const raw of [
      null,
      undefined,
      "",
      "Dark",
      " dark",
      "auto",
      0,
      1,
      true,
      {},
      [],
      ["dark"],
    ]) {
      expect(parseThemePreference(raw)).toBe("system");
    }
  });

  test("P6: total untuk masukan apa pun", () => {
    fc.assert(
      fc.property(fc.anything(), (x) => {
        expect(PREFS).toContain(parseThemePreference(x));
      }),
      { numRuns: 200 },
    );
    fc.assert(
      fc.property(fc.string(), (x) => {
        const out = parseThemePreference(x);
        expect(out === x || out === "system").toBe(true);
      }),
      { numRuns: 200 },
    );
  });
});

describe("resolveTheme", () => {
  test("pilihan eksplisit mengabaikan sistem; system mengikuti OS", () => {
    expect(resolveTheme("light", true)).toBe("light");
    expect(resolveTheme("dark", false)).toBe("dark");
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
  });
});

describe("applyTheme", () => {
  let root: HTMLElement;
  beforeEach(() => {
    root = document.createElement("html");
  });

  test("memasang tepat satu kelas dan color-scheme", () => {
    applyTheme(root, "dark");
    expect(root.classList.contains("dark")).toBe(true);
    expect(root.classList.contains("light")).toBe(false);
    expect(root.style.colorScheme).toBe("dark");
    applyTheme(root, "light");
    expect(root.classList.contains("light")).toBe(true);
    expect(root.classList.contains("dark")).toBe(false);
    expect(root.style.colorScheme).toBe("light");
  });

  test("tidak menyentuh kelas lain", () => {
    root.classList.add("keep");
    applyTheme(root, "dark");
    expect(root.classList.contains("keep")).toBe(true);
  });

  test("P6: urutan penerapan apa pun menyisakan tepat satu kelas tema", () => {
    fc.assert(
      fc.property(
        fc.array(fc.constantFrom<"light" | "dark">("light", "dark"), { minLength: 1 }),
        (seq) => {
          const el = document.createElement("html");
          for (const t of seq) applyTheme(el, t);
          const has = ["light", "dark"].filter((c) => el.classList.contains(c));
          expect(has).toEqual([seq[seq.length - 1] as string]);
        },
      ),
      { numRuns: 100 },
    );
  });
});

describe("konsistensi dengan web/src/theme-init.ts", () => {
  const initPath = new URL("../../../../web/src/theme-init.ts", import.meta.url);

  // Menjalankan ulang isi skrip tema (JS murni tanpa impor) seperti peramban memuatnya sebagai <script src>.
  async function runThemeInit(): Promise<void> {
    new Function(await Bun.file(initPath).text())();
  }

  afterEach(() => {
    localStorage.clear();
    document.documentElement.className = "";
    document.documentElement.style.colorScheme = "";
  });

  test("kunci penyimpanan sama dengan yang dibaca skrip tema", async () => {
    const source = await Bun.file(initPath).text();
    expect(source).toContain(`"${THEME_STORAGE_KEY}"`);
  });

  test.each(["light", "dark"] as const)(
    "pilihan eksplisit %s: kelas dan color-scheme sama",
    async (pref) => {
      localStorage.setItem(THEME_STORAGE_KEY, pref);
      await runThemeInit();
      const fromScript = {
        cls: document.documentElement.classList.contains(pref),
        scheme: document.documentElement.style.colorScheme,
      };
      const el = document.createElement("html");
      applyTheme(el, pref);
      expect(fromScript).toEqual({
        cls: el.classList.contains(pref),
        scheme: el.style.colorScheme,
      });
    },
  );

  test("system atau nilai asing: skrip tidak memasang kelas (CSS prefers-color-scheme bekerja)", async () => {
    for (const raw of ["system", "garbage", null]) {
      if (raw === null) localStorage.removeItem(THEME_STORAGE_KEY);
      else localStorage.setItem(THEME_STORAGE_KEY, raw);
      await runThemeInit();
      expect(document.documentElement.classList.contains("light")).toBe(false);
      expect(document.documentElement.classList.contains("dark")).toBe(false);
    }
  });
});

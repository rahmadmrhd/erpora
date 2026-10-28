import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { anonymousUserSource, emptyNotificationSource, safeInternalHref } from "./index";

describe("emptyNotificationSource", () => {
  test("tanpa notifikasi: 0, kosong, no-op", async () => {
    expect(await emptyNotificationSource.unreadCount()).toBe(0);
    expect(await emptyNotificationSource.list({ limit: 20 })).toEqual([]);
    await expect(emptyNotificationSource.markRead("x")).resolves.toBeUndefined();
    await expect(emptyNotificationSource.markAllRead()).resolves.toBeUndefined();
    expect(emptyNotificationSource.subscribe).toBeUndefined();
  });
});

describe("anonymousUserSource", () => {
  test("pengguna anonim; Kelola Akun dan Keluar tidak tersedia", async () => {
    expect(await anonymousUserSource.get()).toBeNull();
    expect(anonymousUserSource.capabilities).toEqual({ manageAccount: false, signOut: false });
    expect(anonymousUserSource.manageAccountPath?.() ?? null).toBeNull();
  });
});

describe("safeInternalHref", () => {
  test("path internal lolos apa adanya", () => {
    for (const ok of ["/", "/example", "/example/notes?x=1#h", "/settings/modules", "/a%2Fb"]) {
      expect(safeInternalHref(ok)).toBe(ok);
    }
  });

  test("skema, protocol-relative, backslash, spasi/kontrol, dan non-string ditolak", () => {
    const bad: unknown[] = [
      "https://evil.example",
      "http://x",
      "javascript:alert(1)",
      "JaVaScRiPt:alert(1)",
      "data:text/html,<script>",
      "//evil.example",
      "/\\evil.example",
      "\\\\evil.example",
      " /ok",
      "/ok ",
      "/a\nb",
      "/a\tb",
      "\t//evil.example",
      "/a\u0000b",
      "example",
      "",
      "#frag",
      "?q=1",
      undefined,
      null,
      0,
      1,
      true,
      {},
      [],
      ["/ok"],
    ];
    for (const href of bad) expect(safeInternalHref(href)).toBeNull();
  });

  test("P9: untuk masukan apa pun hasilnya null atau path internal aman", () => {
    const check = (x: unknown) => {
      const out = safeInternalHref(x);
      if (out === null) return;
      expect(typeof out).toBe("string");
      expect(out.startsWith("/")).toBe(true);
      expect(out.startsWith("//")).toBe(false);
      expect(out.includes("\\")).toBe(false);
      // biome-ignore lint/suspicious/noControlCharactersInRegex: memeriksa karakter kontrol dilarang
      expect(/[\u0000-\u001f\u007f\s]/u.test(out)).toBe(false);
      expect(out).toBe(x as string);
    };
    fc.assert(fc.property(fc.anything(), check), { numRuns: 300 });
    fc.assert(fc.property(fc.string(), check), { numRuns: 500 });
    fc.assert(
      fc.property(
        fc.string(),
        fc.constantFrom("/", "//", "/\\", "\\", "http:", "javascript:"),
        (s, p) => check(`${p}${s}`),
      ),
      { numRuns: 500 },
    );
  });
});

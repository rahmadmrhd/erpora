import { describe, expect, test } from "bun:test";
import { loginHref, safeReturnTo } from "./return-to";

describe("safeReturnTo (C1.14, Q33)", () => {
  test("path relatif internal diterima apa adanya (termasuk query dan fragmen)", () => {
    for (const ok of ["/", "/core/change-password", "/example/notes?page=2#x", "/a/b/c"]) {
      expect(safeReturnTo(ok)).toBe(ok);
    }
  });

  test("bentuk berbahaya ditolak dan kembali ke bawaan /", () => {
    const evil = [
      "//evil.com",
      "///evil.com",
      "https://evil.com",
      "http://evil.com/x",
      "javascript:alert(1)",
      "JaVaScRiPt:alert(1)",
      "data:text/html,x",
      "\\\\evil.com",
      "/\\evil.com",
      "\\/evil.com",
      "/\tevil.com",
      "/\nevil.com",
      " /ok",
      "/ok ",
      "evil.com",
      "",
      "relatif/tanpa-slash",
      undefined,
      null,
      42,
      {},
      `/${"a".repeat(3000)}`,
    ];
    for (const raw of evil) expect(safeReturnTo(raw), String(raw)).toBe("/");
  });

  test("gerbang auth sendiri tidak pernah menjadi tujuan (mencegah pengalihan berputar)", () => {
    for (const p of [
      "/login",
      "/login?returnTo=%2F",
      "/forgot-password",
      "/reset-password#token=x",
    ]) {
      expect(safeReturnTo(p), p).toBe("/");
    }
    expect(safeReturnTo("/loginx")).toBe("/loginx");
  });

  test("loginHref menyandikan tujuan dan menghilangkan tujuan bawaan", () => {
    expect(loginHref("/example/notes?page=2")).toBe(
      "/login?returnTo=%2Fexample%2Fnotes%3Fpage%3D2",
    );
    expect(loginHref("/")).toBe("/login");
    expect(loginHref("//evil.com")).toBe("/login");
    expect(loginHref(undefined)).toBe("/login");
  });
});

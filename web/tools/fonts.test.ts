import { describe, expect, test } from "bun:test";
import { createHash } from "node:crypto";
import { extractInlinedFonts } from "./fonts";

const bytes = Uint8Array.from([0x77, 0x4f, 0x46, 0x32, 1, 2, 3, 4, 5, 250, 251, 252]);
const b64 = Buffer.from(bytes).toString("base64");
const sha8 = createHash("sha256").update(bytes).digest("hex").slice(0, 8);
const face = (url: string): string => `@font-face{font-family:Figtree;src:${url} format("woff2")}`;

describe("extractInlinedFonts", () => {
  test("memindahkan font data: ke berkas ber-hash dan mengganti URL ke /assets/", () => {
    const res = extractInlinedFonts(face(`url(data:font/woff2;base64,${b64})`));
    expect(res.css).toBe(face(`url(/assets/figtree-${sha8}.woff2)`));
    expect(res.fonts).toHaveLength(1);
    expect(res.fonts[0]?.fileName).toBe(`assets/figtree-${sha8}.woff2`);
    expect(Array.from(res.fonts[0]?.data ?? [])).toEqual(Array.from(bytes));
  });

  test("URL ber-kutip ditangani", () => {
    const res = extractInlinedFonts(face(`url("data:font/woff2;base64,${b64}")`));
    expect(res.css).toBe(face(`url(/assets/figtree-${sha8}.woff2)`));
  });

  test("font identik yang muncul dua kali menjadi satu berkas", () => {
    const css = `${face(`url(data:font/woff2;base64,${b64})`)}${face(`url(data:font/woff2;base64,${b64})`)}`;
    const res = extractInlinedFonts(css);
    expect(res.fonts).toHaveLength(1);
    expect(res.css.split(`/assets/figtree-${sha8}.woff2`)).toHaveLength(3);
  });

  test("idempoten: keluaran diproses lagi tidak berubah dan tidak menghasilkan berkas", () => {
    const once = extractInlinedFonts(face(`url(data:font/woff2;base64,${b64})`));
    const twice = extractInlinedFonts(once.css);
    expect(twice.css).toBe(once.css);
    expect(twice.fonts).toEqual([]);
  });

  test("tanpa font: CSS tidak berubah; data: bukan font dibiarkan", () => {
    const css = "body{color:red;background:url(data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=)}";
    const res = extractInlinedFonts(css);
    expect(res.css).toBe(css);
    expect(res.fonts).toEqual([]);
  });
});

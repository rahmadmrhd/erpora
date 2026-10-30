import { describe, expect, test } from "bun:test";
import { cspViolations, cssViolations } from "../csp-check";

describe("cspViolations", () => {
  test("HTML bersih lolos", () => {
    expect(
      cspViolations(
        `<head><script src="/a.js"></script><script type="module" src="/b.js"></script></head>`,
      ),
    ).toEqual([]);
  });
  test("menolak skrip inline", () => {
    expect(cspViolations("<script>alert(1)</script>")).toContain("inline <script>");
  });
  test("menolak skrip src yang juga punya isi", () => {
    expect(cspViolations(`<script src="/a.js">x()</script>`)).toContain("inline <script>");
  });
  test("menolak event handler inline", () => {
    expect(cspViolations(`<img src="/x.png" onerror="x()">`)).toContain("inline event handler");
  });
  test("menolak URL javascript:", () => {
    expect(cspViolations(`<a href="javascript:void(0)">x</a>`)).toContain("javascript: URL");
  });
});

describe("cssViolations", () => {
  test("CSS bersih lolos (font /assets, url relatif, data: gambar)", () => {
    const css = `@font-face{src:url(/assets/figtree-1.woff2)}.a{background:url(data:image/svg+xml;base64,AAAA)}.b{background:url("/assets/x.png")}`;
    expect(cssViolations(css)).toEqual([]);
  });
  test("menolak font data:", () => {
    expect(cssViolations("@font-face{src:url(data:font/woff2;base64,AAAA)}")).toContain(
      "data:font URL",
    );
    expect(cssViolations(`@font-face{src:url( "data:font/woff2;base64,AAAA")}`)).toContain(
      "data:font URL",
    );
  });
  test("menolak URL http(s) dan protokol-relatif", () => {
    expect(cssViolations("@import url(https://fonts.example/x.css)")).toContain("external URL");
    expect(cssViolations(".a{background:url('http://x/y.png')}")).toContain("external URL");
    expect(cssViolations(".a{background:url(//cdn.x/y.png)}")).toContain("external URL");
  });
});

import { describe, expect, test } from "bun:test";
import { injectHead } from "./head";

const page = `<!doctype html><html lang="id"><head><meta charset="utf-8"><link rel="stylesheet" href="/assets/index-a.css"><script type="module" src="/assets/index-a.js"></script></head><body></body></html>`;
const opts = {
  themeInitSrc: "/assets/theme-init-abc.js",
  fontPreloads: ["/assets/figtree-1234abcd.woff2"],
};
const script = `<script src="/assets/theme-init-abc.js"></script>`;
const preload = `<link rel="preload" href="/assets/figtree-1234abcd.woff2" as="font" type="font/woff2" crossorigin>`;

describe("injectHead", () => {
  test("menyisipkan <script src> sinkron tepat setelah <head> dan preload font sesudahnya", () => {
    const out = injectHead(page, opts);
    expect(out).toContain(`<head>${script}${preload}<meta charset="utf-8">`);
  });

  test("skrip tema sinkron: tanpa type=module, async, atau defer", () => {
    const out = injectHead(page, opts);
    const m = out.match(/<script src="\/assets\/theme-init-abc\.js"([^>]*)>/);
    expect(m).not.toBeNull();
    expect(m?.[1]).toBe("");
  });

  test("idempoten", () => {
    const once = injectHead(page, opts);
    expect(injectHead(once, opts)).toBe(once);
  });

  test("<head> beratribut ditangani; tanpa <head> melempar", () => {
    const out = injectHead(`<html><head data-x="1"><title>x</title></head></html>`, opts);
    expect(out).toContain(`<head data-x="1">${script}`);
    expect(() => injectHead("<html></html>", opts)).toThrow();
  });

  test("tanpa preload font hanya menyisipkan skrip", () => {
    const out = injectHead(page, { themeInitSrc: "/assets/t.js", fontPreloads: [] });
    expect(out).toContain(`<head><script src="/assets/t.js"></script><meta`);
    expect(out).not.toContain('rel="preload"');
  });
});

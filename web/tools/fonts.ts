// Ekstraksi font yang disisipkan Bun sebagai `data:` URI di CSS (M3a K-1).
// CSP produksi (`default-src 'self'`) memblokir `font-src data:`, jadi font dipindahkan ke berkas ber-hash.
// Fungsi murni: tidak menyentuh disk; build.ts menulis `fonts` ke `dist/`.
import { createHash } from "node:crypto";

export interface ExtractedFont {
  /** Jalur relatif terhadap `dist/`, mis. `assets/figtree-1a2b3c4d.woff2`. */
  fileName: string;
  data: Uint8Array;
}

export interface ExtractResult {
  css: string;
  fonts: ExtractedFont[];
}

const dataFont = /url\(\s*(["']?)data:font\/(woff2|woff);base64,([A-Za-z0-9+/=]+)\1\s*\)/g;

/**
 * Mengganti `url(data:font/woff2;base64,…)` dengan `url(/assets/<prefix>-<sha8>.woff2)`.
 * Idempoten (keluaran tidak lagi memuat `data:font`); font identik dijadikan satu berkas.
 */
export function extractInlinedFonts(css: string, prefix = "figtree"): ExtractResult {
  const fonts = new Map<string, ExtractedFont>();
  const out = css.replace(dataFont, (_match, _quote: string, ext: string, b64: string) => {
    const data = new Uint8Array(Buffer.from(b64, "base64"));
    const sha8 = createHash("sha256").update(data).digest("hex").slice(0, 8);
    const fileName = `assets/${prefix}-${sha8}.${ext}`;
    if (!fonts.has(fileName)) fonts.set(fileName, { fileName, data });
    return `url(/${fileName})`;
  });
  return { css: out, fonts: [...fonts.values()] };
}

// Injeksi `<head>` pasca-build (M3a K-18): skrip tema sinkron eksternal dan preload font.
// Bun menggabungkan semua <script> HTML menjadi satu modul yang ditunda, sehingga skrip tema
// (harus berjalan sebelum paint pertama, tanpa <script> inline karena CSP) disisipkan setelah build.

export interface HeadOptions {
  /** URL absolut skrip tema, mis. `/assets/theme-init-abc.js`. */
  themeInitSrc: string;
  /** URL absolut font untuk `<link rel="preload" as="font">`. */
  fontPreloads: string[];
}

const attr = (v: string): string => v.replace(/&/g, "&amp;").replace(/"/g, "&quot;");

/**
 * Menyisipkan `<script src>` sinkron (tanpa `type="module"`) tepat setelah `<head>`, lalu preload font.
 * Idempoten: elemen yang sudah ada tidak disisipkan lagi. Melempar bila HTML tidak punya `<head>`.
 */
export function injectHead(html: string, opts: HeadOptions): string {
  const head = /<head(\s[^>]*)?>/i.exec(html);
  if (!head) throw new Error("injectHead: <head> tidak ditemukan");
  const items = [
    `<script src="${attr(opts.themeInitSrc)}"></script>`,
    ...opts.fontPreloads.map(
      (href) => `<link rel="preload" href="${attr(href)}" as="font" type="font/woff2" crossorigin>`,
    ),
  ].filter((item) => !html.includes(item));
  if (items.length === 0) return html;
  const at = head.index + head[0].length;
  return html.slice(0, at) + items.join("") + html.slice(at);
}

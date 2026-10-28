// Pemeriksaan kompatibilitas CSP (script-src 'self'): HTML hasil build tidak boleh memuat
// <script> inline, atribut event handler inline, atau URL javascript:.
export function cspViolations(html: string): string[] {
  const problems: string[] = [];
  for (const m of html.matchAll(/<script\b([^>]*)>([\s\S]*?)<\/script>/gi)) {
    const attrs = m[1] ?? "";
    const body = (m[2] ?? "").trim();
    if (!/\bsrc\s*=/.test(attrs) || body !== "") problems.push("inline <script>");
  }
  if (/<[a-z][^>]*\son[a-z]+\s*=/i.test(html)) problems.push("inline event handler");
  if (/javascript:/i.test(html)) problems.push("javascript: URL");
  return problems;
}

// CSS hasil build harus kompatibel dengan CSP (`default-src 'self'`, tanpa `font-src data:`):
// tanpa font `data:` dan tanpa URL eksternal (http(s):// atau protokol-relatif //).
export function cssViolations(css: string): string[] {
  const problems: string[] = [];
  if (/url\(\s*["']?\s*data:font/i.test(css)) problems.push("data:font URL");
  if (/url\(\s*["']?\s*(?:https?:)?\/\//i.test(css)) problems.push("external URL");
  return problems;
}

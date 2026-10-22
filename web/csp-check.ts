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

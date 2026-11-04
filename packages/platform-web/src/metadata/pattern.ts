// Pencocokan `pattern` di klien (Requirement 2.5, 19.2). Pola sudah lolos validator subset di `metadata:check` (Go,
// sdk/metadata ValidatePattern); di sini hanya dikompilasi ke RegExp mode unicode dengan dua penyesuaian paritas
// terhadap RE2: `.` berarti "selain \n" (JavaScript juga mengecualikan \r, U+2028, U+2029) dan input dibatasi
// panjangnya karena mesin JavaScript dapat melakukan backtracking eksponensial. Server tetap otoritatif.

import { CLIENT_PATTERN_INPUT_LIMIT } from "./limits";

export interface PatternResult {
  /** null bila tidak diuji (lihat `reason`). */
  readonly match: boolean | null;
  readonly reason?: "value_too_long" | "invalid_pattern";
}

/** Mengganti `.` di luar kelas karakter menjadi `[^\n]`; escape dan kelas dilewati apa adanya. */
function translate(pattern: string): string {
  let out = "";
  let inClass = false;
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern[i] as string;
    if (c === "\\") {
      out += c + (pattern[i + 1] ?? "");
      i++;
    } else if (inClass) {
      if (c === "]") inClass = false;
      out += c;
    } else if (c === "[") {
      inClass = true;
      out += c;
    } else {
      out += c === "." ? "[^\\n]" : c;
    }
  }
  return out;
}

const CACHE_LIMIT = 256;
const cache = new Map<string, RegExp | null>();

function compile(pattern: string): RegExp | null {
  const hit = cache.get(pattern);
  if (hit !== undefined) return hit;
  let re: RegExp | null = null;
  try {
    re = new RegExp(translate(pattern), "u");
  } catch {
    re = null;
  }
  if (cache.size >= CACHE_LIMIT) cache.clear();
  cache.set(pattern, re);
  return re;
}

/**
 * Menguji `input` terhadap `pattern` (tidak berjangkar, seperti JSON Schema `pattern`). Fungsi total: input lebih
 * panjang dari CLIENT_PATTERN_INPUT_LIMIT atau pola yang tidak dapat dikompilasi tidak diuji (`match: null`).
 */
export function testPattern(pattern: string, input: string): PatternResult {
  if (typeof pattern !== "string" || typeof input !== "string")
    return { match: null, reason: "invalid_pattern" };
  if (input.length > CLIENT_PATTERN_INPUT_LIMIT) return { match: null, reason: "value_too_long" };
  const re = compile(pattern);
  if (re === null) return { match: null, reason: "invalid_pattern" };
  return { match: re.test(input) };
}

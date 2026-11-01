// Aritmetika desimal eksak berbasis BigInt berskala. Tidak pernah memakai `Number` untuk perbandingan,
// penjumlahan, atau format (Requirement 2.2, 18.7). Perilaku identik dengan sdk/metadata/decimal.go.

import { MAX_DECIMAL_DIGITS } from "./limits";

/**
 * Mode pembulatan eksplisit. `half_*` menentukan perlakuan nilai tepat di tengah: half_up menjauhi nol,
 * half_down mendekati nol, half_even ke digit genap. `up` menjauhi nol, `down` mendekati nol (pemotongan),
 * `floor` ke bawah, `ceiling` ke atas.
 */
export type RoundMode = "half_up" | "half_down" | "half_even" | "up" | "down" | "floor" | "ceiling";

export const ROUND_MODES: readonly RoundMode[] = [
  "half_up",
  "half_down",
  "half_even",
  "up",
  "down",
  "floor",
  "ceiling",
];

export function isRoundMode(v: unknown): v is RoundMode {
  return typeof v === "string" && (ROUND_MODES as readonly string[]).includes(v);
}

/** Bilangan desimal eksak: nilai = coef × 10^-scale; skala dipertahankan (12.50 tetap skala 2). */
export interface Decimal {
  readonly coef: bigint;
  readonly scale: number;
}

const DECIMAL_RE = /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$/;

const POW10: bigint[] = [1n];
function pow10(n: number): bigint {
  for (let i = POW10.length; i <= n; i++) POW10.push((POW10[i - 1] as bigint) * 10n);
  return POW10[n] as bigint;
}

/**
 * Menerima hanya string desimal kanonik `-?(0|[1-9][0-9]*)(\.[0-9]+)?` dengan paling banyak
 * MAX_DECIMAL_DIGITS digit; selain itu (eksponen, tanda plus, nol di depan, spasi) mengembalikan null.
 */
export function parseDecimal(s: string): Decimal | null {
  if (!DECIMAL_RE.test(s)) return null;
  const neg = s.startsWith("-");
  const body = neg ? s.slice(1) : s;
  const dot = body.indexOf(".");
  const digits = dot < 0 ? body : body.slice(0, dot) + body.slice(dot + 1);
  if (digits.length > MAX_DECIMAL_DIGITS) return null;
  const coef = BigInt(digits);
  return { coef: neg ? -coef : coef, scale: dot < 0 ? 0 : body.length - dot - 1 };
}

/** Desimal berskala 0 dari bilangan bulat. */
export function decimalFromInt(n: number | bigint): Decimal {
  return { coef: BigInt(n), scale: 0 };
}

function align(a: Decimal, b: Decimal): [bigint, bigint, number] {
  const scale = Math.max(a.scale, b.scale);
  return [a.coef * pow10(scale - a.scale), b.coef * pow10(scale - b.scale), scale];
}

/** Membandingkan nilai (bukan skala): -1, 0, atau 1. */
export function compareDecimal(a: Decimal, b: Decimal): -1 | 0 | 1 {
  const [x, y] = align(a, b);
  return x === y ? 0 : x < y ? -1 : 1;
}

/** Penjumlahan eksak; skala hasil = skala terbesar kedua operand. */
export function addDecimal(a: Decimal, b: Decimal): Decimal {
  const [x, y, scale] = align(a, b);
  return { coef: x + y, scale };
}

/** Pengurangan eksak; skala hasil = skala terbesar kedua operand. */
export function subDecimal(a: Decimal, b: Decimal): Decimal {
  const [x, y, scale] = align(a, b);
  return { coef: x - y, scale };
}

/** Perkalian eksak; skala hasil = jumlah skala kedua operand. */
export function mulDecimal(a: Decimal, b: Decimal): Decimal {
  return { coef: a.coef * b.coef, scale: a.scale + b.scale };
}

export function isZeroDecimal(a: Decimal): boolean {
  return a.coef === 0n;
}

/** Membulatkan num/den (den != 0) ke bilangan bulat menurut mode. */
function roundQuotient(num: bigint, den: bigint, mode: RoundMode): bigint {
  const neg = num < 0n !== den < 0n;
  const n = num < 0n ? -num : num;
  const d = den < 0n ? -den : den;
  let q = n / d;
  const r = n % d;
  if (r !== 0n) {
    let inc = false;
    switch (mode) {
      case "up":
        inc = true;
        break;
      case "down":
        break;
      case "floor":
        inc = neg;
        break;
      case "ceiling":
        inc = !neg;
        break;
      default: {
        const twice = 2n * r;
        if (twice > d) inc = true;
        else if (twice === d) inc = mode === "half_up" || (mode === "half_even" && (q & 1n) === 1n);
      }
    }
    if (inc) q += 1n;
  }
  return neg ? -q : q;
}

/** Membulatkan ke `scale` digit pecahan (scale >= 0); hasil berskala tepat `scale`. */
export function roundDecimal(a: Decimal, scale: number, mode: RoundMode): Decimal {
  const s = Math.max(0, scale);
  if (a.scale <= s) return { coef: a.coef * pow10(s - a.scale), scale: s };
  return { coef: roundQuotient(a.coef, pow10(a.scale - s), mode), scale: s };
}

/** Membagi a dengan b dan membulatkan ke `scale` digit pecahan; null bila b nol. */
export function divDecimal(a: Decimal, b: Decimal, scale: number, mode: RoundMode): Decimal | null {
  if (b.coef === 0n) return null;
  const s = Math.max(0, scale);
  const k = b.scale - a.scale + s;
  const num = k >= 0 ? a.coef * pow10(k) : a.coef;
  const den = k >= 0 ? b.coef : b.coef * pow10(-k);
  return { coef: roundQuotient(num, den, mode), scale: s };
}

/** Menulis dengan seluruh digit pecahan yang dipertahankan (12.50 tetap "12.50"); nol tidak pernah bertanda minus. */
export function formatDecimal(a: Decimal): string {
  const neg = a.coef < 0n;
  let digits = (neg ? -a.coef : a.coef).toString();
  if (a.scale > 0) {
    digits = digits.padStart(a.scale + 1, "0");
    digits = `${digits.slice(0, digits.length - a.scale)}.${digits.slice(digits.length - a.scale)}`;
  }
  return neg ? `-${digits}` : digits;
}

// Format tampilan desimal (Requirement 18.7): nilai decimal TIDAK PERNAH dikonversi ke `Number`. Digit berasal dari
// utilitas desimal (string kanonik, BigInt berskala); `Intl` hanya dipakai untuk mengetahui pola bahasa (pemisah
// ribuan dan desimal, simbol mata uang, posisi tanda) lewat angka probe, lalu digit string disisipkan ke pola itu.
// Pengelompokan ribuan tiga digit (pola Indonesia dan Inggris); bahasa dengan pola lain jatuh ke tiga digit.

import { formatDecimal, parseDecimal } from "./decimal";

export interface FormatDecimalOptions {
  /** Skala tampilan minimum (mis. skala field); digit nilai yang lebih banyak tidak pernah dipotong. */
  readonly scale?: number;
  /** Kode mata uang ISO 4217 (mis. "IDR"); simbol dan posisinya mengikuti bahasa. */
  readonly currency?: string;
}

const probes = new Map<string, readonly Intl.NumberFormatPart[]>();

function probe(
  locale: string,
  scale: number,
  currency: string | undefined,
  negative: boolean,
): readonly Intl.NumberFormatPart[] {
  const id = `${locale}|${scale}|${currency ?? ""}|${negative}`;
  let parts = probes.get(id);
  if (!parts) {
    const base: Intl.NumberFormatOptions = {
      minimumFractionDigits: scale,
      maximumFractionDigits: scale,
      useGrouping: true,
      ...(currency ? { style: "currency", currency, currencyDisplay: "narrowSymbol" } : {}),
    };
    let nf: Intl.NumberFormat;
    try {
      nf = new Intl.NumberFormat(locale, base);
    } catch {
      nf = new Intl.NumberFormat("en", base); // bahasa atau mata uang tak dikenal: jangan melempar
    }
    parts = nf.formatToParts(negative ? -1234567.5 : 1234567.5);
    probes.set(id, parts);
  }
  return parts;
}

/**
 * Memformat string desimal kanonik untuk tampilan. Mengembalikan null bila masukan bukan desimal kanonik (tidak
 * ada konversi Number diam-diam). Fungsi total.
 */
export function formatDecimalString(
  value: string,
  locale: string,
  options: FormatDecimalOptions = {},
): string | null {
  if (typeof value !== "string") return null;
  const d = parseDecimal(value);
  if (d === null) return null;
  // pad ke skala yang diminta tanpa memotong digit nilai
  const scale = Math.max(d.scale, Math.min(64, Math.max(0, Math.trunc(options.scale ?? 0))));
  const padded = formatDecimal({ coef: d.coef * 10n ** BigInt(scale - d.scale), scale });
  const negative = padded.startsWith("-");
  const body = negative ? padded.slice(1) : padded;
  const dot = body.indexOf(".");
  const intDigits = dot < 0 ? body : body.slice(0, dot);
  const fracDigits = dot < 0 ? "" : body.slice(dot + 1);

  const parts = probe(locale, scale, options.currency, negative);
  let out = "";
  let wroteInt = false;
  for (const p of parts) {
    switch (p.type) {
      case "integer":
        if (!wroteInt) {
          out += groupDigits(intDigits, parts.find((x) => x.type === "group")?.value ?? ",");
          wroteInt = true;
        }
        break;
      case "group":
        break;
      case "fraction":
        out += fracDigits;
        break;
      default:
        out += p.value;
    }
  }
  return out;
}

function groupDigits(digits: string, sep: string): string {
  let out = "";
  for (let i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 === 0) out += sep;
    out += digits[i];
  }
  return out;
}

// Format tampilan desimal (Requirement 18.7): nilai decimal TIDAK PERNAH dikonversi ke `Number`. Digit berasal dari
// utilitas desimal (string kanonik, BigInt berskala); `Intl` hanya dipakai untuk mengetahui pola bahasa (pemisah
// ribuan dan desimal, simbol mata uang, posisi tanda) lewat angka probe, lalu digit string disisipkan ke pola itu.
// Pengelompokan ribuan tiga digit (pola Indonesia dan Inggris); bahasa dengan pola lain jatuh ke tiga digit.
// Fungsi total: bahasa, mata uang, atau skala yang rusak tidak pernah melempar (jatuh ke pola yang lebih sederhana).

import { formatDecimal, parseDecimal } from "./decimal";

export interface FormatDecimalOptions {
  /** Skala tampilan minimum (mis. skala field), 0 sampai 64; digit nilai yang lebih banyak tidak pernah dipotong. */
  readonly scale?: number;
  /** Kode mata uang ISO 4217 (mis. "IDR"); simbol dan posisinya mengikuti bahasa. Kode tak sah diabaikan. */
  readonly currency?: string;
}

const MAX_DISPLAY_SCALE = 64;

const probes = new Map<string, readonly Intl.NumberFormatPart[] | null>();

/** Probe pola bahasa; null bila tidak ada pola Intl yang dapat dipakai (pemanggil memakai pola dasar). */
function probe(
  locale: string,
  scale: number,
  currency: string | undefined,
  negative: boolean,
): readonly Intl.NumberFormatPart[] | null {
  const id = `${locale}|${scale}|${currency ?? ""}|${negative}`;
  const hit = probes.get(id);
  if (hit !== undefined) return hit;
  const plain: Intl.NumberFormatOptions = {
    minimumFractionDigits: scale,
    maximumFractionDigits: scale,
    useGrouping: true,
  };
  const withCurrency: Intl.NumberFormatOptions | null = currency
    ? { ...plain, style: "currency", currency, currencyDisplay: "narrowSymbol" }
    : null;
  // urutan: bahasa + mata uang, bahasa saja, en + mata uang, en saja
  const attempts: [string, Intl.NumberFormatOptions][] = [];
  if (withCurrency) attempts.push([locale, withCurrency]);
  attempts.push([locale, plain]);
  if (withCurrency) attempts.push(["en", withCurrency]);
  attempts.push(["en", plain]);
  let parts: readonly Intl.NumberFormatPart[] | null = null;
  for (const [loc, opts] of attempts) {
    try {
      parts = new Intl.NumberFormat(loc, opts).formatToParts(negative ? -1234567.5 : 1234567.5);
      break;
    } catch {
      // coba pola berikutnya
    }
  }
  if (probes.size > 512) probes.clear();
  probes.set(id, parts);
  return parts;
}

function groupDigits(digits: string, sep: string): string {
  let out = "";
  for (let i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 === 0) out += sep;
    out += digits[i];
  }
  return out;
}

/**
 * Memformat string desimal kanonik untuk tampilan. Mengembalikan null bila masukan bukan desimal kanonik (tidak
 * ada konversi Number diam-diam). Fungsi total: selain masukan yang bukan desimal, selalu mengembalikan string.
 */
export function formatDecimalString(
  value: string,
  locale: string,
  options: FormatDecimalOptions = {},
): string | null {
  if (typeof value !== "string") return null;
  const d = parseDecimal(value);
  if (d === null) return null;
  const requested = options?.scale;
  const req =
    typeof requested === "number" && Number.isFinite(requested)
      ? Math.min(MAX_DISPLAY_SCALE, Math.max(0, Math.trunc(requested)))
      : 0;
  // pad ke skala yang diminta tanpa memotong digit nilai
  const scale = Math.max(d.scale, req);
  const padded = formatDecimal({ coef: d.coef * 10n ** BigInt(scale - d.scale), scale });
  const negative = padded.startsWith("-");
  const body = negative ? padded.slice(1) : padded;
  const dot = body.indexOf(".");
  const intDigits = dot < 0 ? body : body.slice(0, dot);
  const fracDigits = dot < 0 ? "" : body.slice(dot + 1);

  const currency = typeof options?.currency === "string" ? options.currency : undefined;
  const parts = probe(typeof locale === "string" ? locale : "en", scale, currency, negative);
  if (parts === null) {
    // pola dasar tanpa Intl (tidak seharusnya tercapai)
    return `${negative ? "-" : ""}${groupDigits(intDigits, ",")}${fracDigits ? `.${fracDigits}` : ""}`;
  }
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

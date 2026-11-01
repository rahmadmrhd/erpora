// Koersi nilai bertipe, perbandingan, dan utilitas teks/tanggal. Perilaku identik dengan sdk/metadata (value.go).

import {
  compareDecimal,
  type Decimal,
  decimalFromInt,
  formatDecimal,
  parseDecimal,
} from "./decimal";
import { MAX_STRING_LEN } from "./limits";
import { REASON, type ReasonCode, type ValueType } from "./types";

/** Keluarga nilai; dua nilai hanya dapat dibandingkan bila keluarganya sama (integer dan decimal sama-sama "num"). */
export type Kind = "null" | "bool" | "num" | "text" | "enum" | "uuid" | "date" | "datetime";

export interface Value {
  readonly k: Kind;
  /** Tipe terdeklarasi (untuk tipe hasil ekspresi). */
  readonly ft: ValueType;
  readonly b: boolean;
  readonly num: Decimal;
  readonly isInt: boolean;
  /** Teks, enum, uuid, serta bentuk kanonik date dan datetime. */
  readonly s: string;
  /** date: hari sejak 1970-01-01. */
  readonly day: number;
  /** datetime: detik sejak epoch dan nanodetik. */
  readonly sec: number;
  readonly nsec: number;
}

const BASE: Value = {
  k: "null",
  ft: "null",
  b: false,
  num: { coef: 0n, scale: 0 },
  isInt: false,
  s: "",
  day: 0,
  sec: 0,
  nsec: 0,
};

export const NULL_VALUE: Value = BASE;

export function mkValue(v: Partial<Value> & Pick<Value, "k" | "ft">): Value {
  return { ...BASE, ...v };
}

export function familyOf(ft: string): Kind | null {
  switch (ft) {
    case "string":
    case "text":
      return "text";
    case "integer":
    case "decimal":
      return "num";
    case "boolean":
      return "bool";
    case "date":
      return "date";
    case "datetime":
      return "datetime";
    case "enum":
      return "enum";
    case "uuid":
      return "uuid";
  }
  return null;
}

const LONE_SURROGATE = /[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/;
const LONE_SURROGATE_ALL = new RegExp(LONE_SURROGATE.source, "g");

/** Mengganti surrogat tunggal dengan U+FFFD (setara byte UTF-8 tidak sah di Go) agar urutan dan kesamaan identik. */
export function normText(s: string): string {
  return LONE_SURROGATE.test(s) ? s.replace(LONE_SURROGATE_ALL, "�") : s;
}

/** Panjang melebihi MAX_STRING_LEN satuan UTF-16. */
export function tooLong(s: string): boolean {
  return s.length > MAX_STRING_LEN;
}

const DATE_RE = /^([0-9]{4})-([0-9]{2})-([0-9]{2})$/;
const DATETIME_RE =
  /^([0-9]{4}-[0-9]{2}-[0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]{1,9}))?Z$/;
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const MONTH_DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

function isLeap(y: number): boolean {
  return y % 4 === 0 && (y % 100 !== 0 || y % 400 === 0);
}

/** days_from_civil (Howard Hinnant); tahun dibatasi 1..9999 oleh pemanggil. */
function daysFromCivil(year: number, m: number, d: number): number {
  const y = m <= 2 ? year - 1 : year;
  const era = Math.floor(y / 400);
  const yoe = y - era * 400;
  const mp = (m + 9) % 12;
  const doy = Math.floor((153 * mp + 2) / 5) + d - 1;
  const doe = yoe * 365 + Math.floor(yoe / 4) - Math.floor(yoe / 100) + doy;
  return era * 146097 + doe - 719468;
}

/** Memvalidasi YYYY-MM-DD (tahun 0001..9999, kalender Gregorian); mengembalikan hari sejak epoch atau null. */
export function parseDate(s: string): number | null {
  const m = DATE_RE.exec(s);
  if (!m) return null;
  const y = Number(m[1]);
  const mo = Number(m[2]);
  const d = Number(m[3]);
  if (y < 1 || mo < 1 || mo > 12 || d < 1) return null;
  const dim = mo === 2 && isLeap(y) ? 29 : (MONTH_DAYS[mo - 1] as number);
  if (d > dim) return null;
  return daysFromCivil(y, mo, d);
}

/** Memvalidasi YYYY-MM-DDTHH:MM:SS[.d{1,9}]Z (UTC); mengembalikan detik dan nanodetik atau null. */
export function parseDateTime(s: string): { sec: number; nsec: number } | null {
  const m = DATETIME_RE.exec(s);
  if (!m) return null;
  const day = parseDate(m[1] as string);
  if (day === null) return null;
  const h = Number(m[2]);
  const mi = Number(m[3]);
  const se = Number(m[4]);
  if (h > 23 || mi > 59 || se > 59) return null;
  const frac = m[5];
  return {
    sec: day * 86400 + h * 3600 + mi * 60 + se,
    nsec: frac === undefined ? 0 : Number(frac.padEnd(9, "0")),
  };
}

/**
 * Mengoersi nilai mentah menurut tipe terdeklarasi. Mengembalikan nilai atau kode alasan bila tidak sah.
 * decimal hanya menerima string (tidak pernah `number`; Requirement 18.7).
 */
export function coerce(ft: string, raw: unknown): Value | ReasonCode {
  const fam = familyOf(ft);
  if (fam === null) return REASON.typeMismatch;
  if (raw === null) return NULL_VALUE;
  const t = ft as ValueType;
  switch (ft) {
    case "string":
    case "text":
    case "enum": {
      if (typeof raw !== "string") return REASON.typeMismatch;
      if (tooLong(raw)) return REASON.valueTooLong;
      return mkValue({ k: fam, ft: t, s: normText(raw) });
    }
    case "uuid":
      if (typeof raw !== "string" || !UUID_RE.test(raw)) return REASON.typeMismatch;
      return mkValue({ k: fam, ft: t, s: raw });
    case "integer": {
      let n: number | null = null;
      if (typeof raw === "number" && Number.isSafeInteger(raw)) n = raw;
      else if (typeof raw === "bigint" && raw >= -9007199254740991n && raw <= 9007199254740991n)
        n = Number(raw);
      if (n === null) return REASON.typeMismatch;
      return mkValue({ k: fam, ft: t, num: decimalFromInt(n), isInt: true });
    }
    case "decimal": {
      if (typeof raw !== "string") return REASON.typeMismatch;
      const d = parseDecimal(raw);
      if (d === null) return REASON.typeMismatch;
      return mkValue({ k: fam, ft: t, num: d });
    }
    case "boolean":
      if (typeof raw !== "boolean") return REASON.typeMismatch;
      return mkValue({ k: fam, ft: t, b: raw });
    case "date": {
      if (typeof raw !== "string") return REASON.typeMismatch;
      const day = parseDate(raw);
      if (day === null) return REASON.typeMismatch;
      return mkValue({ k: fam, ft: t, s: raw, day });
    }
    case "datetime": {
      if (typeof raw !== "string") return REASON.typeMismatch;
      const dt = parseDateTime(raw);
      if (dt === null) return REASON.typeMismatch;
      return mkValue({ k: fam, ft: t, s: raw, sec: dt.sec, nsec: dt.nsec });
    }
  }
  return REASON.typeMismatch;
}

/** Kunci urutan: surrogat (U+10000 ke atas) setelah seluruh BMP, sehingga urutan unit UTF-16 = urutan code point. */
function fix(c: number): number {
  return c >= 0xe000 ? c - 0x800 : c >= 0xd800 ? c + 0x2000 : c;
}

/** Urutan code point (identik dengan urutan byte UTF-8) atas string yang sudah dinormalkan. */
export function compareText(a: string, b: string): -1 | 0 | 1 {
  const n = Math.min(a.length, b.length);
  for (let i = 0; i < n; i++) {
    const ca = a.charCodeAt(i);
    const cb = b.charCodeAt(i);
    if (ca !== cb) return fix(ca) < fix(cb) ? -1 : 1;
  }
  return a.length === b.length ? 0 : a.length < b.length ? -1 : 1;
}

function cmpNum(a: number, b: number): -1 | 0 | 1 {
  return a < b ? -1 : a > b ? 1 : 0;
}

/** Membandingkan dua nilai bukan-null dari keluarga yang sama. */
export function cmpValues(a: Value, b: Value): -1 | 0 | 1 {
  switch (a.k) {
    case "num":
      return compareDecimal(a.num, b.num);
    case "bool":
      return a.b === b.b ? 0 : a.b ? 1 : -1;
    case "date":
      return cmpNum(a.day, b.day);
    case "datetime": {
      const c = cmpNum(a.sec, b.sec);
      return c !== 0 ? c : cmpNum(a.nsec, b.nsec);
    }
  }
  return compareText(a.s, b.s);
}

/** Mengubah hanya huruf ASCII A-Z menjadi huruf kecil (paritas deterministik dengan Go). */
export function asciiFold(s: string): string {
  return s.replace(/[A-Z]/g, (c) => String.fromCharCode(c.charCodeAt(0) + 32));
}

/** Teks kanonik nilai (untuk concat dan templat label); null menjadi "". */
export function valueText(v: Value): string {
  switch (v.k) {
    case "bool":
      return v.b ? "true" : "false";
    case "num":
      return formatDecimal(v.num);
    case "text":
    case "enum":
    case "uuid":
    case "date":
    case "datetime":
      return v.s;
  }
  return "";
}

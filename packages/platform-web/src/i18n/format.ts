// Format pesan ICU MessageFormat di frontend (design.md bagian 9; Requirement 2, 9).
//
// Pembungkus tipis di atas `intl-messageformat` (FormatJS) tanpa ketergantungan pada React:
//   compile(locale, source) -> Compiled   (di-cache per (locale, source); melempar I18nFormatError)
//   format(compiled, args)  -> string     (teks polos; tag rich dilucuti)
//   formatRich(compiled, args, tags)      (tag rich -> fungsi pemanggil; adaptor React ditulis kemudian)
//
// Kebijakan yang menyamakan keluaran dengan sisi Go (sdk/i18n; fixture sdk/i18n/testdata/conformance.json):
//   - Argumen yang tidak ada (bukan properti milik `args`) atau bernilai null/undefined dianggap hilang:
//     seluruh elemen {x}, {x, plural, ...}, atau {x, select, ...} diganti literal "{x}" SEBELUM diformat
//     (FormatJS sendiri melempar MissingValueError). format/formatRich tidak melempar untuk argumen hilang.
//   - Angka/tanggal/mata uang diformat di kode lalu diteruskan sebagai teks (Requirement 2.6): konstruksi
//     number/date/time, selectordinal, dan offset plural ditolak saat compile, sama seperti Go.
//   - Teks polos: tag rich dilucuti (anaknya tetap); tag self-closing `<br/>` tetap literal (perilaku parser).

import {
  createLiteralElement,
  type MessageFormatElement,
  type PluralOrSelectOption,
  TYPE,
} from "@formatjs/icu-messageformat-parser";
import { IntlMessageFormat } from "intl-messageformat";

/** Nilai argumen yang diterima. Angka/tanggal diformat pemanggil menjadi string (Requirement 2.6). */
export type ArgValue = string | number | boolean | null | undefined;
export type Args = Readonly<Record<string, ArgValue>>;

/** Pemetaan nama tag rich ke fungsi yang membungkus anak-anaknya menjadi satu simpul tampilan `T`. */
export type RichTags<T> = Readonly<Record<string, (chunks: Array<string | T>) => T>>;

/** Pesan tidak dapat dikompilasi: sintaks ICU tidak sah, konstruksi tak didukung, atau locale tidak sah. */
export class I18nFormatError extends Error {
  override readonly name = "I18nFormatError";
  constructor(message: string, options?: { cause?: unknown }) {
    super(message, options);
  }
}

/** Hasil kompilasi satu pesan. Dibuat lewat `compile`; bidang internal tidak untuk dipakai pemanggil. */
export class Compiled {
  /** @internal */ readonly ast: MessageFormatElement[];
  /** @internal */ readonly argNames: ReadonlySet<string>;
  /** @internal */ readonly tagNames: ReadonlySet<string>;
  /** @internal nama argumen yang dipakai sebagai `{nama}` polos (bool diubah menjadi teks). */
  readonly plainArgNames: ReadonlySet<string>;
  /** @internal pesan lengkap dengan tag utuh (untuk formatRich). */
  readonly full: IntlMessageFormat;
  /** @internal pesan dengan semua tag dilucuti (untuk format teks polos). */
  readonly plain: IntlMessageFormat;
  /** @internal ast tanpa tag, dasar patch argumen hilang pada mode teks polos. */
  readonly plainAst: MessageFormatElement[];

  constructor(
    readonly locale: string,
    readonly source: string,
    ast: MessageFormatElement[],
    argNames: ReadonlySet<string>,
    tagNames: ReadonlySet<string>,
    plainArgNames: ReadonlySet<string>,
    full: IntlMessageFormat,
  ) {
    this.ast = ast;
    this.argNames = argNames;
    this.tagNames = tagNames;
    this.plainArgNames = plainArgNames;
    this.full = full;
    this.plainAst = tagNames.size === 0 ? ast : transform(ast, none, none);
    this.plain = tagNames.size === 0 ? full : new IntlMessageFormat(this.plainAst, locale);
  }
}

const none = (): boolean => false;
const maxCacheEntries = 2000;
const cache = new Map<string, Compiled>();

/**
 * Mengompilasi `source` untuk `locale`. Hasil di-cache per (locale, source) sehingga panggilan berulang
 * murah dan mengembalikan objek yang sama. Melempar I18nFormatError bila pesan tidak sah.
 */
export function compile(locale: string, source: string): Compiled {
  const key = `${locale}\u0000${source}`;
  const hit = cache.get(key);
  if (hit) return hit;

  let compiled: Compiled;
  try {
    const full = new IntlMessageFormat(source, locale);
    const ast = full.getAst();
    const argNames = new Set<string>();
    const tagNames = new Set<string>();
    const plainArgNames = new Set<string>();
    scan(ast, argNames, tagNames, plainArgNames);
    compiled = new Compiled(locale, source, ast, argNames, tagNames, plainArgNames, full);
  } catch (err) {
    if (err instanceof I18nFormatError) throw err;
    throw new I18nFormatError(
      `pesan ICU tidak sah untuk locale ${JSON.stringify(locale)}: ${err instanceof Error ? err.message : String(err)}`,
      { cause: err },
    );
  }

  if (cache.size >= maxCacheEntries) {
    // Buang entri tertua (urutan penyisipan Map); cukup untuk membatasi memori.
    const oldest = cache.keys().next();
    if (!oldest.done) cache.delete(oldest.value);
  }
  cache.set(key, compiled);
  return compiled;
}

const warnedMissing = new Set<string>();

/** Mengosongkan catatan peringatan argumen hilang (untuk test). */
export function resetFormatWarnings(): void {
  warnedMissing.clear();
}

/** Peringatan dev (Requirement 2.3) satu kali per (bahasa, pesan, argumen); tidak ada di produksi. */
function warnMissing(compiled: Compiled, missing: ReadonlySet<string>): void {
  if (process.env.NODE_ENV === "production") return;
  for (const name of missing) {
    const id = `${compiled.locale}\u0000${compiled.source}\u0000${name}`;
    if (warnedMissing.has(id)) continue;
    warnedMissing.add(id);
    console.warn(
      `[erpora] i18n (${compiled.locale}): argumen "${name}" hilang pada pesan "${compiled.source}"; tampil sebagai {${name}}`,
    );
  }
}

/** Memformat menjadi teks polos. Tag rich dilucuti; argumen hilang tampil apa adanya sebagai `{nama}`. */
export function format(compiled: Compiled, args: Args = {}): string {
  const missing = missingArgs(compiled, args);
  warnMissing(compiled, missing);
  const mf =
    missing.size === 0
      ? compiled.plain
      : new IntlMessageFormat(
          transform(compiled.plainAst, (name) => missing.has(name), none),
          compiled.locale,
        );
  const out = mf.format(prepareArgs(compiled, args));
  return typeof out === "string" ? out : String(out);
}

/**
 * Memformat dengan tag rich: setiap `<nama>…</nama>` dipanggil lewat `tags[nama]` dengan anak-anaknya.
 * Tag tanpa pemanggil dilucuti (anaknya tetap). Mengembalikan potongan teks dan simpul `T` berurutan.
 */
export function formatRich<T>(
  compiled: Compiled,
  args: Args,
  tags: RichTags<T>,
): Array<string | T> {
  const missing = missingArgs(compiled, args);
  warnMissing(compiled, missing);
  // Bila nama tag bentrok dengan nama argumen, argumen menang dan tag dilucuti.
  const handled = (name: string): boolean =>
    Object.hasOwn(tags, name) && !compiled.argNames.has(name);
  let allHandled = true;
  for (const name of compiled.tagNames) {
    if (!handled(name)) allHandled = false;
  }
  const mf =
    missing.size === 0 && allHandled
      ? compiled.full
      : new IntlMessageFormat(
          transform(compiled.ast, (name) => missing.has(name), handled),
          compiled.locale,
        );
  const values: Record<string, unknown> = { ...prepareArgs(compiled, args) };
  for (const name of compiled.tagNames) {
    if (handled(name)) values[name] = tags[name];
  }
  const out = mf.format<T>(values as Parameters<typeof mf.format<T>>[0]);
  if (Array.isArray(out)) return out;
  return out === "" ? [] : [out];
}

/**
 * Nilai bool pada `{nama}` polos menjadi "true"/"false" (FormatJS menjadikan false teks kosong dan true
 * objek; Go memilih teks, lihat sdk/i18n/icu_number.go). Pada select tetap bermakna lewat kunci "true"/"false".
 */
function prepareArgs(compiled: Compiled, args: Args): Args {
  let copy: Record<string, ArgValue> | undefined;
  for (const name of compiled.plainArgNames) {
    if (Object.hasOwn(args, name) && typeof args[name] === "boolean") {
      copy ??= { ...args };
      copy[name] = String(args[name]);
    }
  }
  return copy ?? args;
}

function missingArgs(compiled: Compiled, args: Args): Set<string> {
  const missing = new Set<string>();
  for (const name of compiled.argNames) {
    if (!Object.hasOwn(args, name) || args[name] === null || args[name] === undefined) {
      missing.add(name);
    }
  }
  return missing;
}

/** Mengumpulkan nama argumen dan tag, dan menolak konstruksi yang tidak didukung (Requirement 2.1, 2.6). */
function scan(
  els: MessageFormatElement[],
  argNames: Set<string>,
  tagNames: Set<string>,
  plainArgNames: Set<string>,
): void {
  for (const el of els) {
    switch (el.type) {
      case TYPE.literal:
      case TYPE.pound:
        break;
      case TYPE.argument:
        argNames.add(el.value);
        plainArgNames.add(el.value);
        break;
      case TYPE.number:
      case TYPE.date:
      case TYPE.time:
        throw new I18nFormatError(
          `{${el.value}, ...}: number/date/time tidak didukung; format angka dan tanggal di kode`,
        );
      case TYPE.select:
        argNames.add(el.value);
        for (const opt of Object.values(el.options))
          scan(opt.value, argNames, tagNames, plainArgNames);
        break;
      case TYPE.plural:
        if (el.pluralType === "ordinal") {
          throw new I18nFormatError(`{${el.value}, selectordinal, ...} tidak didukung`);
        }
        if (el.offset) {
          throw new I18nFormatError(`{${el.value}, plural, offset:...} tidak didukung`);
        }
        argNames.add(el.value);
        for (const opt of Object.values(el.options))
          scan(opt.value, argNames, tagNames, plainArgNames);
        break;
      case TYPE.tag:
        tagNames.add(el.value);
        scan(el.children, argNames, tagNames, plainArgNames);
        break;
    }
  }
}

/**
 * Menyalin AST dengan: elemen argumen/plural/select yang argumennya hilang diganti literal `{nama}`
 * (seluruh blok, termasuk cabangnya), dan tag yang `keepTag(nama)` bernilai false dilucuti.
 */
function transform(
  els: MessageFormatElement[],
  isMissing: (name: string) => boolean,
  keepTag: (name: string) => boolean,
): MessageFormatElement[] {
  return els.flatMap((el): MessageFormatElement[] => {
    switch (el.type) {
      case TYPE.literal:
      case TYPE.pound:
        return [el];
      case TYPE.argument:
      case TYPE.number:
      case TYPE.date:
      case TYPE.time:
        return isMissing(el.value) ? [createLiteralElement(`{${el.value}}`)] : [el];
      case TYPE.select:
      case TYPE.plural: {
        if (isMissing(el.value)) return [createLiteralElement(`{${el.value}}`)];
        const options: Record<string, PluralOrSelectOption> = {};
        for (const [key, opt] of Object.entries(el.options)) {
          options[key] = { ...opt, value: transform(opt.value, isMissing, keepTag) };
        }
        return [{ ...el, options } as MessageFormatElement];
      }
      case TYPE.tag: {
        const children = transform(el.children, isMissing, keepTag);
        return keepTag(el.value) ? [{ ...el, children }] : children;
      }
      default:
        return [el];
    }
  });
}

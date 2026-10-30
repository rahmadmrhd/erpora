import { Fragment, type ReactNode, useMemo } from "react";
import { type Args, compile, format, formatRich, type RichTags } from "./format";
import type { Translator } from "./types";

type Messages = Readonly<Record<string, string>>;

function source(messages: Messages, key: string): string | undefined {
  return Object.hasOwn(messages, key) ? messages[key] : undefined;
}

const warned = new Set<string>();

/** Mengosongkan catatan peringatan (untuk test). */
export function resetTranslateWarnings(): void {
  warned.clear();
}

/** Peringatan dev satu kali per (bahasa, ns, kunci); tidak ada di produksi. */
function warnOnce(locale: string, ns: string, key: string, reason: string): void {
  if (process.env.NODE_ENV === "production") return;
  const id = `${locale}\u0000${ns}\u0000${key}`;
  if (warned.has(id)) return;
  warned.add(id);
  console.warn(`[erpora] i18n (${locale}) ${ns}::${key}: ${reason}`);
}

/**
 * Teks polos untuk `key`. Kunci tidak ada atau pesan yang gagal dikompilasi (`I18nFormatError`) mengembalikan
 * `ns::kunci` (paritas dengan Go, Requirement 3.3) dan mencatat peringatan dev sekali; tidak pernah melempar.
 */
export function translate(
  locale: string,
  ns: string,
  messages: Messages,
  key: string,
  args?: Readonly<Record<string, unknown>>,
): string {
  const src = source(messages, key);
  if (src === undefined) {
    warnOnce(locale, ns, key, "kunci tidak ada di katalog");
    return `${ns}::${key}`;
  }
  try {
    return format(compile(locale, src), (args ?? {}) as Args);
  } catch {
    warnOnce(locale, ns, key, "pesan tidak dapat diformat");
    return `${ns}::${key}`;
  }
}

/** Seperti `translate` tetapi tag rich dirender lewat fungsi tag pada `args` (nilai berupa fungsi). */
export function translateRich(
  locale: string,
  ns: string,
  messages: Messages,
  key: string,
  args?: Readonly<Record<string, unknown>>,
): ReactNode {
  const src = source(messages, key);
  if (src === undefined) {
    warnOnce(locale, ns, key, "kunci tidak ada di katalog");
    return `${ns}::${key}`;
  }
  try {
    const plain: Record<string, unknown> = {};
    const tags: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(args ?? {})) {
      if (typeof v === "function") tags[k] = v;
      else plain[k] = v;
    }
    const parts = formatRich<ReactNode>(
      compile(locale, src),
      plain as Args,
      tags as RichTags<ReactNode>,
    );
    // Potongan tidak punya key alami: bungkus per indeks (urutan potongan tetap untuk satu pesan).
    return (
      <>
        {parts.map((part, i) => (
          <Fragment key={i}>{part}</Fragment>
        ))}
      </>
    );
  } catch {
    warnOnce(locale, ns, key, "pesan tidak dapat diformat");
    return `${ns}::${key}`;
  }
}

/** Membentuk `Translator` untuk satu namespace; stabil selama `locale`, `messages`, dan `setLocale` tidak berubah. */
export function useTranslator<K>(
  locale: string,
  ns: string,
  messages: Messages,
  setLocale: (locale: string) => void,
): Translator<K> {
  return useMemo(
    () =>
      ({
        t: (key: string, args?: Readonly<Record<string, unknown>>) =>
          translate(locale, ns, messages, key, args),
        rich: (key: string, args?: Readonly<Record<string, unknown>>) =>
          translateRich(locale, ns, messages, key, args),
        locale,
        setLocale,
      }) as unknown as Translator<K>,
    [locale, ns, messages, setLocale],
  );
}

import { Fragment, type ReactNode, useMemo } from "react";
import { type Args, compile, format, formatRich, type RichTags } from "./format";
import type { Translator } from "./types";

type Messages = Readonly<Record<string, string>>;

function source(messages: Messages, key: string): string | undefined {
  return Object.hasOwn(messages, key) ? messages[key] : undefined;
}

/**
 * Teks polos untuk `key`. Kunci tidak ada atau pesan yang gagal dikompilasi (`I18nFormatError`) mengembalikan
 * kunci mentah; tidak pernah melempar.
 */
export function translate(
  locale: string,
  messages: Messages,
  key: string,
  args?: Readonly<Record<string, unknown>>,
): string {
  const src = source(messages, key);
  if (src === undefined) return key;
  try {
    return format(compile(locale, src), (args ?? {}) as Args);
  } catch {
    return key;
  }
}

/** Seperti `translate` tetapi tag rich dirender lewat fungsi tag pada `args` (nilai berupa fungsi). */
export function translateRich(
  locale: string,
  messages: Messages,
  key: string,
  args?: Readonly<Record<string, unknown>>,
): ReactNode {
  const src = source(messages, key);
  if (src === undefined) return key;
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
    return key;
  }
}

/** Membentuk `Translator` untuk satu namespace; stabil selama `locale`, `messages`, dan `setLocale` tidak berubah. */
export function useTranslator<K>(
  locale: string,
  messages: Messages,
  setLocale: (locale: string) => void,
): Translator<K> {
  return useMemo(
    () =>
      ({
        t: (key: string, args?: Readonly<Record<string, unknown>>) =>
          translate(locale, messages, key, args),
        rich: (key: string, args?: Readonly<Record<string, unknown>>) =>
          translateRich(locale, messages, key, args),
        locale,
        setLocale,
      }) as unknown as Translator<K>,
    [locale, messages, setLocale],
  );
}

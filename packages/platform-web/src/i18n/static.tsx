import { type ReactNode, useCallback, useMemo, useState } from "react";
import { MessageTranslatorProvider, type TranslateFn } from "../msg/message";
import { I18nContextProvider, type UseCatalog, useTranslation } from "./provider";

const EMPTY: Readonly<Record<string, string>> = Object.freeze({});

/**
 * Meratakan katalog JSON bersarang (format berkas `modules/<m>/i18n/<locale>.json`) menjadi kunci bertitik.
 * Nilai selain string dan objek diabaikan.
 */
export function flattenCatalog(node: unknown, prefix = ""): Record<string, string> {
  if (typeof node === "string") return { [prefix]: node };
  if (node === null || typeof node !== "object") return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(node)) {
    Object.assign(out, flattenCatalog(v, prefix === "" ? k : `${prefix}.${k}`));
  }
  return out;
}

function useTranslateNs(ns: string): TranslateFn {
  const { t } = useTranslation(ns as never);
  return t as unknown as TranslateFn;
}

export interface StaticI18nProps {
  /** Bahasa awal; `setLocale` dari `useTranslation` mengganti bahasa dari `catalogsByLocale`. */
  locale: string;
  /** `{ bahasa: { namespace: { "kunci.lengkap": "pesan ICU" } } }`, dibundel bersama kodenya (tanpa jaringan). */
  catalogsByLocale: Readonly<
    Record<string, Readonly<Record<string, Readonly<Record<string, string>>>>>
  >;
  children: ReactNode;
}

/**
 * Runtime i18n dengan katalog yang dibundel, tanpa jaringan, `QueryClient`, bootstrap, atau Suspense. Dipakai layar
 * yang dirender SEBELUM bootstrap (gerbang autentikasi, PRD-001 Q33): `I18nProvider` biasa bergantung pada
 * bootstrap yang baru bisa dimuat setelah login. Kunci yang tidak ada menghasilkan `ns::kunci` (perilaku `translate`).
 */
export function StaticI18nProvider({
  locale: initial,
  catalogsByLocale,
  children,
}: StaticI18nProps) {
  const [locale, setLocaleState] = useState(initial);
  const setLocale = useCallback(
    (next: string) => {
      if (Object.hasOwn(catalogsByLocale, next)) setLocaleState(next);
    },
    [catalogsByLocale],
  );
  const useCatalog = useMemo<UseCatalog>(() => {
    const byNs = catalogsByLocale[locale] ?? {};
    return (ns) => (Object.hasOwn(byNs, ns) ? (byNs[ns] ?? EMPTY) : EMPTY);
  }, [catalogsByLocale, locale]);
  const value = useMemo(() => ({ locale, setLocale, useCatalog }), [locale, setLocale, useCatalog]);
  return (
    <I18nContextProvider value={value}>
      <MessageTranslatorProvider useTranslate={useTranslateNs}>
        {children}
      </MessageTranslatorProvider>
    </I18nContextProvider>
  );
}

import { type ComponentType, type ReactNode, useCallback, useMemo, useState } from "react";
import { MessageTranslatorProvider, type TranslateFn } from "../msg/message";
import { I18nContextProvider, type UseCatalog, useTranslation } from "./provider";

const EMPTY: Readonly<Record<string, string>> = Object.freeze({});

export interface TestI18n {
  /** Provider uji: tanpa jaringan, `QueryClient`, atau bootstrap; tanpa Suspense. */
  Provider: ComponentType<{ children: ReactNode }>;
}

function useTranslateNs(ns: string): TranslateFn {
  const { t } = useTranslation(ns as never);
  return t as unknown as TranslateFn;
}

/**
 * Provider uji untuk `bun test`: `createTestI18n({ example: { "note.title": "Judul" } })`. Katalog berbentuk
 * `{ namespace: { "kunci.lengkap": "pesan ICU" } }`; namespace atau kunci yang tak ada menghasilkan kunci mentah.
 * `setLocale` hanya mengganti `locale` (katalog tidak dimuat ulang).
 */
export function createTestI18n(
  catalogs: Readonly<Record<string, Readonly<Record<string, string>>>>,
  opts: { locale?: string } = {},
): TestI18n {
  const useCatalog: UseCatalog = (ns) =>
    Object.hasOwn(catalogs, ns) ? (catalogs[ns] ?? EMPTY) : EMPTY;
  function Provider({ children }: { children: ReactNode }) {
    const [locale, setLocaleState] = useState(opts.locale ?? "id");
    const setLocale = useCallback((l: string) => setLocaleState(l), []);
    const value = useMemo(() => ({ locale, setLocale, useCatalog }), [locale, setLocale]);
    return (
      <I18nContextProvider value={value}>
        <MessageTranslatorProvider useTranslate={useTranslateNs}>
          {children}
        </MessageTranslatorProvider>
      </I18nContextProvider>
    );
  }
  return { Provider };
}

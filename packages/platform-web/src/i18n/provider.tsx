import type { PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import {
  createContext,
  type ReactNode,
  startTransition,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useBootstrap, useTenantId } from "../bootstrap";
import { MessageTranslatorProvider, type TranslateFn } from "../msg/message";
import { PREF_KEYS, readPref, writePref } from "../prefs";
import { scopedKey } from "../tenancy";
import { type CatalogEntry, createCatalogLoader } from "./loader";
import { chooseLocale, getActiveLocale, setActiveLocale } from "./locale";
import { useTranslator } from "./translate";
import type { I18nCatalogs, Translator } from "./types";

type Messages = Readonly<Record<string, string>>;

/** Sumber katalog satu namespace sebagai hook (produksi: jaringan; uji: statis). Identitas hook harus stabil. */
export type UseCatalog = (ns: string) => Messages;

export interface I18nContextValue {
  locale: string;
  setLocale(locale: string): void;
  useCatalog: UseCatalog;
}

const I18nContext = createContext<I18nContextValue | null>(null);

/** Hanya untuk `createTestI18n` dan jembatan internal. */
export const I18nContextProvider = I18nContext.Provider;

interface RemoteValue {
  locale: string;
  load: (locale: string, ns: string) => Promise<CatalogEntry>;
}
const RemoteContext = createContext<RemoteValue | null>(null);

const FALLBACK_DEFAULT_LOCALE = "id";

/** Katalog satu namespace dari API (Suspense; tanpa kunci mentah selama memuat). */
function useRemoteCatalog(ns: string): Messages {
  const remote = useContext(RemoteContext);
  if (remote === null) throw new Error("useTranslation harus dipakai di dalam I18nProvider");
  const { locale, load } = remote;
  const tenantId = useTenantId();
  const queryClient = useQueryClient();
  const revision = useBootstrap().i18n?.revisions[ns];
  const { data } = useSuspenseQuery({
    queryKey: scopedKey(tenantId, "i18n", locale, ns),
    queryFn: () => load(locale, ns),
    staleTime: Number.POSITIVE_INFINITY,
  });

  // Muat ulang (kondisional lewat ETag) bila revisi di bootstrap berbeda dari katalog termuat. Satu percobaan per
  // revisi bootstrap: bila server membalas 304 revisi tetap berbeda, tidak mengulang tanpa henti.
  const attempted = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (revision === undefined || data.revision === revision || attempted.current === revision) {
      return;
    }
    attempted.current = revision;
    void queryClient.invalidateQueries({ queryKey: scopedKey(tenantId, "i18n", locale, ns) });
  }, [revision, data.revision, queryClient, tenantId, locale, ns]);

  return data.messages;
}

/** Jembatan `<Message>`/`useMessage` ke `useTranslation` (M3a 9.4b). */
function useTranslateNs(ns: string): TranslateFn {
  const { t } = useTranslation(ns as never);
  return t as unknown as TranslateFn;
}

/**
 * Menyediakan bahasa aktif dan katalog (dimuat malas per namespace dari API) ke `useTranslation`. Membaca
 * `tenantId` dan `bootstrap.i18n` dari konteks `BootstrapProvider` (satu query bootstrap bersama). Dipasang di dalam
 * `QueryClientProvider` dan `BootstrapProvider`; anak yang memakai `useTranslation` harus berada di dalam batas Suspense.
 */
export function I18nProvider({
  client,
  children,
}: {
  client: TypedClient<PlatformPaths>;
  children: ReactNode;
}) {
  const info = useBootstrap().i18n;
  const locales = useMemo(() => info?.locales ?? [], [info]);
  const defaultLocale = info?.defaultLocale ?? FALLBACK_DEFAULT_LOCALE;
  const codes = useMemo(() => locales.map((l) => l.code), [locales]);

  const initial = () =>
    chooseLocale({
      defaultLocale,
      locales,
      stored: readPref(PREF_KEYS.locale),
      languages: typeof navigator === "undefined" ? [] : (navigator.languages ?? []),
    });
  const [selected, setSelected] = useState<string>(initial);
  // Daftar bahasa dapat berubah (bootstrap baru): pilihan yang tak lagi terdaftar dipilih ulang.
  const locale = codes.length === 0 || codes.includes(selected) ? selected : initial();

  // Idempoten: klien API membaca bahasa ini pada request katalog yang dimulai selama render.
  if (getActiveLocale() !== locale) setActiveLocale(locale);
  useLayoutEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  const setLocale = useCallback(
    (next: string) => {
      if (!codes.includes(next)) return;
      writePref(PREF_KEYS.locale, next);
      // Transisi: UI lama tetap tampil sampai katalog bahasa baru termuat (tanpa kilatan kerangka).
      startTransition(() => setSelected(next));
    },
    [codes],
  );

  const load = useMemo(() => createCatalogLoader(client), [client]);
  const remote = useMemo(() => ({ locale, load }), [locale, load]);
  const value = useMemo<I18nContextValue>(
    () => ({ locale, setLocale, useCatalog: useRemoteCatalog }),
    [locale, setLocale],
  );

  return (
    <RemoteContext.Provider value={remote}>
      <I18nContext.Provider value={value}>
        <MessageTranslatorProvider useTranslate={useTranslateNs}>
          {children}
        </MessageTranslatorProvider>
      </I18nContext.Provider>
    </RemoteContext.Provider>
  );
}

/**
 * Terjemahan satu namespace: `t` (teks polos), `rich` (tag rich sebagai elemen React), `locale`, `setLocale`.
 * Menangguhkan (Suspense) sampai katalog termuat, sehingga kunci mentah tidak pernah tampil selama memuat.
 */
export function useTranslation<N extends keyof I18nCatalogs>(ns: N): Translator<I18nCatalogs[N]> {
  const ctx = useContext(I18nContext);
  if (ctx === null) throw new Error("useTranslation harus dipakai di dalam I18nProvider");
  const messages = ctx.useCatalog(ns as string);
  return useTranslator<I18nCatalogs[N]>(ctx.locale, messages, ctx.setLocale);
}

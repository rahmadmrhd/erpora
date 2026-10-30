import type { MessageRef } from "@erpora/module-sdk";
import { createContext, type ReactNode, useContext } from "react";

export type MessageArgs = Record<string, string | number>;
export type TranslateFn = (key: string, args?: MessageArgs) => string;
/** Hook yang mengembalikan fungsi terjemah untuk satu namespace (mis. pembungkus `useTranslation(ns).t`). */
export type UseTranslate = (ns: string) => TranslateFn;

/** Cadangan tanpa runtime i18n: mengembalikan kunci apa adanya. */
const fallbackUseTranslate: UseTranslate = () => (key) => key;

const TranslatorContext = createContext<UseTranslate>(fallbackUseTranslate);

/**
 * Menyambungkan `<Message>`/`useMessage` ke runtime terjemahan. `useTranslate` adalah sebuah hook (dipanggil di
 * setiap render dengan urutan tetap).
 *
 * TITIK INTEGRASI: setelah `I18nProvider` dari spec i18n-platform tersedia, shell memasang
 * `useTranslate={(ns) => useTranslation(ns).t}` (satu jembatan, di dalam `I18nProvider`).
 */
export function MessageTranslatorProvider({
  useTranslate,
  children,
}: {
  useTranslate: UseTranslate;
  children: ReactNode;
}) {
  return <TranslatorContext.Provider value={useTranslate}>{children}</TranslatorContext.Provider>;
}

/** Teks untuk `ref` (untuk atribut seperti `aria-label` atau judul dokumen). */
export function useMessage(ref: MessageRef, args?: MessageArgs): string {
  const useTranslate = useContext(TranslatorContext);
  return useTranslate(ref.ns)(ref.key, args);
}

/** Merender pesan terjemahan sebagai teks (tidak pernah HTML). */
export function Message({ message, args }: { message: MessageRef; args?: MessageArgs }) {
  return <>{useMessage(message, args)}</>;
}

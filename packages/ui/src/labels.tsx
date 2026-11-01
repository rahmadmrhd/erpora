import { createContext, type ReactNode, useContext } from "react";

/**
 * Kunci namespace `ui` yang dipakai komponen `@erpora/ui` (katalog: `packages/ui/i18n/{id,en}.json`).
 * Ditulis tangan sampai generator `i18n:generate` (keys.gen.ts) tersedia; `labels.test.tsx` menjaga setiap kunci ada di katalog.
 */
export const uiKeys = [
  "actions.close",
  "actions.retry",
  "status.loading",
  "breadcrumb.label",
  "breadcrumb.more",
  "sidebar.label",
  "sidebar.toggle",
  "sidebar.title",
  "sidebar.description",
  "toast.close",
  "toast.region",
] as const;

export type UiKey = (typeof uiKeys)[number];

/** Fungsi terjemah tipis yang diisi shell (dari `useTranslation("ui")`); `ui` tidak mengimpor runtime i18n (F4). */
export type UiLabelFn = (key: UiKey, args?: Record<string, string | number>) => string;

const identity: UiLabelFn = (key) => key;
const UiLabelsContext = createContext<UiLabelFn>(identity);

export function UiLabelsProvider(props: { t: UiLabelFn; children: ReactNode }) {
  return <UiLabelsContext.Provider value={props.t}>{props.children}</UiLabelsContext.Provider>;
}

/** Tanpa provider mengembalikan kunci apa adanya (komponen tetap dapat dirender di test dan layar boot). */
export function useUiLabel(): UiLabelFn {
  return useContext(UiLabelsContext);
}

/**
 * Menampilkan satu label `ui` sebagai teks. Dipakai berkas shadcn hasil registry lewat patch deklaratif
 * (`shadcn.patches.json`) untuk mengganti teks Inggris tertanam tanpa mengubah struktur komponen.
 */
export function UiLabel(props: { k: UiKey; args?: Record<string, string | number> }) {
  const t = useUiLabel();
  return <>{t(props.k, props.args)}</>;
}

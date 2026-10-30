import { useTranslation } from "@erpora/platform-web";
import { type UiLabelFn, UiLabelsProvider } from "@erpora/ui";
import { type ReactNode, useCallback } from "react";

/**
 * Menjembatani `@erpora/ui` (tanpa impor runtime i18n, aturan F4) ke namespace `ui`: komponen membaca label lewat
 * `useUiLabel`, shell mengisinya dari `useTranslation("ui")` (menangguhkan sampai katalog `ui` termuat).
 */
export function UiLabelsBridge({ children }: { children: ReactNode }) {
  const { t } = useTranslation("ui");
  const label = useCallback<UiLabelFn>(
    (key, args) =>
      (t as unknown as (k: string, a?: Record<string, string | number>) => string)(key, args),
    [t],
  );
  return <UiLabelsProvider t={label}>{children}</UiLabelsProvider>;
}

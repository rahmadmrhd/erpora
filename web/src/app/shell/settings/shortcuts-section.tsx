import { useTranslation } from "@erpora/platform-web";
import { Kbd, KbdGroup } from "@erpora/ui";
import { GLOBAL_SHORTCUTS, type ShortcutId } from "./shortcuts";

/** Bagian Pintasan: daftar pintasan keyboard global (`GLOBAL_SHORTCUTS`). */
export function ShortcutsSection() {
  const { t } = useTranslation("platform");
  const labels: Record<ShortcutId, string> = {
    openPalette: t("settings.shortcuts.openPalette"),
    showShortcuts: t("settings.shortcuts.showShortcuts"),
    toggleSidebar: t("settings.shortcuts.toggleSidebar"),
    openDock: t("settings.shortcuts.openDock"),
    toggleTheme: t("settings.shortcuts.toggleTheme"),
  };
  return (
    <section className="flex flex-col gap-3">
      <p className="text-sm text-muted-foreground">{t("settings.shortcuts.description")}</p>
      <ul className="divide-y rounded-lg border">
        {GLOBAL_SHORTCUTS.map((s) => (
          <li key={s.id} className="flex items-center justify-between gap-4 px-3 py-2 text-sm">
            <span>{labels[s.id]}</span>
            <KbdGroup>
              {s.keys.map((k) => (
                <Kbd key={k}>{k}</Kbd>
              ))}
            </KbdGroup>
          </li>
        ))}
      </ul>
    </section>
  );
}

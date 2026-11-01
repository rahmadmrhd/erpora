import {
  type DockMode,
  parseThemePreference,
  type ResolvedTheme,
  type ThemePreference,
  useDockMode,
  useTheme,
  useTranslation,
  writeDockMode,
} from "@erpora/platform-web";
import { cn, RadioGroup } from "@erpora/ui";
import { useId } from "react";
import { ChoiceCard } from "./choice-card";

/**
 * Pratinjau miniatur tema. Sisi gelap dirender di dalam kelas `.dark` (variabel token digeser sehingga akurat pada tema
 * apa pun); sisi terang memakai token apa adanya, atau token terbalik bila tema aktif gelap (tidak ada kelas `.light`
 * bersarang). Hanya token semantik, tanpa nilai palet langsung.
 */
function ThemeSwatch({ tone, resolved }: { tone: ResolvedTheme; resolved: ResolvedTheme }) {
  const inverted = tone === "light" && resolved === "dark";
  const surface = inverted ? "bg-foreground" : "bg-background";
  const bar = inverted ? "bg-background/15" : "bg-foreground/15";
  const line = inverted ? "bg-background/50" : "bg-foreground/50";
  return (
    <span
      className={cn(
        tone === "dark" && "dark",
        surface,
        "flex h-16 w-full overflow-hidden rounded-md border",
      )}
    >
      <span className={cn(bar, "w-1/4")} />
      <span className="flex flex-1 flex-col gap-1.5 p-2">
        <span className={cn(line, "h-1.5 w-2/3 rounded-full")} />
        <span className={cn(bar, "h-1.5 w-full rounded-full")} />
        <span className={cn(bar, "h-1.5 w-4/5 rounded-full")} />
      </span>
    </span>
  );
}

function ThemePreview({ mode, resolved }: { mode: ThemePreference; resolved: ResolvedTheme }) {
  if (mode !== "system") return <ThemeSwatch tone={mode} resolved={resolved} />;
  return (
    <span className="relative block h-16 w-full">
      <span className="absolute inset-0" style={{ clipPath: "polygon(0 0, 100% 0, 0 100%)" }}>
        <ThemeSwatch tone="light" resolved={resolved} />
      </span>
      <span className="absolute inset-0" style={{ clipPath: "polygon(100% 0, 100% 100%, 0 100%)" }}>
        <ThemeSwatch tone="dark" resolved={resolved} />
      </span>
    </span>
  );
}

/** Pratinjau Dock: Pinned = rel ikon menempel di sisi panel; Floating = pil melayang di atas panel. */
function DockPreview({ mode }: { mode: DockMode }) {
  const icons = (
    <>
      <span className="size-2.5 rounded-sm bg-primary" />
      <span className="size-2.5 rounded-sm bg-foreground/30" />
      <span className="size-2.5 rounded-sm bg-foreground/30" />
    </>
  );
  return (
    <span className="relative flex h-16 w-full overflow-hidden rounded-md border bg-background">
      {mode === "pinned" ? (
        <span className="flex w-5 flex-col items-center gap-1.5 border-r bg-muted py-2">
          {icons}
        </span>
      ) : null}
      <span className="w-1/4 border-r bg-muted/50" />
      <span className="flex flex-1 flex-col gap-1.5 p-2">
        <span className="h-1.5 w-2/3 rounded-full bg-foreground/50" />
        <span className="h-1.5 w-full rounded-full bg-foreground/15" />
      </span>
      {mode === "floating" ? (
        <span className="absolute top-1/2 left-1 flex -translate-y-1/2 flex-col items-center gap-1.5 rounded-full border bg-card px-1 py-1.5 shadow-sm">
          {icons}
        </span>
      ) : null}
    </span>
  );
}

/** Bagian Tampilan: Tema dan Dock sebagai kartu pratinjau; setiap pilihan langsung tersimpan dan berlaku. */
export function AppearanceSection() {
  const { t } = useTranslation("platform");
  const { preference, resolved, setPreference } = useTheme();
  const dock = useDockMode();
  const themeId = useId();
  const dockId = useId();
  return (
    <div className="flex flex-col gap-8">
      <section className="flex flex-col gap-3">
        <div>
          <h3 id={themeId} className="text-sm font-medium">
            {t("settings.theme.title")}
          </h3>
          <p className="text-sm text-muted-foreground">{t("settings.theme.description")}</p>
        </div>
        <RadioGroup
          aria-labelledby={themeId}
          value={preference}
          onValueChange={(v) => setPreference(parseThemePreference(v))}
          className="grid grid-cols-3 gap-3"
        >
          <ChoiceCard
            value="system"
            label={t("settings.theme.automatic")}
            preview={<ThemePreview mode="system" resolved={resolved} />}
          />
          <ChoiceCard
            value="light"
            label={t("theme.light")}
            preview={<ThemePreview mode="light" resolved={resolved} />}
          />
          <ChoiceCard
            value="dark"
            label={t("theme.dark")}
            preview={<ThemePreview mode="dark" resolved={resolved} />}
          />
        </RadioGroup>
      </section>
      <section className="flex flex-col gap-3">
        <div>
          <h3 id={dockId} className="text-sm font-medium">
            {t("settings.dock.title")}
          </h3>
          <p className="text-sm text-muted-foreground">{t("settings.dock.description")}</p>
        </div>
        <RadioGroup
          aria-labelledby={dockId}
          value={dock}
          onValueChange={(v) => writeDockMode(v === "pinned" ? "pinned" : "floating")}
          className="grid grid-cols-2 gap-3"
        >
          <ChoiceCard
            value="pinned"
            label={t("settings.dock.pinned")}
            preview={<DockPreview mode="pinned" />}
          />
          <ChoiceCard
            value="floating"
            label={t("settings.dock.floating")}
            preview={<DockPreview mode="floating" />}
          />
        </RadioGroup>
      </section>
    </div>
  );
}

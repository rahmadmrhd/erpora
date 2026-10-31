import { useTranslation } from "@erpora/platform-web";
import {
  cn,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  useIsMobile,
} from "@erpora/ui";
import { useEffect, useState } from "react";
import { useReturnFocus } from "../overlay-focus";
import { AppearanceSection } from "./appearance-section";
import { PreferencesSection } from "./preferences-section";
import { ShortcutsSection } from "./shortcuts-section";

export type SettingsSection = "appearance" | "preferences" | "shortcuts";

const SECTIONS: readonly SettingsSection[] = ["appearance", "preferences", "shortcuts"];

function isSection(value: string): value is SettingsSection {
  return (SECTIONS as readonly string[]).includes(value);
}

/**
 * Dialog Settings dua kolom (navigasi kiri, konten kanan): Tampilan (Tema, Dock), Preferensi (Bahasa, Zona waktu),
 * dan Pintasan. Setiap perubahan langsung tersimpan di `localStorage` (preferensi UI, tanpa data sensitif) dan
 * berlaku seketika; tidak ada tombol simpan. Dimuat malas oleh pemanggil (`lazy(() => import(...))`).
 * `initialSection` memilih bagian saat dialog dibuka (mis. Pintasan dari popover logo).
 */
export function SettingsDialog({
  open,
  onOpenChange,
  initialSection,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initialSection?: SettingsSection;
}) {
  const { t } = useTranslation("platform");
  const [section, setSection] = useState<SettingsSection>(initialSection ?? "appearance");
  const focus = useReturnFocus(open);

  // Setiap pembukaan dimulai dari bagian yang diminta (bawaan: Tampilan).
  useEffect(() => {
    if (open) setSection(initialSection ?? "appearance");
  }, [open, initialSection]);

  // Di bawah 768 px (ponsel) navigasi menjadi tab horizontal di atas konten; di atasnya kolom vertikal di kiri.
  // Orientasi ikut `Tabs` (peran, `aria-orientation`, dan tombol panah) tanpa mengubah berkas shadcn.
  const vertical = !useIsMobile();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="h-[calc(100svh-1rem)] max-w-[calc(100%-1rem)] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden p-0 sm:max-w-4xl md:h-[min(40rem,calc(100svh-2rem))] xl:max-w-5xl"
        onCloseAutoFocus={focus.onCloseAutoFocus}
      >
        <DialogHeader className="border-b px-4 py-4 md:px-6">
          <DialogTitle>{t("settings.title")}</DialogTitle>
          <DialogDescription>{t("settings.description")}</DialogDescription>
        </DialogHeader>
        <Tabs
          orientation={vertical ? "vertical" : "horizontal"}
          value={section}
          onValueChange={(v) => {
            if (isSection(v)) setSection(v);
          }}
          className={cn(
            "min-h-0 gap-0",
            vertical && "grid md:grid-cols-[14rem_1fr] md:grid-rows-[minmax(0,1fr)]",
          )}
        >
          {/* Garis pembatas ada di kolom navigasi (sel grid yang meregang setinggi area dialog), bukan di daftar tab:
              `TabsList` shadcn vertikal bertinggi `fit` sehingga garisnya terpotong setinggi isinya. Pada ponsel
              kolom ini menjadi baris tab yang dapat digulir ke samping (garis di bawahnya). */}
          <div
            data-slot="settings-nav"
            className={cn(
              "min-h-0 shrink-0",
              vertical ? "overflow-y-auto md:border-r" : "overflow-x-auto border-b pb-1",
            )}
          >
            <TabsList
              variant="line"
              aria-label={t("settings.navLabel")}
              className={cn(
                vertical
                  ? "h-fit w-full gap-1 p-3 md:items-start md:justify-start"
                  : "w-max min-w-full justify-start gap-1 px-3",
              )}
            >
              {SECTIONS.map((value) => (
                <TabsTrigger
                  key={value}
                  value={value}
                  className={vertical ? "justify-start" : "flex-none"}
                >
                  {t(`settings.sections.${value}`)}
                </TabsTrigger>
              ))}
            </TabsList>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto p-4 md:p-6">
            <TabsContent value="appearance">
              <AppearanceSection />
            </TabsContent>
            <TabsContent value="preferences">
              <PreferencesSection />
            </TabsContent>
            <TabsContent value="shortcuts">
              <ShortcutsSection />
            </TabsContent>
          </div>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

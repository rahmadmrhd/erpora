import { useTranslation } from "@erpora/platform-web";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="gap-0 overflow-hidden p-0 sm:max-w-3xl"
        onCloseAutoFocus={focus.onCloseAutoFocus}
      >
        <DialogHeader className="border-b px-6 py-4">
          <DialogTitle>{t("settings.title")}</DialogTitle>
          <DialogDescription>{t("settings.description")}</DialogDescription>
        </DialogHeader>
        <Tabs
          orientation="vertical"
          value={section}
          onValueChange={(v) => {
            if (isSection(v)) setSection(v);
          }}
          className="grid min-h-0 gap-0 md:h-[min(26rem,calc(100svh-9rem))] md:grid-cols-[12rem_1fr]"
        >
          {/* Garis pembatas ada di kolom navigasi (sel grid yang meregang setinggi area dialog), bukan di daftar tab:
              `TabsList` shadcn vertikal bertinggi `fit` sehingga garisnya terpotong setinggi isinya. */}
          <div
            data-slot="settings-nav"
            className="min-h-0 overflow-y-auto border-b md:border-r md:border-b-0"
          >
            <TabsList
              variant="line"
              aria-label={t("settings.navLabel")}
              className="h-fit w-full gap-1 p-3 md:items-start md:justify-start"
            >
              <TabsTrigger value="appearance" className="justify-start">
                {t("settings.sections.appearance")}
              </TabsTrigger>
              <TabsTrigger value="preferences" className="justify-start">
                {t("settings.sections.preferences")}
              </TabsTrigger>
              <TabsTrigger value="shortcuts" className="justify-start">
                {t("settings.sections.shortcuts")}
              </TabsTrigger>
            </TabsList>
          </div>
          <div className="min-h-0 overflow-y-auto p-6">
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

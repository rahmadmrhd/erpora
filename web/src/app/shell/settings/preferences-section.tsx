import {
  isValidTimezone,
  useBootstrap,
  useTimezone,
  useTranslation,
  writeTimezone,
} from "@erpora/platform-web";
import { Label, Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@erpora/ui";
import { useId } from "react";
import { timezoneOffsetLabel, timezoneOptions } from "./timezones";

/** Nilai khusus `Select` untuk "Otomatis" (nilai kosong tidak diizinkan Radix). */
const AUTO = "auto";

/** Bagian Preferensi: Bahasa (runtime i18n) dan Zona waktu (`erpora.timezone`); setiap pilihan langsung tersimpan. */
export function PreferencesSection() {
  const { t, locale, setLocale } = useTranslation("platform");
  const locales = useBootstrap().i18n?.locales ?? [];
  const timezone = useTimezone();
  const languageId = useId();
  const timezoneId = useId();
  return (
    <div className="flex flex-col gap-8">
      <section className="flex flex-col gap-2">
        <Label htmlFor={languageId}>{t("shell.language.label")}</Label>
        <Select value={locale} onValueChange={setLocale}>
          <SelectTrigger id={languageId} className="w-full max-w-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {locales.map((l) => (
              <SelectItem key={l.code} value={l.code}>
                {l.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </section>
      <section className="flex flex-col gap-2">
        <Label htmlFor={timezoneId}>{t("settings.timezone.title")}</Label>
        <Select
          value={timezone ?? AUTO}
          onValueChange={(v) => writeTimezone(v !== AUTO && isValidTimezone(v) ? v : null)}
        >
          <SelectTrigger id={timezoneId} className="w-full max-w-xs">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={AUTO}>{t("settings.timezone.automatic")}</SelectItem>
            {timezoneOptions(timezone).map((zone) => {
              const offset = timezoneOffsetLabel(zone, locale);
              return (
                <SelectItem key={zone} value={zone}>
                  {offset === "" ? zone : `${zone} (${offset})`}
                </SelectItem>
              );
            })}
          </SelectContent>
        </Select>
        <p className="text-sm text-muted-foreground">{t("settings.timezone.description")}</p>
      </section>
    </div>
  );
}

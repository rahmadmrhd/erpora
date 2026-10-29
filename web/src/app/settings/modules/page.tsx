import { useTranslation } from "@erpora/platform-web";
import { EmptyState } from "@erpora/ui";

/**
 * Placeholder halaman `/settings/modules` (lazy). Isinya (tabel modul, dialog dampak, riwayat) dikerjakan di grup 14;
 * route dan chunk-nya sudah terpasang di router.
 */
export function ModulesPage() {
  const { t } = useTranslation("platform");
  return <EmptyState icon="blocks" title={t("nav.settingsModules")} />;
}

import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

/**
 * Definisi modul frontend `stock-opname`: route lazy `/stock-opname/items` (menu dari manifest `desk` di module.json;
 * indeks `/stock-opname` adalah Dashboard bawaan shell, jangan didaftarkan di sini), online-only.
 */
export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("stock-opname", "desk.title"),
  registerRoutes,
  offline: defaultOfflinePolicy,
});

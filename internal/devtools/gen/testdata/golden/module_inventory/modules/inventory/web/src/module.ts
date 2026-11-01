import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

/**
 * Definisi modul frontend `inventory`: route lazy `/inventory/items` (menu dari manifest `desk` di module.json;
 * indeks `/inventory` adalah Dashboard bawaan shell, jangan didaftarkan di sini), online-only.
 */
export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("inventory", "desk.title"),
  registerRoutes,
  offline: defaultOfflinePolicy,
});

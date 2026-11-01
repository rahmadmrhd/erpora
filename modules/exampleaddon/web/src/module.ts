import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

/** Definisi modul frontend `example-addon` (R6.1): route lazy `/example-addon/summary` (menu dari manifest `desk`), online-only. */
export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("example-addon", "nav.summary"),
  registerRoutes,
  offline: defaultOfflinePolicy,
});

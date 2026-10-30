import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

/** Definisi modul frontend `example-addon` (R6.1): navigasi satu item, route lazy `/example-addon`, online-only. */
export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("example-addon", "nav.summary"),
  navigation: [
    {
      type: "item",
      id: "summary",
      label: msg("example-addon", "nav.summary"),
      to: "/",
      icon: "layout-dashboard",
    },
  ],
  registerRoutes,
  offline: defaultOfflinePolicy,
});

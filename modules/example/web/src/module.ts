import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

/** Definisi modul frontend `example` (R6.1): navigasi satu item, route lazy `/example`, online-only. */
export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("example", "nav.notes"),
  navigation: [
    {
      type: "item",
      id: "notes",
      label: msg("example", "nav.notes"),
      to: "/",
      icon: "file-text",
    },
  ],
  registerRoutes,
  offline: defaultOfflinePolicy,
});

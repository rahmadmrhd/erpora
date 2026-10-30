import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

/** Definisi modul frontend `inventory`: navigasi satu item, route lazy `/inventory`, online-only. */
export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("inventory", "nav.home"),
  navigation: [
    {
      type: "item",
      id: "home",
      label: msg("inventory", "nav.home"),
      to: "/",
      icon: "layout-dashboard",
    },
  ],
  registerRoutes,
  offline: defaultOfflinePolicy,
});

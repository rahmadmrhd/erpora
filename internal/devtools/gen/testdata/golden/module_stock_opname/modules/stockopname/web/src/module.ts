import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

/** Definisi modul frontend `stock-opname`: navigasi satu item, route lazy `/stock-opname`, online-only. */
export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("stock-opname", "nav.home"),
  navigation: [
    {
      type: "item",
      id: "home",
      label: msg("stock-opname", "nav.home"),
      to: "/",
      icon: "layout-dashboard",
    },
  ],
  registerRoutes,
  offline: defaultOfflinePolicy,
});

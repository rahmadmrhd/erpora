import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { msg } from "@erpora/platform-web";
import "./i18n/keys.gen";
import { moduleInfo, moduleVersion } from "./info";
import { registerRoutes } from "./routes";

/** Definisi modul frontend `core`: halaman "Ubah sandi" (menu dari manifest `desk`), online-only. */
export const frontendModule = defineModule({
  key: moduleInfo.key,
  slug: moduleInfo.slug,
  version: moduleVersion,
  title: msg("core", "nav.change_password"),
  registerRoutes,
  offline: defaultOfflinePolicy,
});

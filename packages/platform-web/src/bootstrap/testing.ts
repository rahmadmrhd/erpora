import type { Bootstrap, BootstrapModule, ModuleStatus } from "./types";

/** Bootstrap contoh untuk test dan fixture (tanpa jaringan). Bidang dapat ditimpa. */
export function makeBootstrap(overrides: Partial<Bootstrap> = {}): Bootstrap {
  return {
    appVersion: "0.1.0",
    tenantId: "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b",
    modules: [],
    permissions: [],
    revision: 1,
    ...overrides,
  };
}

/** Entri modul bootstrap contoh; `basePath` mengikuti `/<slug>`. */
export function makeBootstrapModule(
  key: string,
  slug: string,
  status: ModuleStatus = "enabled",
  overrides: Partial<BootstrapModule> = {},
): BootstrapModule {
  return {
    key,
    slug,
    version: "1.0.0",
    status,
    basePath: `/${slug}`,
    offline: false,
    ...overrides,
  };
}

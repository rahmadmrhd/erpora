import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  createAppRootRoute,
  createModuleBaseRoute,
  defaultOfflinePolicy,
} from "@erpora/module-sdk";
import { assertDeskRoutes } from "@erpora/module-sdk/testing";
import * as entry from "./index";

const here = dirname(fileURLToPath(import.meta.url));
const moduleDir = join(here, "..", "..");
const manifest = JSON.parse(readFileSync(join(moduleDir, "module.json"), "utf8")) as {
  key: string;
  slug: string;
  version: string;
  offline: { mode: string };
  desk?: Parameters<typeof assertDeskRoutes>[1];
};

describe("frontendModule inventory", () => {
  const mod = entry.frontendModule;

  test("key, slug, dan version sama dengan module.json", () => {
    expect<string>(mod.key).toBe(manifest.key);
    expect<string>(mod.slug).toBe(manifest.slug);
    expect(mod.version).toBe(manifest.version);
    expect<unknown>(entry.moduleInfo).toEqual({ key: manifest.key, slug: manifest.slug });
  });

  test("hasil defineModule beku dan kebijakan offline mengikuti manifest (online-only)", () => {
    expect(Object.isFrozen(mod)).toBe(true);
    expect(manifest.offline.mode).toBe("online-only");
    expect(mod.offline).toEqual(defaultOfflinePolicy);
  });

  test("tanpa navigation: menu dari manifest desk, tiap `to` punya rute terdaftar (assertDeskRoutes)", () => {
    expect("navigation" in mod).toBe(false);
    expect(manifest.desk?.menus.map((m) => m.id)).toEqual(["items"]);
    expect(() => assertDeskRoutes(mod, manifest.desk)).not.toThrow();
  });

  test("registerRoutes mengembalikan route anak dari base modul dengan crumb berpesan terjemahan", () => {
    const root = createAppRootRoute();
    const base = createModuleBaseRoute(root, mod.slug);
    const routes = mod.registerRoutes(base);
    expect(routes).toHaveLength(1);
    const [index] = routes;
    expect(index?.options.getParentRoute()).toBe(base);
    expect((index?.options as { path?: string } | undefined)?.path).toBe("/items");
    expect(index?.options.staticData?.crumb).toEqual({
      ref: { ns: "inventory", key: "page.items.title" },
    });
  });

  test("entry paket mengekspor moduleInfo, api (kontrak terketik), dan frontendModule", () => {
    expect(Object.keys(entry).sort()).toEqual(["api", "frontendModule", "moduleInfo"]);
    expect(typeof entry.api.client.get).toBe("function");
  });
});

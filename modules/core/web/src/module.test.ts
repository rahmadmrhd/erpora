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
import { flattenCatalog } from "@erpora/platform-web";
import * as entry from "./index";

const here = dirname(fileURLToPath(import.meta.url));
const moduleDir = join(here, "..", "..");
const manifest = JSON.parse(readFileSync(join(moduleDir, "module.json"), "utf8")) as {
  key: string;
  slug: string;
  version: string;
  offline: { mode: string };
  capabilities: string[];
  desk?: Parameters<typeof assertDeskRoutes>[1] & { menus: { permission: string }[] };
};

describe("frontendModule core", () => {
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

  test("desk minimal: hanya menu Ubah sandi, `to` punya rute, access authenticated tanpa permission/capability (PRD-000 Q43)", () => {
    expect("navigation" in mod).toBe(false);
    expect(manifest.desk?.menus.map((m) => m.id)).toEqual(["change-password"]);
    const menu = manifest.desk?.menus[0] as { access?: string; permission?: string } | undefined;
    expect(menu?.access).toBe("authenticated");
    expect(menu?.permission).toBeUndefined();
    expect(manifest.capabilities).toEqual([]);
    expect(() => assertDeskRoutes(mod, manifest.desk)).not.toThrow();
  });

  test("route berawalan slug modul; tidak ada rute indeks dan tidak ada rute gerbang auth", () => {
    const root = createAppRootRoute();
    const base = createModuleBaseRoute(root, mod.slug);
    const routes = mod.registerRoutes(base);
    expect(routes).toHaveLength(1);
    const [page] = routes;
    expect(page?.options.getParentRoute()).toBe(base);
    expect((page?.options as { path?: string } | undefined)?.path).toBe("/change-password");
    expect(page?.options.staticData?.crumb).toEqual({
      ref: { ns: "core", key: "changePassword.title" },
    });
  });

  test("entry paket mengekspor api, AuthScreens (gerbang malas), moduleInfo, dan frontendModule", () => {
    expect(Object.keys(entry).sort()).toEqual([
      "AuthScreens",
      "api",
      "frontendModule",
      "moduleInfo",
    ]);
    expect(typeof entry.api.client.post).toBe("function");
    // Komponen malas React: objek bertanda `$$typeof` lazy, bukan fungsi (kodenya tidak ikut JS awal).
    expect(typeof entry.AuthScreens).toBe("object");
  });
});

describe("katalog i18n modul core", () => {
  const read = (locale: string) =>
    flattenCatalog(JSON.parse(readFileSync(join(moduleDir, "i18n", `${locale}.json`), "utf8")));

  test("bahasa id dan en memiliki kunci dan argumen yang sama (paritas)", () => {
    const id = read("id");
    const en = read("en");
    expect(Object.keys(id).sort()).toEqual(Object.keys(en).sort());
    const params = (s: string) => [...s.matchAll(/\{(\w+)\s*[,}]/g)].map((m) => m[1]).sort();
    for (const [k, v] of Object.entries(id)) expect(params(en[k] as string)).toEqual(params(v));
  });

  test("tanpa HTML dan tanpa nilai kosong", () => {
    for (const locale of ["id", "en"]) {
      for (const [key, text] of Object.entries(read(locale))) {
        expect(text, `${locale}:${key}`).not.toMatch(/<[a-z/]/i);
        expect(text.trim(), `${locale}:${key}`).not.toBe("");
      }
    }
  });
});

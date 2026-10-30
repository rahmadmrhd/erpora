import { describe, expect, test } from "bun:test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  createAppRootRoute,
  createModuleBaseRoute,
  defaultOfflinePolicy,
} from "@erpora/module-sdk";
import * as entry from "./index";
import { flattenCatalog } from "./testing";

const here = dirname(fileURLToPath(import.meta.url));
const moduleDir = join(here, "..", "..");
const manifest = JSON.parse(readFileSync(join(moduleDir, "module.json"), "utf8")) as {
  key: string;
  slug: string;
  version: string;
  offline: { mode: string };
};

describe("frontendModule example", () => {
  const mod = entry.frontendModule;

  test("key, slug, dan version sama dengan module.json (R6.1)", () => {
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

  test("tepat satu item navigasi ke halaman dasar dengan label terketik dari namespace modul", () => {
    expect(mod.navigation).toHaveLength(1);
    const item = mod.navigation[0];
    expect(item?.type).toBe("item");
    expect(item).toMatchObject({
      id: "notes",
      to: "/",
      label: { ns: "example", key: "nav.notes" },
    });
  });

  test("registerRoutes mengembalikan route anak dari base modul dengan crumb berpesan terjemahan", () => {
    const root = createAppRootRoute();
    const base = createModuleBaseRoute(root, mod.slug);
    const routes = mod.registerRoutes(base);
    expect(routes).toHaveLength(1);
    const [index] = routes;
    expect(index?.options.getParentRoute()).toBe(base);
    expect((index?.options as { path?: string } | undefined)?.path).toBe("/");
    expect(index?.options.staticData?.crumb).toEqual({
      ref: { ns: "example", key: "page.notes.title" },
    });
  });

  test("entry paket mengekspor moduleInfo, api (kontrak terketik), dan frontendModule", () => {
    expect(Object.keys(entry).sort()).toEqual(["api", "frontendModule", "moduleInfo"]);
    expect(typeof entry.api.client.get).toBe("function");
  });
});

describe("katalog i18n modul example", () => {
  const read = (locale: string) =>
    flattenCatalog(JSON.parse(readFileSync(join(moduleDir, "i18n", `${locale}.json`), "utf8")));

  test("bahasa id dan en memiliki kunci dan argumen yang sama (paritas)", () => {
    const id = read("id");
    const en = read("en");
    expect(Object.keys(id).sort()).toEqual(Object.keys(en).sort());
    const params = (s: string) => [...s.matchAll(/\{(\w+)\s*[,}]/g)].map((m) => m[1]).sort();
    for (const [k, v] of Object.entries(id)) expect(params(en[k] as string)).toEqual(params(v));
  });

  test("memuat kunci yang dirujuk modul dan tanpa HTML", () => {
    const id = read("id");
    for (const key of [
      "nav.notes",
      "page.notes.title",
      "page.notes.description",
      "page.notes.empty",
      "page.notes.count",
      "page.notes.error",
      "page.notes.columns.title",
      "page.notes.columns.version",
      "page.notes.columns.updated",
      "actions.refresh",
    ]) {
      expect(id[key], key).toBeTruthy();
    }
    for (const text of Object.values(id)) expect(text).not.toMatch(/<[a-z/]/i);
  });
});

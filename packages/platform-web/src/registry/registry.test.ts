import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { defaultOfflinePolicy, defineModule, type FrontendModule } from "@erpora/module-sdk";
import fc from "fast-check";
import { makeBootstrap, makeBootstrapModule } from "../bootstrap/testing";
import type { ModuleStatus } from "../bootstrap/types";
import {
  DuplicateModuleKeyError,
  DuplicateModuleSlugError,
  moduleStatus,
  type NavNode,
  type RegistryWarning,
  resetRegistryWarnings,
  resolveModules,
  selectNavigation,
  validateModules,
} from "./index";

const label = (key: string) => ({ ns: "t", key });

function fm(key: string, slug: string, over: Partial<FrontendModule> = {}): FrontendModule {
  return defineModule({
    key,
    slug,
    version: "1.0.0",
    navigation: [
      { type: "item", id: "home", label: label("home"), to: "/", icon: "home" },
      { type: "item", id: "notes", label: label("notes"), to: "/notes" },
      {
        type: "group",
        id: "reports",
        label: label("reports"),
        children: [{ type: "item", id: "sales", label: label("sales"), to: "/reports/sales" }],
      },
    ],
    registerRoutes: () => [],
    offline: defaultOfflinePolicy,
    ...over,
  });
}

const STATUSES: readonly ModuleStatus[] = [
  "available",
  "installed",
  "enabled",
  "disabled",
  "upgrade_pending",
  "failed",
  "uninstalled",
  "missing",
  "incompatible",
];

beforeEach(() => {
  resetRegistryWarnings();
});

afterEach(() => {
  resetRegistryWarnings();
});

describe("validateModules (P2)", () => {
  test("key ganda melempar DuplicateModuleKeyError yang menyebut key", () => {
    const mods = [fm("acme.one", "one"), fm("acme.two", "two"), fm("acme.one", "uno")];
    expect(() => validateModules(mods)).toThrow(DuplicateModuleKeyError);
    expect(() => validateModules(mods)).toThrow(/acme\.one/);
  });

  test("slug ganda melempar DuplicateModuleSlugError", () => {
    const mods = [fm("acme.one", "same"), fm("acme.two", "same")];
    expect(() => validateModules(mods)).toThrow(DuplicateModuleSlugError);
  });

  test("daftar kosong dan daftar unik tidak melempar", () => {
    expect(() => validateModules([])).not.toThrow();
    expect(() => validateModules([fm("acme.one", "one"), fm("acme.two", "two")])).not.toThrow();
  });

  test("P2: untuk daftar acak, melempar bila dan hanya bila ada key ganda", () => {
    const pool = ["a.one", "a.two", "a.three", "b.one", "b.two"];
    fc.assert(
      fc.property(fc.array(fc.constantFrom(...pool), { maxLength: 8 }), (keys) => {
        const mods = keys.map((k, i) => fm(k, `slug${i}`));
        const hasDup = new Set(keys).size !== keys.length;
        if (hasDup) expect(() => validateModules(mods)).toThrow(DuplicateModuleKeyError);
        else expect(() => validateModules(mods)).not.toThrow();
      }),
      { numRuns: 200 },
    );
  });
});

describe("resolveModules", () => {
  test("modul yang tidak ada di bootstrap diabaikan dengan peringatan not_in_bootstrap", () => {
    const warns: RegistryWarning[] = [];
    const out = resolveModules(
      [fm("acme.one", "one"), fm("acme.ghost", "ghost")],
      makeBootstrap({ modules: [makeBootstrapModule("acme.one", "one")] }),
      { warn: (w) => warns.push(w) },
    );
    expect(out.resolved.map((r) => r.module.key)).toEqual(["acme.one"]);
    expect(out.warnings).toEqual([
      { kind: "not_in_bootstrap", key: "acme.ghost", detail: expect.any(String) },
    ]);
    expect(warns).toEqual([...out.warnings]);
  });

  test("versi frontend berbeda dari backend: tetap resolved, peringatan version_mismatch", () => {
    const out = resolveModules(
      [fm("acme.one", "one")],
      makeBootstrap({
        modules: [makeBootstrapModule("acme.one", "one", "enabled", { version: "1.1.0" })],
      }),
      { warn: () => {} },
    );
    expect(out.resolved).toHaveLength(1);
    expect(out.warnings.map((w) => w.kind)).toEqual(["version_mismatch"]);
    expect(out.warnings[0]?.detail).toContain("1.0.0");
    expect(out.warnings[0]?.detail).toContain("1.1.0");
  });

  test("offline tidak cocok: peringatan offline_mismatch", () => {
    const offlineModule = fm("acme.one", "one", {
      offline: { ...defaultOfflinePolicy, mode: "offline-read" },
    });
    const out = resolveModules(
      [offlineModule],
      makeBootstrap({
        modules: [makeBootstrapModule("acme.one", "one", "enabled", { offline: false })],
      }),
      { warn: () => {} },
    );
    expect(out.warnings.map((w) => w.kind)).toEqual(["offline_mismatch"]);
    const ok = resolveModules(
      [offlineModule],
      makeBootstrap({
        modules: [makeBootstrapModule("acme.one", "one", "enabled", { offline: true })],
      }),
      { warn: () => {} },
    );
    expect(ok.warnings).toEqual([]);
  });

  test("slug berbeda dari backend: peringatan slug_mismatch (basePath router mengikuti slug frontend)", () => {
    const out = resolveModules(
      [fm("acme.one", "one")],
      makeBootstrap({ modules: [makeBootstrapModule("acme.one", "uno")] }),
      { warn: () => {} },
    );
    expect(out.warnings.map((w) => w.kind)).toEqual(["slug_mismatch"]);
  });

  test("tanpa peringatan bila semuanya sinkron; status apa pun tetap resolved", () => {
    for (const status of STATUSES) {
      const out = resolveModules(
        [fm("acme.one", "one")],
        makeBootstrap({ modules: [makeBootstrapModule("acme.one", "one", status)] }),
        { warn: () => {} },
      );
      expect(out.warnings).toEqual([]);
      expect(out.resolved).toHaveLength(1);
    }
  });

  test("urutan resolved mengikuti urutan daftar modul frontend, bukan urutan bootstrap", () => {
    const out = resolveModules(
      [fm("acme.b", "b"), fm("acme.a", "a")],
      makeBootstrap({
        modules: [makeBootstrapModule("acme.a", "a"), makeBootstrapModule("acme.b", "b")],
      }),
      { warn: () => {} },
    );
    expect(out.resolved.map((r) => r.module.key)).toEqual(["acme.b", "acme.a"]);
  });

  test("peringatan bawaan dicatat sekali per (kind, key) lewat console.warn", () => {
    const spy = spyOn(console, "warn").mockImplementation(() => {});
    try {
      const mods = [fm("acme.ghost", "ghost")];
      const b = makeBootstrap({ modules: [] });
      resolveModules(mods, b);
      resolveModules(mods, b);
      expect(spy).toHaveBeenCalledTimes(1);
      expect(String(spy.mock.calls[0]?.[0])).toContain("acme.ghost");
    } finally {
      spy.mockRestore();
    }
  });
});

describe("moduleStatus", () => {
  test("status modul menurut key; tidak ada berarti undefined", () => {
    const b = makeBootstrap({
      modules: [makeBootstrapModule("acme.one", "one", "disabled")],
    });
    expect(moduleStatus(b, "acme.one")).toBe("disabled");
    expect(moduleStatus(b, "acme.none")).toBeUndefined();
  });
});

function flatIds(nodes: readonly NavNode[]): string[] {
  return nodes.flatMap((n) =>
    n.type === "item"
      ? [`${n.moduleKey}/${n.id}`]
      : [`${n.moduleKey}/${n.id}`, ...n.children.map((c) => `${c.moduleKey}/${c.id}`)],
  );
}

describe("selectNavigation (P1)", () => {
  test("hanya modul enabled; to absolut; grup, ikon, dan moduleKey dipertahankan", () => {
    const mods = [fm("acme.one", "one")];
    const b = makeBootstrap({ modules: [makeBootstrapModule("acme.one", "one", "enabled")] });
    const { resolved } = resolveModules(mods, b, { warn: () => {} });
    const nav = selectNavigation(resolved, b);
    expect(nav).toEqual([
      {
        type: "item",
        id: "home",
        label: label("home"),
        to: "/one",
        icon: "home",
        moduleKey: "acme.one",
      },
      { type: "item", id: "notes", label: label("notes"), to: "/one/notes", moduleKey: "acme.one" },
      {
        type: "group",
        id: "reports",
        label: label("reports"),
        moduleKey: "acme.one",
        children: [
          {
            type: "item",
            id: "sales",
            label: label("sales"),
            to: "/one/reports/sales",
            moduleKey: "acme.one",
          },
        ],
      },
    ]);
  });

  test("status bukan enabled menyembunyikan navigasi, dan mengikuti bootstrap terbaru", () => {
    const mods = [fm("acme.one", "one")];
    const enabled = makeBootstrap({ modules: [makeBootstrapModule("acme.one", "one", "enabled")] });
    const { resolved } = resolveModules(mods, enabled, { warn: () => {} });
    expect(selectNavigation(resolved, enabled)).not.toHaveLength(0);
    for (const status of STATUSES.filter((s) => s !== "enabled")) {
      const b = makeBootstrap({ modules: [makeBootstrapModule("acme.one", "one", status)] });
      expect(selectNavigation(resolved, b)).toEqual([]);
    }
    const gone = makeBootstrap({ modules: [] });
    expect(selectNavigation(resolved, gone)).toEqual([]);
  });

  test("P1: untuk modul dan bootstrap acak, hanya modul enabled muncul dengan urutan modul", () => {
    const pool = ["a.one", "a.two", "a.three", "b.one"];
    const statusArb = fc.constantFrom(...STATUSES);
    fc.assert(
      fc.property(
        fc.shuffledSubarray(pool, { minLength: 0 }),
        fc.dictionary(fc.constantFrom(...pool, "x.extra"), statusArb),
        (keys, statuses) => {
          const mods = keys.map((k, i) => fm(k, `s${i}`));
          const bootstrapModules = Object.entries(statuses).map(([k, st]) => {
            const idx = keys.indexOf(k);
            return makeBootstrapModule(k, idx >= 0 ? `s${idx}` : "extra", st);
          });
          const b = makeBootstrap({ modules: bootstrapModules });
          const { resolved } = resolveModules(mods, b, { warn: () => {} });
          const nav = selectNavigation(resolved, b);
          const expected = mods
            .filter((m) => statuses[m.key] === "enabled")
            .flatMap((m) => ["home", "notes", "reports", "sales"].map((id) => `${m.key}/${id}`));
          expect(flatIds(nav)).toEqual(expected);
          for (const n of nav) {
            expect(statuses[n.moduleKey]).toBe("enabled");
          }
        },
      ),
      { numRuns: 200 },
    );
  });
});

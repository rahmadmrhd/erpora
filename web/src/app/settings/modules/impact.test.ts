import { describe, expect, test } from "bun:test";
import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { availableOps, computeImpact, type Effect } from "./impact";
import { forSeeds, type Rng } from "./seeded";
import type { ModuleOp, ModuleStatus, ModuleView } from "./types";

const STATUSES: ModuleStatus[] = [
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
const OPS: ModuleOp[] = ["install", "enable", "disable", "upgrade", "uninstall", "retry"];

function mod(key: string, over: Partial<ModuleView> = {}): ModuleView {
  return {
    key,
    slug: key.split(".").at(-1) ?? key,
    name: key,
    binaryVersion: "1.0.0",
    installedVersion: "1.0.0",
    status: "enabled",
    required: false,
    compatible: true,
    incompatibleCodes: [],
    dependencies: [],
    dependents: [],
    lastOperationId: null,
    ...over,
  };
}

const dep = (module: string, optional = false) => ({ module, version: "^1.0", optional });

describe("availableOps (cermin tabel transisi ADR-0007)", () => {
  test.each([
    ["available", ["install"]],
    ["uninstalled", ["install"]],
    ["installed", ["enable", "uninstall"]],
    ["disabled", ["enable", "uninstall"]],
    ["enabled", ["disable"]],
    ["upgrade_pending", ["upgrade"]],
    ["failed", ["retry"]],
    ["missing", []],
    ["incompatible", ["disable", "uninstall"]],
  ] as const)("%s menawarkan %j", (status, ops) => {
    expect(availableOps(mod("a.x", { status }))).toEqual([...ops]);
  });

  test("modul required menyembunyikan disable dan uninstall", () => {
    expect(availableOps(mod("a.x", { status: "enabled", required: true }))).toEqual([]);
    expect(availableOps(mod("a.x", { status: "installed", required: true }))).toEqual(["enable"]);
    expect(availableOps(mod("a.x", { status: "incompatible", required: true }))).toEqual([]);
  });
});

describe("computeImpact: blocker", () => {
  test("module_required untuk disable/uninstall modul required", () => {
    const t = mod("a.core", { required: true });
    const r = computeImpact({ op: "disable", target: t, modules: [t], frontend: [] });
    expect(r.blockers).toContainEqual({ code: "module_required", modules: ["a.core"] });
  });

  test("dependents_active: disable ditolak bila dependen wajib masih enabled; opsional tidak", () => {
    const t = mod("a.base", { dependents: ["a.req", "a.opt"] });
    const req = mod("a.req", { dependencies: [dep("a.base")] });
    const opt = mod("a.opt", { dependencies: [dep("a.base", true)] });
    const r = computeImpact({ op: "disable", target: t, modules: [t, req, opt], frontend: [] });
    expect(r.blockers).toEqual([{ code: "dependents_active", modules: ["a.req"] }]);
    const ok = computeImpact({
      op: "disable",
      target: t,
      modules: [t, { ...req, status: "disabled" }, opt],
      frontend: [],
    });
    expect(ok.blockers).toEqual([]);
  });

  test("dependents_active: uninstall ditolak bila dependen wajib belum uninstalled", () => {
    const t = mod("a.base", { status: "disabled", dependents: ["a.req"] });
    const req = mod("a.req", { status: "disabled", dependencies: [dep("a.base")] });
    expect(
      computeImpact({ op: "uninstall", target: t, modules: [t, req], frontend: [] }).blockers,
    ).toEqual([{ code: "dependents_active", modules: ["a.req"] }]);
    const gone = { ...req, status: "uninstalled" as const };
    expect(
      computeImpact({ op: "uninstall", target: t, modules: [t, gone], frontend: [] }).blockers,
    ).toEqual([]);
  });

  test("dependency_not_ready: enable butuh dependensi wajib enabled (platform.* diabaikan)", () => {
    const t = mod("a.top", {
      status: "disabled",
      dependencies: [dep("platform.core"), dep("a.low")],
    });
    const low = mod("a.low", { status: "disabled" });
    const r = computeImpact({ op: "enable", target: t, modules: [t, low], frontend: [] });
    expect(r.blockers).toEqual([{ code: "dependency_not_ready", modules: ["a.low"] }]);
    const ok = computeImpact({
      op: "enable",
      target: t,
      modules: [t, { ...low, status: "enabled" }],
      frontend: [],
    });
    expect(ok.blockers).toEqual([]);
  });

  test("install: dependensi belum terpasang menjadi efek needs_dependencies bila withDeps, blocker bila tidak", () => {
    const t = mod("a.top", {
      status: "available",
      installedVersion: null,
      dependencies: [dep("a.low")],
    });
    const low = mod("a.low", { status: "available", installedVersion: null });
    const withDeps = computeImpact({
      op: "install",
      target: t,
      modules: [t, low],
      frontend: [],
      options: { withDeps: true, enable: false },
    });
    expect(withDeps.blockers).toEqual([]);
    expect(withDeps.effects).toContainEqual({ kind: "needs_dependencies", modules: ["a.low"] });
    const without = computeImpact({
      op: "install",
      target: t,
      modules: [t, low],
      frontend: [],
      options: { withDeps: false, enable: false },
    });
    expect(without.blockers).toEqual([{ code: "dependency_not_ready", modules: ["a.low"] }]);
  });

  test("module_incompatible untuk install/enable/upgrade pada modul tidak kompatibel, tidak untuk disable", () => {
    const t = mod("a.bad", { status: "incompatible", compatible: false });
    expect(
      computeImpact({ op: "enable", target: t, modules: [t], frontend: [] }).blockers,
    ).toContainEqual({
      code: "module_incompatible",
      modules: ["a.bad"],
    });
    expect(
      computeImpact({ op: "disable", target: t, modules: [t], frontend: [] }).blockers.map(
        (b) => b.code,
      ),
    ).not.toContain("module_incompatible");
  });

  test("downgrade_not_supported bila versi binary lebih lama daripada ter-install", () => {
    const t = mod("a.x", {
      status: "upgrade_pending",
      binaryVersion: "1.0.0",
      installedVersion: "1.2.0",
    });
    expect(
      computeImpact({ op: "upgrade", target: t, modules: [t], frontend: [] }).blockers,
    ).toContainEqual({
      code: "downgrade_not_supported",
      modules: ["a.x"],
    });
  });

  test("operasi di luar tabel transisi menjadi invalid_transition", () => {
    const t = mod("a.x", { status: "enabled" });
    expect(
      computeImpact({ op: "install", target: t, modules: [t], frontend: [] }).blockers,
    ).toContainEqual({
      code: "invalid_transition",
      modules: ["a.x"],
    });
  });
});

describe("computeImpact: efek", () => {
  const frontend = [
    defineModule({
      key: "a.shop",
      slug: "shop",
      version: "1.0.0",
      navigation: [
        { type: "item", id: "home", label: { ns: "shop", key: "nav.home" }, to: "/" },
        {
          type: "group",
          id: "rep",
          label: { ns: "shop", key: "nav.rep" },
          children: [
            {
              type: "item",
              id: "s",
              label: { ns: "shop", key: "nav.sales" },
              to: "/reports/sales",
            },
          ],
        },
      ],
      registerRoutes: () => [],
      offline: defaultOfflinePolicy,
    }),
  ];
  const t = mod("a.shop", { slug: "shop" });

  test("disable: menu disembunyikan, route tidak aktif, API menolak module_disabled, data tetap", () => {
    const r = computeImpact({ op: "disable", target: t, modules: [t], frontend });
    const kinds = r.effects.map((e) => e.kind);
    expect(kinds).toEqual(["menu_hidden", "routes_inactive", "api_closed", "data_kept"]);
    const hidden = r.effects.find((e) => e.kind === "menu_hidden") as Extract<
      Effect,
      { kind: "menu_hidden" }
    >;
    expect(hidden.labels).toEqual([
      { ns: "shop", key: "nav.home" },
      { ns: "shop", key: "nav.rep" },
    ]);
    const routes = r.effects.find((e) => e.kind === "routes_inactive") as Extract<
      Effect,
      { kind: "routes_inactive" }
    >;
    expect(routes.paths).toEqual(["/shop", "/shop/reports/sales"]);
    expect(r.effects).toContainEqual({
      kind: "api_closed",
      prefix: "/api/shop",
      code: "module_disabled",
    });
  });

  test("uninstall: API menolak module_not_found; modul tanpa frontend tidak punya efek menu/route", () => {
    const u = computeImpact({
      op: "uninstall",
      target: { ...t, status: "disabled" },
      modules: [t],
      frontend,
    });
    expect(u.effects).toContainEqual({
      kind: "api_closed",
      prefix: "/api/shop",
      code: "module_not_found",
    });
    const noFe = computeImpact({ op: "disable", target: t, modules: [t], frontend: [] });
    expect(noFe.effects.map((e) => e.kind)).toEqual(["api_closed", "data_kept"]);
  });

  test("enable: menu dan route aktif; upgrade: perubahan versi dan migrasi", () => {
    const d = mod("a.shop", { status: "disabled", slug: "shop" });
    expect(
      computeImpact({ op: "enable", target: d, modules: [d], frontend }).effects.map((e) => e.kind),
    ).toEqual(["menu_shown", "routes_active"]);
    const up = mod("a.shop", {
      status: "upgrade_pending",
      binaryVersion: "1.1.0",
      installedVersion: "1.0.0",
    });
    const r = computeImpact({ op: "upgrade", target: up, modules: [up], frontend: [] });
    expect(r.effects).toContainEqual({ kind: "version_change", from: "1.0.0", to: "1.1.0" });
    expect(r.effects).toContainEqual({ kind: "migrations_run" });
  });

  test("install: opsi bawaan, migrasi, versi dari null, dan menu bila diaktifkan", () => {
    const av = mod("a.shop", { status: "available", installedVersion: null, slug: "shop" });
    const r = computeImpact({ op: "install", target: av, modules: [av], frontend });
    expect(r.installOptions).toEqual({ withDeps: false, enable: false });
    expect(r.effects).toContainEqual({ kind: "version_change", from: null, to: "1.0.0" });
    expect(r.effects).toContainEqual({ kind: "migrations_run" });
    const on = computeImpact({
      op: "install",
      target: av,
      modules: [av],
      frontend,
      options: { withDeps: false, enable: true },
    });
    expect(on.effects.map((e) => e.kind)).toContain("menu_shown");
    expect(
      computeImpact({
        op: "enable",
        target: mod("a.x", { status: "disabled" }),
        modules: [],
        frontend,
      }).installOptions,
    ).toBeUndefined();
  });
});

describe("P7: dampak tidak melanggar transisi (properti)", () => {
  /** Dunia acak: 1-6 modul dengan status, flag, dan dependensi (wajib/opsional) acak; `dependents` konsisten. */
  function randomWorld(r: Rng): ModuleView[] {
    const n = r.int(1, 6);
    const keys = Array.from({ length: n }, (_, i) => `m.k${i}`);
    const modules = keys.map((key, i) => {
      const optional = r.bool();
      const picks = new Set<number>();
      for (let c = r.int(0, 3); c > 0; c--) picks.add(r.int(0, n - 1));
      picks.delete(i);
      return mod(key, {
        status: r.pick(STATUSES),
        required: r.bool(),
        compatible: r.bool(),
        dependencies: [...picks].map((d) => dep(keys[d] as string, optional)),
      });
    });
    for (const m of modules) {
      m.dependents = modules
        .filter((o) => o.dependencies.some((d) => d.module === m.key))
        .map((o) => o.key);
    }
    return modules;
  }

  test("blocker memakai kode lifecycle yang dikenal; operasi non-tawaran selalu invalid_transition", async () => {
    const codes = new Set([
      "module_required",
      "dependency_not_ready",
      "dependents_active",
      "module_incompatible",
      "invalid_transition",
      "downgrade_not_supported",
    ]);
    await forSeeds(300, (r) => {
      const modules = randomWorld(r);
      const op = r.pick(OPS);
      const target = modules[0] as ModuleView;
      const res = computeImpact({ op, target, modules, frontend: [] });
      for (const b of res.blockers) expect(codes.has(b.code)).toBe(true);
      if (!availableOps(target).includes(op)) {
        expect(res.blockers.map((b) => b.code)).toContain("invalid_transition");
      }
    });
  });

  test("blockers kosong untuk operasi yang ditawarkan berarti aturan dependensi terpenuhi", async () => {
    await forSeeds(500, (r) => {
      const modules = randomWorld(r);
      const op = r.pick(OPS);
      const target = modules[0] as ModuleView;
      if (!availableOps(target).includes(op)) return;
      const res = computeImpact({
        op,
        target,
        modules,
        frontend: [],
        options: { withDeps: true, enable: false },
      });
      if (res.blockers.length > 0) return;
      const byKey = new Map(modules.map((m) => [m.key, m]));
      const requiredDeps = target.dependencies.filter(
        (d) => !d.optional && !d.module.startsWith("platform."),
      );
      if (op === "enable") {
        for (const d of requiredDeps) expect(byKey.get(d.module)?.status).toBe("enabled");
      }
      const requiredDependents = target.dependents.filter((key) =>
        byKey.get(key)?.dependencies.some((x) => x.module === target.key && !x.optional),
      );
      if (op === "disable") {
        for (const key of requiredDependents) expect(byKey.get(key)?.status).not.toBe("enabled");
      }
      if (op === "uninstall") {
        for (const key of requiredDependents)
          expect(["uninstalled", "available"]).toContain(byKey.get(key)?.status as string);
      }
      if (op === "disable" || op === "uninstall") expect(target.required).toBe(false);
    });
  });
});

// Dampak aksi modul (M3a K-6, PK-1): dihitung di klien dari status, dependensi, dan navigasi modul frontend; backend tetap
// otoritas dan menolak dengan kode yang sama. Antarmuka (`Impact`) sengaja terisolasi sehingga dapat diganti hasil `plan`
// server tanpa mengubah dialog.
import type { FrontendModule, MessageRef, NavigationEntry } from "@erpora/module-sdk";
import type { ModuleOp, ModuleStatus, ModuleView } from "./types";

export type BlockerCode =
  | "module_required"
  | "dependency_not_ready"
  | "dependents_active"
  | "module_incompatible"
  | "invalid_transition"
  | "downgrade_not_supported";

export interface Blocker {
  code: BlockerCode;
  /** Key modul yang terlibat (target atau dependensi/dependen). */
  modules: readonly string[];
}

export type Effect =
  | { kind: "menu_hidden"; labels: readonly MessageRef[] }
  | { kind: "menu_shown"; labels: readonly MessageRef[] }
  | { kind: "routes_inactive"; paths: readonly string[] }
  | { kind: "routes_active"; paths: readonly string[] }
  | { kind: "api_closed"; prefix: string; code: "module_disabled" | "module_not_found" }
  | { kind: "data_kept" }
  | { kind: "migrations_run" }
  | { kind: "version_change"; from: string | null; to: string }
  | { kind: "needs_dependencies"; modules: readonly string[] }
  /** Dependen wajib `upgrade_pending`/`failed` yang mungkin berstatus tersimpan `enabled` (previous_status tak diketahui klien). */
  | { kind: "dependents_uncertain"; modules: readonly string[] };

export interface InstallOptions {
  withDeps: boolean;
  enable: boolean;
}

export interface Impact {
  blockers: readonly Blocker[];
  effects: readonly Effect[];
  /** Hanya untuk install: nilai awal opsi dialog. */
  installOptions?: InstallOptions;
}

/** Tabel transisi ADR-0007 (tanpa aturan `required`). `incompatible` hanya disable/uninstall. */
const TRANSITIONS: Record<ModuleStatus, readonly ModuleOp[]> = {
  available: ["install"],
  uninstalled: ["install"],
  installed: ["enable", "uninstall"],
  disabled: ["enable", "uninstall"],
  enabled: ["disable"],
  upgrade_pending: ["upgrade"],
  failed: ["retry"],
  missing: [],
  incompatible: ["disable", "uninstall"],
};

export function transitionAllowed(status: ModuleStatus, op: ModuleOp): boolean {
  return TRANSITIONS[status].includes(op);
}

const REMOVING: readonly ModuleOp[] = ["disable", "uninstall"];

/** Operasi yang ditawarkan UI untuk satu modul; `required` menyembunyikan disable dan uninstall. */
export function availableOps(m: Pick<ModuleView, "status" | "required">): readonly ModuleOp[] {
  return TRANSITIONS[m.status].filter((op) => !(m.required && REMOVING.includes(op)));
}

/** Perbandingan versi numerik `x.y.z` (sufiks pra-rilis diabaikan). */
function compareVersions(a: string, b: string): number {
  const part = (v: string) =>
    v
      .split(/[-+]/)[0]
      ?.split(".")
      .map((n) => Number.parseInt(n, 10) || 0) ?? [];
  const pa = part(a);
  const pb = part(b);
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] ?? 0) - (pb[i] ?? 0);
    if (d !== 0) return d;
  }
  return 0;
}

const isPlatform = (key: string) => key.startsWith("platform.");
// Prasyarat install backend (ADR-0007 butir 3): dependensi minimal installed|enabled|disabled.
const INSTALLED: readonly ModuleStatus[] = ["installed", "enabled", "disabled"];

function requiredDeps(m: ModuleView): string[] {
  return m.dependencies.filter((d) => !d.optional && !isPlatform(d.module)).map((d) => d.module);
}

/** Dependen yang membutuhkan `target` sebagai dependensi WAJIB. */
function requiredDependents(
  target: ModuleView,
  byKey: ReadonlyMap<string, ModuleView>,
): ModuleView[] {
  return target.dependents
    .map((key) => byKey.get(key))
    .filter((m): m is ModuleView => m !== undefined)
    .filter((m) => m.dependencies.some((d) => d.module === target.key && !d.optional));
}

function navigationOf(fe: FrontendModule | undefined): { labels: MessageRef[]; paths: string[] } {
  if (!fe) return { labels: [], paths: [] };
  const labels: MessageRef[] = [];
  const paths: string[] = [];
  const add = (to: string) => {
    const p = `/${fe.slug}${to === "/" ? "" : to}`;
    if (!paths.includes(p)) paths.push(p);
  };
  const walk = (entry: NavigationEntry) => {
    labels.push(entry.label);
    if (entry.type === "item") add(entry.to);
    else for (const c of entry.children) add(c.to);
  };
  for (const e of fe.navigation) walk(e);
  return { labels, paths };
}

export interface ImpactInput {
  op: ModuleOp;
  target: ModuleView;
  modules: readonly ModuleView[];
  frontend: readonly FrontendModule[];
  /** Opsi install terpilih di dialog; tanpa ini memakai nilai awal. */
  options?: InstallOptions;
}

export function computeImpact({ op, target, modules, frontend, options }: ImpactInput): Impact {
  const byKey = new Map(modules.map((m) => [m.key, m]));
  const blockers: Blocker[] = [];
  const effects: Effect[] = [];
  const block = (code: BlockerCode, keys: readonly string[]) => {
    if (keys.length > 0) blockers.push({ code, modules: keys });
  };

  if (!transitionAllowed(target.status, op)) block("invalid_transition", [target.key]);
  if (target.required && REMOVING.includes(op)) block("module_required", [target.key]);
  if (
    (op === "install" || op === "enable" || op === "upgrade") &&
    (!target.compatible || target.status === "incompatible")
  ) {
    block("module_incompatible", [target.key]);
  }
  if (
    op === "upgrade" &&
    target.installedVersion !== null &&
    compareVersions(target.binaryVersion, target.installedVersion) < 0
  ) {
    block("downgrade_not_supported", [target.key]);
  }

  const deps = requiredDeps(target);
  const needsInstall = deps.filter((k) => {
    const s = byKey.get(k)?.status;
    return s === "available" || s === "uninstalled";
  });
  const installOptions: InstallOptions | undefined =
    op === "install"
      ? (options ?? { withDeps: needsInstall.length > 0, enable: false })
      : undefined;

  if (op === "install") {
    const broken = deps.filter((k) => {
      const s = byKey.get(k)?.status;
      return s === undefined || (!INSTALLED.includes(s) && !needsInstall.includes(k));
    });
    block("dependency_not_ready", broken);
    if (needsInstall.length > 0) {
      if (installOptions?.withDeps)
        effects.push({ kind: "needs_dependencies", modules: needsInstall });
      else block("dependency_not_ready", needsInstall);
    }
  }
  if (op === "enable") {
    block(
      "dependency_not_ready",
      deps.filter((k) => byKey.get(k)?.status !== "enabled"),
    );
  }
  if (op === "disable") {
    // Backend memakai status TERSIMPAN yang tidak dibuka API: dependen `incompatible`/`upgrade_pending`/`failed` bisa
    // aktif atau tidak. Hanya `enabled` yang pasti memblokir; sisanya peringatan (keputusan G30), dan backend tetap
    // otoritas (422 `dependents_active` dipetakan ke pesan katalog).
    const dependents = requiredDependents(target, byKey);
    block(
      "dependents_active",
      dependents.filter((m) => m.status === "enabled").map((m) => m.key),
    );
    const uncertain = dependents.filter(
      (m) => m.status === "incompatible" || m.status === "upgrade_pending" || m.status === "failed",
    );
    if (uncertain.length > 0) {
      effects.push({ kind: "dependents_uncertain", modules: uncertain.map((m) => m.key) });
    }
  }
  if (op === "uninstall") {
    block(
      "dependents_active",
      requiredDependents(target, byKey)
        .filter((m) => m.status !== "uninstalled" && m.status !== "available")
        .map((m) => m.key),
    );
  }

  const nav = navigationOf(frontend.find((f) => f.key === target.key));
  const showUi = () => {
    if (nav.labels.length > 0) effects.push({ kind: "menu_shown", labels: nav.labels });
    if (nav.paths.length > 0) effects.push({ kind: "routes_active", paths: nav.paths });
  };
  const hideUi = () => {
    if (nav.labels.length > 0) effects.push({ kind: "menu_hidden", labels: nav.labels });
    if (nav.paths.length > 0) effects.push({ kind: "routes_inactive", paths: nav.paths });
  };

  switch (op) {
    case "disable":
      hideUi();
      effects.push({ kind: "api_closed", prefix: `/api/${target.slug}`, code: "module_disabled" });
      effects.push({ kind: "data_kept" });
      break;
    case "uninstall":
      hideUi();
      effects.push({ kind: "api_closed", prefix: `/api/${target.slug}`, code: "module_not_found" });
      effects.push({ kind: "data_kept" });
      break;
    case "enable":
      showUi();
      break;
    case "upgrade":
      effects.push({
        kind: "version_change",
        from: target.installedVersion,
        to: target.binaryVersion,
      });
      effects.push({ kind: "migrations_run" });
      break;
    case "install":
      effects.push({ kind: "version_change", from: null, to: target.binaryVersion });
      effects.push({ kind: "migrations_run" });
      if (installOptions?.enable) showUi();
      break;
    case "retry":
      break;
  }

  return { blockers, effects, ...(installOptions ? { installOptions } : {}) };
}

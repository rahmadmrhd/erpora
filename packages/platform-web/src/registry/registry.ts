import type { FrontendModule, MessageRef, NavigationItem } from "@erpora/module-sdk";
import type { Bootstrap, BootstrapModule, ModuleStatus } from "../bootstrap/types";

/** Dilempar saat dua modul frontend memakai `key` atau `slug` yang sama (aplikasi gagal start). */
export class ModuleRegistryError extends Error {
  override readonly name: string = "ModuleRegistryError";
}
export class DuplicateModuleKeyError extends ModuleRegistryError {
  override readonly name = "DuplicateModuleKeyError";
  readonly key: string;
  constructor(key: string) {
    super(`modul frontend dengan key "${key}" didaftarkan lebih dari sekali`);
    this.key = key;
  }
}
export class DuplicateModuleSlugError extends ModuleRegistryError {
  override readonly name = "DuplicateModuleSlugError";
  readonly slug: string;
  constructor(slug: string, keys: readonly [string, string]) {
    super(`slug "${slug}" dipakai oleh dua modul frontend: ${keys[0]} dan ${keys[1]}`);
    this.slug = slug;
  }
}

/** Menolak `key` ganda (R6.2) dan `slug` ganda (basePath bentrok). Daftar kosong atau unik tidak melempar. */
export function validateModules(mods: readonly FrontendModule[]): void {
  const keys = new Set<string>();
  const slugs = new Map<string, string>();
  for (const m of mods) {
    if (keys.has(m.key)) throw new DuplicateModuleKeyError(m.key);
    keys.add(m.key);
    const other = slugs.get(m.slug);
    if (other !== undefined) throw new DuplicateModuleSlugError(m.slug, [other, m.key]);
    slugs.set(m.slug, m.key);
  }
}

export interface RegistryWarning {
  kind: "not_in_bootstrap" | "version_mismatch" | "offline_mismatch" | "slug_mismatch";
  key: string;
  detail: string;
}

export interface ResolvedModule {
  readonly module: FrontendModule;
  /** Entri bootstrap saat resolusi (status sebenarnya dibaca lewat `moduleStatus` pada bootstrap terbaru). */
  readonly entry: BootstrapModule;
}

export interface ResolveOptions {
  /** Penerima peringatan; bawaan `console.warn`, sekali per `(kind, key)`. */
  warn?: (w: RegistryWarning) => void;
}

const warned = new Set<string>();

/** Mengosongkan catatan peringatan bawaan (untuk test). */
export function resetRegistryWarnings(): void {
  warned.clear();
}

function defaultWarn(w: RegistryWarning): void {
  const id = `${w.kind}:${w.key}`;
  if (warned.has(id)) return;
  warned.add(id);
  console.warn(`[erpora] registry modul (${w.kind}) ${w.key}: ${w.detail}`);
}

/**
 * Mencocokkan modul frontend dengan bootstrap berdasarkan `key` (R6.3, R6.4). Modul yang tidak ada di bootstrap
 * diabaikan; versi, `offline`, dan `slug` yang tidak sinkron hanya memunculkan peringatan. Dijalankan sekali untuk
 * router; urutan hasil mengikuti urutan `mods`.
 */
export function resolveModules(
  mods: readonly FrontendModule[],
  bootstrap: Bootstrap,
  opts: ResolveOptions = {},
): { resolved: readonly ResolvedModule[]; warnings: readonly RegistryWarning[] } {
  const warn = opts.warn ?? defaultWarn;
  const byKey = new Map(bootstrap.modules.map((e) => [e.key, e]));
  const resolved: ResolvedModule[] = [];
  const warnings: RegistryWarning[] = [];
  const report = (w: RegistryWarning) => {
    warnings.push(w);
    warn(w);
  };
  for (const module of mods) {
    const entry = byKey.get(module.key);
    if (!entry) {
      report({
        kind: "not_in_bootstrap",
        key: module.key,
        detail: "modul frontend tidak ada di bootstrap backend; diabaikan",
      });
      continue;
    }
    if (entry.version !== module.version) {
      report({
        kind: "version_mismatch",
        key: module.key,
        detail: `versi frontend ${module.version} berbeda dari backend ${entry.version} (build tidak sinkron?)`,
      });
    }
    if (entry.slug !== module.slug) {
      report({
        kind: "slug_mismatch",
        key: module.key,
        detail: `slug frontend "${module.slug}" berbeda dari backend "${entry.slug}"`,
      });
    }
    if ((module.offline.mode !== "online-only") !== entry.offline) {
      report({
        kind: "offline_mismatch",
        key: module.key,
        detail: `mode offline frontend "${module.offline.mode}" tidak cocok dengan backend (offline=${entry.offline})`,
      });
    }
    resolved.push({ module, entry });
  }
  return { resolved, warnings };
}

export type NavNodeItem = Omit<NavigationItem, "label"> & {
  label: MessageRef;
  moduleKey: string;
};
export type NavNode =
  | NavNodeItem
  | {
      type: "group";
      id: string;
      label: MessageRef;
      icon?: string;
      moduleKey: string;
      children: readonly NavNodeItem[];
    };

/** Status modul pada bootstrap; `undefined` bila modul tidak ada. */
export function moduleStatus(bootstrap: Bootstrap, key: string): ModuleStatus | undefined {
  return bootstrap.modules.find((m) => m.key === key)?.status;
}

function absolute(slug: string, to: string): string {
  return to === "/" ? `/${slug}` : `/${slug}${to}`;
}

/**
 * Navigasi untuk sidebar: hanya modul berstatus `enabled` pada bootstrap terbaru (R6.5; backend tetap otoritas),
 * dalam urutan modul; `to` sudah absolut (`/<slug>` + `to`).
 */
export function selectNavigation(
  resolved: readonly ResolvedModule[],
  bootstrap: Bootstrap,
): readonly NavNode[] {
  const out: NavNode[] = [];
  for (const { module } of resolved) {
    if (moduleStatus(bootstrap, module.key) !== "enabled") continue;
    const item = (e: NavigationItem): NavNodeItem => ({
      type: "item",
      id: e.id,
      label: e.label,
      to: absolute(module.slug, e.to),
      ...(e.icon === undefined ? {} : { icon: e.icon }),
      moduleKey: module.key,
    });
    for (const entry of module.navigation) {
      if (entry.type === "item") {
        out.push(item(entry));
      } else {
        out.push({
          type: "group",
          id: entry.id,
          label: entry.label,
          ...(entry.icon === undefined ? {} : { icon: entry.icon }),
          moduleKey: module.key,
          children: entry.children.map(item),
        });
      }
    }
  }
  return out;
}

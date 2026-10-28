import {
  ASSET_STRATEGIES,
  CONFLICT_STRATEGIES,
  OFFLINE_MODES,
  type OfflinePolicy,
} from "./offline";
import type { FrontendModule, MessageRef, NavigationEntry, NavigationItem } from "./types";

/** Dilempar `defineModule` bila definisi modul tidak valid; `field` menyebut bidang yang salah. */
export class ModuleDefinitionError extends Error {
  readonly field: string;
  constructor(moduleKey: string, field: string, reason: string) {
    super(`defineModule(${moduleKey}): ${field}: ${reason}`);
    this.name = "ModuleDefinitionError";
    this.field = field;
  }
}

// Pola sama dengan `contracts/module.schema.json` (key, slug, version) agar frontend dan manifest tidak berbeda.
const KEY_PATTERN = /^[a-z][a-z0-9]*(-[a-z0-9]+)*\.[a-z][a-z0-9]*(-[a-z0-9]+)*$/;
const SLUG_PATTERN = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;
const SLUG_MAX = 50;
const SEMVER_PATTERN =
  /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$/;
// Path internal: diawali satu "/", tanpa spasi, karakter kontrol, atau "\", dan bukan protocol-relative ("//").
const NAV_TO_PATTERN = /^\/(?!\/)[^\s\\\p{Cc}]*$/u;

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function checkMessageRef(key: string, field: string, ref: unknown): asserts ref is MessageRef {
  if (
    !isRecord(ref) ||
    typeof ref.ns !== "string" ||
    ref.ns === "" ||
    typeof ref.key !== "string" ||
    ref.key === ""
  ) {
    throw new ModuleDefinitionError(
      key,
      field,
      "harus MessageRef { ns, key } berisi string tidak kosong",
    );
  }
}

function checkOffline(key: string, offline: unknown): asserts offline is OfflinePolicy {
  if (!isRecord(offline)) {
    throw new ModuleDefinitionError(key, "offline", "harus objek OfflinePolicy");
  }
  if (!OFFLINE_MODES.includes(offline.mode as never)) {
    throw new ModuleDefinitionError(
      key,
      "offline.mode",
      `harus salah satu dari ${OFFLINE_MODES.join(", ")}`,
    );
  }
  const assets = offline.assets;
  if (!isRecord(assets) || !ASSET_STRATEGIES.includes(assets.strategy as never)) {
    throw new ModuleDefinitionError(
      key,
      "offline.assets.strategy",
      `harus salah satu dari ${ASSET_STRATEGIES.join(", ")}`,
    );
  }
  const data = offline.data;
  if (
    !isRecord(data) ||
    !Array.isArray(data.collections) ||
    !data.collections.every((c) => typeof c === "string")
  ) {
    throw new ModuleDefinitionError(key, "offline.data.collections", "harus array string");
  }
  if (!Number.isInteger(data.maxAgeSeconds) || (data.maxAgeSeconds as number) < 0) {
    throw new ModuleDefinitionError(key, "offline.data.maxAgeSeconds", "harus bilangan bulat >= 0");
  }
  if (
    data.maxRecords !== undefined &&
    (!Number.isInteger(data.maxRecords) || (data.maxRecords as number) < 1)
  ) {
    throw new ModuleDefinitionError(
      key,
      "offline.data.maxRecords",
      "harus bilangan bulat >= 1 bila diisi",
    );
  }
  const sync = offline.sync;
  if (!isRecord(sync) || typeof sync.enabled !== "boolean") {
    throw new ModuleDefinitionError(key, "offline.sync.enabled", "harus boolean");
  }
  if (!CONFLICT_STRATEGIES.includes(sync.conflict as never)) {
    throw new ModuleDefinitionError(
      key,
      "offline.sync.conflict",
      `harus salah satu dari ${CONFLICT_STRATEGIES.join(", ")}`,
    );
  }
}

function checkNavId(
  key: string,
  path: string,
  entry: Record<string, unknown>,
  seen: Set<string>,
): string {
  if (typeof entry.id !== "string" || entry.id === "") {
    throw new ModuleDefinitionError(key, `${path}.id`, "harus string tidak kosong");
  }
  if (seen.has(entry.id)) {
    throw new ModuleDefinitionError(
      key,
      `${path}.id`,
      `id "${entry.id}" ganda dalam navigasi modul`,
    );
  }
  seen.add(entry.id);
  return entry.id;
}

function checkIcon(key: string, path: string, icon: unknown): void {
  if (icon !== undefined && (typeof icon !== "string" || icon === "")) {
    throw new ModuleDefinitionError(key, `${path}.icon`, "harus string tidak kosong bila diisi");
  }
}

function checkItem(key: string, path: string, item: unknown, seen: Set<string>): NavigationItem {
  if (!isRecord(item) || item.type !== "item") {
    throw new ModuleDefinitionError(key, path, 'harus entri { type: "item" }');
  }
  const id = checkNavId(key, path, item, seen);
  if (typeof item.to !== "string" || !NAV_TO_PATTERN.test(item.to)) {
    throw new ModuleDefinitionError(
      key,
      `${path}.to`,
      "harus path internal berawalan '/' (bukan URL atau '//')",
    );
  }
  checkMessageRef(key, `${path}.label`, item.label);
  checkIcon(key, path, item.icon);
  return Object.freeze({
    type: "item",
    id,
    label: Object.freeze({ ns: item.label.ns, key: item.label.key }),
    to: item.to,
    ...(item.icon === undefined ? {} : { icon: item.icon as string }),
  });
}

function checkNavigation(key: string, navigation: unknown): readonly NavigationEntry[] {
  if (!Array.isArray(navigation)) {
    throw new ModuleDefinitionError(key, "navigation", "harus array");
  }
  const seen = new Set<string>();
  const out: NavigationEntry[] = [];
  navigation.forEach((entry: unknown, i) => {
    const path = `navigation[${i}]`;
    if (!isRecord(entry)) throw new ModuleDefinitionError(key, path, "harus objek");
    if (entry.type === "item") {
      out.push(checkItem(key, path, entry, seen));
      return;
    }
    if (entry.type !== "group") {
      throw new ModuleDefinitionError(key, `${path}.type`, 'harus "item" atau "group"');
    }
    const id = checkNavId(key, path, entry, seen);
    checkMessageRef(key, `${path}.label`, entry.label);
    checkIcon(key, path, entry.icon);
    if (!Array.isArray(entry.children) || entry.children.length === 0) {
      throw new ModuleDefinitionError(
        key,
        `${path}.children`,
        "grup harus punya minimal satu anak",
      );
    }
    const children = entry.children.map((child: unknown, j) =>
      checkItem(key, `${path}.children[${j}]`, child, seen),
    );
    out.push(
      Object.freeze({
        type: "group",
        id,
        label: Object.freeze({ ns: entry.label.ns, key: entry.label.key }),
        ...(entry.icon === undefined ? {} : { icon: entry.icon as string }),
        children: Object.freeze(children),
      }),
    );
  });
  return Object.freeze(out);
}

function freezeOffline(o: OfflinePolicy): OfflinePolicy {
  return Object.freeze({
    mode: o.mode,
    assets: Object.freeze({ strategy: o.assets.strategy }),
    data: Object.freeze({
      collections: Object.freeze([...o.data.collections]),
      maxAgeSeconds: o.data.maxAgeSeconds,
      ...(o.data.maxRecords === undefined ? {} : { maxRecords: o.data.maxRecords }),
    }),
    sync: Object.freeze({ enabled: o.sync.enabled, conflict: o.sync.conflict }),
  });
}

/**
 * Mendeklarasikan modul frontend. Memvalidasi bentuk (key/slug/version berpola sama dengan manifest, navigasi,
 * offline) dan mengembalikan salinan beku; objek masukan tidak diubah. Melempar `ModuleDefinitionError` yang
 * menyebut bidang yang salah.
 */
export function defineModule<const S extends string>(m: FrontendModule<S>): FrontendModule<S> {
  const input = m as unknown as Record<string, unknown>;
  const key = typeof input.key === "string" ? input.key : "?";
  if (typeof input.key !== "string" || !KEY_PATTERN.test(input.key)) {
    throw new ModuleDefinitionError(
      key,
      "key",
      "harus berpola <publisher>.<nama> (huruf kecil, angka, '-')",
    );
  }
  if (
    typeof input.slug !== "string" ||
    input.slug.length > SLUG_MAX ||
    !SLUG_PATTERN.test(input.slug)
  ) {
    throw new ModuleDefinitionError(
      key,
      "slug",
      `harus huruf kecil/angka/'-' diawali huruf, maks ${SLUG_MAX} karakter`,
    );
  }
  if (typeof input.version !== "string" || !SEMVER_PATTERN.test(input.version)) {
    throw new ModuleDefinitionError(key, "version", "harus semver (mis. 1.0.0)");
  }
  if (input.title !== undefined) checkMessageRef(key, "title", input.title);
  const navigation = checkNavigation(key, input.navigation);
  if (typeof input.registerRoutes !== "function") {
    throw new ModuleDefinitionError(key, "registerRoutes", "harus fungsi (base) => route[]");
  }
  checkOffline(key, input.offline);

  return Object.freeze({
    key: m.key,
    slug: m.slug,
    version: m.version,
    ...(m.title === undefined
      ? {}
      : { title: Object.freeze({ ns: m.title.ns, key: m.title.key }) as MessageRef }),
    navigation,
    registerRoutes: m.registerRoutes,
    offline: freezeOffline(m.offline),
  });
}

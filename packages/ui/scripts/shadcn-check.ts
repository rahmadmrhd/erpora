// Pemeriksaan kunci snapshot komponen shadcn (M3a revisi, task 1.3 dan 2.x; R1.1, R1.3, D-1).
//
// `packages/ui/shadcn.lock.json` menyimpan hash isi setiap berkas di `src/components/ui` dan hook hasil
// CLI di `src/hooks` setelah normalisasi impor `cn` DAN patch label deklaratif (`shadcn.patches.json`).
// Pemeriksaan ini offline (tanpa jaringan, tanpa CLI shadcn): gagal bila ada berkas yang berbeda dari
// kunci, berkas tanpa kunci, kunci tanpa berkas, atau patch terdaftar yang belum terpasang. Pembaruan
// HANYA lewat `task shadcn:sync` (shadcn-sync.ts), yang menulis ulang kunci.
//
// Fungsi di berkas ini murni atas sistem berkas lokal; `main()` hanya dijalankan sebagai skrip.
import { createHash } from "node:crypto";
import { existsSync, readdirSync, readFileSync } from "node:fs";
import { basename, dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

/** Jalur impor `cn` milik repo; registry shadcn menulis `from "cn"`. */
export const LOCAL_CN_IMPORT = "@erpora/ui/lib/utils";

export interface LockEntry {
  /** Nama berkas relatif terhadap folder asalnya (`components/ui` atau `hooks`). */
  file: string;
  sha256: string;
}

export interface ShadcnLock {
  /** Versi CLI shadcn terpin yang dipakai `shadcn:sync` terakhir. */
  cli: string;
  /**
   * `false` = belum disinkronkan penuh: masih ada komponen yang menyimpang dari registry dan belum
   * dikunci. Pemeriksaan dilewati dengan peringatan. `shadcn:sync` mengubahnya menjadi `true`
   * otomatis begitu semua berkas komponen sudah punya entri.
   */
  synced: boolean;
  components: Record<string, LockEntry>;
}

export interface CheckResult {
  skipped: boolean;
  problems: string[];
}

/** Mengganti `from "cn"` (kutip ganda atau tunggal) menjadi jalur impor `cn` repo. */
export function normalizeCn(src: string): string {
  return src.replace(/(\bfrom\s+)(["'])cn\2/g, `$1"${LOCAL_CN_IMPORT}"`);
}

/** sha256 (hex) isi setelah normalisasi impor `cn` dan akhir baris CRLF menjadi LF. */
export function hashContent(src: string): string {
  const normalized = normalizeCn(src).replace(/\r\n/g, "\n");
  return createHash("sha256").update(normalized, "utf8").digest("hex");
}

const SOURCE_FILE = /\.tsx?$/;
const TEST_FILE = /\.test\.tsx?$/;

function listSourceFiles(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .filter((e) => e.isFile() && SOURCE_FILE.test(e.name) && !TEST_FILE.test(e.name))
    .map((e) => e.name)
    .sort();
}

/** Berkas komponen (bukan tes, bukan subfolder) di folder `components/ui`, terurut. */
export const listComponentFiles = listSourceFiles;

export const componentName = (file: string): string => file.replace(SOURCE_FILE, "");

/** Awalan kunci untuk berkas hook hasil CLI (`src/hooks/*.ts`), mis. `hooks/use-mobile`. */
export const HOOK_PREFIX = "hooks/";

const shortName = (key: string): string => key.replace(HOOK_PREFIX, "");

interface Item {
  file: string;
  path: string;
}

/** Semua berkas yang dikunci: komponen (kunci = nama) dan hook (kunci = `hooks/<nama>`). */
function collectItems(uiDir: string, hooksDir?: string): Map<string, Item> {
  const items = new Map<string, Item>();
  for (const file of listSourceFiles(uiDir)) {
    items.set(componentName(file), { file, path: join(uiDir, file) });
  }
  if (hooksDir) {
    for (const file of listSourceFiles(hooksDir)) {
      items.set(HOOK_PREFIX + componentName(file), { file, path: join(hooksDir, file) });
    }
  }
  return items;
}

const hashPath = (path: string): string => hashContent(readFileSync(path, "utf8"));

/**
 * Patch deklaratif (`shadcn.patches.json`): penggantian string yang diizinkan atas berkas hasil registry
 * setelah normalisasi `cn`, mis. teks Inggris tertanam menjadi `<UiLabel k="..." />` atau kelas palet
 * langsung menjadi token DS. Hash di kunci dihitung SETELAH patch.
 */
export interface LabelPatch {
  /** Kunci komponen (`dialog`) atau hook (`hooks/use-mobile`). */
  component: string;
  find: string;
  replace: string;
  /** Jumlah kemunculan `find` yang diharapkan (default 1). */
  count?: number;
  reason?: string;
}

const countOccurrences = (src: string, needle: string): number =>
  needle === "" ? 0 : src.split(needle).length - 1;

/** Menerapkan patch milik `component`; idempoten; melempar bila string asal tidak sesuai harapan. */
export function applyPatches(component: string, src: string, patches: LabelPatch[]): string {
  let out = src;
  for (const p of patches) {
    if (p.component !== component) continue;
    const expected = p.count ?? 1;
    if (countOccurrences(out, p.replace) >= expected) continue; // sudah terpasang
    const found = countOccurrences(out, p.find);
    if (found !== expected) {
      throw new Error(
        `patch untuk "${component}": string asal ditemukan ${found}x (diharapkan ${expected}x); ` +
          `registry berubah? Perbarui shadcn.patches.json. String: ${JSON.stringify(p.find)}`,
      );
    }
    out = out.split(p.find).join(p.replace);
  }
  return out;
}

/** Patch milik `component` yang belum terpasang pada `src`. */
export function patchProblems(component: string, src: string, patches: LabelPatch[]): string[] {
  const problems: string[] = [];
  for (const p of patches) {
    if (p.component !== component) continue;
    if (countOccurrences(src, p.replace) < (p.count ?? 1)) {
      problems.push(
        `patch belum terpasang pada "${component}" (${JSON.stringify(p.replace.slice(0, 60))}). ` +
          `Terapkan lewat \`task shadcn:sync -- ${shortName(component)}\`.`,
      );
    }
  }
  return problems;
}

export interface BuildLockOptions {
  /** Hanya kunci ini yang di-hash ulang; sisanya dibawa dari `previous`. Default: semua berkas. */
  only?: string[];
  previous?: ShadcnLock;
  hooksDir?: string;
}

/** Membangun kunci dari kondisi berkas saat ini. */
export function buildLock(uiDir: string, cli: string, opts: BuildLockOptions = {}): ShadcnLock {
  const items = collectItems(uiDir, opts.hooksDir);
  const entries = new Map<string, LockEntry>();
  const entryOf = (item: Item): LockEntry => ({ file: item.file, sha256: hashPath(item.path) });

  if (opts.only) {
    for (const [name, entry] of Object.entries(opts.previous?.components ?? {})) {
      if (items.has(name)) entries.set(name, entry);
    }
    for (const name of opts.only) {
      const item = items.get(name);
      if (!item) throw new Error(`buildLock: berkas "${name}" tidak ada di ${uiDir}`);
      entries.set(name, entryOf(item));
    }
  } else {
    for (const [name, item] of items) entries.set(name, entryOf(item));
  }

  const components = Object.fromEntries(
    [...entries.entries()].sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)),
  );
  const synced = [...items.keys()].every((name) => name in components);
  return { cli, synced, components };
}

const syncHint = (name: string): string =>
  `Perbarui lewat \`task shadcn:sync -- ${shortName(name)}\`.`;

export interface CheckOptions {
  hooksDir?: string;
  patches?: LabelPatch[];
}

/** Membandingkan berkas dengan kunci. Kunci belum synced → dilewati (bukan gagal). */
export function checkLock(uiDir: string, lock: ShadcnLock, opts: CheckOptions = {}): CheckResult {
  if (!lock.synced) return { skipped: true, problems: [] };

  const problems: string[] = [];
  const items = collectItems(uiDir, opts.hooksDir);

  for (const [name, item] of items) {
    const entry = lock.components[name];
    if (!entry) {
      problems.push(
        `"${name}" (${item.file}) ada tanpa entri di shadcn.lock.json. ` +
          `Pasang/sinkronkan lewat \`task shadcn:sync -- ${shortName(name)}\`.`,
      );
    } else if (hashPath(item.path) !== entry.sha256) {
      problems.push(
        `"${name}" (${item.file}) berbeda dari kunci snapshot shadcn (berkas komponen shadcn ` +
          `tidak boleh diubah tangan). ${syncHint(name)}`,
      );
    }
  }
  for (const [name, entry] of Object.entries(lock.components)) {
    if (!items.has(name)) {
      problems.push(
        `entri kunci "${name}" menunjuk ${entry.file} yang tidak ada. ` +
          "Bila komponen dihapus sengaja, buang entrinya lewat `task shadcn:sync -- --prune`; " +
          `bila tidak, pulihkan berkasnya (task shadcn:sync -- ${shortName(name)}).`,
      );
    }
  }
  for (const p of opts.patches ?? []) {
    if (!items.has(p.component)) {
      problems.push(`patch menunjuk "${p.component}" yang berkasnya tidak ada.`);
    }
  }
  for (const [name, item] of items) {
    problems.push(...patchProblems(name, readFileSync(item.path, "utf8"), opts.patches ?? []));
  }
  return { skipped: false, problems };
}

/** Membaca `shadcn.patches.json` (`{ patches: LabelPatch[] }`); berkas tak ada = tanpa patch. */
export function loadPatches(path: string): LabelPatch[] {
  if (!existsSync(path)) return [];
  const raw = JSON.parse(readFileSync(path, "utf8")) as { patches?: LabelPatch[] };
  if (!Array.isArray(raw.patches)) throw new Error(`${path}: butuh larik "patches"`);
  for (const p of raw.patches) {
    if (
      typeof p.component !== "string" ||
      typeof p.find !== "string" ||
      typeof p.replace !== "string"
    ) {
      throw new Error(`${path}: patch butuh component, find, replace (string)`);
    }
  }
  return raw.patches;
}

/** Membaca dan memvalidasi bentuk `shadcn.lock.json`. */
export function loadLock(path: string): ShadcnLock {
  if (!existsSync(path)) {
    throw new Error(`${path} tidak ada. Buat lewat \`task shadcn:sync\`.`);
  }
  const raw = JSON.parse(readFileSync(path, "utf8")) as Partial<ShadcnLock>;
  if (
    typeof raw.cli !== "string" ||
    typeof raw.synced !== "boolean" ||
    typeof raw.components !== "object" ||
    raw.components === null
  ) {
    throw new Error(`${path}: bentuk tidak valid (butuh cli, synced, components)`);
  }
  return raw as ShadcnLock;
}

export const PACKAGE_DIR = resolve(dirname(fileURLToPath(import.meta.url)), "..");
export const UI_DIR = join(PACKAGE_DIR, "src", "components", "ui");
export const HOOKS_DIR = join(PACKAGE_DIR, "src", "hooks");
export const LOCK_PATH = join(PACKAGE_DIR, "shadcn.lock.json");
export const PATCHES_PATH = join(PACKAGE_DIR, "shadcn.patches.json");

function main(): number {
  const lock = loadLock(LOCK_PATH);
  const res = checkLock(UI_DIR, lock, { hooksDir: HOOKS_DIR, patches: loadPatches(PATCHES_PATH) });
  if (res.skipped) {
    console.warn(
      `shadcn-check: DILEWATI. ${basename(LOCK_PATH)} belum synced (masih ada komponen yang menyimpang dari registry). ` +
        "Pemeriksaan diwajibkan otomatis begitu `task shadcn:sync` mengunci semua komponen.",
    );
    return 0;
  }
  if (res.problems.length > 0) {
    console.error(`shadcn-check: ${res.problems.length} masalah:`);
    for (const p of res.problems) console.error(`  - ${p}`);
    return 1;
  }
  console.log(
    `shadcn-check: OK (${Object.keys(lock.components).length} berkas cocok dengan kunci, CLI ${lock.cli}).`,
  );
  return 0;
}

if (import.meta.main) process.exit(main());

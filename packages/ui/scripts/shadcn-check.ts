// Pemeriksaan kunci snapshot komponen shadcn (M3a revisi, task 1.3; R1.1, R1.3, D-1).
//
// `packages/ui/shadcn.lock.json` menyimpan hash isi setiap berkas di `src/components/ui` setelah
// normalisasi impor `cn`. Pemeriksaan ini offline (tanpa jaringan, tanpa CLI shadcn): gagal bila ada
// berkas yang berbeda dari kunci, berkas tanpa kunci, atau kunci tanpa berkas. Pembaruan komponen
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
  /** Nama berkas relatif terhadap folder `components/ui`. */
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

const COMPONENT_FILE = /\.tsx?$/;
const TEST_FILE = /\.test\.tsx?$/;

/** Berkas komponen (bukan tes, bukan subfolder) di folder `components/ui`, terurut. */
export function listComponentFiles(uiDir: string): string[] {
  return readdirSync(uiDir, { withFileTypes: true })
    .filter((e) => e.isFile() && COMPONENT_FILE.test(e.name) && !TEST_FILE.test(e.name))
    .map((e) => e.name)
    .sort();
}

export const componentName = (file: string): string => file.replace(COMPONENT_FILE, "");

const hashFile = (uiDir: string, file: string): string =>
  hashContent(readFileSync(join(uiDir, file), "utf8"));

export interface BuildLockOptions {
  /** Hanya komponen ini yang di-hash ulang; sisanya dibawa dari `previous`. Default: semua berkas. */
  only?: string[];
  previous?: ShadcnLock;
}

/** Membangun kunci dari kondisi berkas saat ini. */
export function buildLock(uiDir: string, cli: string, opts: BuildLockOptions = {}): ShadcnLock {
  const files = listComponentFiles(uiDir);
  const byName = new Map(files.map((f) => [componentName(f), f]));
  const entries = new Map<string, LockEntry>();

  if (opts.only) {
    for (const [name, entry] of Object.entries(opts.previous?.components ?? {})) {
      if (byName.has(name)) entries.set(name, entry);
    }
    for (const name of opts.only) {
      const file = byName.get(name);
      if (!file) throw new Error(`buildLock: berkas komponen "${name}" tidak ada di ${uiDir}`);
      entries.set(name, { file, sha256: hashFile(uiDir, file) });
    }
  } else {
    for (const [name, file] of byName) entries.set(name, { file, sha256: hashFile(uiDir, file) });
  }

  const components = Object.fromEntries(
    [...entries.entries()].sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0)),
  );
  const synced = [...byName.keys()].every((name) => name in components);
  return { cli, synced, components };
}

const syncHint = (name: string): string => `Perbarui lewat \`task shadcn:sync -- ${name}\`.`;

/** Membandingkan berkas di `uiDir` dengan kunci. Kunci belum synced → dilewati (bukan gagal). */
export function checkLock(uiDir: string, lock: ShadcnLock): CheckResult {
  if (!lock.synced) return { skipped: true, problems: [] };

  const problems: string[] = [];
  const files = listComponentFiles(uiDir);
  const present = new Set(files.map(componentName));

  for (const file of files) {
    const name = componentName(file);
    const entry = lock.components[name];
    if (!entry) {
      problems.push(
        `komponen "${name}" (${file}) ada tanpa entri di shadcn.lock.json. ` +
          `Pasang/sinkronkan lewat \`task shadcn:sync -- ${name}\`.`,
      );
    } else if (hashFile(uiDir, file) !== entry.sha256) {
      problems.push(
        `komponen "${name}" (${file}) berbeda dari kunci snapshot shadcn (berkas komponen shadcn ` +
          `tidak boleh diubah tangan). ${syncHint(name)}`,
      );
    }
  }
  for (const [name, entry] of Object.entries(lock.components)) {
    if (!present.has(name)) {
      problems.push(
        `entri kunci "${name}" menunjuk ${entry.file} yang tidak ada. ` +
          "Bila komponen dihapus sengaja, buang entrinya lewat `task shadcn:sync -- --prune`; " +
          "bila tidak, pulihkan berkasnya (task shadcn:sync -- " +
          `${name}).`,
      );
    }
  }
  return { skipped: false, problems };
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
export const LOCK_PATH = join(PACKAGE_DIR, "shadcn.lock.json");

function main(): number {
  const lock = loadLock(LOCK_PATH);
  const res = checkLock(UI_DIR, lock);
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
    `shadcn-check: OK (${Object.keys(lock.components).length} komponen cocok dengan kunci, CLI ${lock.cli}).`,
  );
  return 0;
}

if (import.meta.main) process.exit(main());

// Sinkronisasi komponen shadcn (M3a revisi, task 1.3; R1.1, R1.3, R1.9, D-1).
//
//   task shadcn:sync -- badge checkbox ...     # pasang/timpa komponen dari registry (CLI terpin)
//   task shadcn:sync -- --prune                # buang entri kunci yang berkasnya dihapus sengaja
//
// Alur: jalankan `bun run shadcn add <k...> -o -y` (CLI terpin di package.json) → ganti `from "cn"`
// menjadi `from "@erpora/ui/lib/utils"` → hapus dependensi `cn` yang ditulis CLI → pin versi pasti
// dependensi baru (aturan pin repo) → perbarui `shadcn.lock.json`.
//
// Hanya nama komponen polos yang diterima: URL, namespace registry (`@x/y`), dan flag ditolak agar
// skrip tidak bisa diarahkan ke registry tak tepercaya (SSRF/rantai pasok).
import { createHash } from "node:crypto";
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { join, relative, resolve } from "node:path";
import {
  applyPatches,
  buildLock,
  componentName,
  HOOK_PREFIX,
  HOOKS_DIR,
  LOCK_PATH,
  listComponentFiles,
  loadLock,
  loadPatches,
  normalizeCn,
  PACKAGE_DIR,
  PATCHES_PATH,
  type ShadcnLock,
  UI_DIR,
} from "./shadcn-check";

const REPO_ROOT = resolve(PACKAGE_DIR, "..", "..");
const NAME_RE = /^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$/;
const EXACT_VERSION_RE = /^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/;
const RANGE_PREFIX_RE = /^[\^~](\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$/;

/** Memvalidasi argumen: hanya nama komponen polos (huruf kecil, angka, tanda hubung); duplikat dibuang. */
export function parseComponentNames(args: string[]): string[] {
  if (args.length === 0) throw new Error("sebutkan minimal satu nama komponen shadcn");
  const names: string[] = [];
  for (const arg of args) {
    if (!NAME_RE.test(arg)) {
      throw new Error(
        `nama komponen tidak valid: ${JSON.stringify(arg)} (hanya nama polos seperti "badge"; ` +
          "URL, namespace registry, jalur, dan flag ditolak)",
      );
    }
    if (!names.includes(arg)) names.push(arg);
  }
  return names;
}

type Deps = Record<string, string>;
type Pkg = { dependencies?: Deps; devDependencies?: Deps } & Record<string, unknown>;

/** Menghapus dependensi `cn` (alias registry yang ditulis CLI sebagai paket npm, bukan paket kita). */
export function stripCnDependency(pkg: Pkg): { pkg: Pkg; removed: boolean } {
  let removed = false;
  const out: Pkg = { ...pkg };
  for (const field of ["dependencies", "devDependencies"] as const) {
    const deps = pkg[field];
    if (deps && "cn" in deps) {
      const { cn: _cn, ...rest } = deps;
      out[field] = rest;
      removed = true;
    }
  }
  return { pkg: out, removed };
}

export interface PinResult {
  deps: Deps;
  pinned: string[];
  /** Rentang yang tak dapat dipin otomatis (perlu tindakan manual). */
  unpinned: string[];
}

/**
 * Mem-pin ke versi pasti dependensi yang BARU atau BERUBAH spesifikasinya setelah `shadcn add`.
 * Versi diambil dari paket terpasang; bila tak ada, awalan ^/~ dibuang; rentang lain dilaporkan.
 * Dependensi lama yang tidak berubah tidak disentuh; `cn` tidak pernah dibawa.
 */
export function pinDependencies(
  before: Deps,
  after: Deps,
  installed: (name: string) => string | undefined,
): PinResult {
  const pinned: string[] = [];
  const unpinned: string[] = [];
  const entries: [string, string][] = [];
  for (const [name, spec] of Object.entries(after)) {
    if (name === "cn") continue;
    if (before[name] === spec || EXACT_VERSION_RE.test(spec)) {
      entries.push([name, spec]);
      continue;
    }
    const version = installed(name) ?? RANGE_PREFIX_RE.exec(spec)?.[1];
    if (version && EXACT_VERSION_RE.test(version)) {
      entries.push([name, version]);
      pinned.push(name);
    } else {
      entries.push([name, spec]);
      unpinned.push(name);
    }
  }
  entries.sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0));
  return { deps: Object.fromEntries(entries), pinned, unpinned };
}

// ---- I/O (hanya dipakai saat dijalankan sebagai skrip) ----

const sha1 = (data: Buffer | string): string => createHash("sha1").update(data).digest("hex");

/** Peta jalur-relatif → hash untuk semua berkas di bawah `dir` (tanpa node_modules). */
function snapshotTree(dir: string, base = dir): Map<string, string> {
  const out = new Map<string, string>();
  if (!existsSync(dir)) return out;
  for (const e of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, e.name);
    if (e.isDirectory()) {
      if (e.name === "node_modules") continue;
      for (const [k, v] of snapshotTree(full, base)) out.set(k, v);
    } else if (e.isFile()) {
      out.set(relative(base, full).replaceAll("\\", "/"), sha1(readFileSync(full)));
    }
  }
  return out;
}

const changedPaths = (before: Map<string, string>, after: Map<string, string>): string[] =>
  [...after].filter(([k, v]) => before.get(k) !== v).map(([k]) => k);

function cliVersion(): string {
  const pkg = JSON.parse(readFileSync(join(PACKAGE_DIR, "package.json"), "utf8")) as {
    scripts?: Record<string, string>;
  };
  const m = /shadcn@(\d+\.\d+\.\d+)\b/.exec(pkg.scripts?.shadcn ?? "");
  if (!m?.[1])
    throw new Error('package.json: skrip "shadcn" harus memakai CLI terpin shadcn@x.y.z');
  return m[1];
}

function installedVersion(name: string): string | undefined {
  for (const base of [join(PACKAGE_DIR, "node_modules"), join(REPO_ROOT, "node_modules")]) {
    const file = join(base, name, "package.json");
    if (existsSync(file)) {
      return (JSON.parse(readFileSync(file, "utf8")) as { version?: string }).version;
    }
  }
  return undefined;
}

function run(cmd: string[], cwd: string): void {
  const proc = Bun.spawnSync(cmd, { cwd, stdout: "inherit", stderr: "inherit", stdin: "inherit" });
  if (proc.exitCode !== 0)
    throw new Error(`perintah gagal (kode ${proc.exitCode}): ${cmd.join(" ")}`);
}

const writeJson = (path: string, value: unknown): void =>
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`);

function readPackageJson(): Pkg {
  return JSON.parse(readFileSync(join(PACKAGE_DIR, "package.json"), "utf8")) as Pkg;
}

function prune(cli: string): void {
  const previous = loadLock(LOCK_PATH);
  const present = new Set([
    ...listComponentFiles(UI_DIR).map(componentName),
    ...listComponentFiles(HOOKS_DIR).map((f) => HOOK_PREFIX + componentName(f)),
  ]);
  const kept = Object.keys(previous.components).filter((n) => present.has(n));
  const lock = buildLock(UI_DIR, cli, { only: kept, previous, hooksDir: HOOKS_DIR });
  writeJson(LOCK_PATH, lock);
  const dropped = Object.keys(previous.components).filter((n) => !present.has(n));
  console.log(
    `shadcn-sync --prune: ${dropped.length} entri dibuang (${dropped.join(", ") || "-"}).`,
  );
}

function sync(names: string[], cli: string): void {
  const themePath = join(PACKAGE_DIR, "src", "styles", "theme.css");
  const treeBefore = snapshotTree(join(PACKAGE_DIR, "src"));
  const pkgBefore = readPackageJson();

  run([process.execPath, "run", "shadcn", "add", ...names, "-o", "-y"], PACKAGE_DIR);

  // 1) normalisasi `from "cn"` lalu patch deklaratif, hanya pada berkas yang disentuh CLI (komponen
  //    menyimpang lain tak diubah). Patch gagal (string asal hilang) = sync gagal keras: registry berubah.
  const treeAfterCli = snapshotTree(join(PACKAGE_DIR, "src"));
  const uiPrefix = "components/ui/";
  const hookPrefix = "hooks/";
  const changed = changedPaths(treeBefore, treeAfterCli);
  const touchedUi = changed
    .filter((p) => p.startsWith(uiPrefix) && !p.slice(uiPrefix.length).includes("/"))
    .map((p) => p.slice(uiPrefix.length))
    .filter((f) => listComponentFiles(UI_DIR).includes(f));
  const touchedHooks = changed
    .filter((p) => p.startsWith(hookPrefix) && !p.slice(hookPrefix.length).includes("/"))
    .map((p) => p.slice(hookPrefix.length))
    .filter((f) => listComponentFiles(HOOKS_DIR).includes(f));
  const patches = loadPatches(PATCHES_PATH);
  const touchedItems: [string, string][] = [
    ...touchedUi.map((f): [string, string] => [componentName(f), join(UI_DIR, f)]),
    ...touchedHooks.map((f): [string, string] => [
      HOOK_PREFIX + componentName(f),
      join(HOOKS_DIR, f),
    ]),
  ];
  for (const [key, path] of touchedItems) {
    const src = readFileSync(path, "utf8");
    const out = applyPatches(key, normalizeCn(src), patches);
    if (out !== src) writeFileSync(path, out);
  }

  // 2) hapus `cn`, pin dependensi baru, rekonsiliasi lockfile
  const pkgAfter = readPackageJson();
  const stripped = stripCnDependency(pkgAfter);
  const next: Pkg = { ...stripped.pkg };
  const pinnedAll: string[] = [];
  const unpinnedAll: string[] = [];
  for (const field of ["dependencies", "devDependencies"] as const) {
    const after = stripped.pkg[field];
    if (!after) continue;
    const res = pinDependencies(pkgBefore[field] ?? {}, after, installedVersion);
    next[field] = res.deps;
    pinnedAll.push(...res.pinned);
    unpinnedAll.push(...res.unpinned);
  }
  const same = (x: Pkg, y: Pkg): boolean => JSON.stringify(x) === JSON.stringify(y);
  // Bandingkan dengan keadaan SETELAH CLI (bukan sebelum): CLI menulis `cn` dan rentang ^, yang harus dibuang.
  if (!same(next, pkgAfter)) writeJson(join(PACKAGE_DIR, "package.json"), next);
  if (!same(next, pkgBefore) || !same(pkgAfter, pkgBefore)) {
    run([process.execPath, "install"], REPO_ROOT);
  }

  // 3) kunci snapshot: komponen diminta + dependensi registry yang ikut tertimpa
  const only = [...new Set([...names, ...touchedItems.map(([key]) => key)])];
  const previous = existsSync(LOCK_PATH) ? loadLock(LOCK_PATH) : undefined;
  const lock: ShadcnLock = buildLock(UI_DIR, cli, { only, previous, hooksDir: HOOKS_DIR });
  writeJson(LOCK_PATH, lock);

  // 4) laporan
  const treeAfter = snapshotTree(join(PACKAGE_DIR, "src"));
  const outside = changedPaths(treeBefore, treeAfter).filter(
    (p) => !p.startsWith(uiPrefix) && !p.startsWith(hookPrefix),
  );
  console.log(`\nshadcn-sync: CLI ${cli}; komponen terkunci: ${only.join(", ")}.`);
  if (stripped.removed) console.log("  - dependensi `cn` dihapus dari package.json.");
  if (pinnedAll.length > 0) console.log(`  - dipin ke versi pasti: ${pinnedAll.join(", ")}.`);
  if (unpinnedAll.length > 0) {
    console.warn(`  ! RENTANG TIDAK DAPAT DIPIN OTOMATIS, pin manual: ${unpinnedAll.join(", ")}.`);
  }
  if (outside.length > 0) {
    console.warn(
      `  ! berkas di luar components/ui ikut berubah (tinjau diff; token DS hanya di theme.css): ${outside.join(", ")}`,
    );
  }
  if (
    existsSync(themePath) &&
    treeBefore.get("styles/theme.css") !== treeAfter.get("styles/theme.css")
  ) {
    console.warn(
      "  ! styles/theme.css berubah oleh CLI: periksa bahwa nilai DS dan uji tema tetap benar.",
    );
  }
  console.log(
    lock.synced
      ? "  - kunci synced: `shadcn-check` kini diwajibkan di web:check."
      : "  - kunci BELUM synced (masih ada komponen menyimpang yang belum disinkronkan); shadcn-check dilewati.",
  );
}

function main(argv: string[]): number {
  try {
    const cli = cliVersion();
    if (argv.length === 1 && argv[0] === "--prune") prune(cli);
    else sync(parseComponentNames(argv), cli);
    return 0;
  } catch (err) {
    console.error(`shadcn-sync: ${err instanceof Error ? err.message : String(err)}`);
    return 1;
  }
}

if (import.meta.main) process.exit(main(process.argv.slice(2)));

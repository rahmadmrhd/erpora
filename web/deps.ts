// Menjalankan dependency-cruiser dari root repo pada folder frontend yang ada.
// web/src, packages, dan modules (modules/*/web) dipindai; folder yang belum ada dilewati.
// Hanya memakai dependency-cruiser yang terpasang lokal (terkunci di bun.lock); tidak pernah
// mengunduh paket tak terpin (NFR-02), jadi tidak memakai `bun x`.
import { existsSync, readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = resolve(here, "..");

let bin: string;
try {
  const pkgPath = createRequire(join(here, "package.json")).resolve(
    "dependency-cruiser/package.json",
  );
  const pkg = JSON.parse(readFileSync(pkgPath, "utf8")) as {
    bin?: Record<string, string> | string;
  };
  const rel = typeof pkg.bin === "string" ? pkg.bin : pkg.bin?.depcruise;
  if (!rel) throw new Error("bin depcruise tidak ada di package.json");
  bin = join(dirname(pkgPath), rel);
  if (!existsSync(bin)) throw new Error(`${bin} tidak ada`);
} catch (err) {
  console.error(
    `dependency-cruiser tidak terpasang lokal (${String(err)}). Jalankan \`task setup\` (atau \`bun install\`) terlebih dahulu.`,
  );
  process.exit(1);
}

const targets = ["web/src", "packages", "modules"].filter((p) => existsSync(join(root, p)));
const proc = Bun.spawnSync(
  [process.execPath, bin, ...targets, "--config", ".dependency-cruiser.cjs"],
  {
    cwd: root,
    stdout: "inherit",
    stderr: "inherit",
  },
);
process.exit(proc.exitCode ?? 1);

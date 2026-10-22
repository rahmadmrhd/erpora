// Menjalankan dependency-cruiser dari root repo pada folder frontend yang ada.
// web/src, packages, dan modules (modules/*/web) dipindai; folder yang belum ada dilewati.
import { existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const targets = ["web/src", "packages", "modules"].filter((p) => existsSync(join(root, p)));

const proc = Bun.spawnSync(
  [process.execPath, "x", "depcruise", ...targets, "--config", ".dependency-cruiser.cjs"],
  { cwd: root, stdout: "inherit", stderr: "inherit" },
);
process.exit(proc.exitCode ?? 1);

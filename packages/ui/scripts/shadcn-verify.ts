// Pelaporan drift snapshot terhadap registry shadcn hidup (M3a revisi, task 1.3; R1.3, D-1).
//
//   task shadcn:verify                 # semua komponen di shadcn.lock.json
//   task shadcn:verify -- badge tabs   # hanya komponen tertentu
//
// DARING dan TIDAK masuk `web:check` (registry shadcn tidak berversi). Untuk tiap komponen menjalankan
// `shadcn add <k> --dry-run --diff` (CLI terpin; tidak menulis berkas) dan melaporkan selisih sebagai
// kabar drift. Kode keluar selalu 0: drift bukan kegagalan build; pembaruan lewat `task shadcn:sync`.
import { join } from "node:path";
import {
  type LabelPatch,
  LOCK_PATH,
  loadLock,
  loadPatches,
  PACKAGE_DIR,
  PATCHES_PATH,
} from "./shadcn-check";
import { parseComponentNames } from "./shadcn-sync";

const BOX_LINE = /^│ │ ?(.*)$/;
const HEADER = /^(?:---|\+\+\+) |^@@ /;
/** Satu-satunya penyimpangan sah: jalur impor `cn` (registry `cn`, lokal `@erpora/ui/lib/utils`). */
const CN_IMPORT_LINE = /^[+-]import \{ cn \} from "(?:cn|@erpora\/ui\/lib\/utils)";?\s*$/;

/**
 * Mengambil baris selisih (+/-) dari keluaran `shadcn add --dry-run --diff`, tanpa header diff dan
 * tanpa pertukaran impor `cn`. Daftar kosong = identik dengan registry (di luar impor `cn`).
 */
export function driftLines(output: string, patches: LabelPatch[] = []): string[] {
  // Baris yang berasal dari patch terdaftar bukan drift: sisi registry ('+') = find, sisi lokal ('-') = replace.
  const patchLines = (text: string): string[] =>
    text
      .split("\n")
      .map((l) => l.trim())
      .filter((l) => l !== "");
  const registrySide = new Set(patches.flatMap((p) => patchLines(p.find)));
  const localSide = new Set(patches.flatMap((p) => patchLines(p.replace)));
  const out: string[] = [];
  for (const raw of output.split(/\r?\n/)) {
    const m = BOX_LINE.exec(raw);
    if (!m) continue;
    const line = m[1] ?? "";
    if (HEADER.test(line)) continue;
    if (!(line.startsWith("+") || line.startsWith("-"))) continue;
    if (CN_IMPORT_LINE.test(line)) continue;
    const body = line.slice(1).trim();
    if (line.startsWith("+") && registrySide.has(body)) continue;
    if (line.startsWith("-") && localSide.has(body)) continue;
    out.push(line);
  }
  return out;
}

const patches = loadPatches(PATCHES_PATH);

async function diffOne(name: string): Promise<{ name: string; drift: string[]; error?: string }> {
  const proc = Bun.spawn([process.execPath, "run", "shadcn", "add", name, "--dry-run", "--diff"], {
    cwd: PACKAGE_DIR,
    stdout: "pipe",
    stderr: "pipe",
  });
  const [stdout, stderr, code] = await Promise.all([
    new Response(proc.stdout).text(),
    new Response(proc.stderr).text(),
    proc.exited,
  ]);
  if (code !== 0) return { name, drift: [], error: (stderr || stdout).trim().split("\n").pop() };
  return { name, drift: driftLines(stdout, patches) };
}

async function main(argv: string[]): Promise<number> {
  const lock = loadLock(LOCK_PATH);
  const names = argv.length > 0 ? parseComponentNames(argv) : Object.keys(lock.components);
  if (names.length === 0) {
    console.log("shadcn-verify: tidak ada komponen terkunci; tidak ada yang diperiksa.");
    return 0;
  }
  const results: Awaited<ReturnType<typeof diffOne>>[] = [];
  const queue = [...names];
  const workers = Array.from({ length: Math.min(4, queue.length) }, async () => {
    for (let n = queue.shift(); n !== undefined; n = queue.shift()) results.push(await diffOne(n));
  });
  await Promise.all(workers);
  results.sort((a, b) => (a.name < b.name ? -1 : 1));

  const drifted = results.filter((r) => r.drift.length > 0);
  const failed = results.filter((r) => r.error);
  for (const r of results) {
    if (r.error) console.log(`  ? ${r.name}: tidak dapat diperiksa (${r.error})`);
    else if (r.drift.length > 0)
      console.log(`  ~ ${r.name}: ${r.drift.length} baris berbeda dari registry hidup`);
    else console.log(`  = ${r.name}`);
  }
  console.log(
    `\nshadcn-verify (CLI ${lock.cli}, kunci di ${join("packages", "ui", "shadcn.lock.json")}): ` +
      `${drifted.length} drift, ${failed.length} gagal diperiksa, dari ${results.length} komponen.`,
  );
  if (drifted.length > 0) {
    console.log(
      `Untuk menyerap perubahan registry: task shadcn:sync -- ${drifted.map((r) => r.name).join(" ")}`,
    );
  }
  return 0;
}

if (import.meta.main) process.exit(await main(process.argv.slice(2)));

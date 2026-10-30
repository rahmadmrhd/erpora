// Larangan dangerouslySetInnerHTML (R3.6; SafeHtml baru dibuat di M3g): diuji lewat Biome nyata dengan konfigurasi repo.
import { describe, expect, test } from "bun:test";
import { existsSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");

function biomeBin(): string {
  // Biome terpasang lokal (workspace web); jalur dicari dari root repo agar tidak bergantung pada hoisting.
  const candidates = [
    join(root, "web", "node_modules", "@biomejs", "biome", "bin", "biome"),
    join(root, "node_modules", "@biomejs", "biome", "bin", "biome"),
  ];
  const found = candidates.find((p) => existsSync(p));
  if (!found) throw new Error("@biomejs/biome tidak terpasang lokal; jalankan bun install");
  return found;
}

// Mode stdin Biome tidak melaporkan diagnostik lint yang tak bisa diperbaiki otomatis, jadi kode
// ditulis ke berkas sementara di dalam `web/` (tercakup `files.includes`) dan dihapus sesudahnya.
function lint(code: string): { exitCode: number; output: string } {
  const dir = mkdtempSync(join(root, "web", ".lint-tmp-"));
  try {
    writeFileSync(join(dir, "x.tsx"), code);
    const proc = Bun.spawnSync([process.execPath, biomeBin(), "lint", join(dir, "x.tsx")], {
      cwd: root,
      stdout: "pipe",
      stderr: "pipe",
    });
    return { exitCode: proc.exitCode ?? 1, output: `${proc.stdout}${proc.stderr}` };
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
}

describe("aturan lint dangerouslySetInnerHTML", () => {
  test("dangerouslySetInnerHTML ditolak", () => {
    const res = lint(`export const a = <div dangerouslySetInnerHTML={{ __html: "x" }} />;\n`);
    expect(res.exitCode).not.toBe(0);
    expect(res.output).toContain("noDangerouslySetInnerHtml");
  });

  test("dangerouslySetInnerHTML dengan children ditolak", () => {
    const res = lint(
      `export const a = <div dangerouslySetInnerHTML={{ __html: "x" }}>anak</div>;\n`,
    );
    expect(res.exitCode).not.toBe(0);
    expect(res.output).toContain("noDangerouslySetInnerHtml");
  });

  test("kode bersih lolos", () => {
    const res = lint(`export const a = <div>teks</div>;\n`);
    expect(res.output).not.toContain("noDangerouslySetInnerHtml");
    expect(res.exitCode).toBe(0);
  });

  test("konfigurasi repo menetapkan kedua aturan sebagai error", async () => {
    const cfg = await Bun.file(join(root, "biome.json")).text();
    expect(cfg).toMatch(/"noDangerouslySetInnerHtml":\s*"error"/);
    expect(cfg).toMatch(/"noDangerouslySetInnerHtmlWithChildren":\s*"error"/);
  });
});

// Aturan arsitektur F4 (M3a K-22): `packages/ui` adalah daun, tidak mengimpor paket repo lain.
// Diuji dengan pohon sementara di luar repo dan konfigurasi dependency-cruiser repo yang nyata.
import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { cruise } from "dependency-cruiser";
import extractDepcruiseOptions from "dependency-cruiser/config-utl/extract-depcruise-options";

const repoRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..", "..");
const configPath = join(repoRoot, ".dependency-cruiser.cjs");

let tree: string;

function write(rel: string, content: string): void {
  const file = join(tree, rel);
  mkdirSync(dirname(file), { recursive: true });
  writeFileSync(file, content);
}

/**
 * Paket workspace tiruan: direktori nyata di `dir` dan tautan (junction) `node_modules/<name>` ke sana,
 * sehingga terselesaikan ke jalur repo (`packages/...`) seperti pada monorepo asli. Dependensi yang
 * berada di `node_modules` sendiri dikecualikan dari graf oleh konfigurasi repo (`exclude`).
 */
function workspacePackage(name: string, dir: string, exportsMap?: Record<string, string>): void {
  write(
    `${dir}/package.json`,
    JSON.stringify({ name, main: "src/index.ts", ...(exportsMap ? { exports: exportsMap } : {}) }),
  );
  write(`${dir}/src/index.ts`, "export const marker = 1;\n");
  const link = join(tree, "node_modules", name);
  mkdirSync(dirname(link), { recursive: true });
  symlinkSync(join(tree, dir), link, "junction");
}

/** Berkas sumber di `packages/ui/src` yang mengimpor `spec` dan memakai nilainya. */
function uiFileImporting(file: string, spec: string): void {
  write(`packages/ui/src/${file}`, `import { marker } from "${spec}";\nexport const m = marker;\n`);
}

beforeAll(() => {
  tree = mkdtempSync(join(tmpdir(), "erpora-depcruise-"));
  workspacePackage("@erpora/ui", "packages/ui", {
    ".": "./src/index.ts",
    "./lib/*": "./src/lib/*.ts",
  });
  workspacePackage("@erpora/platform-web", "packages/platform-web");
  workspacePackage("@erpora/module-sdk", "packages/module-sdk");
  workspacePackage("@erpora/platform-contracts", "packages/platform-contracts");
  workspacePackage("@erpora/module-example", "modules/example/web");
  write("packages/ui/src/lib/utils.ts", "export const marker = 1;\n");
  uiFileImporting("self.ts", "@erpora/ui/lib/utils");
  uiFileImporting("ok.ts", "./lib/utils");
  write("packages/ui/src/ok-external.ts", 'import "react";\nimport "radix-ui";\n');
  uiFileImporting("bad-platform-web.ts", "@erpora/platform-web");
  uiFileImporting("bad-module-sdk.ts", "@erpora/module-sdk");
  uiFileImporting("bad-contracts.ts", "@erpora/platform-contracts");
  uiFileImporting("bad-module.ts", "@erpora/module-example");
});

afterAll(() => {
  rmSync(tree, { recursive: true, force: true });
});

async function violationsFor(file: string): Promise<string[]> {
  const options = await extractDepcruiseOptions(configPath);
  const result = await cruise([file], { ...options, baseDir: tree });
  const output = result.output;
  if (typeof output === "string") throw new Error("keluaran cruise bukan objek");
  return output.summary.violations.map((v) => `${v.rule.name}: ${v.from} -> ${v.to}`);
}

describe("F4-ui-is-a-leaf", () => {
  test.each([
    ["bad-platform-web.ts", "packages/platform-web"],
    ["bad-module-sdk.ts", "packages/module-sdk"],
    ["bad-contracts.ts", "packages/platform-contracts"],
    ["bad-module.ts", "modules/example"],
  ])("%s mengimpor paket repo (%s) dilarang", async (file, target) => {
    const found = await violationsFor(`packages/ui/src/${file}`);
    expect(found.some((v) => v.startsWith("F4-ui-is-a-leaf") && v.includes(target))).toBe(true);
  });

  test("impor berkas sendiri lolos", async () => {
    expect(await violationsFor("packages/ui/src/ok.ts")).toEqual([]);
  });

  test("impor react dan radix-ui lolos", async () => {
    expect(await violationsFor("packages/ui/src/ok-external.ts")).toEqual([]);
  });

  test("self-reference lewat nama paket @erpora/ui lolos", async () => {
    expect(await violationsFor("packages/ui/src/self.ts")).toEqual([]);
  });
});

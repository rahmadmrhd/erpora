// Bukti lazy untuk modul referensi (M3a R5.4, R7.4, K-21): dari build nyata (Bun.build, splitting) yang memuat daftar modul
// frontend dan memanggil registerRoutes, chunk halaman /example dan TanStack DB terpisah dan TIDAK ada di JS awal.
import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { initialJsGzip, type Metafile } from "./budget";

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");

let meta: Metafile;
let probeDir: string;
let outDir: string;

beforeAll(async () => {
  // Titik masuk uji di dalam web/ agar impor @erpora/* terselesaikan lewat node_modules workspace.
  probeDir = mkdtempSync(join(webRoot, "src", ".lazy-probe-"));
  outDir = mkdtempSync(join(tmpdir(), "erpora-lazy-"));
  mkdirSync(probeDir, { recursive: true });
  writeFileSync(
    join(probeDir, "entry.ts"),
    [
      'import { createAppRootRoute, createModuleBaseRoute, attachModuleRoutes } from "@erpora/module-sdk";',
      'import { frontendModules } from "../app/modules";',
      "const root = createAppRootRoute();",
      "const children = frontendModules.map((m) => attachModuleRoutes(createModuleBaseRoute(root, m.slug), m.registerRoutes(createModuleBaseRoute(root, m.slug))));",
      "console.log(children.length);",
      "",
    ].join("\n"),
  );
  const result = await Bun.build({
    entrypoints: [join(probeDir, "entry.ts")],
    outdir: outDir,
    splitting: true,
    minify: true,
    metafile: true,
    naming: { entry: "[name].[ext]", chunk: "assets/[name]-[hash].[ext]" },
    define: { "process.env.NODE_ENV": JSON.stringify("production") },
  });
  if (!result.success) throw new Error(result.logs.map(String).join("\n"));
  const raw = result.metafile as unknown as Metafile;
  // initialJsGzip mengenali entry lewat entryPoint berakhiran .html; titik masuk uji ini berupa .ts, jadi disamakan.
  meta = {
    ...raw,
    outputs: Object.fromEntries(
      Object.entries(raw.outputs).map(([path, out]) => [
        path,
        out.entryPoint?.endsWith("entry.ts") ? { ...out, entryPoint: "src/index.html" } : out,
      ]),
    ),
  };
});

afterAll(() => {
  rmSync(probeDir, { recursive: true, force: true });
  rmSync(outDir, { recursive: true, force: true });
});

const outputsWith = (needle: RegExp): string[] =>
  Object.entries(meta.outputs)
    .filter(
      ([path, out]) => path.endsWith(".js") && Object.keys(out.inputs).some((i) => needle.test(i)),
    )
    .map(([path]) => path);

describe("chunk lazy modul referensi", () => {
  test("halaman /example dan /example-addon berada di chunk terpisah, bukan di JS awal", () => {
    const initial = new Set(initialJsGzip(meta, () => 1).files.map((f) => f.path));
    const example = outputsWith(/modules\/example\/web\/src\/pages\/notes-page\.tsx$/);
    const addon = outputsWith(/modules\/exampleaddon\/web\/src\/pages\/summary-page\.tsx$/);
    expect(example.length).toBeGreaterThan(0);
    expect(addon.length).toBeGreaterThan(0);
    for (const path of [...example, ...addon]) expect(initial.has(path)).toBe(false);
    expect(example[0]).not.toBe(addon[0]);
  });

  test("TanStack DB hanya ada di chunk lazy; tidak ada di JS awal", () => {
    const initial = new Set(initialJsGzip(meta, () => 1).files.map((f) => f.path));
    const dbChunks = outputsWith(
      /@tanstack\+(react-db|db)@|node_modules\/@tanstack\/(react-db|db)\//,
    );
    expect(dbChunks.length).toBeGreaterThan(0);
    for (const path of dbChunks) expect(initial.has(path)).toBe(false);
  });

  test("modul example tidak menarik kode halaman ke entry: registerRoutes saja yang awal", () => {
    const initialInputs = [
      ...new Set(initialJsGzip(meta, () => 1).files.map((f) => f.path)),
    ].flatMap((p) => Object.keys(meta.outputs[p]?.inputs ?? {}));
    expect(initialInputs.some((i) => /modules\/example\/web\/src\/routes\.tsx$/.test(i))).toBe(
      true,
    );
    expect(initialInputs.some((i) => /pages\/(notes|summary)-page\.tsx$/.test(i))).toBe(false);
  });
});

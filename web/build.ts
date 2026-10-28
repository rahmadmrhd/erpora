import { readFileSync } from "node:fs";
import { mkdir, readdir, readFile, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { gzipSync } from "node:zlib";
import tailwind from "bun-plugin-tailwind";
import { cspViolations, cssViolations } from "./csp-check";
import { budgetViolation, formatBudgetReport, initialJsGzip, type Metafile } from "./tools/budget";
import { extractInlinedFonts } from "./tools/fonts";
import { injectHead } from "./tools/head";

const outdir = "dist";
await mkdir(outdir, { recursive: true });
// Bersihkan build lama, pertahankan .gitkeep (dibutuhkan go:embed).
for (const name of await readdir(outdir)) {
  if (name !== ".gitkeep") await rm(join(outdir, name), { recursive: true, force: true });
}

function fail(message: string): never {
  console.error(message);
  process.exit(1);
}

const result = await Bun.build({
  entrypoints: ["src/index.html"],
  outdir,
  minify: true,
  splitting: true,
  publicPath: "/",
  plugins: [tailwind],
  metafile: true,
  define: { "process.env.NODE_ENV": JSON.stringify("production") },
  naming: {
    entry: "[name].[ext]",
    chunk: "assets/[name]-[hash].[ext]",
    asset: "assets/[name]-[hash].[ext]",
  },
});
if (!result.success) {
  for (const log of result.logs) console.error(log);
  process.exit(1);
}

// Skrip tema dibangun terpisah: Bun menggabungkan semua <script> HTML menjadi satu modul yang
// ditunda, sedangkan tema harus terpasang sebelum paint pertama (K-18).
const themeInit = await Bun.build({
  entrypoints: ["src/theme-init.ts"],
  outdir,
  minify: true,
  naming: "assets/theme-init-[hash].js",
});
if (!themeInit.success) {
  for (const log of themeInit.logs) console.error(log);
  process.exit(1);
}
const themeOut = themeInit.outputs.find((o) => o.path.endsWith(".js"));
if (!themeOut) fail("theme-init: keluaran JS tidak ditemukan");
const themeSrc = `/assets/${themeOut.path.split(/[\\/]/).pop()}`;

// Font yang disisipkan Bun sebagai data: URI dipindahkan ke berkas ber-hash (CSP memblokir font-src data:).
const meta = result.metafile as unknown as Metafile;
const cssFiles = Object.keys(meta.outputs).filter((p) => p.endsWith(".css"));
const fontFiles: Array<{ path: string; bytes: number }> = [];
const cssProblems: string[] = [];
for (const rel of cssFiles) {
  const file = join(outdir, rel);
  const extracted = extractInlinedFonts(await readFile(file, "utf8"));
  for (const font of extracted.fonts) {
    const target = join(outdir, font.fileName);
    await mkdir(dirname(target), { recursive: true });
    await writeFile(target, font.data);
    fontFiles.push({ path: font.fileName, bytes: font.data.byteLength });
  }
  if (extracted.fonts.length > 0) await writeFile(file, extracted.css);
  for (const p of cssViolations(extracted.css)) cssProblems.push(`${rel}: ${p}`);
}

const htmlPath = join(outdir, "index.html");
const html = injectHead(await readFile(htmlPath, "utf8"), {
  themeInitSrc: themeSrc,
  fontPreloads: fontFiles.map((f) => `/${f.path}`),
});
await writeFile(htmlPath, html);

// Aman terhadap CSP produksi (script-src 'self'; font-src jatuh ke 'self')?
const problems = [
  ...cspViolations(html).map((p) => `dist/index.html: ${p}`),
  ...cssProblems.map((p) => `dist/${p}`),
];
if (problems.length > 0) fail(`build tidak kompatibel dengan CSP: ${problems.join("; ")}`);

// Anggaran bundle (NFR-04): JS awal <= 200 KB gzip.
const gzipSize = (rel: string): number =>
  gzipSync(readFileSync(join(outdir, rel)), { level: 9 }).byteLength;
console.log(formatBudgetReport(meta, gzipSize, { fonts: fontFiles }));
const violation = budgetViolation(initialJsGzip(meta, gzipSize).total);
if (violation) fail(violation);

import { mkdir, readdir, rm } from "node:fs/promises";
import { join } from "node:path";
import { cspViolations } from "./csp-check";

const outdir = "dist";
await mkdir(outdir, { recursive: true });
// Bersihkan build lama, pertahankan .gitkeep (dibutuhkan go:embed).
for (const name of await readdir(outdir)) {
  if (name !== ".gitkeep") await rm(join(outdir, name), { recursive: true, force: true });
}

const result = await Bun.build({
  entrypoints: ["src/index.html"],
  outdir,
  minify: true,
  publicPath: "/",
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
for (const out of result.outputs) console.log(out.path);

// Aman terhadap CSP produksi (script-src 'self')?
const html = await Bun.file(join(outdir, "index.html")).text();
const violations = cspViolations(html);
if (violations.length > 0) {
  console.error(`dist/index.html tidak kompatibel dengan CSP: ${violations.join(", ")}`);
  process.exit(1);
}

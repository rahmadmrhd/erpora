import { mkdir, readdir, rm } from "node:fs/promises";
import { join } from "node:path";

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

// Anggaran bundle (NFR-04, M3a K-12): JS awal app shell <= 200 KB gzip.
// Fungsi murni: masukan metafile `Bun.build` dan fungsi ukuran gzip; I/O dilakukan build.ts.

export const BUDGET = 200 * 1024;
export const TOP_CONTRIBUTORS = 15;

export interface MetaImport {
  path: string;
  kind: string;
  external?: boolean;
}

export interface MetaOutput {
  bytes: number;
  inputs: Record<string, { bytesInOutput: number }>;
  imports: MetaImport[];
  exports: string[];
  entryPoint?: string;
  cssBundle?: string;
}

/** Bagian metafile `Bun.build` yang dipakai (hanya `outputs`). */
export interface Metafile {
  inputs: Record<string, unknown>;
  outputs: Record<string, MetaOutput>;
}

export interface InitialJs {
  total: number;
  files: Array<{ path: string; gzip: number }>;
}

const isJs = (path: string): boolean => path.endsWith(".js");

/**
 * Menjumlah gzip JS awal: entry JS milik entry HTML ditambah seluruh `import-statement` transitif.
 * `dynamic-import` diabaikan; tiap berkas dihitung (dan diukur) sekali walau diimpor beberapa kali.
 */
export function initialJsGzip(meta: Metafile, gzipSize: (path: string) => number): InitialJs {
  const seen = new Set<string>();
  const files: InitialJs["files"] = [];
  const visit = (path: string): void => {
    if (seen.has(path)) return;
    seen.add(path);
    const out = meta.outputs[path];
    if (!out || !isJs(path)) return;
    files.push({ path, gzip: gzipSize(path) });
    for (const imp of out.imports) {
      if (imp.kind === "import-statement" && !imp.external) visit(imp.path);
    }
  };
  for (const [path, out] of Object.entries(meta.outputs)) {
    if (isJs(path) && out.entryPoint?.endsWith(".html")) visit(path);
  }
  return { total: files.reduce((sum, f) => sum + f.gzip, 0), files };
}

/** Pesan pelanggaran bila `total` melebihi `budget`; `null` bila lolos. */
export function budgetViolation(total: number, budget: number = BUDGET): string | null {
  if (total <= budget) return null;
  return `JS awal ${total} byte gzip melebihi anggaran ${budget} byte (${kb(total)} > ${kb(budget)})`;
}

const kb = (n: number): string => `${(n / 1024).toFixed(1)} KB`;

export interface ReportOptions {
  fonts?: Array<{ path: string; bytes: number }>;
  budget?: number;
  topN?: number;
}

/** Laporan anggaran: JS awal, chunk lazy, CSS, font, dan kontributor `inputs` terbesar pada chunk awal. */
export function formatBudgetReport(
  meta: Metafile,
  gzipSize: (path: string) => number,
  opts: ReportOptions = {},
): string {
  const budget = opts.budget ?? BUDGET;
  const topN = opts.topN ?? TOP_CONTRIBUTORS;
  const initial = initialJsGzip(meta, gzipSize);
  const initialPaths = new Set(initial.files.map((f) => f.path));
  const lines: string[] = [];

  lines.push("Anggaran bundle (gzip level 9)");
  lines.push(
    `JS awal: ${kb(initial.total)} dari ${kb(budget)} (${((initial.total / budget) * 100).toFixed(1)}%), ${initial.files.length} berkas`,
  );
  for (const f of initial.files) lines.push(`  ${f.path}  ${kb(f.gzip)}`);

  const lazy = Object.keys(meta.outputs).filter((p) => isJs(p) && !initialPaths.has(p));
  lines.push(`Chunk lazy: ${lazy.length}`);
  for (const p of lazy) lines.push(`  ${p}  ${kb(gzipSize(p))}`);

  const css = Object.keys(meta.outputs).filter((p) => p.endsWith(".css"));
  lines.push(`CSS: ${css.length}`);
  for (const p of css) lines.push(`  ${p}  ${kb(gzipSize(p))}`);

  const fonts = opts.fonts ?? [];
  lines.push(`Font: ${fonts.length}`);
  for (const f of fonts) lines.push(`  ${f.path}  ${kb(f.bytes)}`);

  const contributions = new Map<string, number>();
  for (const f of initial.files) {
    for (const [input, info] of Object.entries(meta.outputs[f.path]?.inputs ?? {})) {
      contributions.set(input, (contributions.get(input) ?? 0) + info.bytesInOutput);
    }
  }
  const top = [...contributions.entries()].sort((a, b) => b[1] - a[1]).slice(0, topN);
  lines.push(`Kontributor terbesar pada chunk awal (${top.length} teratas, byte dalam keluaran):`);
  for (const [input, bytes] of top) lines.push(`  ${input}  ${bytes}`);

  return lines.join("\n");
}

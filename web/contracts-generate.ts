// Runner pembangkit tipe TS dari kontrak (PRD-000 FR-23.4, FR-06), dipanggil `erpora contracts:generate`:
//   bun web/contracts-generate.ts --job <job.json>
// Job dibuat Go: {root, jobs:[{id, kind, spec|schemaFile, header, footer, virtualPath, name}]}.
//   kind "openapi"    : spec (OpenAPI 3.1 terbundel) -> openapi-typescript
//   kind "jsonschema" : schemaFile (path relatif root) -> json-schema-to-typescript
// Keluaran stdout: JSON {id: isiTS}; setiap isi sudah diberi header dan diformat Biome lokal (tanpa bun x/npx).
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { isAbsolute, join } from "node:path";
import { compile } from "json-schema-to-typescript";
import openapiTS, { astToString, type OpenAPI3 } from "openapi-typescript";

interface Job {
  id: string;
  kind: "openapi" | "jsonschema";
  spec?: unknown;
  schemaFile?: string; // path absolut, atau relatif terhadap root
  name?: string; // nama tipe akar untuk jsonschema
  header: string;
  footer?: string;
  virtualPath: string; // path file tujuan (relatif root); dipakai Biome untuk memilih konfigurasi
}
interface JobFile {
  root: string;
  jobs: Job[];
}

function arg(name: string): string {
  const i = process.argv.indexOf(name);
  const v = i >= 0 ? process.argv[i + 1] : undefined;
  if (!v) throw new Error(`argumen ${name} wajib`);
  return v;
}

const job = JSON.parse(readFileSync(arg("--job"), "utf8")) as JobFile;
const require = createRequire(import.meta.url);
const biomeCli = require.resolve("@biomejs/biome/bin/biome");

/** Memformat dengan Biome lokal lewat stdin; gagal -> error (bukan diam-diam tanpa format). */
function format(content: string, virtualPath: string): string {
  const p = Bun.spawnSync(
    [process.execPath, biomeCli, "format", `--stdin-file-path=${virtualPath}`],
    {
      cwd: job.root,
      stdin: Buffer.from(content),
      stdout: "pipe",
      stderr: "pipe",
    },
  );
  if (p.exitCode !== 0) {
    throw new Error(`biome format ${virtualPath} gagal: ${p.stderr.toString()}`);
  }
  return p.stdout.toString();
}

async function generate(j: Job): Promise<string> {
  let body: string;
  if (j.kind === "openapi") {
    const ast = await openapiTS(j.spec as OpenAPI3, { alphabetize: true, exportType: true });
    body = astToString(ast);
  } else {
    const file = j.schemaFile ?? "";
    const schema = JSON.parse(
      readFileSync(isAbsolute(file) ? file : join(job.root, file), "utf8"),
    ) as Record<string, unknown>;
    // json-schema-to-typescript menamai tipe akar dari `title`; paksa memakai nama job agar stabil.
    if (j.name) schema.title = j.name;
    body = await compile(schema as never, j.name ?? "Root", {
      bannerComment: "",
      additionalProperties: false,
      format: false,
      cwd: job.root,
    });
  }
  return format(`${j.header}\n\n${body}\n${j.footer ?? ""}`, j.virtualPath);
}

const out: Record<string, string> = {};
for (const j of job.jobs) out[j.id] = await generate(j);
process.stdout.write(`${JSON.stringify(out)}\n`);

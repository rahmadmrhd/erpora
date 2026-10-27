// Runner linter kontrak OpenAPI (PRD-000 FR-23.3), dipanggil `erpora contracts:lint`:
//   bun web/contracts-lint.ts --catalog <katalog.json>
// Katalog dibuat Go: {root, ruleset, targets:[{name,file,slug,dependencies}], docs:{nama: file}}.
// Keluaran stdout: JSON []{target,file,rule,path,message,line}. Exit 0 walau ada temuan (Go yang menilai);
// exit 2 bila runner gagal. Spectral dijalankan secara programatik (tanpa bundler ruleset).
import { readFileSync } from "node:fs";
import { isAbsolute, join } from "node:path";
import { Document, Spectral } from "@stoplight/spectral-core";
import * as spectralFunctions from "@stoplight/spectral-functions";
import * as Parsers from "@stoplight/spectral-parsers";
import { Resolver } from "@stoplight/spectral-ref-resolver";
import { oas } from "@stoplight/spectral-rulesets";
import { parse as parseYaml } from "yaml";
import {
  functions as customFunctions,
  type LintContext,
  setLintContext,
} from "../contracts/spectral-functions/index";

interface Target {
  name: string;
  file: string;
  slug: string;
  dependencies: string[] | null;
}
interface Catalog {
  root: string;
  ruleset: string;
  targets: Target[];
  docs: Record<string, string>;
}
interface Finding {
  target: string;
  file: string;
  rule: string;
  path: string;
  message: string;
  line: number;
}
type Json = { [k: string]: unknown };

function arg(name: string): string {
  const i = process.argv.indexOf(name);
  const v = i >= 0 ? process.argv[i + 1] : undefined;
  if (!v) throw new Error(`argumen ${name} wajib`);
  return v;
}

const catalog = JSON.parse(readFileSync(arg("--catalog"), "utf8")) as Catalog;
const abs = (f: string): string => (isAbsolute(f) ? f : join(catalog.root, f));

/** Memetakan nama function pada ruleset YAML ke fungsi bawaan Spectral atau fungsi kustom contracts/. */
function bindFunctions(node: unknown): void {
  if (Array.isArray(node)) {
    for (const n of node) bindFunctions(n);
    return;
  }
  if (node === null || typeof node !== "object") return;
  const o = node as Json;
  if (typeof o.function === "string") {
    const name = o.function;
    const builtin = (spectralFunctions as unknown as Record<string, unknown>)[name];
    const fn = customFunctions[name] ?? builtin;
    if (typeof fn !== "function") throw new Error(`fungsi ruleset "${name}" tidak dikenal`);
    o.function = fn;
  }
  for (const v of Object.values(o)) bindFunctions(v);
}

function loadRuleset(): Json {
  const def = parseYaml(readFileSync(catalog.ruleset, "utf8")) as Json;
  const ext = (def.extends as unknown[]).map((e) => {
    const [name, severity] = e as [string, string];
    if (name !== "spectral:oas") throw new Error(`extends "${name}" tidak didukung`);
    return [oas, severity];
  });
  def.extends = ext;
  bindFunctions(def.rules);
  return def;
}

// Resolver skema "erp": erp:<dokumen>#/pointer -> isi file dokumen pada katalog.
const resolver = new Resolver({
  resolvers: {
    erp: {
      async resolve(ref: { path(): string }) {
        const file = catalog.docs[ref.path()];
        if (!file) throw new Error(`dokumen erp:${ref.path()} tidak ada di katalog`);
        return readFileSync(abs(file), "utf8");
      },
    },
  },
});

const docs: Record<string, Json> = {};
for (const [name, file] of Object.entries(catalog.docs)) {
  docs[name] = parseYaml(readFileSync(abs(file), "utf8")) as Json;
}

const spectral = new Spectral({ resolver });
spectral.setRuleset(loadRuleset() as never);

const findings: Finding[] = [];
for (const t of catalog.targets) {
  const ctxValue: LintContext = {
    target: t.name,
    slug: t.slug,
    dependencies: t.dependencies ?? [],
    docs,
  };
  setLintContext(ctxValue);
  const text = readFileSync(abs(t.file), "utf8");
  const results = await spectral.run(new Document(text, Parsers.Yaml, t.file));
  for (const r of results) {
    // 0 error, 1 warn; info dan hint tidak dilaporkan.
    if (r.severity > 1) continue;
    findings.push({
      target: t.name,
      file: t.file,
      rule: String(r.code),
      path: r.path.join("."),
      message: r.message,
      line: r.range.start.line + 1,
    });
  }
}
process.stdout.write(`${JSON.stringify(findings)}\n`);

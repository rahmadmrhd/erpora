// Membuat SBOM CycloneDX 1.5 frontend langsung dari bun.lock (terkunci, tanpa install/unduh apa pun).
// Dipakai CI sesudah `bun install --frozen-lockfile`. cdxgen tidak membaca bun.lock dan menjalankan
// `npm install` sendiri, sehingga tidak dipakai. Pemakaian: bun sbom.ts <berkas-keluaran>
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

type LockEntry = [ident: string, registry: string, meta?: LockMeta, integrity?: string];
interface LockMeta {
  dependencies?: Record<string, string>;
  optionalDependencies?: Record<string, string>;
  peerDependencies?: Record<string, string>;
}
interface Lock {
  workspaces: Record<
    string,
    {
      name?: string;
      dependencies?: Record<string, string>;
      devDependencies?: Record<string, string>;
    }
  >;
  packages: Record<string, LockEntry>;
}

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const out = process.argv[2];
if (!out) {
  console.error("pemakaian: bun sbom.ts <berkas-keluaran>");
  process.exit(2);
}

const lock = Bun.JSONC.parse(readFileSync(join(root, "bun.lock"), "utf8")) as Lock;

function split(ident: string): { name: string; version: string } {
  const at = ident.lastIndexOf("@");
  return { name: ident.slice(0, at), version: ident.slice(at + 1) };
}

function purl(name: string, version: string): string {
  const enc = name.startsWith("@") ? `%40${name.slice(1)}` : name;
  return `pkg:npm/${enc}@${version}`;
}

function hashes(integrity?: string): { alg: string; content: string }[] | undefined {
  const m = integrity?.match(/^sha(256|384|512)-(.+)$/);
  if (!m) return undefined;
  return [{ alg: `SHA-${m[1]}`, content: Buffer.from(m[2] ?? "", "base64").toString("hex") }];
}

// Paket workspace (ident "...@workspace:...") bukan dependensi pihak ketiga.
const keys = Object.keys(lock.packages).filter(
  (k) => !(lock.packages[k]?.[0] ?? "").includes("@workspace:"),
);

function resolveDep(parentKey: string, dep: string): string | undefined {
  for (const k of [`${parentKey}/${dep}`, dep]) if (keys.includes(k)) return k;
  return undefined;
}

function childrenOf(key: string): string[] {
  const meta = lock.packages[key]?.[2] ?? {};
  const names = [
    ...Object.keys(meta.dependencies ?? {}),
    ...Object.keys(meta.optionalDependencies ?? {}),
  ];
  return names.flatMap((d) => {
    const k = resolveDep(key, d);
    return k ? [k] : [];
  });
}

// Scope: "required" bila terjangkau dari dependencies produksi workspace, selain itu "optional" (dev).
const prodRoots = Object.values(lock.workspaces).flatMap((w) => Object.keys(w.dependencies ?? {}));
const required = new Set<string>();
const stack = prodRoots.flatMap((d) => (keys.includes(d) ? [d] : []));
while (stack.length > 0) {
  const k = stack.pop() as string;
  if (required.has(k)) continue;
  required.add(k);
  stack.push(...childrenOf(k));
}

const ref = (key: string): string => {
  const { name, version } = split(lock.packages[key]?.[0] ?? key);
  return purl(name, version);
};

const components = keys.map((key) => {
  const entry = lock.packages[key] as LockEntry;
  const { name, version } = split(entry[0]);
  const scoped = name.startsWith("@") ? name.split("/")[0]?.slice(1) : undefined;
  const isDev = !required.has(key);
  return {
    type: "library",
    "bom-ref": `${purl(name, version)}#${key}`,
    ...(scoped ? { group: `@${scoped}` } : {}),
    name: scoped ? name.split("/").slice(1).join("/") : name,
    version,
    scope: isDev ? "optional" : "required",
    purl: purl(name, version),
    hashes: hashes(entry[3]),
    properties: [{ name: "cdx:npm:package:development", value: String(isDev) }],
  };
});

const bomRef = (key: string): string => `${ref(key)}#${key}`;
const rootName = lock.workspaces[""]?.name ?? "erpora";
const bom = {
  bomFormat: "CycloneDX",
  specVersion: "1.5",
  serialNumber: `urn:uuid:${crypto.randomUUID()}`,
  version: 1,
  metadata: {
    timestamp: new Date().toISOString(),
    tools: { components: [{ type: "application", name: "web/sbom.ts (dari bun.lock)" }] },
    component: { type: "application", name: rootName, "bom-ref": `root:${rootName}` },
  },
  components,
  dependencies: [
    {
      ref: `root:${rootName}`,
      dependsOn: Object.values(lock.workspaces)
        .flatMap((w) => [
          ...Object.keys(w.dependencies ?? {}),
          ...Object.keys(w.devDependencies ?? {}),
        ])
        .flatMap((d) => (keys.includes(d) ? [bomRef(d)] : [])),
    },
    ...keys.map((k) => ({ ref: bomRef(k), dependsOn: childrenOf(k).map(bomRef) })),
  ],
};

mkdirSync(dirname(resolve(out)), { recursive: true });
writeFileSync(out, `${JSON.stringify(bom, null, 2)}\n`);
console.log(`SBOM frontend: ${components.length} komponen -> ${out}`);

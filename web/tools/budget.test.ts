import { describe, expect, test } from "bun:test";
import {
  BUDGET,
  budgetViolation,
  formatBudgetReport,
  initialJsGzip,
  type Metafile,
} from "./budget";

/** Metafile minimal berbentuk keluaran Bun.build (HTML entry, splitting, metafile: true). */
function meta(outputs: Metafile["outputs"]): Metafile {
  return { inputs: {}, outputs };
}

const html = { bytes: 1, inputs: {}, imports: [], exports: [], entryPoint: "src/index.html" };

function sizes(map: Record<string, number>): (path: string) => number {
  return (path) => {
    const n = map[path];
    if (n === undefined) throw new Error(`ukuran tak dikenal: ${path}`);
    return n;
  };
}

describe("initialJsGzip", () => {
  test("menjumlah entry JS dan seluruh import-statement transitif; dynamic-import diabaikan", () => {
    const m = meta({
      "./index.html": html,
      "./assets/index-a.js": {
        bytes: 100,
        inputs: {},
        imports: [
          { path: "./assets/chunk-b.js", kind: "import-statement" },
          { path: "./assets/lazy-c.js", kind: "dynamic-import" },
        ],
        exports: [],
        entryPoint: "src/index.html",
        cssBundle: "./assets/index-a.css",
      },
      "./assets/chunk-b.js": {
        bytes: 50,
        inputs: {},
        imports: [{ path: "./assets/chunk-d.js", kind: "import-statement" }],
        exports: [],
      },
      "./assets/chunk-d.js": { bytes: 10, inputs: {}, imports: [], exports: [] },
      "./assets/lazy-c.js": {
        bytes: 999,
        inputs: {},
        imports: [{ path: "./assets/index-a.js", kind: "import-statement" }],
        exports: [],
      },
      "./assets/index-a.css": { bytes: 7, inputs: {}, imports: [], exports: [] },
    });
    const res = initialJsGzip(
      m,
      sizes({ "./assets/index-a.js": 30, "./assets/chunk-b.js": 20, "./assets/chunk-d.js": 5 }),
    );
    expect(res.total).toBe(55);
    expect(res.files.map((f) => f.path).sort()).toEqual([
      "./assets/chunk-b.js",
      "./assets/chunk-d.js",
      "./assets/index-a.js",
    ]);
  });

  test("chunk bersama dihitung sekali dan gzip dihitung sekali per berkas", () => {
    const leaf = { bytes: 1, inputs: {}, imports: [], exports: [] };
    const m = meta({
      "./index.html": html,
      "./assets/index-a.js": {
        bytes: 1,
        inputs: {},
        imports: [
          { path: "./assets/x.js", kind: "import-statement" },
          { path: "./assets/y.js", kind: "import-statement" },
        ],
        exports: [],
        entryPoint: "src/index.html",
      },
      "./assets/x.js": {
        ...leaf,
        imports: [{ path: "./assets/shared.js", kind: "import-statement" }],
      },
      "./assets/y.js": {
        ...leaf,
        imports: [{ path: "./assets/shared.js", kind: "import-statement" }],
      },
      "./assets/shared.js": leaf,
    });
    const calls: string[] = [];
    const res = initialJsGzip(m, (p) => {
      calls.push(p);
      return 10;
    });
    expect(res.total).toBe(40);
    expect(calls.filter((p) => p === "./assets/shared.js")).toHaveLength(1);
    expect(new Set(calls).size).toBe(calls.length);
  });

  test("siklus import tidak membuat perulangan tak berhingga", () => {
    const m = meta({
      "./assets/index-a.js": {
        bytes: 1,
        inputs: {},
        imports: [{ path: "./assets/b.js", kind: "import-statement" }],
        exports: [],
        entryPoint: "src/index.html",
      },
      "./assets/b.js": {
        bytes: 1,
        inputs: {},
        imports: [{ path: "./assets/index-a.js", kind: "import-statement" }],
        exports: [],
      },
    });
    expect(initialJsGzip(m, () => 5).total).toBe(10);
  });

  test("CSS, HTML, dan berkas bukan JS tidak dihitung; tanpa entry JS hasilnya nol", () => {
    const m = meta({
      "./index.html": html,
      "./assets/s.css": { bytes: 9, inputs: {}, imports: [], exports: [] },
    });
    expect(initialJsGzip(m, () => 1000).total).toBe(0);
  });
});

describe("budgetViolation", () => {
  test("tepat 200 KB lolos, satu byte lebih gagal dengan pesan berisi ukuran", () => {
    expect(BUDGET).toBe(200 * 1024);
    expect(budgetViolation(BUDGET)).toBeNull();
    expect(budgetViolation(BUDGET + 1)).toContain(String(BUDGET + 1));
  });

  test("anggaran dapat diganti", () => {
    expect(budgetViolation(11, 10)).not.toBeNull();
    expect(budgetViolation(10, 10)).toBeNull();
  });
});

describe("formatBudgetReport", () => {
  const m = meta({
    "./index.html": html,
    "./assets/index-a.js": {
      bytes: 300,
      inputs: {
        "src/a.ts": { bytesInOutput: 100 },
        "node_modules/react/index.js": { bytesInOutput: 200 },
      },
      imports: [{ path: "./assets/lazy-c.js", kind: "dynamic-import" }],
      exports: [],
      entryPoint: "src/index.html",
      cssBundle: "./assets/index-a.css",
    },
    "./assets/lazy-c.js": {
      bytes: 40,
      inputs: { "src/c.ts": { bytesInOutput: 40 } },
      imports: [],
      exports: [],
    },
    "./assets/index-a.css": { bytes: 70, inputs: {}, imports: [], exports: [] },
  });
  const gz = sizes({
    "./assets/index-a.js": 120,
    "./assets/lazy-c.js": 25,
    "./assets/index-a.css": 30,
  });

  test("memuat JS awal, chunk lazy, CSS, font, dan kontributor terbesar", () => {
    const out = formatBudgetReport(m, gz, {
      fonts: [{ path: "assets/figtree-1.woff2", bytes: 20480 }],
    });
    expect(out).toContain("JS awal");
    expect(out).toContain("lazy-c.js");
    expect(out).toContain("index-a.css");
    expect(out).toContain("figtree-1.woff2");
    expect(out).toContain("node_modules/react/index.js");
    expect(out.indexOf("node_modules/react/index.js")).toBeLessThan(out.indexOf("src/a.ts"));
  });

  test("kontributor dibatasi 15 baris", () => {
    const inputs: Record<string, { bytesInOutput: number }> = {};
    for (let i = 0; i < 40; i++) inputs[`src/f${i}.ts`] = { bytesInOutput: i + 1 };
    const big = meta({
      "./assets/index-a.js": {
        bytes: 1,
        inputs,
        imports: [],
        exports: [],
        entryPoint: "src/index.html",
      },
    });
    const out = formatBudgetReport(big, () => 1);
    expect(out.match(/src\/f\d+\.ts/g)).toHaveLength(15);
    expect(out).toContain("src/f39.ts");
    expect(out).not.toContain("src/f0.ts");
  });
});

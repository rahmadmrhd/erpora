import { describe, expect, test } from "bun:test";
import {
  parseComponentNames,
  pinDependencies,
  stripCnDependency,
} from "../../packages/ui/scripts/shadcn-sync";
import { driftLines } from "../../packages/ui/scripts/shadcn-verify";

// Fungsi murni alat `shadcn:sync` dan `shadcn:verify` (M3a revisi, task 1.3; R1.9, D-1).

describe("parseComponentNames", () => {
  test("menerima nama komponen polos", () => {
    expect(parseComponentNames(["badge", "alert-dialog", "toggle-group"])).toEqual([
      "badge",
      "alert-dialog",
      "toggle-group",
    ]);
  });

  test("menolak URL, namespace registry, jalur, dan flag (cegah SSRF/registry tak tepercaya)", () => {
    for (const bad of [
      "https://evil.example/r/x.json",
      "http://localhost/x",
      "@acme/button",
      "../x",
      "a/b",
      "-o",
      "--all",
      "Badge",
      "",
    ]) {
      expect(() => parseComponentNames([bad])).toThrow();
    }
  });

  test("tanpa nama komponen melempar", () => {
    expect(() => parseComponentNames([])).toThrow();
  });

  test("duplikat dihapus, urutan dipertahankan", () => {
    expect(parseComponentNames(["badge", "tabs", "badge"])).toEqual(["badge", "tabs"]);
  });
});

describe("stripCnDependency", () => {
  test("menghapus cn dari dependencies dan devDependencies", () => {
    const pkg = {
      name: "x",
      dependencies: { cn: "^0.1.0", sonner: "2.0.8" },
      devDependencies: { cn: "1.0.0", typescript: "6.0.3" },
    };
    const res = stripCnDependency(pkg);
    expect(res.removed).toBe(true);
    expect(res.pkg.dependencies).toEqual({ sonner: "2.0.8" });
    expect(res.pkg.devDependencies).toEqual({ typescript: "6.0.3" });
  });

  test("tanpa cn: tidak berubah dan removed false", () => {
    const pkg = { dependencies: { sonner: "2.0.8" } };
    const res = stripCnDependency(pkg);
    expect(res.removed).toBe(false);
    expect(res.pkg).toEqual(pkg);
  });
});

describe("pinDependencies", () => {
  const installed = (versions: Record<string, string>) => (name: string) => versions[name];

  test("dependensi baru berentang dipin ke versi terpasang; yang sudah pasti tidak diubah", () => {
    const before = { sonner: "2.0.8", "lucide-react": "1.57.0" };
    const after = { sonner: "2.0.8", "lucide-react": "1.57.0", cmdk: "^1.1.1" };
    const res = pinDependencies(before, after, installed({ cmdk: "1.1.2" }));
    expect(res.deps).toEqual({ cmdk: "1.1.2", "lucide-react": "1.57.0", sonner: "2.0.8" });
    expect(res.pinned).toEqual(["cmdk"]);
  });

  test("tanpa versi terpasang: awalan ^ dan ~ dibuang; rentang lain dilaporkan, tidak ditebak", () => {
    const res = pinDependencies(
      {},
      { vaul: "~1.0.4", "react-resizable": ">=2.0.0" },
      installed({}),
    );
    expect(res.deps.vaul).toBe("1.0.4");
    expect(res.unpinned).toEqual(["react-resizable"]);
  });

  test("dependensi lama yang spesifikasinya berubah juga dipin", () => {
    const res = pinDependencies({ a: "1.0.0" }, { a: "^1.2.0" }, installed({ a: "1.2.3" }));
    expect(res.deps.a).toBe("1.2.3");
  });

  test("dependensi lama berentang yang tidak berubah dibiarkan", () => {
    const res = pinDependencies({ a: "^1.0.0" }, { a: "^1.0.0" }, installed({ a: "1.2.3" }));
    expect(res.deps.a).toBe("^1.0.0");
    expect(res.pinned).toEqual([]);
  });

  test("cn tidak pernah muncul di hasil", () => {
    const res = pinDependencies({}, { cn: "^0.1.0", cmdk: "1.1.2" }, installed({}));
    expect(res.deps).toEqual({ cmdk: "1.1.2" });
  });
});

describe("driftLines (shadcn:verify)", () => {
  const box = (lines: string[]): string =>
    [
      "- Resolving items.",
      "┌ shadcn add badge (dry run)",
      "│",
      ...lines.map((l) => `│ │ ${l}`),
      "└ Run without --dry-run to apply.",
    ].join("\n");

  test("selisih hanya pada impor cn tidak dianggap drift", () => {
    const out = box([
      "--- a/src\\components\\ui\\badge.tsx",
      "+++ b/src\\components\\ui\\badge.tsx",
      "@@ -1,3 +1,3 @@",
      '-import { cn } from "@erpora/ui/lib/utils"',
      '+import { cn } from "cn"',
      ' import { cva } from "class-variance-authority"',
    ]);
    expect(driftLines(out)).toEqual([]);
  });

  test("selisih isi dilaporkan tanpa baris header dan tanpa baris impor cn", () => {
    const out = box([
      "--- a/src\\components\\ui\\badge.tsx",
      "+++ b/src\\components\\ui\\badge.tsx",
      "@@ -1,3 +1,3 @@",
      '-import { cn } from "@erpora/ui/lib/utils"',
      '+import { cn } from "cn"',
      "-const a = 1",
      "+const a = 2",
    ]);
    expect(driftLines(out)).toEqual(["-const a = 1", "+const a = 2"]);
  });

  test("tanpa blok diff (identik) tidak ada drift", () => {
    expect(driftLines(box([]))).toEqual([]);
  });
});

describe("driftLines dengan patch terdaftar", () => {
  const patches = [
    {
      component: "dialog",
      find: '<span className="sr-only">Close</span>',
      replace: '<span className="sr-only"><UiLabel k="actions.close" /></span>',
    },
  ];
  const box = (lines: string[]): string => lines.map((l) => `│ │ ${l}`).join("\n");

  test("baris hasil patch (lokal) dan string asal patch (registry) tidak dianggap drift", () => {
    const out = box([
      "@@ -1,2 +1,2 @@",
      '-<span className="sr-only"><UiLabel k="actions.close" /></span>',
      '+<span className="sr-only">Close</span>',
    ]);
    expect(driftLines(out, patches)).toEqual([]);
    expect(driftLines(out)).toHaveLength(2);
  });

  test("selisih lain tetap dilaporkan", () => {
    const out = box(["-const a = 1", "+const a = 2"]);
    expect(driftLines(out, patches)).toEqual(["-const a = 1", "+const a = 2"]);
  });
});

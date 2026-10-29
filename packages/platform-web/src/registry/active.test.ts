import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { activeLeaf, groupHasActive, isPathActive, type NavNode } from "./index";

const label = { ns: "t", key: "x" };
const item = (id: string, to: string): Extract<NavNode, { type: "item" }> => ({
  type: "item",
  id,
  label,
  to,
  moduleKey: "acme.m",
});
const group = (id: string, children: ReturnType<typeof item>[]): NavNode => ({
  type: "group",
  id,
  label,
  moduleKey: "acme.m",
  children,
});

describe("isPathActive", () => {
  test("persis atau awalan segmen; '/' hanya persis", () => {
    expect(isPathActive("/example", "/example")).toBe(true);
    expect(isPathActive("/example/notes", "/example")).toBe(true);
    expect(isPathActive("/example2", "/example")).toBe(false);
    expect(isPathActive("/", "/")).toBe(true);
    expect(isPathActive("/example", "/")).toBe(false);
  });

  test("garis miring di akhir path diabaikan", () => {
    expect(isPathActive("/example/", "/example")).toBe(true);
    expect(isPathActive("/example", "/example/")).toBe(true);
  });
});

describe("activeLeaf", () => {
  const nav: NavNode[] = [
    item("home", "/example"),
    item("notes", "/example/notes"),
    group("reports", [
      item("sales", "/example/reports/sales"),
      item("stock", "/example/reports/stock"),
    ]),
  ];

  test("memilih item dengan awalan terpanjang (paling spesifik)", () => {
    expect(activeLeaf(nav, "/example")?.id).toBe("home");
    expect(activeLeaf(nav, "/example/notes/42")?.id).toBe("notes");
    expect(activeLeaf(nav, "/example/reports/sales")?.id).toBe("sales");
  });

  test("tidak ada yang cocok: undefined", () => {
    expect(activeLeaf(nav, "/lain")).toBeUndefined();
    expect(activeLeaf([], "/example")).toBeUndefined();
  });

  test("groupHasActive: grup terbuka bila dan hanya bila memuat anak aktif", () => {
    const g = nav[2] as Extract<NavNode, { type: "group" }>;
    expect(groupHasActive(g, "/example/reports/stock/1", nav)).toBe(true);
    expect(groupHasActive(g, "/example/notes", nav)).toBe(false);
  });

  test("P3: untuk path acak paling banyak satu item daun aktif; grup terbuka bila memuat anak aktif", () => {
    const seg = fc.constantFrom("example", "notes", "reports", "sales", "stock", "x");
    fc.assert(
      fc.property(fc.array(seg, { maxLength: 4 }), (parts) => {
        const pathname = `/${parts.join("/")}`;
        const active = activeLeaf(nav, pathname);
        const leaves = nav.flatMap((n) => (n.type === "item" ? [n] : n.children));
        const matching = leaves.filter((l) => isPathActive(pathname, l.to));
        if (matching.length === 0) expect(active).toBeUndefined();
        else expect(matching).toContain(active as NonNullable<typeof active>);
        const g = nav[2] as Extract<NavNode, { type: "group" }>;
        expect(groupHasActive(g, pathname, nav)).toBe(
          active !== undefined && g.children.includes(active),
        );
      }),
      { numRuns: 300 },
    );
  });
});

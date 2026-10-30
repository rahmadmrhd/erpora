import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { type CrumbSlot, collapseCrumbs } from "./breadcrumb";

const labels = (slots: CrumbSlot<string>[]): string[] =>
  slots.map((s) => (s.kind === "ellipsis" ? "…" : s.crumb));

describe("collapseCrumbs (contoh)", () => {
  test("kosong dan satu item tidak dilipat", () => {
    expect(collapseCrumbs([], { compact: false })).toEqual({ visible: [], folded: [] });
    expect(labels(collapseCrumbs(["A"], { compact: false }).visible)).toEqual(["A"]);
    expect(labels(collapseCrumbs(["A"], { compact: true }).visible)).toEqual(["A"]);
  });

  test("n <= 3 dan tidak compact: semua terlihat", () => {
    for (const items of [
      ["A", "B"],
      ["A", "B", "C"],
    ]) {
      const r = collapseCrumbs(items, { compact: false });
      expect(labels(r.visible)).toEqual(items);
      expect(r.folded).toEqual([]);
    }
  });

  test("n > 3: pertama, elipsis, terakhir; sisanya terlipat", () => {
    const r = collapseCrumbs(["A", "B", "C", "D", "E"], { compact: false });
    expect(labels(r.visible)).toEqual(["A", "…", "E"]);
    expect(r.folded).toEqual(["B", "C", "D"]);
  });

  test("compact: elipsis lalu terakhir; semua kecuali terakhir terlipat", () => {
    const r = collapseCrumbs(["A", "B", "C"], { compact: true });
    expect(labels(r.visible)).toEqual(["…", "C"]);
    expect(r.folded).toEqual(["A", "B"]);
    const two = collapseCrumbs(["A", "B"], { compact: true });
    expect(labels(two.visible)).toEqual(["…", "B"]);
    expect(two.folded).toEqual(["A"]);
  });

  test("item terakhir ditandai halaman saat ini", () => {
    const r = collapseCrumbs(["A", "B", "C", "D"], { compact: false });
    const last = r.visible.at(-1);
    expect(last).toEqual({ kind: "crumb", crumb: "D", current: true });
    expect(r.visible[0]).toEqual({ kind: "crumb", crumb: "A", current: false });
  });
});

describe("collapseCrumbs P4 (properti)", () => {
  const input = fc.array(fc.string({ minLength: 1, maxLength: 6 }), { maxLength: 12 });

  test("terakhir selalu halaman dan item terakhir masukan", () => {
    fc.assert(
      fc.property(input, fc.boolean(), (crumbs, compact) => {
        const { visible } = collapseCrumbs(crumbs, { compact });
        if (crumbs.length === 0) return expect(visible).toEqual([]);
        const last = visible.at(-1);
        expect(last).toEqual({ kind: "crumb", crumb: crumbs.at(-1) as string, current: true });
      }),
    );
  });

  test("visible + folded selalu memuat seluruh masukan sesuai urutan", () => {
    fc.assert(
      fc.property(input, fc.boolean(), (crumbs, compact) => {
        const { visible, folded } = collapseCrumbs(crumbs, { compact });
        const rebuilt: string[] = [];
        for (const slot of visible) {
          if (slot.kind === "ellipsis") rebuilt.push(...folded);
          else rebuilt.push(slot.crumb);
        }
        expect(rebuilt).toEqual(crumbs);
      }),
    );
  });

  test("tidak lebih dari 3 tingkat terlihat; elipsis hanya bila ada yang terlipat", () => {
    fc.assert(
      fc.property(input, fc.boolean(), (crumbs, compact) => {
        const { visible, folded } = collapseCrumbs(crumbs, { compact });
        expect(visible.length).toBeLessThanOrEqual(3);
        const ellipses = visible.filter((s) => s.kind === "ellipsis").length;
        expect(ellipses).toBe(folded.length > 0 ? 1 : 0);
        expect(ellipses).toBeLessThanOrEqual(1);
      }),
    );
  });

  test("tepat satu penanda halaman saat ini bila masukan tidak kosong", () => {
    fc.assert(
      fc.property(input, fc.boolean(), (crumbs, compact) => {
        const { visible } = collapseCrumbs(crumbs, { compact });
        const current = visible.filter((s) => s.kind === "crumb" && s.current).length;
        expect(current).toBe(crumbs.length === 0 ? 0 : 1);
      }),
    );
  });
});

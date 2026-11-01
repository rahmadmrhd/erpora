import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { type FitInput, type FitPlan, fitBreadcrumb } from "./breadcrumb-fit";

const keys = (n: number): string[] => Array.from({ length: n }, (_, i) => `s${i}`);

function input(n: number, over: Partial<FitInput> = {}): FitInput {
  return {
    segments: keys(n),
    widths: Array.from({ length: n }, () => 50),
    available: 10_000,
    mobile: false,
    separatorWidth: 10,
    ellipsisWidth: 20,
    ...over,
  };
}

/** Indeks segmen terlihat (tanpa elipsis) dan posisi elipsis (indeks slot, -1 bila tidak ada). */
function shape(plan: FitPlan): { visible: number[]; ellipsisAt: number } {
  return {
    visible: plan.slots.flatMap((s) => (s.kind === "segment" ? [s.index] : [])),
    ellipsisAt: plan.slots.findIndex((s) => s.kind === "ellipsis"),
  };
}

// Lebar penuh n=6: 6*50 + 5*10 = 350; Opsi A: 4*50 + 20 + 4*10 = 260; Opsi B: 3*50 + 20 + 3*10 = 200;
// B lanjutan: 2*50 + 20 + 2*10 = 140; terakhir saja: 50 + 20 + 10 = 80.
describe("fitBreadcrumb (contoh)", () => {
  test("semua muat: tahap utuh tanpa elipsis", () => {
    const plan = fitBreadcrumb(input(6, { available: 350 }));
    expect(plan.stage).toBe("full");
    expect(shape(plan)).toEqual({ visible: [0, 1, 2, 3, 4, 5], ellipsisAt: -1 });
    expect(plan.folded).toEqual([]);
    expect(plan.width).toBe(350);
    expect(plan.truncateLast).toBe(false);
  });

  test("Opsi A: Home, Desk, elipsis, dua langkah terakhir", () => {
    const plan = fitBreadcrumb(input(6, { available: 349 }));
    expect(plan.stage).toBe("optionA");
    expect(shape(plan)).toEqual({ visible: [0, 1, 4, 5], ellipsisAt: 2 });
    expect(plan.folded).toEqual([2, 3]);
    expect(plan.width).toBe(260);
  });

  test("Opsi B: Home, elipsis, dua langkah terakhir (Desk ikut terlipat)", () => {
    const plan = fitBreadcrumb(input(6, { available: 259 }));
    expect(plan.stage).toBe("optionB");
    expect(shape(plan)).toEqual({ visible: [0, 4, 5], ellipsisAt: 1 });
    expect(plan.folded).toEqual([1, 2, 3]);
    expect(plan.width).toBe(200);
  });

  test("Opsi B lanjutan: Home, elipsis, langkah terakhir", () => {
    const plan = fitBreadcrumb(input(6, { available: 199 }));
    expect(plan.stage).toBe("optionBLast");
    expect(shape(plan)).toEqual({ visible: [0, 5], ellipsisAt: 1 });
    expect(plan.folded).toEqual([1, 2, 3, 4]);
    expect(plan.width).toBe(140);
  });

  test("sangat sempit: elipsis lalu langkah terakhir, Home ikut terlipat", () => {
    const plan = fitBreadcrumb(input(6, { available: 139 }));
    expect(plan.stage).toBe("lastOnly");
    expect(shape(plan)).toEqual({ visible: [5], ellipsisAt: 0 });
    expect(plan.folded).toEqual([0, 1, 2, 3, 4]);
    expect(plan.width).toBe(80);
  });

  test("lebih sempit lagi: langkah terakhir dipotong elipsis dengan lebar maksimum", () => {
    const plan = fitBreadcrumb(input(6, { available: 60 }));
    expect(plan.stage).toBe("truncated");
    expect(shape(plan)).toEqual({ visible: [5], ellipsisAt: 0 });
    expect(plan.truncateLast).toBe(true);
    expect(plan.lastMaxWidth).toBe(30); // 60 - elipsis 20 - pemisah 10
    expect(plan.width).toBe(60);
  });

  test("mobile: langsung elipsis lalu langkah terakhir walau semua muat", () => {
    const plan = fitBreadcrumb(input(6, { mobile: true }));
    expect(plan.stage).toBe("lastOnly");
    expect(shape(plan)).toEqual({ visible: [5], ellipsisAt: 0 });
    expect(plan.folded).toEqual([0, 1, 2, 3, 4]);
  });

  test("mobile dan sangat sempit: langkah terakhir dipotong", () => {
    const plan = fitBreadcrumb(input(6, { mobile: true, available: 45 }));
    expect(plan.stage).toBe("truncated");
    expect(plan.lastMaxWidth).toBe(15);
  });

  test("tanpa segmen Desk (pinned: 1): Opsi A tidak ada, langsung Opsi B", () => {
    // [Home, c1, c2, c3, c4]: utuh 5*50+4*10 = 290; Opsi B: Home, elipsis, c3, c4 = 200
    const plan = fitBreadcrumb(input(5, { available: 289, pinned: 1 }));
    expect(plan.stage).toBe("optionB");
    expect(shape(plan).visible).toEqual([0, 3, 4]);
  });

  test("tiga segmen [Home, Desk, Halaman]: langsung Opsi B lanjutan bila tak muat", () => {
    // utuh: 3*50+2*10 = 170; Home, elipsis, Halaman = 140
    const plan = fitBreadcrumb(input(3, { available: 169 }));
    expect(plan.stage).toBe("optionBLast");
    expect(shape(plan)).toEqual({ visible: [0, 2], ellipsisAt: 1 });
    expect(plan.folded).toEqual([1]);
  });

  test("dua segmen [Home, Halaman]: utuh, lalu elipsis + Halaman, lalu terpotong", () => {
    expect(fitBreadcrumb(input(2, { available: 110 })).stage).toBe("full");
    const last = fitBreadcrumb(input(2, { available: 109 }));
    expect(last.stage).toBe("lastOnly");
    expect(last.folded).toEqual([0]);
    expect(fitBreadcrumb(input(2, { available: 79 })).stage).toBe("truncated");
  });

  test("satu segmen: hanya halaman itu, tanpa elipsis; terpotong bila tak muat", () => {
    expect(fitBreadcrumb(input(1)).stage).toBe("full");
    const t = fitBreadcrumb(input(1, { available: 30 }));
    expect(t.stage).toBe("truncated");
    expect(shape(t)).toEqual({ visible: [0], ellipsisAt: -1 });
    expect(t.lastMaxWidth).toBe(30);
  });

  test("tanpa segmen: rencana kosong", () => {
    const plan = fitBreadcrumb(input(0));
    expect(plan.slots).toEqual([]);
    expect(plan.folded).toEqual([]);
    expect(plan.stage).toBe("full");
  });

  test("lebar tak valid (NaN, negatif, kurang) diperlakukan 0", () => {
    const plan = fitBreadcrumb({
      segments: keys(3),
      widths: [Number.NaN, -5],
      available: 100,
      mobile: false,
      separatorWidth: 0,
      ellipsisWidth: 0,
    });
    expect(plan.stage).toBe("full");
    expect(plan.width).toBe(0);
  });
});

const arbInput = fc
  .record({
    n: fc.integer({ min: 0, max: 10 }),
    seed: fc.array(fc.integer({ min: 0, max: 300 }), { minLength: 10, maxLength: 10 }),
    available: fc.integer({ min: 0, max: 3000 }),
    mobile: fc.boolean(),
    separatorWidth: fc.integer({ min: 0, max: 24 }),
    ellipsisWidth: fc.integer({ min: 0, max: 40 }),
    pinned: fc.integer({ min: 1, max: 3 }),
  })
  .map(({ n, seed, ...rest }) => ({
    segments: keys(n),
    widths: seed.slice(0, n),
    ...rest,
  }));

describe("fitBreadcrumb (properti)", () => {
  test("Home dan halaman saat ini selalu tersedia (terlihat atau di dalam elipsis); tanpa duplikasi", () => {
    fc.assert(
      fc.property(arbInput, (inp) => {
        const plan = fitBreadcrumb(inp);
        const n = inp.segments.length;
        const visible = shape(plan).visible;
        const all = [...visible, ...plan.folded].sort((a, b) => a - b);
        expect(all).toEqual(Array.from({ length: n }, (_, i) => i));
        expect(new Set(visible).size).toBe(visible.length);
        if (n > 0) expect(visible.includes(n - 1)).toBe(true); // halaman saat ini selalu terlihat
      }),
    );
  });

  test("elipsis ada tepat bila ada yang terlipat; folded terurut dan berkesinambungan", () => {
    fc.assert(
      fc.property(arbInput, (inp) => {
        const plan = fitBreadcrumb(inp);
        const ellipses = plan.slots.filter((s) => s.kind === "ellipsis").length;
        expect(ellipses).toBe(plan.folded.length > 0 ? 1 : 0);
        for (let i = 1; i < plan.folded.length; i++) {
          expect(plan.folded[i]).toBe((plan.folded[i - 1] as number) + 1);
        }
      }),
    );
  });

  test("lebar total tidak melampaui available kecuali tahap akhir yang dipotong elipsis", () => {
    fc.assert(
      fc.property(arbInput, (inp) => {
        const plan = fitBreadcrumb(inp);
        if (plan.stage !== "truncated") {
          expect(plan.truncateLast).toBe(false);
          expect(plan.lastMaxWidth).toBeNull();
          expect(plan.width).toBeLessThanOrEqual(inp.available);
        } else {
          expect(plan.truncateLast).toBe(true);
          expect(plan.lastMaxWidth).not.toBeNull();
          expect(plan.lastMaxWidth as number).toBeGreaterThanOrEqual(0);
          const others = plan.width - (plan.lastMaxWidth as number);
          if (others <= inp.available) expect(plan.width).toBeLessThanOrEqual(inp.available);
        }
      }),
    );
  });

  test("tahap monoton menyempit bersama available (mobile dan lainnya tetap)", () => {
    fc.assert(
      fc.property(arbInput, fc.integer({ min: 0, max: 3000 }), (inp, other) => {
        const a = fitBreadcrumb(inp);
        const b = fitBreadcrumb({ ...inp, available: other });
        if (inp.available >= other) expect(a.level).toBeLessThanOrEqual(b.level);
        else expect(a.level).toBeGreaterThanOrEqual(b.level);
      }),
    );
  });

  test("mobile hanya menampilkan elipsis lalu langkah terakhir (Home di dalam elipsis)", () => {
    fc.assert(
      fc.property(arbInput, (inp) => {
        const plan = fitBreadcrumb({ ...inp, mobile: true });
        const n = inp.segments.length;
        if (n < 2) return;
        expect(["lastOnly", "truncated"]).toContain(plan.stage);
        expect(shape(plan)).toEqual({ visible: [n - 1], ellipsisAt: 0 });
        expect(plan.folded.includes(0)).toBe(true);
      }),
    );
  });

  test("tahap utuh tepat bila semua muat dan bukan mobile", () => {
    fc.assert(
      fc.property(arbInput, (inp) => {
        const n = inp.segments.length;
        if (n === 0) return;
        const total =
          inp.widths.reduce((s, w) => s + w, 0) + inp.separatorWidth * Math.max(0, n - 1);
        const plan = fitBreadcrumb({ ...inp, mobile: false });
        expect(plan.stage === "full").toBe(total <= inp.available);
      }),
    );
  });

  test("tidak mengubah masukan", () => {
    fc.assert(
      fc.property(arbInput, (inp) => {
        const copy = JSON.stringify(inp);
        fitBreadcrumb(inp);
        expect(JSON.stringify(inp)).toBe(copy);
      }),
    );
  });
});

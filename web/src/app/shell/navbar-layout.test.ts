import { describe, expect, test } from "bun:test";
import { NARROW_NAVBAR_WIDTH, type NavbarLayoutInput, planNavbar } from "./navbar-layout";

// Breadcrumb [Home, Desk, A, B] lebar 16, 60, 40, 80; pemisah 20; elipsis 24; utuh = 196 + 3*20 = 256.
function input(over: Partial<NavbarLayoutInput> = {}): NavbarLayoutInput {
  return {
    navWidth: 1000,
    inset: 28,
    gap: 10,
    leadingWidth: 36,
    breadcrumb: {
      segments: ["home", "desk", "a", "b"],
      widths: [16, 60, 40, 80],
      separatorWidth: 20,
      ellipsisWidth: 24,
      pinned: 2,
    },
    actions: {
      primaryWidth: 100,
      iconWidth: 36,
      secondaryWidths: [90, 70],
      moreWidth: 36,
      gap: 8,
    },
    ...over,
  };
}

describe("planNavbar", () => {
  test("lebar lega: semua aksi inline dan breadcrumb utuh", () => {
    const l = planNavbar(input());
    expect(l.narrow).toBe(false);
    expect(l.collapsedActions).toBe(0);
    expect(l.plan.stage).toBe("full");
  });

  test("belum terukur (navWidth <= 0): tanpa pembatasan", () => {
    const l = planNavbar(input({ navWidth: 0 }));
    expect(l.narrow).toBe(false);
    expect(l.collapsedActions).toBe(0);
    expect(l.plan.stage).toBe("full");
  });

  test("ruang menyempit: aksi sekunder masuk ke ⋯ lebih dulu sehingga breadcrumb tetap utuh", () => {
    // header: 28 + leading 36 + gap 10 + breadcrumb 256 + gap 10 + aksi; aksi k=0: 100+90+70+2*8 = 276; k=1: 100+90+36+16 = 242; k=2: 100+36+8 = 144
    const base = 28 + 36 + 10 + 256 + 10;
    expect(planNavbar(input({ navWidth: base + 276 })).collapsedActions).toBe(0);
    expect(planNavbar(input({ navWidth: base + 275 })).collapsedActions).toBe(1);
    expect(planNavbar(input({ navWidth: base + 241 })).collapsedActions).toBe(2);
    const l = planNavbar(input({ navWidth: base + 144 }));
    expect(l.collapsedActions).toBe(2);
    expect(l.plan.stage).toBe("full");
  });

  test("hanya setelah semua sekunder terlipat, breadcrumb diringkas", () => {
    const base = 28 + 36 + 10 + 10;
    const l = planNavbar(input({ navWidth: base + 144 + 255 }));
    expect(l.collapsedActions).toBe(2);
    expect(l.plan.stage).toBe("optionB"); // empat segmen: Opsi A identik dengan utuh sehingga dilewati
  });

  test("tanpa aksi sekunder: tidak ada ⋯", () => {
    const l = planNavbar(
      input({
        navWidth: 400,
        actions: { primaryWidth: 100, iconWidth: 36, secondaryWidths: [], moreWidth: 36, gap: 8 },
      }),
    );
    expect(l.collapsedActions).toBe(0);
  });

  test("tanpa aksi sama sekali: tidak ada jarak ke blok aksi", () => {
    const l = planNavbar(
      input({
        navWidth: 480, // sisa untuk breadcrumb: 480 - 28 - (36 + 10) = 406 = lebar utuh tepat
        breadcrumb: {
          segments: ["home", "desk", "a", "b"],
          widths: [16, 60, 40, 230],
          separatorWidth: 20,
          ellipsisWidth: 24,
          pinned: 2,
        },
        actions: { primaryWidth: 0, iconWidth: 36, secondaryWidths: [], moreWidth: 36, gap: 8 },
      }),
    );
    expect(l.plan.stage).toBe("full");
  });

  test("tanpa leading: tidak ada jarak untuk blok leading", () => {
    const l = planNavbar(
      input({
        leadingWidth: 0,
        navWidth: 28 + 256 + 10 + 276,
      }),
    );
    expect(l.plan.stage).toBe("full");
    expect(l.collapsedActions).toBe(0);
  });

  test("navbar sempit (< 480 px): ponsel, semua sekunder di ⋯, aksi utama berupa ikon, breadcrumb terakhir saja", () => {
    const l = planNavbar(input({ navWidth: NARROW_NAVBAR_WIDTH - 1 }));
    expect(l.narrow).toBe(true);
    expect(l.collapsedActions).toBe(2);
    expect(l.plan.stage).toBe("lastOnly");
    expect(l.plan.folded).toEqual([0, 1, 2]);
  });

  test("tepat 480 px bukan ponsel", () => {
    expect(planNavbar(input({ navWidth: NARROW_NAVBAR_WIDTH })).narrow).toBe(false);
  });

  test("ponsel sangat sempit: langkah terakhir dipotong", () => {
    const l = planNavbar(input({ navWidth: 150 }));
    expect(l.narrow).toBe(true);
    expect(l.plan.stage).toBe("truncated");
    expect(l.plan.lastMaxWidth).toBeGreaterThanOrEqual(0);
  });

  test("tanpa Desk (pinned: 1) diteruskan ke fitBreadcrumb", () => {
    const l = planNavbar(
      input({
        navWidth: 700,
        breadcrumb: {
          segments: ["home", "a", "b", "c", "d"],
          widths: [16, 100, 100, 100, 100],
          separatorWidth: 20,
          ellipsisWidth: 24,
          pinned: 1,
        },
      }),
    );
    expect(l.plan.stage).toBe("optionB");
  });
});

/** PRNG deterministik (mulberry32) agar pengujian acak dapat diulang tanpa dependensi tambahan. */
function rng(seed: number): () => number {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function randomInput(r: () => number, navWidth: number): NavbarLayoutInput {
  const int = (min: number, max: number) => min + Math.floor(r() * (max - min + 1));
  const n = int(1, 8);
  return input({
    navWidth,
    breadcrumb: {
      segments: Array.from({ length: n }, (_, i) => `s${i}`),
      widths: Array.from({ length: n }, () => int(0, 200)),
      separatorWidth: 16,
      ellipsisWidth: 24,
      pinned: 2,
    },
    actions: {
      primaryWidth: int(0, 200),
      iconWidth: 36,
      secondaryWidths: Array.from({ length: int(0, 4) }, () => int(20, 160)),
      moreWidth: 36,
      gap: 8,
    },
  });
}

describe("planNavbar (acak deterministik)", () => {
  test("jumlah sekunder terlipat 0..S; ponsel melipat semuanya", () => {
    const r = rng(1);
    for (let i = 0; i < 400; i++) {
      const inp = randomInput(r, Math.floor(r() * 2000));
      const l = planNavbar(inp);
      expect(l.collapsedActions).toBeGreaterThanOrEqual(0);
      expect(l.collapsedActions).toBeLessThanOrEqual(inp.actions.secondaryWidths.length);
      if (l.narrow) expect(l.collapsedActions).toBe(inp.actions.secondaryWidths.length);
    }
  });

  test("lebih lebar tidak pernah melipat lebih banyak atau meringkas lebih jauh", () => {
    const r = rng(2);
    for (let i = 0; i < 400; i++) {
      const base = Math.floor(r() * 2000);
      const extra = Math.floor(r() * 600);
      const seed = Math.floor(r() * 1e9);
      const narrow = planNavbar(randomInput(rng(seed), base));
      const wide = planNavbar(randomInput(rng(seed), base + extra));
      if (base <= 0 || narrow.narrow !== wide.narrow) continue; // lintas ambang ponsel atau belum terukur
      const rank = (l: typeof narrow) => l.collapsedActions * 10 + l.plan.level;
      expect(rank(wide)).toBeLessThanOrEqual(rank(narrow));
    }
  });
});

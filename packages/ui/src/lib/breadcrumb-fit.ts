/**
 * Peringkasan breadcrumb bertahap berdasarkan lebar (M3a revisi, R7). Fungsi murni: tidak mengukur DOM; pemanggil
 * memberi lebar tiap segmen (`widths`) dan ruang yang tersedia (`available`), hasilnya rencana gambar (`FitPlan`).
 *
 * `segments` berurutan `[Home, Desk?, ...langkah]` dan segmen terakhir adalah halaman saat ini. Tahap dicoba berurutan
 * dan yang pertama muat dipilih (sehingga tahap menyempit secara monoton bersama `available`):
 *
 * 0 `full`         semua segmen.
 * 1 `optionA`      Home, Desk, elipsis, dua langkah terakhir (hanya bila `pinned >= 2`).
 * 2 `optionB`      Home, elipsis, dua langkah terakhir (Desk ikut terlipat).
 * 3 `optionBLast`  Home, elipsis, langkah terakhir.
 * 4 `lastOnly`     elipsis, langkah terakhir (Home ikut terlipat); satu-satunya tahap bila `mobile`.
 * 5 `truncated`    seperti `lastOnly` tetapi langkah terakhir dipotong elipsis teks (`lastMaxWidth`).
 *
 * Tahap yang tidak melipat apa pun (identik dengan tahap sebelumnya, mis. Opsi A pada tiga segmen) dilewati. Elipsis
 * menampung segmen yang terlipat (`folded`, indeks berurutan dan berkesinambungan); halaman saat ini selalu terlihat.
 */

export interface FitInput {
  /** Kunci segmen berurutan; `[0]` Home, terakhir = halaman saat ini. */
  segments: readonly string[];
  /** Lebar tiap segmen (px), sejajar dengan `segments`; nilai tak valid (NaN, negatif, hilang) dianggap 0. */
  widths: readonly number[];
  /** Ruang yang tersedia untuk breadcrumb (px). */
  available: number;
  /** Layar ponsel (navbar < 480 px): langsung tahap `lastOnly`. */
  mobile: boolean;
  /** Lebar satu pemisah termasuk jarak di kiri dan kanannya; bawaan 16. */
  separatorWidth?: number;
  /** Lebar pemicu elipsis; bawaan 24. */
  ellipsisWidth?: number;
  /** Jumlah segmen awal yang dipertahankan pada Opsi A (Home + Desk = 2, tanpa Desk = 1); bawaan 2. */
  pinned?: number;
}

export type FitStage = "full" | "optionA" | "optionB" | "optionBLast" | "lastOnly" | "truncated";

export type FitSlot = { kind: "segment"; index: number } | { kind: "ellipsis" };

export interface FitPlan {
  stage: FitStage;
  /** Urutan tahap (0 utuh sampai 5 terpotong); membesar saat ruang menyempit. */
  level: number;
  /** Urutan gambar: segmen (menurut indeks `segments`) dan paling banyak satu elipsis. */
  slots: FitSlot[];
  /** Indeks segmen di dalam elipsis (berurutan); kosong bila tidak ada elipsis. */
  folded: number[];
  /** Lebar hasil gambar (langkah terakhir sudah dibatasi `lastMaxWidth` pada tahap `truncated`). */
  width: number;
  /** Langkah terakhir harus dipotong dengan elipsis teks. */
  truncateLast: boolean;
  /** Lebar maksimum langkah terakhir (px) pada tahap `truncated`, selain itu `null`. */
  lastMaxWidth: number | null;
}

const LEVELS: Record<FitStage, number> = {
  full: 0,
  optionA: 1,
  optionB: 2,
  optionBLast: 3,
  lastOnly: 4,
  truncated: 5,
};

const clean = (v: number | undefined): number =>
  typeof v === "number" && Number.isFinite(v) && v > 0 ? v : 0;

interface Candidate {
  stage: Exclude<FitStage, "truncated">;
  /** Indeks terlihat berurutan. */
  visible: number[];
}

function range(from: number, to: number): number[] {
  const out: number[] = [];
  for (let i = from; i < to; i++) out.push(i);
  return out;
}

/** Himpunan indeks terlihat untuk awalan `head` segmen pertama dan `tail` segmen terakhir (tanpa duplikasi). */
function headTail(n: number, head: number, tail: number): number[] {
  const set = new Set<number>([...range(0, Math.min(head, n)), ...range(Math.max(0, n - tail), n)]);
  return [...set].sort((a, b) => a - b);
}

function candidates(n: number, mobile: boolean, pinned: number): Candidate[] {
  const all: Candidate[] = [{ stage: "full", visible: range(0, n) }];
  if (pinned >= 2) all.push({ stage: "optionA", visible: headTail(n, pinned, 2) });
  all.push(
    { stage: "optionB", visible: headTail(n, 1, 2) },
    { stage: "optionBLast", visible: headTail(n, 1, 1) },
    { stage: "lastOnly", visible: headTail(n, 0, 1) },
  );
  const kept: Candidate[] = [];
  for (const c of all) {
    const prev = kept.at(-1);
    const sameAsPrev = prev !== undefined && prev.visible.join(",") === c.visible.join(",");
    if (!sameAsPrev) kept.push(c);
  }
  return mobile && n >= 2 ? kept.filter((c) => c.stage === "lastOnly") : kept;
}

function slotsOf(visible: readonly number[], n: number): { slots: FitSlot[]; folded: number[] } {
  const present = new Set(visible);
  const folded = range(0, n).filter((i) => !present.has(i));
  const slots: FitSlot[] = [];
  let placed = folded.length === 0;
  const first = folded[0] ?? n;
  for (const index of visible) {
    if (!placed && index > first) {
      slots.push({ kind: "ellipsis" });
      placed = true;
    }
    slots.push({ kind: "segment", index });
  }
  return { slots, folded };
}

export function fitBreadcrumb(input: FitInput): FitPlan {
  const n = input.segments.length;
  const sep = clean(input.separatorWidth ?? 16);
  const ell = clean(input.ellipsisWidth ?? 24);
  const pinned = Math.max(1, Math.floor(input.pinned ?? 2));
  const width = (i: number): number => clean(input.widths[i]);

  if (n === 0) {
    return {
      stage: "full",
      level: 0,
      slots: [],
      folded: [],
      width: 0,
      truncateLast: false,
      lastMaxWidth: null,
    };
  }

  const measure = (visible: readonly number[], hasEllipsis: boolean): number => {
    const items = visible.length + (hasEllipsis ? 1 : 0);
    const content = visible.reduce((sum, i) => sum + width(i), 0) + (hasEllipsis ? ell : 0);
    return content + sep * Math.max(0, items - 1);
  };

  const list = candidates(n, input.mobile, pinned);
  for (const c of list) {
    const { slots, folded } = slotsOf(c.visible, n);
    const total = measure(c.visible, folded.length > 0);
    if (total <= input.available) {
      return {
        stage: c.stage,
        level: LEVELS[c.stage],
        slots,
        folded,
        width: total,
        truncateLast: false,
        lastMaxWidth: null,
      };
    }
  }

  // Tidak ada tahap yang muat: tahap tersempit dengan langkah terakhir dipotong elipsis teks.
  const narrowest = list.at(-1) as Candidate;
  const { slots, folded } = slotsOf(narrowest.visible, n);
  const hasEllipsis = folded.length > 0;
  const others = measure(narrowest.visible, hasEllipsis) - width(n - 1);
  const lastMaxWidth = Math.max(0, input.available - others);
  return {
    stage: "truncated",
    level: LEVELS.truncated,
    slots,
    folded,
    width: others + lastMaxWidth,
    truncateLast: true,
    lastMaxWidth,
  };
}

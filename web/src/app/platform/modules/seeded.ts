// Pembangkit acak berbenih untuk test properti P7 dan P12 (design: "pembangkit berbenih"): deterministik dan tanpa
// dependensi baru. Seed ikut dicetak pada kegagalan sehingga kasus dapat diulang.
export interface Rng {
  next(): number;
  int(min: number, max: number): number;
  pick<T>(xs: readonly T[]): T;
  bool(): boolean;
}

export function rng(seed: number): Rng {
  let a = seed >>> 0;
  const next = () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
  return {
    next,
    int: (min, max) => min + Math.floor(next() * (max - min + 1)),
    pick: (xs) => xs[Math.floor(next() * xs.length)] as (typeof xs)[number],
    bool: () => next() < 0.5,
  };
}

/** Menjalankan `fn` untuk `count` seed berurutan; galat diberi tahu seed-nya. */
export async function forSeeds(
  count: number,
  fn: (r: Rng, seed: number) => void | Promise<void>,
): Promise<void> {
  for (let seed = 1; seed <= count; seed++) {
    try {
      await fn(rng(seed), seed);
    } catch (error) {
      throw new Error(
        `kegagalan pada seed ${seed}: ${error instanceof Error ? error.message : String(error)}`,
      );
    }
  }
}

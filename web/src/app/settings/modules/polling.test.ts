import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import {
  POLL_MAX_MS,
  POLL_MIN_MS,
  POLL_TIMEOUT_MS,
  type PollerDeps,
  pollDelay,
  startPolling,
} from "./polling";
import type { OperationView } from "./types";

const op = (id: string, status: OperationView["status"]): OperationView =>
  ({
    id,
    moduleKey: "a.x",
    operation: "install",
    status,
    steps: [],
    failure: null,
  }) as unknown as OperationView;

/** Jam dan penjadwal palsu: `advance(ms)` menjalankan timer yang jatuh tempo berurutan. */
function fakeClock() {
  let now = 0;
  let seq = 0;
  const timers = new Map<number, { at: number; fn: () => void }>();
  return {
    now: () => now,
    setTimer: (fn: () => void, ms: number) => {
      const id = ++seq;
      timers.set(id, { at: now + ms, fn });
      return id;
    },
    clearTimer: (id: unknown) => void timers.delete(id as number),
    pending: () => timers.size,
    async advance(ms: number) {
      const end = now + ms;
      for (;;) {
        const next = [...timers.entries()].sort((a, b) => a[1].at - b[1].at)[0];
        if (!next || next[1].at > end) break;
        now = next[1].at;
        timers.delete(next[0]);
        next[1].fn();
        await Promise.resolve();
        await Promise.resolve();
      }
      now = end;
    },
  };
}

function harness(script: (call: number) => readonly OperationView[] | Error) {
  const clock = fakeClock();
  const log = {
    fetches: [] as number[],
    updates: 0,
    finished: undefined as readonly OperationView[] | undefined,
    timeout: false,
    errors: 0,
  };
  let calls = 0;
  const deps: PollerDeps = {
    fetchStates: async () => {
      log.fetches.push(clock.now());
      const r = script(calls++);
      if (r instanceof Error) throw r;
      return r;
    },
    onUpdate: () => void log.updates++,
    onFinish: (ops) => void (log.finished = ops),
    onTimeout: () => void (log.timeout = true),
    onError: () => void log.errors++,
    now: clock.now,
    setTimer: clock.setTimer,
    clearTimer: clock.clearTimer,
  };
  return { clock, log, deps };
}

describe("pollDelay", () => {
  test("mulai 1 dtk, tidak turun, dan tidak melebihi 5 dtk", () => {
    expect(pollDelay(0)).toBe(POLL_MIN_MS);
    fc.assert(
      fc.property(fc.integer({ min: 0, max: 10_000 }), (n) => {
        const d = pollDelay(n);
        expect(d).toBeGreaterThanOrEqual(POLL_MIN_MS);
        expect(d).toBeLessThanOrEqual(POLL_MAX_MS);
        expect(pollDelay(n + 1)).toBeGreaterThanOrEqual(d);
      }),
    );
    expect(pollDelay(1000)).toBe(POLL_MAX_MS);
  });
});

describe("startPolling (P12)", () => {
  test("berhenti setelah semua operasi succeeded/failed dan melaporkan hasil akhir", async () => {
    const h = harness((n) =>
      n < 2 ? [op("a", "running"), op("b", "running")] : [op("a", "succeeded"), op("b", "failed")],
    );
    startPolling(h.deps);
    await h.clock.advance(60_000);
    expect(h.log.finished?.map((o) => o.status)).toEqual(["succeeded", "failed"]);
    expect(h.log.fetches).toHaveLength(3);
    expect(h.clock.pending()).toBe(0);
  });

  test("selama masih ada yang running, polling berlanjut", async () => {
    const h = harness(() => [op("a", "succeeded"), op("b", "running")]);
    startPolling(h.deps);
    await h.clock.advance(30_000);
    expect(h.log.finished).toBeUndefined();
    expect(h.log.fetches.length).toBeGreaterThan(3);
  });

  test("jeda antarpermintaan selalu di [1 dtk, 5 dtk]", async () => {
    const h = harness(() => [op("a", "running")]);
    startPolling(h.deps);
    await h.clock.advance(120_000);
    const gaps = h.log.fetches.slice(1).map((t, i) => t - (h.log.fetches[i] as number));
    expect(gaps.length).toBeGreaterThan(10);
    for (const g of gaps) {
      expect(g).toBeGreaterThanOrEqual(POLL_MIN_MS);
      expect(g).toBeLessThanOrEqual(POLL_MAX_MS);
    }
    expect(gaps[0]).toBe(POLL_MIN_MS);
    expect(gaps.at(-1)).toBe(POLL_MAX_MS);
  });

  test("berhenti setelah 10 menit dengan onTimeout", async () => {
    const h = harness(() => [op("a", "running")]);
    startPolling(h.deps);
    await h.clock.advance(POLL_TIMEOUT_MS + 6000);
    expect(h.log.timeout).toBe(true);
    expect(h.clock.pending()).toBe(0);
    const last = h.log.fetches.at(-1) as number;
    expect(last).toBeLessThanOrEqual(POLL_TIMEOUT_MS);
  });

  test("stop() (unmount) membatalkan timer dan tidak ada permintaan lagi", async () => {
    const h = harness(() => [op("a", "running")]);
    const p = startPolling(h.deps);
    await h.clock.advance(3000);
    const n = h.log.fetches.length;
    p.stop();
    await h.clock.advance(60_000);
    expect(h.log.fetches).toHaveLength(n);
    expect(h.clock.pending()).toBe(0);
  });

  test("galat jaringan dilaporkan dan polling dicoba lagi sampai selesai atau habis waktu", async () => {
    const h = harness((n) => (n < 2 ? new Error("jaringan putus") : [op("a", "succeeded")]));
    startPolling(h.deps);
    await h.clock.advance(60_000);
    expect(h.log.errors).toBe(2);
    expect(h.log.finished?.[0]?.status).toBe("succeeded");
  });

  test("properti: untuk urutan status apa pun, berhenti tepat saat semuanya selesai", async () => {
    await fc.assert(
      fc.asyncProperty(
        fc.array(
          fc.array(fc.constantFrom("running", "succeeded", "failed"), {
            minLength: 1,
            maxLength: 3,
          }),
          { minLength: 1, maxLength: 8 },
        ),
        async (script) => {
          const h = harness((n) =>
            (script[Math.min(n, script.length - 1)] ?? []).map((s, i) => op(`o${i}`, s)),
          );
          startPolling(h.deps);
          await h.clock.advance(POLL_TIMEOUT_MS + 10_000);
          const firstDone = script.findIndex((row) => row.every((s) => s !== "running"));
          if (firstDone >= 0) {
            expect(h.log.fetches).toHaveLength(firstDone + 1);
            expect(h.log.finished).toBeDefined();
          } else {
            expect(h.log.timeout).toBe(true);
          }
          expect(h.clock.pending()).toBe(0);
        },
      ),
      { numRuns: 40 },
    );
  });
});

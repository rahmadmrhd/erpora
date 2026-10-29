import type { OperationView } from "./types";

/** Jeda polling operasi (M3a K-32, P12): mulai 1 dtk, mundur ke 5 dtk; berhenti paling lama 10 menit. */
export const POLL_MIN_MS = 1000;
export const POLL_MAX_MS = 5000;
export const POLL_TIMEOUT_MS = 10 * 60 * 1000;

/** Jeda sebelum percobaan ke-`attempt+1` (attempt mulai 0): naik 1 dtk per percobaan, dibatasi [1 dtk, 5 dtk]. */
export function pollDelay(attempt: number): number {
  const n = Number.isFinite(attempt) ? Math.max(0, Math.floor(attempt)) : 0;
  return Math.min(POLL_MAX_MS, POLL_MIN_MS * (n + 1));
}

export interface PollerDeps {
  /** Status terbaru seluruh operasi yang dipantau. */
  fetchStates(): Promise<readonly OperationView[]>;
  onUpdate(ops: readonly OperationView[]): void;
  /** Dipanggil sekali saat seluruh operasi `succeeded`/`failed`. */
  onFinish(ops: readonly OperationView[]): void;
  /** Dipanggil sekali bila 10 menit lewat tanpa selesai. */
  onTimeout(): void;
  /** Galat jaringan/server saat membaca status; polling dicoba lagi. */
  onError?(error: unknown): void;
  /** Galat permanen (mis. 401/403/404): mencoba lagi tidak akan membantu, polling dihentikan segera. */
  isPermanent?(error: unknown): boolean;
  /** Dipanggil sekali bila polling dihentikan karena galat permanen. */
  onFatal?(error: unknown): void;
  now(): number;
  setTimer(fn: () => void, ms: number): unknown;
  clearTimer(handle: unknown): void;
}

function allFinished(ops: readonly OperationView[]): boolean {
  return ops.length > 0 && ops.every((o) => o.status !== "running");
}

/**
 * Memantau operasi sampai selesai, timeout, atau `stop()` (unmount). Permintaan pertama langsung dikirim; sesudahnya
 * dengan `pollDelay`. Pengatur waktu disuntikkan sehingga dapat diuji dengan jam palsu.
 */
export function startPolling(deps: PollerDeps): { stop(): void } {
  const startedAt = deps.now();
  let stopped = false;
  let handle: unknown;
  let attempt = 0;

  const schedule = () => {
    if (stopped) return;
    const delay = pollDelay(attempt);
    attempt += 1;
    if (deps.now() + delay - startedAt > POLL_TIMEOUT_MS) {
      // Percobaan berikutnya melewati batas: tunggu sampai batas lalu laporkan timeout.
      handle = deps.setTimer(
        () => {
          if (stopped) return;
          stopped = true;
          deps.onTimeout();
        },
        Math.max(0, startedAt + POLL_TIMEOUT_MS - deps.now()),
      );
      return;
    }
    handle = deps.setTimer(() => void tick(), delay);
  };

  const tick = async () => {
    if (stopped) return;
    try {
      const ops = await deps.fetchStates();
      if (stopped) return;
      deps.onUpdate(ops);
      if (allFinished(ops)) {
        stopped = true;
        deps.onFinish(ops);
        return;
      }
    } catch (error) {
      if (stopped) return;
      if (deps.isPermanent?.(error)) {
        stopped = true;
        deps.onFatal?.(error);
        return;
      }
      deps.onError?.(error);
    }
    schedule();
  };

  void tick();
  return {
    stop() {
      stopped = true;
      if (handle !== undefined) deps.clearTimer(handle);
    },
  };
}

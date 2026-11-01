import type { ApiError, PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import { scopedKey, useTenantId } from "@erpora/platform-web";
import { type UseQueryResult, useQuery } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { useModulesClient } from "./client";
import { type PollerDeps, startPolling } from "./polling";
import type { EventView, ModuleOp, ModuleView, OperationView } from "./types";

type Client = TypedClient<PlatformPaths>;

/** Ukuran halaman daftar modul: modul terkompilasi jumlahnya kecil, jadi satu permintaan cukup (K-6). */
export const MODULES_PAGE_SIZE = 200;
export const HISTORY_PAGE_SIZE = 25;
// Pengaman perulangan paginasi bila server salah melaporkan `hasMore`.
const MAX_PAGES = 50;

/** Galat HTTP dari API modul; `apiError` dipetakan ke pesan katalog oleh `mapModuleError`. */
export class ModulesApiError extends Error {
  override readonly name = "ModulesApiError";
  readonly status: number;
  readonly apiError: ApiError;
  constructor(status: number, apiError: ApiError) {
    super(`${status} ${apiError.code}`);
    this.status = status;
    this.apiError = apiError;
  }
}

/** 401/403/404 tidak akan membaik dengan percobaan ulang (sesi habis, hak dicabut, operasi hilang). */
export function isPermanentApiError(error: unknown): boolean {
  return (
    error instanceof ModulesApiError &&
    (error.status === 401 || error.status === 403 || error.status === 404)
  );
}

const hasMoreOf = (page: unknown): boolean =>
  typeof page === "object" && page !== null && (page as { hasMore?: unknown }).hasMore === true;

/** Seluruh modul (urut topologis dari server) dengan mengikuti `hasMore`; `etag` = ETag halaman pertama. */
export async function fetchModules(
  client: Client,
): Promise<{ modules: ModuleView[]; etag: string | undefined }> {
  const modules: ModuleView[] = [];
  let etag: string | undefined;
  for (let page = 1; page <= MAX_PAGES; page++) {
    const res = await client.get("/api/platform/modules", {
      params: { query: { page, pageSize: MODULES_PAGE_SIZE } },
    });
    if (!res.ok) throw new ModulesApiError(res.status, res.error);
    if (page === 1) etag = res.etag;
    modules.push(...res.data);
    if (!hasMoreOf(res.page)) break;
  }
  return { modules, etag };
}

export async function fetchHistory(
  client: Client,
  key: string,
  page: number,
): Promise<{ events: EventView[]; hasMore: boolean }> {
  const res = await client.get("/api/platform/modules/{key}/history", {
    params: { path: { key }, query: { page, pageSize: HISTORY_PAGE_SIZE } },
  });
  if (!res.ok) throw new ModulesApiError(res.status, res.error);
  return { events: res.data, hasMore: hasMoreOf(res.page) };
}

export async function fetchOperation(client: Client, id: string): Promise<OperationView> {
  const res = await client.get("/api/platform/modules/operations/{id}", {
    params: { path: { id } },
  });
  if (!res.ok) throw new ModulesApiError(res.status, res.error);
  return res.data;
}

export interface RunArgs {
  key: string;
  op: ModuleOp;
  /** Hanya untuk install. */
  withDeps?: boolean;
  /** Hanya untuk install. */
  enable?: boolean;
  /** ETag daftar yang dipakai menghitung dampak (412 bila status berubah). */
  ifMatch?: string;
}

export type RunResult =
  | { kind: "done"; module: ModuleView; operations: readonly OperationView[] }
  | { kind: "accepted"; operationIds: readonly string[] };

/**
 * Menjalankan operasi lifecycle. Body selalu objek JSON (`{}` bila tidak ada opsi; mutasi platform menolak tanpa
 * `Content-Type: application/json`, 415). Hanya install membawa `withDeps`/`enable`. Tidak pernah mengirim
 * `tenantId`/`userId` (P11).
 */
export async function runModuleOperation(client: Client, args: RunArgs): Promise<RunResult> {
  const body: { withDeps?: boolean; enable?: boolean } = {};
  if (args.op === "install") {
    if (args.withDeps !== undefined) body.withDeps = args.withDeps;
    if (args.enable !== undefined) body.enable = args.enable;
  }
  const res = await client.post("/api/platform/modules/{key}/{operation}", {
    params: { path: { key: args.key, operation: args.op } },
    body,
    headers: args.ifMatch ? { "If-Match": args.ifMatch } : undefined,
  });
  if (!res.ok) throw new ModulesApiError(res.status, res.error);
  const data = res.data as {
    module?: ModuleView;
    operations?: OperationView[];
    operationId?: string;
    operationIds?: string[];
  };
  if (res.status === 202) {
    const ids = data.operationIds?.length
      ? data.operationIds
      : data.operationId
        ? [data.operationId]
        : [];
    return { kind: "accepted", operationIds: ids };
  }
  return {
    kind: "done",
    module: data.module as ModuleView,
    operations: data.operations ?? [],
  };
}

// ---- Hook ----

export function useModules(): UseQueryResult<{ modules: ModuleView[]; etag: string | undefined }> {
  const client = useModulesClient();
  const tenantId = useTenantId();
  return useQuery({
    queryKey: scopedKey(tenantId, "platform", "modules"),
    queryFn: () => fetchModules(client),
  });
}

export function useModuleHistory(key: string, page: number, enabled = true) {
  const client = useModulesClient();
  const tenantId = useTenantId();
  return useQuery({
    queryKey: scopedKey(tenantId, "platform", "modules", key, "history", page),
    queryFn: () => fetchHistory(client, key, page),
    enabled,
  });
}

/** Satu operasi (untuk langkah di riwayat); hanya diminta saat `enabled` (baris dibuka). */
export function useOperation(id: string, enabled: boolean) {
  const client = useModulesClient();
  const tenantId = useTenantId();
  return useQuery({
    queryKey: scopedKey(tenantId, "platform", "modules", "operations", id),
    queryFn: () => fetchOperation(client, id),
    enabled,
  });
}

export interface OperationPolling {
  operations: readonly OperationView[];
  phase: "idle" | "running" | "finished" | "timeout" | "stopped";
  /** Pembacaan status terakhir gagal (polling terus dicoba lagi). */
  pollError: boolean;
}

/**
 * Memantau operasi hasil `202`: polling 1 dtk mundur ke 5 dtk, berhenti saat semua selesai, unmount, atau 10 menit.
 * `onFinish` dipanggil sekali dengan hasil akhir.
 */
export function useOperationPolling(
  ids: readonly string[] | null,
  onFinish?: (ops: readonly OperationView[]) => void,
): OperationPolling {
  const client = useModulesClient();
  const [state, setState] = useState<OperationPolling>({
    operations: [],
    phase: "idle",
    pollError: false,
  });
  const finishRef = useRef(onFinish);
  finishRef.current = onFinish;
  const idsKey = ids ? ids.join(",") : "";

  useEffect(() => {
    if (!ids || ids.length === 0) {
      setState({ operations: [], phase: "idle", pollError: false });
      return;
    }
    setState({ operations: [], phase: "running", pollError: false });
    // Operasi yang membalas galat permanen (mis. 404) dikeluarkan sendiri-sendiri; satu operasi mati tidak boleh
    // menghentikan pemantauan operasi lain. Polling berhenti (fatal) hanya bila SEMUA operasi mati.
    const dead = new Set<string>();
    const deps: PollerDeps = {
      fetchStates: async () => {
        const live = ids.filter((id) => !dead.has(id));
        const results = await Promise.allSettled(live.map((id) => fetchOperation(client, id)));
        const operations: OperationView[] = [];
        let transient: unknown;
        let permanent: unknown;
        results.forEach((r, i) => {
          if (r.status === "fulfilled") {
            operations.push(r.value);
          } else if (isPermanentApiError(r.reason)) {
            dead.add(live[i] as string);
            permanent = r.reason;
          } else {
            transient = r.reason;
          }
        });
        if (transient !== undefined) throw transient;
        if (operations.length === 0 && permanent !== undefined) throw permanent;
        return operations;
      },
      onUpdate: (operations) => setState((s) => ({ ...s, operations, pollError: false })),
      onFinish: (operations) => {
        setState({ operations, phase: "finished", pollError: false });
        finishRef.current?.(operations);
      },
      onTimeout: () => setState((s) => ({ ...s, phase: "timeout" })),
      onError: () => setState((s) => ({ ...s, pollError: true })),
      isPermanent: isPermanentApiError,
      onFatal: () => setState((s) => ({ ...s, phase: "stopped", pollError: false })),
      now: () => Date.now(),
      setTimer: (fn, ms) => setTimeout(fn, ms),
      clearTimer: (h) => clearTimeout(h as ReturnType<typeof setTimeout>),
    };
    const poller = startPolling(deps);
    return () => poller.stop();
  }, [idsKey, client]);

  return state;
}

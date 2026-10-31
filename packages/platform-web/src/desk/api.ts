import type { ApiError, ApiResult, PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import { type UseQueryResult, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef } from "react";
import { useBootstrap } from "../bootstrap";
import { safeInternalHref } from "../sources/href";
import { scopedKey } from "../tenancy";
import { isValidDeskId } from "./storage";
import type { Desk, DeskList, DeskMenuGroup, DeskMenuItem, MessageRef } from "./types";

/**
 * CATATAN: `GET /api/platform/desks` belum ada di `platform.gen.ts` (kontrak dibuat agent backend). Sampai tipe
 * terbangkit tersedia, klien dipersempit lewat `DesksClient`/`toDesksClient` dan bentuk respons divalidasi saat
 * runtime (`parseDeskList`). Setelah `contracts:generate`: hapus `DesksClient`/`toDesksClient`, ketik fetcher dengan
 * `TypedClient<PlatformPaths>` langsung, dan ganti `types.ts` dengan `PlatformComponents["schemas"]["Desk"]` dst.
 */
export const DESKS_PATH = "/api/platform/desks";

/** Bagian klien yang dipakai fetcher Desk. */
export interface DesksClient {
  get(
    path: typeof DESKS_PATH,
    init?: { headers?: Record<string, string> },
  ): Promise<ApiResult<unknown>>;
}

/** Mempersempit klien bertipe ke `DesksClient` (sementara; lihat catatan di atas). */
export function toDesksClient(client: TypedClient<PlatformPaths>): DesksClient {
  return client as unknown as DesksClient;
}

/** Dilempar fetcher Desk bila server membalas galat, respons rusak, atau 304 tanpa data sebelumnya. */
export class DesksError extends Error {
  override readonly name = "DesksError";
  readonly status: number;
  readonly error: ApiError;
  constructor(status: number, error: ApiError) {
    super(`daftar Desk gagal (${status}): ${error.code}`);
    this.status = status;
    this.error = error;
  }
}

const invalid = (message: string): DesksError =>
  new DesksError(200, { code: "invalid_response", message });

const isRecord = (v: unknown): v is Record<string, unknown> =>
  typeof v === "object" && v !== null && !Array.isArray(v);

function parseLabel(v: unknown): MessageRef {
  if (!isRecord(v) || typeof v.ns !== "string" || typeof v.key !== "string") {
    throw invalid("label Desk tidak valid");
  }
  return { ns: v.ns, key: v.key };
}

/** `null` bila `to` bukan path internal aman (open redirect/skema) sehingga item dibuang. */
function parseItem(v: unknown): DeskMenuItem | null {
  if (!isRecord(v) || v.type !== "item") throw invalid("item menu tidak valid");
  if (typeof v.id !== "string" || typeof v.ownerDesk !== "string" || typeof v.to !== "string") {
    throw invalid("item menu tidak valid");
  }
  const to = safeInternalHref(v.to);
  if (to === null) return null;
  const item: DeskMenuItem = {
    type: "item",
    id: v.id,
    label: parseLabel(v.label),
    to,
    ownerDesk: v.ownerDesk,
  };
  if (typeof v.icon === "string") item.icon = v.icon;
  if (v.locked === true) item.locked = true;
  return item;
}

function parseMenu(v: unknown): DeskMenuItem | DeskMenuGroup | null {
  if (isRecord(v) && v.type === "group") {
    if (typeof v.id !== "string" || !Array.isArray(v.children)) {
      throw invalid("grup menu tidak valid");
    }
    const children = v.children.flatMap((c) => parseItem(c) ?? []);
    if (children.length === 0) return null;
    const group: DeskMenuGroup = { type: "group", id: v.id, label: parseLabel(v.label), children };
    if (typeof v.icon === "string") group.icon = v.icon;
    return group;
  }
  return parseItem(v);
}

function parseDesk(v: unknown): Desk {
  if (!isRecord(v) || !isValidDeskId(v.id) || !Array.isArray(v.menus)) {
    throw invalid("Desk tidak valid");
  }
  if (v.source !== "module" && v.source !== "platform" && v.source !== "custom") {
    throw invalid("sumber Desk tidak valid");
  }
  const desk: Desk = {
    id: v.id,
    source: v.source,
    label: parseLabel(v.label),
    menus: v.menus.flatMap((m) => parseMenu(m) ?? []),
  };
  if (typeof v.moduleKey === "string") desk.moduleKey = v.moduleKey;
  if (typeof v.icon === "string") desk.icon = v.icon;
  return desk;
}

/**
 * Memvalidasi dan menormalkan respons server (tidak dipercaya buta): bentuk salah melempar `DesksError`; item
 * dengan `to` yang bukan path internal aman dibuang (`safeInternalHref`), grup yang menjadi kosong ikut dibuang.
 */
export function parseDeskList(data: unknown): DeskList {
  if (!isRecord(data) || !Array.isArray(data.desks) || typeof data.revision !== "string") {
    throw invalid("respons daftar Desk tidak valid");
  }
  return { desks: data.desks.map(parseDesk), revision: data.revision };
}

/**
 * Fetcher ber-`ETag`: menyimpan `{ etag, data }` terakhir, mengirim `If-None-Match`, dan pada `304` mengembalikan
 * **referensi objek yang sama** (tanpa render ulang). Galat melempar `DesksError`.
 */
export function createDesksFetcher(client: DesksClient): () => Promise<DeskList> {
  let last: { etag: string | undefined; data: DeskList } | undefined;
  return async () => {
    const res = await client.get(DESKS_PATH, {
      headers: last?.etag ? { "If-None-Match": last.etag } : undefined,
    });
    if (res.ok) {
      const data = parseDeskList(res.data);
      last = { etag: res.etag, data };
      return data;
    }
    if (res.status === 304 && last) return last.data;
    throw new DesksError(res.status, res.error);
  };
}

/**
 * Memuat daftar Desk yang diizinkan server (TanStack Query, ber-`ETag`, refetch saat focus). Key `[tenantId, "desks"]`;
 * hanya di memori (tanpa persister): menu adalah data per pengguna.
 */
export function useDesksQuery(client: TypedClient<PlatformPaths>): UseQueryResult<DeskList> {
  const { tenantId, revision } = useBootstrap();
  const queryClient = useQueryClient();
  const fetcher = useMemo(() => createDesksFetcher(toDesksClient(client)), [client]);
  // Revisi bootstrap berubah = modul diaktifkan/dinonaktifkan/diperbarui: daftar Desk (dan menu) ikut dimuat ulang
  // tanpa muat ulang halaman (ber-ETag, sehingga murah bila tidak berubah).
  const seen = useRef(revision);
  useEffect(() => {
    if (seen.current === revision) return;
    seen.current = revision;
    void queryClient.invalidateQueries({ queryKey: scopedKey(tenantId, "desks") });
  }, [revision, tenantId, queryClient]);
  return useQuery({
    queryKey: scopedKey(tenantId, "desks"),
    queryFn: fetcher,
    staleTime: 30_000,
    refetchOnWindowFocus: true,
  });
}

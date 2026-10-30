import { type ApiError, createClient } from "@erpora/platform-contracts";
import { getActiveLocale } from "@erpora/platform-web";
import type { components, paths } from "../api/contract.gen";

export type Note = components["schemas"]["Note"];

/** Klien modul: kontrak terketik + `Accept-Language` dari bahasa aktif (tidak pernah mengirim tenantId/userId). */
const client = createClient<paths>({ getLocale: getActiveLocale });

/** Galat permintaan API yang dibawa ke TanStack Query; `apiError` dipetakan ke pesan katalog oleh halaman. */
export class NotesRequestError extends Error {
  override readonly name = "NotesRequestError";
  readonly apiError: ApiError;
  constructor(apiError: ApiError) {
    super(apiError.code);
    this.apiError = apiError;
  }
}

const PAGE_SIZE = 100;
// Pengaman: daftar contoh tidak pernah sebesar ini; mencegah perulangan bila server salah melaporkan hasMore.
const MAX_PAGES = 50;

function hasMore(page: unknown): boolean {
  return (
    typeof page === "object" && page !== null && (page as { hasMore?: unknown }).hasMore === true
  );
}

/** Mengambil seluruh notes tenant aktif dengan mengikuti paginasi (`hasMore`). */
export async function fetchAllNotes(): Promise<Note[]> {
  const notes: Note[] = [];
  for (let page = 1; page <= MAX_PAGES; page++) {
    const res = await client.get("/api/example/notes", {
      params: { query: { page, pageSize: PAGE_SIZE } },
    });
    if (!res.ok) throw new NotesRequestError(res.error);
    notes.push(...res.data);
    if (!hasMore(res.page)) break;
  }
  return notes;
}

/** Galat apa pun (termasuk jaringan putus) menjadi `ApiError` agar dapat dipetakan ke pesan katalog. */
export function toApiError(error: unknown): ApiError {
  return error instanceof NotesRequestError
    ? error.apiError
    : { code: "internal_error", message: "" };
}

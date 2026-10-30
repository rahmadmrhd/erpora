import { type DbApi, scopedKey, useDb, useTenantId } from "@erpora/platform-web";
import { type QueryClient, useQueryClient } from "@tanstack/react-query";
import { fetchAllNotes, type Note } from "../notes-api";

/**
 * Koleksi TanStack DB `example/notes` di atas TanStack Query (M3a K-21, R7.4). `db` berasal dari `loadDb()`/`useDb()`
 * (chunk lazy); modul tidak pernah mengimpor `@tanstack/react-db` secara statis. Query key selalu berawalan tenant
 * (`scopedKey`), dan koleksi hanya di memori (persistence menyusul di PRD offline).
 */
export function createNotesCollection(
  db: DbApi,
  { queryClient, tenantId }: { queryClient: QueryClient; tenantId: string },
) {
  return db.createCollection(
    db.queryCollectionOptions({
      queryKey: scopedKey(tenantId, "example", "notes"),
      queryFn: fetchAllNotes,
      queryClient,
      getKey: (note: Note) => note.id,
    }),
  );
}

export type NotesCollection = ReturnType<typeof createNotesCollection>;

// Satu koleksi per (queryClient, tenant): dibuat ulang tiap render akan menggandakan observer dan sinkronisasi.
const collections = new WeakMap<QueryClient, Map<string, NotesCollection>>();

/**
 * Koleksi notes tenant aktif. Menangguhkan (Suspense) sampai chunk TanStack DB termuat; hasilnya di-cache per
 * (queryClient, tenant) sehingga render ulang dan komponen lain memakai koleksi yang sama.
 */
export function useNotesCollection(): NotesCollection {
  const db = useDb();
  const queryClient = useQueryClient();
  const tenantId = useTenantId();
  let perClient = collections.get(queryClient);
  if (!perClient) {
    perClient = new Map();
    collections.set(queryClient, perClient);
  }
  let collection = perClient.get(tenantId);
  if (!collection) {
    collection = createNotesCollection(db, { queryClient, tenantId });
    perClient.set(tenantId, collection);
  }
  return collection;
}

import { use } from "react";

/**
 * TanStack DB (M3a K-21): menambah ±100 KB gzip, sehingga TIDAK boleh ada di bundle awal. Satu-satunya jalan masuk
 * adalah `import()` dinamis di sini; halaman yang memakainya (mis. `/example`) memuatnya lewat `useDb()` (Suspense).
 * Tidak ada impor statis `@tanstack/react-db` / `@tanstack/query-db-collection` di mana pun (diuji pemindai).
 * Tanpa persistence IndexedDB (menyusul di PRD offline): koleksi hanya di memori.
 */
export interface DbApi {
  createCollection: typeof import("@tanstack/react-db")["createCollection"];
  useLiveQuery: typeof import("@tanstack/react-db")["useLiveQuery"];
  queryCollectionOptions: typeof import("@tanstack/query-db-collection")["queryCollectionOptions"];
}

export interface DbImporters {
  db: () => Promise<typeof import("@tanstack/react-db")>;
  query: () => Promise<typeof import("@tanstack/query-db-collection")>;
}

const defaultImporters: DbImporters = {
  db: () => import("@tanstack/react-db"),
  query: () => import("@tanstack/query-db-collection"),
};

/**
 * Pemuat DB ber-cache: modul hanya diminta sekali (promise yang sama dikembalikan); bila pemuatan gagal
 * (jaringan putus) cache dibuang sehingga pemanggilan berikutnya mencoba lagi.
 */
export function createDbLoader(importers: DbImporters = defaultImporters): () => Promise<DbApi> {
  let pending: Promise<DbApi> | undefined;
  return () => {
    pending ??= Promise.all([importers.db(), importers.query()]).then(
      ([db, query]) => ({
        createCollection: db.createCollection,
        useLiveQuery: db.useLiveQuery,
        queryCollectionOptions: query.queryCollectionOptions,
      }),
      (err: unknown) => {
        pending = undefined;
        throw err;
      },
    );
    return pending;
  };
}

/** Memuat TanStack DB (chunk terpisah) sekali. */
export const loadDb: () => Promise<DbApi> = createDbLoader();

/** API DB untuk komponen; menangguhkan (Suspense) sampai chunk DB termuat. Hasil promise di-cache. */
export function useDb(): DbApi {
  return use(loadDb());
}

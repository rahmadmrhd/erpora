import type { OfflinePolicy } from "./offline";
import type { AnyModuleChildRoute, ModuleBaseRoute } from "./routing";

/**
 * Rujukan pesan terjemahan: namespace (slug modul, `ui`, atau `platform`) dan kunci. Dibuat oleh `msg()` di
 * `@erpora/platform-web` (kunci terketik) dan dirender oleh `<Message>`; SDK ini tidak bergantung pada runtime i18n.
 */
export interface MessageRef {
  readonly ns: string;
  readonly key: string;
}

/** Satu tingkat breadcrumb: pesan terjemahan (teks UI) atau teks data (mis. nama record; bukan teks UI). */
export type Crumb = { readonly ref: MessageRef } | { readonly text: string };

/**
 * Modul frontend. Menu modul TIDAK dideklarasikan di sini: berasal dari server (manifest `desk`, GET
 * /api/platform/desks, M3a revisi R10.8/R11); `registerRoutes` hanya mendaftarkan halaman anak `/<slug>/<halaman>`,
 * tidak boleh rute indeks (indeks `/<slug>` adalah Dashboard bawaan shell).
 */
export interface FrontendModule<S extends string = string> {
  /** = `manifest.key`, mis. "acme.example". */
  readonly key: string;
  /** = `manifest.slug`; basePath "/" + slug; namespace i18n = slug. */
  readonly slug: S;
  /** = `manifest.version`. */
  readonly version: string;
  /** Opsional: crumb dasar modul. */
  readonly title?: MessageRef;
  /** Menerima base route modul (sudah berguard) dan mengembalikan anak langsungnya. */
  readonly registerRoutes: (base: ModuleBaseRoute) => readonly AnyModuleChildRoute[];
  readonly offline: OfflinePolicy;
}

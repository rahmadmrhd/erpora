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

/** Item menu langsung. `to` relatif terhadap basePath modul ("/" menjadi "/<slug>", "/notes" menjadi "/<slug>/notes"). */
export interface NavigationItem {
  readonly type: "item";
  readonly id: string;
  readonly label: MessageRef;
  readonly to: string;
  /** Nama ikon di registry `@erpora/ui` (kebab-case); nama tak dikenal tampil sebagai lingkaran berinisial. */
  readonly icon?: string;
}

/** Grup menu yang dapat dilipat; hanya satu tingkat (anak harus item). */
export interface NavigationGroup {
  readonly type: "group";
  readonly id: string;
  readonly label: MessageRef;
  readonly icon?: string;
  readonly children: readonly NavigationItem[];
}

export type NavigationEntry = NavigationItem | NavigationGroup;

export interface FrontendModule<S extends string = string> {
  /** = `manifest.key`, mis. "acme.example". */
  readonly key: string;
  /** = `manifest.slug`; basePath "/" + slug; namespace i18n = slug. */
  readonly slug: S;
  /** = `manifest.version`. */
  readonly version: string;
  /** Opsional: crumb dasar modul. */
  readonly title?: MessageRef;
  readonly navigation: readonly NavigationEntry[];
  /** Menerima base route modul (sudah berguard) dan mengembalikan anak langsungnya. */
  readonly registerRoutes: (base: ModuleBaseRoute) => readonly AnyModuleChildRoute[];
  readonly offline: OfflinePolicy;
}

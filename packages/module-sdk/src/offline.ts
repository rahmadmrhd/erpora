/**
 * Deskriptor kebijakan offline sisi UI. Bentuknya persis `docs/architecture/offline-strategy.md` §3; sisi backend
 * (`manifest.offline`) adalah otoritas, sisi ini hanya untuk UX.
 */
export type OfflineMode = "online-only" | "offline-read" | "offline-draft" | "offline-write";
export type AssetStrategy = "on-demand" | "precache";
export type ConflictStrategy = "server-wins" | "reject" | "custom";

export interface OfflinePolicy {
  readonly mode: OfflineMode;
  readonly assets: { readonly strategy: AssetStrategy };
  readonly data: {
    /** Resource yang boleh disalin ke perangkat. */
    readonly collections: readonly string[];
    /** Umur maksimum salinan sebelum wajib diperbarui. */
    readonly maxAgeSeconds: number;
    /** Batas jumlah record per collection. */
    readonly maxRecords?: number;
  };
  readonly sync: {
    readonly enabled: boolean;
    readonly conflict: ConflictStrategy;
  };
}

/** Kebijakan bawaan: hanya online, tanpa salinan data dan tanpa sinkronisasi. */
export const defaultOfflinePolicy: OfflinePolicy = Object.freeze({
  mode: "online-only",
  assets: Object.freeze({ strategy: "on-demand" }),
  data: Object.freeze({ collections: Object.freeze([] as string[]), maxAgeSeconds: 0 }),
  sync: Object.freeze({ enabled: false, conflict: "server-wins" }),
});

export const OFFLINE_MODES: readonly OfflineMode[] = [
  "online-only",
  "offline-read",
  "offline-draft",
  "offline-write",
];
export const ASSET_STRATEGIES: readonly AssetStrategy[] = ["on-demand", "precache"];
export const CONFLICT_STRATEGIES: readonly ConflictStrategy[] = ["server-wins", "reject", "custom"];

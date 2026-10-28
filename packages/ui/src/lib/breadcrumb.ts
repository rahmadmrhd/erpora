/** Satu slot pada jejak halaman: item (dengan penanda halaman saat ini) atau elipsis penampung item terlipat. */
export type CrumbSlot<T> = { kind: "crumb"; crumb: T; current: boolean } | { kind: "ellipsis" };

export interface CollapsedCrumbs<T> {
  /** Slot yang digambar, paling banyak tiga. */
  visible: CrumbSlot<T>[];
  /** Item yang disembunyikan di balik elipsis (urutan asli); kosong bila tidak ada elipsis. */
  folded: T[];
}

/**
 * Pelipatan jejak halaman yang deterministik (M3a K-25, P4), tanpa pengukuran lebar:
 * n <= 3 dan tidak compact: semua terlihat; n > 3: [pertama, elipsis, terakhir];
 * compact (layar kecil): [elipsis, terakhir]. Item terakhir selalu halaman saat ini dan tidak pernah terlipat.
 */
export function collapseCrumbs<T>(
  crumbs: readonly T[],
  { compact }: { compact: boolean },
): CollapsedCrumbs<T> {
  const n = crumbs.length;
  if (n === 0) return { visible: [], folded: [] };
  const lastCrumb = crumbs[n - 1] as T;
  const last: CrumbSlot<T> = { kind: "crumb", crumb: lastCrumb, current: true };
  const item = (crumb: T): CrumbSlot<T> => ({ kind: "crumb", crumb, current: false });

  if (n === 1) return { visible: [last], folded: [] };
  if (compact) return { visible: [{ kind: "ellipsis" }, last], folded: crumbs.slice(0, n - 1) };
  if (n <= 3) return { visible: [...crumbs.slice(0, n - 1).map(item), last], folded: [] };
  return {
    visible: [item(crumbs[0] as T), { kind: "ellipsis" }, last],
    folded: crumbs.slice(1, n - 1),
  };
}

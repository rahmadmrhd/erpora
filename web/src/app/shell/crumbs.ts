import type { ReactNode } from "react";

/** Satu tingkat jejak halaman yang sudah siap dirender; `to` hanya ada bila tingkat itu punya halaman sendiri. */
export interface ResolvedCrumb {
  key: string;
  label: ReactNode;
  /** Teks tampil bila sudah diketahui (crumb berupa teks data); crumb berpesan memakai teks hasil probe. */
  text?: string;
  to?: string;
}

/**
 * Membuang crumb yang teksnya sama dengan crumb sesudahnya (mis. crumb dasar modul "Catatan" dan halaman "Catatan"):
 * yang lebih dalam (halaman) dipertahankan. `texts` memuat teks terjemahan crumb berpesan menurut `key`; teks yang belum
 * diketahui tidak dianggap sama.
 */
export function dedupeCrumbs(
  crumbs: readonly ResolvedCrumb[],
  texts: Readonly<Record<string, string>>,
): ResolvedCrumb[] {
  const textOf = (c: ResolvedCrumb): string | undefined => c.text ?? texts[c.key];
  return crumbs.filter((c, i) => {
    const next = crumbs[i + 1];
    if (next === undefined) return true;
    const a = textOf(c);
    return a === undefined || a !== textOf(next);
  });
}

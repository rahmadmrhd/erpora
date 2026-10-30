import type { NavNode, NavNodeItem } from "./registry";

const trim = (p: string): string => (p.length > 1 && p.endsWith("/") ? p.slice(0, -1) : p);

/** `to` aktif bila `pathname` sama persis atau berada di bawahnya (batas segmen); "/" hanya aktif persis. */
export function isPathActive(pathname: string, to: string): boolean {
  const p = trim(pathname);
  const t = trim(to);
  if (t === "/") return p === "/";
  return p === t || p.startsWith(`${t}/`);
}

const leaves = (nodes: readonly NavNode[]): NavNodeItem[] =>
  nodes.flatMap((n) => (n.type === "item" ? [n] : [...n.children]));

/**
 * Item daun yang aktif untuk `pathname`: yang `to`-nya paling panjang di antara yang cocok, sehingga paling banyak
 * satu item aktif (P3) walau `/example` dan `/example/notes` sama-sama berawalan. (Id item hanya unik per modul,
 * sehingga hasilnya adalah node itu sendiri.)
 */
export function activeLeaf(nodes: readonly NavNode[], pathname: string): NavNodeItem | undefined {
  let best: NavNodeItem | undefined;
  for (const leaf of leaves(nodes)) {
    if (!isPathActive(pathname, leaf.to)) continue;
    if (best === undefined || trim(leaf.to).length > trim(best.to).length) best = leaf;
  }
  return best;
}

/** Grup terbuka bila dan hanya bila memuat anak yang aktif di antara `nodes` (bawaan: grup itu sendiri). */
export function groupHasActive(
  group: Extract<NavNode, { type: "group" }>,
  pathname: string,
  nodes: readonly NavNode[] = [group],
): boolean {
  const active = activeLeaf(nodes, pathname);
  return active !== undefined && group.children.includes(active);
}

import type { Desk, DeskMenuItem } from "./types";

const trim = (p: string): string => (p.length > 1 && p.endsWith("/") ? p.slice(0, -1) : p);

function leaves(desk: Desk): DeskMenuItem[] {
  return desk.menus.flatMap((m) => (m.type === "item" ? [m] : m.children));
}

/**
 * Pencocokan segmen-aman: item terkunci (Dashboard `/<slug>`) hanya cocok persis; item lain cocok bila
 * `path === to` atau `path` diawali `to + "/"` (sehingga `/example2` tidak cocok dengan `/example`).
 */
function itemMatches(item: DeskMenuItem, path: string): boolean {
  const to = trim(item.to);
  if (item.locked === true) return path === to;
  return path === to || path.startsWith(`${to}/`);
}

/** Apakah `desk` memuat halaman `path` (ada item yang cocok). */
export function deskContains(desk: Desk, path: string): boolean {
  const p = trim(path);
  return leaves(desk).some((item) => itemMatches(item, p));
}

/**
 * Mencocokkan `path` dengan menu semua Desk. `owner` = `ownerDesk` item yang cocok dengan `to` terpanjang (seri:
 * yang pertama di `desks`); `containing` = id semua Desk yang memuat halaman, berurutan seperti `desks`.
 */
export function matchMenu(
  path: string,
  desks: readonly Desk[],
): { owner: string | null; containing: string[] } {
  const p = trim(path);
  const containing: string[] = [];
  let best: DeskMenuItem | undefined;
  for (const desk of desks) {
    let contains = false;
    for (const item of leaves(desk)) {
      if (!itemMatches(item, p)) continue;
      contains = true;
      if (best === undefined || trim(item.to).length > trim(best.to).length) best = item;
    }
    if (contains) containing.push(desk.id);
  }
  return { owner: best?.ownerDesk ?? null, containing };
}

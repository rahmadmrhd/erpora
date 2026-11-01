import type { Desk, DeskMenuGroup, DeskMenuItem } from "./types";

/** Item menu contoh untuk test dan fixture; `ownerDesk` bawaan = `deskId`. */
export function makeDeskItem(
  deskId: string,
  id: string,
  to: string,
  overrides: Partial<DeskMenuItem> = {},
): DeskMenuItem {
  return {
    type: "item",
    id,
    label: { ns: deskId, key: `nav.${id}` },
    to,
    ownerDesk: deskId,
    ...overrides,
  };
}

/** Dashboard terkunci `/<slug>` sebagai item pertama (bentuk yang dikirim server). */
export function makeDashboardItem(deskId: string, slug: string): DeskMenuItem {
  return makeDeskItem(deskId, "dashboard", `/${slug}`, { locked: true });
}

export function makeDeskGroup(deskId: string, id: string, children: DeskMenuItem[]): DeskMenuGroup {
  return { type: "group", id, label: { ns: deskId, key: `nav.${id}` }, children };
}

/** Desk contoh; `menus` bawaan kosong. */
export function makeDesk(
  id: string,
  menus: Desk["menus"] = [],
  overrides: Partial<Desk> = {},
): Desk {
  return { id, source: "module", label: { ns: id, key: "desk.title" }, menus, ...overrides };
}

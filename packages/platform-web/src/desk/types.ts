/**
 * Tipe Desk di klien (design M3a "Komponen 2"). Sementara didefinisikan lokal karena `platform.gen.ts` belum memuat
 * `GET /api/platform/desks`; ganti ke tipe terbangkit setelah `contracts:generate` (lihat `api.ts`).
 */

/** Rujukan kunci i18n (namespace + kunci); teks tidak pernah datang dari server. */
export type MessageRef = { ns: string; key: string };

export type DeskMenuItem = {
  type: "item";
  id: string;
  label: MessageRef;
  /** Path internal absolut (mis. `/example/notes`); Dashboard berupa `/<slug>`. */
  to: string;
  icon?: string;
  /** Item terkunci (Dashboard): hanya cocok persis dengan `to`, tidak dapat disembunyikan. */
  locked?: boolean;
  /** Id Desk pemilik utama menu (tahap 4 penentuan Desk). */
  ownerDesk: string;
};

export type DeskMenuGroup = {
  type: "group";
  id: string;
  label: MessageRef;
  icon?: string;
  children: DeskMenuItem[];
};

export type Desk = {
  id: string;
  source: "module" | "platform" | "custom";
  moduleKey?: string;
  label: MessageRef;
  icon?: string;
  menus: (DeskMenuItem | DeskMenuGroup)[];
};

export type DeskList = { desks: Desk[]; revision: string };

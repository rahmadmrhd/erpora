// Pemeriksaan tipe saja (dijalankan lewat `typecheck`; M3a 8.3): anak route modul hanya valid bila parent-nya
// `ModuleBaseRoute`. Tidak diekspor dan tidak dijalankan.
import { type AnyRoute, createRoute } from "@tanstack/react-router";
import {
  type AnyModuleChildRoute,
  createAppRootRoute,
  createModuleBaseRoute,
  type ModuleBaseRoute,
} from "./routing";

const root = createAppRootRoute();
const base: ModuleBaseRoute = createModuleBaseRoute(root, "example", {});
const otherBase: ModuleBaseRoute = createModuleBaseRoute(root, "lain", {});

// Valid: parent adalah base route modul.
const notes = createRoute({
  getParentRoute: () => base,
  path: "/notes",
  staticData: { crumb: { text: "Catatan" } },
});
export const okChildren: readonly AnyModuleChildRoute[] = [notes];

// Valid: halaman bersarang di bawah anak (parent-nya anak modul, bukan base langsung) tidak masuk daftar
// `registerRoutes`; yang dikembalikan hanya anak langsung base.
const detail = createRoute({ getParentRoute: () => notes, path: "$id" });
export const nested = notes.addChildren([detail]);

const underRoot = createRoute({ getParentRoute: () => root, path: "/x" });
// @ts-expect-error parent bukan base route modul (root aplikasi)
export const wrongParentRoot: readonly AnyModuleChildRoute[] = [underRoot];

// @ts-expect-error parent adalah route lain yang bukan base route modul
export const wrongParentChild: readonly AnyModuleChildRoute[] = [detail];

// `crumb` pada staticData memakai tipe `Crumb` (ref MessageRef atau teks data).
createRoute({
  getParentRoute: () => base,
  path: "/ok",
  staticData: { crumb: [{ ref: { ns: "example", key: "page.ok" } }, { text: "Catatan 1" }] },
});
// @ts-expect-error crumb harus Crumb, bukan string polos
createRoute({ getParentRoute: () => base, path: "/bad", staticData: { crumb: "polos" } });

export type Checked = [typeof otherBase];

// Parent `AnyRoute` (tak berlabel) juga tidak boleh lolos sebagai anak base modul.
declare const anyParent: AnyRoute;
const underAny = createRoute({ getParentRoute: () => anyParent, path: "/y" });
// @ts-expect-error parent AnyRoute tak berlabel
export const wrongParentAny: readonly AnyModuleChildRoute[] = [underAny];

import type { ModuleManifest } from "./manifest.gen";
import { listModuleRoutePaths } from "./routing";
import type { FrontendModule } from "./types";

/**
 * Pembantu pengujian modul (M3a revisi 7.4): memastikan setiap `to` menu Desk di manifest (relatif base route,
 * mis. `/notes`) punya rute terdaftar di `registerRoutes` modul (termasuk rute bersarang), dan tidak ada `to`
 * indeks (`/`: Dashboard dibangkitkan platform). Melempar `Error` yang menyebut semua `to` yang bermasalah.
 */
export function assertDeskRoutes(module: FrontendModule, desk: ModuleManifest["desk"]): void {
  if (desk === undefined) {
    throw new Error(`modul ${module.key}: manifest tidak punya properti desk`);
  }
  const registered = new Set(listModuleRoutePaths(module));

  const missing: string[] = [];
  const indexes: string[] = [];
  const check = (to: string) => {
    if (to === "/" || to === "") indexes.push(to);
    else if (!registered.has(to.length > 1 && to.endsWith("/") ? to.slice(0, -1) : to)) {
      missing.push(to);
    }
  };
  for (const menu of desk.menus) {
    if ("children" in menu) for (const child of menu.children) check(child.to);
    else check(menu.to);
  }
  const problems: string[] = [];
  if (indexes.length > 0) {
    problems.push(
      "to indeks tidak diizinkan (Dashboard dibangkitkan platform, bukan menu): " +
        indexes.join(", "),
    );
  }
  if (missing.length > 0) {
    problems.push(
      `to tanpa rute terdaftar: ${missing.join(", ")} (terdaftar: ${[...registered].sort().join(", ") || "tidak ada"})`,
    );
  }
  if (problems.length > 0) {
    throw new Error(`modul ${module.key}: ${problems.join("; ")}`);
  }
}

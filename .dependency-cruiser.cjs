/**
 * Aturan batas frontend (PRD-000 §4.2 / Q21). Dijalankan dari root repo: `depcruise web/src --config .dependency-cruiser.cjs`.
 * Struktur target: web/ = app shell; packages/{ui,platform-web,module-sdk,platform-contracts};
 * modules/<name>/web = frontend modul. Folder modul/packages belum ada di M1; aturan tetap valid dan aktif.
 */
const SHARED = "^packages/(ui|platform-web|module-sdk)/";
const MODULE_WEB = "^modules/[^/]+/web/";

/** @type {import('dependency-cruiser').IConfiguration} */
module.exports = {
  forbidden: [
    {
      name: "F1-shared-must-not-depend-on-modules",
      comment: "@erp/ui, @erp/platform-web, @erp/module-sdk tidak boleh bergantung pada @erp/module-*.",
      severity: "error",
      from: { path: SHARED },
      to: { path: [MODULE_WEB, "node_modules/@erp/module-"] },
    },
    {
      name: "F2-module-only-uses-allowed-packages",
      comment:
        "Paket web modul hanya boleh mengimpor @erp/ui, @erp/platform-web, @erp/module-sdk, @erp/platform-contracts " +
        "dan modul lain yang ada di dependencies manifest (divalidasi terpisah), tidak pernah app shell.",
      severity: "error",
      from: { path: MODULE_WEB },
      to: { path: "^web/src/" },
    },
    {
      name: "F2-no-deep-import-between-modules",
      comment: "Antar-modul hanya lewat entry paket (modules/<x>/web/src/index.ts), bukan deep import.",
      severity: "error",
      from: { path: "^modules/([^/]+)/web/" },
      to: {
        path: MODULE_WEB,
        pathNot: ["^modules/$1/web/", "^modules/[^/]+/web/(src/)?index\\.(ts|tsx)$"],
      },
    },
    {
      name: "F2-no-deep-import-into-packages",
      comment: "Impor ke packages/* hanya lewat entry paket.",
      severity: "error",
      from: { path: "^(modules/[^/]+/web|web/src)/" },
      to: {
        path: "^packages/[^/]+/",
        pathNot: "^packages/[^/]+/(src/)?index\\.(ts|tsx)$",
      },
    },
    {
      name: "F3-only-shell-aggregates-modules",
      comment: "Hanya web/ (app shell) yang boleh bergantung pada banyak @erp/module-*.",
      severity: "error",
      from: { path: "^packages/" , pathNot: "^packages/(module-sdk|platform-contracts)/node_modules/" },
      to: { path: MODULE_WEB },
    },
  ],
  options: {
    doNotFollow: { path: "node_modules" },
    tsPreCompilationDeps: true,
    exclude: { path: ["(^|/)dist/", "(^|/)node_modules/"] },
  },
};

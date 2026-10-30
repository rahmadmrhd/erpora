// Dibangkitkan oleh `erpora i18n:generate` dari packages/ui/i18n/id.json. JANGAN DIEDIT.
// Paket ui adalah daun (F4) dan tidak memperluas I18nCatalogs; augmentasi namespace "ui" ada di
// packages/platform-web/src/i18n/keys.platform.gen.ts.

export interface UiCatalog {
  "actions.add": {};
  "actions.back": {};
  "actions.cancel": {};
  "actions.close": {};
  "actions.confirm": {};
  "actions.delete": {};
  "actions.edit": {};
  "actions.next": {};
  "actions.previous": {};
  "actions.retry": {};
  "actions.save": {};
  "actions.search": {};
  "breadcrumb.label": {};
  "breadcrumb.more": {};
  "pagination.page": { page: string | number; pages: string | number };
  "pagination.summary": { total: number };
  "sidebar.description": {};
  "sidebar.label": {};
  "sidebar.title": {};
  "sidebar.toggle": {};
  "status.empty": {};
  "status.loading": {};
  "toast.close": {};
  "toast.region": {};
}

/** Kunci lengkap namespace ui. */
export type UiKey = keyof UiCatalog;

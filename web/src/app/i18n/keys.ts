// Kunci terketik namespace `platform` dan `ui`. Ditulis tangan sampai generator `i18n:generate` (keys.gen.ts) tersedia;
// `keys.test.ts` menjaga kunci di sini sama persis dengan katalog JSON (id dan en). Bentuk nilai = bentuk argumen pesan.
import type {} from "@erpora/platform-web";

type NoArgs = Record<never, never>;

declare module "@erpora/platform-web" {
  interface I18nCatalogs {
    platform: {
      "app.name": NoArgs;
      "shell.loading": NoArgs;
      "shell.retry": NoArgs;
      "shell.skipToContent": NoArgs;
      "shell.language.label": NoArgs;
      "nav.home": NoArgs;
      "nav.menu": NoArgs;
      "nav.settings": NoArgs;
      "nav.settingsModules": NoArgs;
      "nav.empty": NoArgs;
      "notifications.title": NoArgs;
      "notifications.empty": NoArgs;
      "notifications.loadError": NoArgs;
      "notifications.markAllRead": NoArgs;
      "notifications.unreadLabel": { count: number };
      "user.anonymous": NoArgs;
      "user.menu": NoArgs;
      "user.manageAccount": NoArgs;
      "user.signOut": NoArgs;
      "user.unavailable": NoArgs;
      "theme.label": NoArgs;
      "theme.light": NoArgs;
      "theme.dark": NoArgs;
      "theme.system": NoArgs;
      "moduleInactive.title": NoArgs;
      "moduleInactive.notEnabled": NoArgs;
      "moduleInactive.notInstalled": NoArgs;
      "moduleInactive.unavailable": NoArgs;
      "moduleInactive.openSettings": NoArgs;
      "notFound.title": NoArgs;
      "notFound.description": NoArgs;
      "notFound.backHome": NoArgs;
      "routeError.title": NoArgs;
      "routeError.description": NoArgs;
      "routeError.requestId": { requestId: string };
      "routeError.traceId": { traceId: string };
      "dashboard.title": NoArgs;
      "dashboard.description": NoArgs;
      "pageActions.more": NoArgs;
      "errors.bad_request": NoArgs;
      "errors.invalid_filter": NoArgs;
      "errors.invalid_sort": NoArgs;
      "errors.invalid_i18n_override": NoArgs;
      "errors.tenant_unresolved": NoArgs;
      "errors.unauthenticated": NoArgs;
      "errors.token_expired": NoArgs;
      "errors.permission_denied": NoArgs;
      "errors.field_forbidden": NoArgs;
      "errors.mfa_required": NoArgs;
      "errors.module_disabled": NoArgs;
      "errors.not_found": NoArgs;
      "errors.module_not_found": NoArgs;
      "errors.invalid_state": NoArgs;
      "errors.request_in_progress": NoArgs;
      "errors.conflict": NoArgs;
      "errors.version_mismatch": NoArgs;
      "errors.validation_failed": NoArgs;
      "errors.business_rule_violated": NoArgs;
      "errors.idempotency_key_reused": NoArgs;
      "errors.rate_limited": NoArgs;
      "errors.unsupported_media_type": NoArgs;
      "errors.internal_error": NoArgs;
      "errors.not_implemented": NoArgs;
      "errors.method_not_allowed": NoArgs;
      "errors.module_unavailable": NoArgs;
      "errors.not_ready": NoArgs;
      "errors.unknown": NoArgs;
    };
    ui: {
      "actions.save": NoArgs;
      "actions.cancel": NoArgs;
      "actions.close": NoArgs;
      "actions.confirm": NoArgs;
      "actions.delete": NoArgs;
      "actions.edit": NoArgs;
      "actions.add": NoArgs;
      "actions.search": NoArgs;
      "actions.retry": NoArgs;
      "actions.back": NoArgs;
      "actions.next": NoArgs;
      "actions.previous": NoArgs;
      "status.loading": NoArgs;
      "status.empty": NoArgs;
      "pagination.summary": { total: number };
      "pagination.page": { page: number; pages: number };
      "breadcrumb.label": NoArgs;
      "breadcrumb.more": NoArgs;
      "sidebar.label": NoArgs;
      "sidebar.toggle": NoArgs;
      "sidebar.title": NoArgs;
      "sidebar.description": NoArgs;
      "toast.close": NoArgs;
      "toast.region": NoArgs;
    };
  }
}

type Catalogs = import("@erpora/platform-web").I18nCatalogs;

/** Daftar kunci eksak (kompilator menjamin lengkap): dipakai `keys.test.ts` untuk paritas dengan katalog JSON. */
export const platformKeyList: Record<keyof Catalogs["platform"], true> = {
  "app.name": true,
  "shell.loading": true,
  "shell.retry": true,
  "shell.skipToContent": true,
  "shell.language.label": true,
  "nav.home": true,
  "nav.menu": true,
  "nav.settings": true,
  "nav.settingsModules": true,
  "nav.empty": true,
  "notifications.title": true,
  "notifications.empty": true,
  "notifications.loadError": true,
  "notifications.markAllRead": true,
  "notifications.unreadLabel": true,
  "user.anonymous": true,
  "user.menu": true,
  "user.manageAccount": true,
  "user.signOut": true,
  "user.unavailable": true,
  "theme.label": true,
  "theme.light": true,
  "theme.dark": true,
  "theme.system": true,
  "moduleInactive.title": true,
  "moduleInactive.notEnabled": true,
  "moduleInactive.notInstalled": true,
  "moduleInactive.unavailable": true,
  "moduleInactive.openSettings": true,
  "notFound.title": true,
  "notFound.description": true,
  "notFound.backHome": true,
  "routeError.title": true,
  "routeError.description": true,
  "routeError.requestId": true,
  "routeError.traceId": true,
  "dashboard.title": true,
  "dashboard.description": true,
  "pageActions.more": true,
  "errors.bad_request": true,
  "errors.invalid_filter": true,
  "errors.invalid_sort": true,
  "errors.invalid_i18n_override": true,
  "errors.tenant_unresolved": true,
  "errors.unauthenticated": true,
  "errors.token_expired": true,
  "errors.permission_denied": true,
  "errors.field_forbidden": true,
  "errors.mfa_required": true,
  "errors.module_disabled": true,
  "errors.not_found": true,
  "errors.module_not_found": true,
  "errors.invalid_state": true,
  "errors.request_in_progress": true,
  "errors.conflict": true,
  "errors.version_mismatch": true,
  "errors.validation_failed": true,
  "errors.business_rule_violated": true,
  "errors.idempotency_key_reused": true,
  "errors.rate_limited": true,
  "errors.unsupported_media_type": true,
  "errors.internal_error": true,
  "errors.not_implemented": true,
  "errors.method_not_allowed": true,
  "errors.module_unavailable": true,
  "errors.not_ready": true,
  "errors.unknown": true,
};

export const uiKeyList: Record<keyof Catalogs["ui"], true> = {
  "actions.save": true,
  "actions.cancel": true,
  "actions.close": true,
  "actions.confirm": true,
  "actions.delete": true,
  "actions.edit": true,
  "actions.add": true,
  "actions.search": true,
  "actions.retry": true,
  "actions.back": true,
  "actions.next": true,
  "actions.previous": true,
  "status.loading": true,
  "status.empty": true,
  "pagination.summary": true,
  "pagination.page": true,
  "breadcrumb.label": true,
  "breadcrumb.more": true,
  "sidebar.label": true,
  "sidebar.toggle": true,
  "sidebar.title": true,
  "sidebar.description": true,
  "toast.close": true,
  "toast.region": true,
};

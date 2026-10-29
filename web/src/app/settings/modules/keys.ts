// Kunci terketik `platform:modules.*` (halaman /settings/modules; M3a grup 14). Satu sumber: `MODULE_KEYS` (nilai = bentuk
// argumen pesan). Ditulis tangan sampai generator `i18n:generate` tersedia; `keys.test.ts` menjaga himpunan kunci dan argumen
// sama persis dengan subpohon `modules` di internal/platform/i18n/catalog/{id,en}.json.
import { type ArgsOf, useTranslation } from "@erpora/platform-web";
import { useCallback } from "react";

type NoArgs = Record<never, never>;
const a = <A extends object = NoArgs>(): A => undefined as unknown as A;

export const MODULE_KEYS = {
  "modules.title": a(),
  "modules.description": a(),
  "modules.empty": a(),
  "modules.required": a(),
  "modules.refresh": a(),
  "modules.loadError": a(),
  "modules.forbidden.title": a(),
  "modules.forbidden.description": a(),
  "modules.readonly.title": a(),
  "modules.readonly.description": a(),
  "modules.columns.name": a(),
  "modules.columns.version": a(),
  "modules.columns.status": a(),
  "modules.columns.dependencies": a(),
  "modules.columns.dependents": a(),
  "modules.columns.actions": a(),
  "modules.version.installed": a<{ version: string }>(),
  "modules.version.binary": a<{ version: string }>(),
  "modules.version.notInstalled": a(),
  "modules.version.upgradeAvailable": a(),
  "modules.dependencies.none": a(),
  "modules.dependencies.optional": a<{ name: string }>(),
  "modules.status.available": a(),
  "modules.status.installed": a(),
  "modules.status.enabled": a(),
  "modules.status.disabled": a(),
  "modules.status.upgrade_pending": a(),
  "modules.status.failed": a(),
  "modules.status.uninstalled": a(),
  "modules.status.missing": a(),
  "modules.status.incompatible": a(),
  "modules.op.install": a(),
  "modules.op.enable": a(),
  "modules.op.disable": a(),
  "modules.op.upgrade": a(),
  "modules.op.uninstall": a(),
  "modules.op.retry": a(),
  "modules.op.history": a(),
  "modules.op.menu": a<{ name: string }>(),
  "modules.dialog.title": a<{ operation: string; name: string }>(),
  "modules.dialog.confirm": a(),
  "modules.dialog.cancel": a(),
  "modules.dialog.close": a(),
  "modules.dialog.noImpact": a(),
  "modules.impact.effects": a(),
  "modules.impact.blockers": a(),
  "modules.impact.menu_hidden": a<{ labels: string }>(),
  "modules.impact.menu_shown": a<{ labels: string }>(),
  "modules.impact.routes_inactive": a<{ paths: string }>(),
  "modules.impact.routes_active": a<{ paths: string }>(),
  "modules.impact.api_closed": a<{ prefix: string; code: string }>(),
  "modules.impact.data_kept": a(),
  "modules.impact.migrations_run": a(),
  "modules.impact.version_change": a<{ from: string; to: string }>(),
  "modules.impact.needs_dependencies": a<{ modules: string }>(),
  "modules.blocker.module_required": a<{ modules: string }>(),
  "modules.blocker.dependency_not_ready": a<{ modules: string }>(),
  "modules.blocker.dependents_active": a<{ modules: string }>(),
  "modules.blocker.module_incompatible": a<{ modules: string }>(),
  "modules.blocker.invalid_transition": a<{ modules: string }>(),
  "modules.blocker.downgrade_not_supported": a<{ modules: string }>(),
  "modules.install.withDeps": a(),
  "modules.install.enable": a(),
  "modules.result.title": a(),
  "modules.result.running": a(),
  "modules.result.succeeded": a(),
  "modules.result.failed": a(),
  "modules.result.noop": a(),
  "modules.result.timeout": a(),
  "modules.result.pollError": a(),
  "modules.result.operation": a<{ operation: string; status: string }>(),
  "modules.error.invalid_transition": a(),
  "modules.error.operation_in_progress": a(),
  "modules.error.module_required": a(),
  "modules.error.dependency_not_ready": a(),
  "modules.error.dependents_active": a(),
  "modules.error.module_incompatible": a(),
  "modules.error.downgrade_not_supported": a(),
  "modules.error.module_not_found": a(),
  "modules.error.version_mismatch": a(),
  "modules.error.mutation_disabled": a(),
  "modules.error.requestId": a<{ requestId: string }>(),
  "modules.failure.migration_failed": a(),
  "modules.failure.interrupted": a(),
  "modules.failure.health_check_failed": a(),
  "modules.failure.hook_failed": a(),
  "modules.failure.dependency_not_ready": a(),
  "modules.failure.downgrade_not_supported": a(),
  "modules.failure.internal_error": a(),
  "modules.failure.unknown": a(),
  "modules.history.title": a<{ name: string }>(),
  "modules.history.description": a(),
  "modules.history.empty": a(),
  "modules.history.error": a(),
  "modules.history.actor": a<{ actor: string }>(),
  "modules.history.page": a<{ page: number }>(),
  "modules.history.details": a(),
  "modules.history.operationSteps": a(),
  "modules.history.stepsEmpty": a(),
  "modules.history.step": a<{ name: string; status: string; durationMs: number }>(),
  "modules.history.event.installed": a(),
  "modules.history.event.enabled": a(),
  "modules.history.event.disabled": a(),
  "modules.history.event.upgraded": a(),
  "modules.history.event.uninstalled": a(),
  "modules.history.event.operation_failed": a(),
  "modules.history.event.interrupted": a(),
  "modules.history.event.retried": a(),
  "modules.history.event.reconciled": a(),
  "modules.history.event.unknown": a<{ event: string }>(),
} as const;

export type ModulesCatalog = typeof MODULE_KEYS;
export type ModulesKey = keyof ModulesCatalog;
/** Daftar kunci eksak untuk test paritas dengan katalog JSON. */
export const modulesKeyList = Object.keys(MODULE_KEYS) as ModulesKey[];

/** Pesan yang dapat dirender: kunci `modules.*` atau kunci galat umum `errors.*` dengan argumen opsional. */
export interface MessageDescriptor {
  key: ModulesKey | `errors.${string}`;
  args?: Record<string, string | number>;
}

type Translate = (key: string, args?: Record<string, string | number>) => string;

/** `t` terketik untuk kunci `modules.*` (argumen wajib sesuai bentuk pesan). */
export function useModulesT() {
  const { t } = useTranslation("platform");
  return useCallback(
    <K extends ModulesKey>(key: K, ...args: ArgsOf<ModulesCatalog[K]>): string =>
      (t as unknown as Translate)(key, args[0] as Record<string, string | number> | undefined),
    [t],
  );
}

/** Menerjemahkan `MessageDescriptor` (kunci dinamis dari pemetaan galat). */
export function useDescribe() {
  const { t } = useTranslation("platform");
  return useCallback(
    (d: MessageDescriptor): string => (t as unknown as Translate)(d.key, d.args),
    [t],
  );
}

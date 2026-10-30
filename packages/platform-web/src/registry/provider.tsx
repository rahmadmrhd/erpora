import type { FrontendModule } from "@erpora/module-sdk";
import { createContext, type ReactNode, useContext, useMemo, useState } from "react";
import { useBootstrap } from "../bootstrap";
import type { ModuleStatus } from "../bootstrap/types";
import {
  moduleStatus,
  type NavNode,
  type RegistryWarning,
  type ResolvedModule,
  resolveModules,
  selectNavigation,
  validateModules,
} from "./registry";

export interface ModuleRegistry {
  /** Modul frontend yang ada di bootstrap saat dimuat pertama (dipakai router; dihitung sekali). */
  resolved: readonly ResolvedModule[];
  warnings: readonly RegistryWarning[];
  /** Navigasi modul `enabled` pada bootstrap terbaru. */
  navigation: readonly NavNode[];
  statusOf(key: string): ModuleStatus | undefined;
  isEnabled(key: string): boolean;
}

const RegistryContext = createContext<ModuleRegistry | null>(null);

/**
 * Registry modul frontend. `modules` (daftar `web/src/app/modules.ts`) divalidasi dan dicocokkan dengan bootstrap
 * sekali saat pertama dirender; navigasi dan status mengikuti bootstrap terbaru tanpa reload. Dipasang di dalam
 * `BootstrapProvider`.
 */
export function ModuleRegistryProvider({
  modules,
  warn,
  children,
}: {
  modules: readonly FrontendModule[];
  warn?: (w: RegistryWarning) => void;
  children: ReactNode;
}) {
  const bootstrap = useBootstrap();
  const [resolution] = useState(() => {
    validateModules(modules);
    return resolveModules(modules, bootstrap, { warn });
  });
  const navigation = useMemo(
    () => selectNavigation(resolution.resolved, bootstrap),
    [resolution, bootstrap],
  );
  const value = useMemo<ModuleRegistry>(
    () => ({
      resolved: resolution.resolved,
      warnings: resolution.warnings,
      navigation,
      statusOf: (key) => moduleStatus(bootstrap, key),
      isEnabled: (key) => moduleStatus(bootstrap, key) === "enabled",
    }),
    [resolution, navigation, bootstrap],
  );
  return <RegistryContext.Provider value={value}>{children}</RegistryContext.Provider>;
}

export function useModuleRegistry(): ModuleRegistry {
  const value = useContext(RegistryContext);
  if (value === null) {
    throw new Error("useModuleRegistry harus dipakai di dalam ModuleRegistryProvider");
  }
  return value;
}

import { createContext, type ReactNode, useContext } from "react";

/** Bagian dialog Settings yang dapat dibuka langsung (mis. pintasan dari popover logo). */
export type SettingsSection = "appearance" | "preferences" | "shortcuts";

/**
 * Aksi tingkat shell yang dipanggil komponen dock, panel, popover, dan menu pengguna. `AppShell` mengisinya (membuka
 * palet, panel Notifikasi, dan dialog Settings yang dimuat malas) sehingga komponen tidak mengenal modul malas itu
 * dan dapat diuji terpisah.
 */
export interface ShellActions {
  openPalette(): void;
  openNotifications(): void;
  openSettings(section?: SettingsSection): void;
  reload(): void;
}

const defaultActions: ShellActions = {
  openPalette: () => {},
  openNotifications: () => {},
  openSettings: () => {},
  reload: () => window.location.reload(),
};

const ShellActionsContext = createContext<ShellActions>(defaultActions);

export function ShellActionsProvider({
  value,
  children,
}: {
  value: ShellActions;
  children: ReactNode;
}) {
  return <ShellActionsContext.Provider value={value}>{children}</ShellActionsContext.Provider>;
}

export function useShellActions(): ShellActions {
  return useContext(ShellActionsContext);
}

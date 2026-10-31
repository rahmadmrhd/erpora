import { useSyncExternalStore } from "react";
import { type DockMode, readDockMode, readTimezone, subscribePrefs } from "./index";

/** Mode Dock tersimpan, diperbarui seketika saat berubah (Settings di tab ini atau tab lain). */
export function useDockMode(): DockMode {
  return useSyncExternalStore(subscribePrefs, readDockMode, () => "floating");
}

/** Zona waktu pilihan pengguna (`null` = otomatis, ikuti peramban), diperbarui seketika saat berubah. */
export function useTimezone(): string | null {
  return useSyncExternalStore(subscribePrefs, readTimezone, () => null);
}

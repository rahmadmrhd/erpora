import { useEffect, useRef } from "react";

function isEditable(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  return (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    target.isContentEditable ||
    target.getAttribute("contenteditable") === "" ||
    target.getAttribute("contenteditable") === "true"
  );
}

export interface GlobalShortcutHandlers {
  /** `Ctrl+Shift+L`: ganti tema (berlaku juga saat fokus di kolom isian karena memakai modifier). */
  toggleTheme(): void;
  /** `Shift+/`: buka daftar pintasan; diabaikan saat mengetik di kolom isian. */
  showShortcuts(): void;
}

/**
 * Pintasan global shell yang dicantumkan di `GLOBAL_SHORTCUTS`: `Ctrl+Shift+L` dan `Shift+/`. `Ctrl+K` (palet),
 * `Ctrl+B` (panel; bawaan `SidebarProvider`), dan `Ctrl+Shift+/` (Dock Floating) ditangani komponen pemiliknya.
 */
export function useGlobalShortcuts(handlers: GlobalShortcutHandlers): void {
  const ref = useRef(handlers);
  ref.current = handlers;
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.altKey || event.metaKey) return;
      const isL = event.code === "KeyL" || event.key.toLowerCase() === "l";
      if (isL && event.ctrlKey && event.shiftKey) {
        event.preventDefault();
        ref.current.toggleTheme();
        return;
      }
      const isSlash = event.code === "Slash" || event.key === "/" || event.key === "?";
      if (isSlash && event.shiftKey && !event.ctrlKey && !isEditable(event.target)) {
        event.preventDefault();
        ref.current.showShortcuts();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);
}

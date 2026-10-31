import { useEffect, useRef } from "react";

/**
 * Pintasan `Ctrl+K` (`Cmd+K` di Mac) pembuka palet perintah (R18.1). Dipasang oleh AppShell, di luar chunk malas palet
 * agar pintasan tetap hidup sebelum palet termuat. Memakai modifier sehingga juga berlaku saat fokus di kolom isian;
 * mencegah perilaku bawaan peramban (fokus bilah alamat/cari). Penekanan berulang dan peristiwa yang sudah ditangani
 * diabaikan.
 */
export function useCommandPaletteHotkey(onOpen: () => void): void {
  const ref = useRef(onOpen);
  ref.current = onOpen;
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.repeat) return;
      if (event.altKey || event.shiftKey) return;
      if (!(event.ctrlKey || event.metaKey)) return;
      if (event.key.toLowerCase() !== "k" && event.code !== "KeyK") return;
      event.preventDefault();
      ref.current();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);
}

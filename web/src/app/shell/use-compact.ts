import { useSyncExternalStore } from "react";

/** Di bawah lebar ini panel menjadi `Sheet` dan Dock/pembatas tidak ditampilkan (R4.6, R5.8). */
export const COMPACT_BREAKPOINT = 1024;

function subscribe(onChange: () => void): () => void {
  window.addEventListener("resize", onChange);
  return () => window.removeEventListener("resize", onChange);
}

const read = (): boolean => window.innerWidth < COMPACT_BREAKPOINT;

/**
 * `true` bila lebar jendela < 1024 px. Ambang ini berbeda dari `useIsMobile` shadcn (768 px, tidak boleh diubah
 * karena berkas registry), sehingga shell menggambar `Sheet` sendiri di antara 768 dan 1023 px.
 */
export function useIsCompact(): boolean {
  return useSyncExternalStore(subscribe, read, () => false);
}

import { createContext, type ReactNode, useContext, useLayoutEffect } from "react";

/**
 * Varian kerangka shell. `default`: Dock/panel/pembatas dan navbar breadcrumb. `desks`: halaman Desks (`/` yang
 * menampilkan Desks, K1) tanpa Dock, panel, pembatas, dan tombol sidebar, dengan navbar khusus (`DesksNavbar`).
 */
export type ShellChrome = "default" | "desks";

type SetChrome = (chrome: ShellChrome) => void;

const ShellChromeContext = createContext<SetChrome>(() => {});

/** Dipasang `AppShell`; `setChrome` mengganti varian kerangka. */
export function ShellChromeProvider({
  setChrome,
  children,
}: {
  setChrome: SetChrome;
  children: ReactNode;
}) {
  return <ShellChromeContext.Provider value={setChrome}>{children}</ShellChromeContext.Provider>;
}

/**
 * Meminta varian kerangka shell selama komponen terpasang (sinkron lewat `useLayoutEffect`, sehingga tidak ada kilatan
 * kerangka lama) dan mengembalikan `default` saat dilepas. Dipanggil halaman yang menampilkan Desks:
 *
 * ```tsx
 * function DesksPage() {
 *   useShellChrome("desks");
 *   ...
 * }
 * ```
 *
 * Di luar `AppShell` (mis. halaman diuji sendiri) hook ini tidak berefek.
 */
export function useShellChrome(chrome: ShellChrome): void {
  const setChrome = useContext(ShellChromeContext);
  useLayoutEffect(() => {
    setChrome(chrome);
    return () => setChrome("default");
  }, [chrome, setChrome]);
}

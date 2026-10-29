import { useBootstrap, useModuleRegistry } from "@erpora/platform-web";
import { useQueryClient } from "@tanstack/react-query";
import { type RouterHistory, RouterProvider } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { buildRouter } from "./router";

/**
 * Membuat router sekali (setelah bootstrap) dan memanggil `router.invalidate()` setiap `bootstrap.revision` berubah,
 * sehingga guard dievaluasi ulang: modul yang diaktifkan kembali pulih tanpa reload (R8.2).
 */
export function AppRouter({ history }: { history?: RouterHistory }) {
  const queryClient = useQueryClient();
  const { resolved } = useModuleRegistry();
  const { revision } = useBootstrap();
  const [router] = useState(() => buildRouter({ modules: resolved, queryClient, history }));
  const seen = useRef(revision);
  useEffect(() => {
    if (seen.current === revision) return;
    seen.current = revision;
    void router.invalidate();
  }, [revision, router]);
  return <RouterProvider router={router} />;
}

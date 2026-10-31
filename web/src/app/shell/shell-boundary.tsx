import { useQueryErrorResetBoundary } from "@tanstack/react-query";
import { useRouterState } from "@tanstack/react-router";
import type { ReactNode } from "react";
import { LocalBoundary } from "./local-boundary";

/**
 * Batas galat lokal untuk bagian shell yang memuat sesuatu dari jaringan (katalog i18n sebuah Desk, chunk malas):
 * kegagalan terisolasi di bagian itu dan shell tetap hidup. `fallback` menerima `retry`, yang mereset query galat
 * (katalog dimuat ulang) lalu merender anak lagi; berpindah halaman juga mengembalikan anak. Galat dicatat satu baris
 * konsol tanpa PII (lihat `LocalBoundary`).
 */
export function ShellBoundary({
  label,
  fallback,
  children,
}: {
  label: string;
  fallback: ReactNode | ((retry: () => void) => ReactNode);
  children: ReactNode;
}) {
  const { reset } = useQueryErrorResetBoundary();
  const resetKey = useRouterState({ select: (s) => s.location.href });
  return (
    <LocalBoundary label={label} resetKey={resetKey} fallback={fallback} onRetry={reset}>
      {children}
    </LocalBoundary>
  );
}

import type { PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import { useQuery } from "@tanstack/react-query";
import { createContext, type ReactNode, useContext, useMemo } from "react";
import {
  type BootstrapQueryTuning,
  bootstrapQueryOptions,
  createBootstrapFetcher,
} from "./fetcher";
import type { Bootstrap, BootstrapFeatures } from "./types";

const BootstrapContext = createContext<Bootstrap | null>(null);

export interface BootstrapProviderProps {
  client: TypedClient<PlatformPaths>;
  /** Ditampilkan selama bootstrap pertama dimuat. */
  loading: ReactNode;
  /** Ditampilkan bila bootstrap pertama gagal; `retry` memuat ulang. Tidak dipakai bila data sudah ada. */
  error: (retry: () => void, error: unknown) => ReactNode;
  children: ReactNode;
  options?: BootstrapQueryTuning;
}

/**
 * Memuat bootstrap (query `["bootstrap"]`, ber-`ETag`, refetch saat focus) dan menyediakannya ke anak. Anak hanya
 * dirender setelah data ada; galat refetch berikutnya tidak menggantikan layar selama data lama masih ada.
 */
export function BootstrapProvider({
  client,
  loading,
  error,
  children,
  options,
}: BootstrapProviderProps) {
  const fetcher = useMemo(() => createBootstrapFetcher(client), [client]);
  const query = useQuery(bootstrapQueryOptions(fetcher, options));
  if (query.data !== undefined) {
    return <BootstrapContext.Provider value={query.data}>{children}</BootstrapContext.Provider>;
  }
  if (query.isError) {
    return (
      <>
        {error(() => {
          void query.refetch();
        }, query.error)}
      </>
    );
  }
  return <>{loading}</>;
}

/** Menyediakan bootstrap statis (test, fixture, dan Storybook); bukan untuk produksi. */
export function StaticBootstrapProvider({
  value,
  children,
}: {
  value: Bootstrap;
  children: ReactNode;
}) {
  return <BootstrapContext.Provider value={value}>{children}</BootstrapContext.Provider>;
}

/** Bootstrap saat ini; hanya di dalam `BootstrapProvider` (anak sudah menerima data siap). */
export function useBootstrap(): Bootstrap {
  const value = useContext(BootstrapContext);
  if (value === null) {
    throw new Error("useBootstrap harus dipakai di dalam BootstrapProvider");
  }
  return value;
}

export function useTenantId(): string {
  return useBootstrap().tenantId;
}

/** Bendera fitur; bidang yang tidak ada berarti `false`. */
export function useFeature(name: keyof BootstrapFeatures): boolean {
  return useBootstrap().features?.[name] === true;
}

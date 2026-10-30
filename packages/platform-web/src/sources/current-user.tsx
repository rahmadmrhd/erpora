import { useQuery } from "@tanstack/react-query";
import { createContext, type ReactNode, useContext } from "react";
import { useTenantId } from "../bootstrap";
import { scopedKey } from "../tenancy";
import { anonymousUserSource } from "./empty";
import type { CurrentUser, CurrentUserSource } from "./types";

const CurrentUserSourceContext = createContext<CurrentUserSource>(anonymousUserSource);

/** Memasang sumber pengguna; tanpa provider dipakai `anonymousUserSource`. */
export function CurrentUserSourceProvider({
  source,
  children,
}: {
  source: CurrentUserSource;
  children: ReactNode;
}) {
  return (
    <CurrentUserSourceContext.Provider value={source}>{children}</CurrentUserSourceContext.Provider>
  );
}

/** Sumber terpasang (kemampuan Kelola Akun/Keluar, `manageAccountPath`, `signOut`). */
export function useCurrentUserSource(): CurrentUserSource {
  return useContext(CurrentUserSourceContext);
}

/**
 * Pengguna saat ini dari sumber terpasang. Nama dan email adalah PII (Rahasia): hanya di cache memori Query
 * (tanpa persister), tidak di storage, log, atau URL.
 */
export function useCurrentUser(): {
  user: CurrentUser | null;
  status: "loading" | "ready" | "error";
} {
  const source = useCurrentUserSource();
  const tenantId = useTenantId();
  const query = useQuery({
    queryKey: scopedKey(tenantId, "current-user"),
    queryFn: ({ signal }) => source.get(signal),
    staleTime: 5 * 60_000,
  });
  return {
    user: query.data ?? null,
    status: query.isError ? "error" : query.isSuccess ? "ready" : "loading",
  };
}

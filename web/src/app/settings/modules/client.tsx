import type { PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import { createPlatformClient, getActiveLocale } from "@erpora/platform-web";
import { createContext, type ReactNode, useContext } from "react";

type Client = TypedClient<PlatformPaths>;

const ClientContext = createContext<Client | null>(null);
let defaultClient: Client | undefined;

/** Menyuntik klien API (test); tanpa provider dipakai klien bawaan (`fetch` global, `Accept-Language` bahasa aktif). */
export function ModulesClientProvider({
  client,
  children,
}: {
  client: Client;
  children: ReactNode;
}) {
  return <ClientContext.Provider value={client}>{children}</ClientContext.Provider>;
}

export function useModulesClient(): Client {
  const injected = useContext(ClientContext);
  if (injected) return injected;
  defaultClient ??= createPlatformClient({ getLocale: getActiveLocale });
  return defaultClient;
}

import type { FrontendModule } from "@erpora/module-sdk";
import type { PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import {
  anonymousUserSource,
  BootstrapProvider,
  type BootstrapQueryTuning,
  type CurrentUserSource,
  CurrentUserSourceProvider,
  createPlatformClient,
  emptyNotificationSource,
  getActiveLocale,
  I18nProvider,
  ModuleRegistryProvider,
  type NotificationSource,
  NotificationSourceProvider,
  PageChromeProvider,
  ThemeProvider,
} from "@erpora/platform-web";
import { Toaster, TooltipProvider } from "@erpora/ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RouterHistory } from "@tanstack/react-router";
import { Suspense, useState } from "react";
import { AppRouter } from "../router/app-router";
import { BootErrorScreen, BootLoading } from "./boot-screens";
import { UiLabelsBridge } from "./ui-labels-bridge";

function isRetryable(error: unknown): boolean {
  const status = (error as { status?: unknown } | null)?.status;
  return typeof status === "number" ? status >= 500 : true;
}

/** `QueryClient` tunggal aplikasi: tanpa persister (cache hanya di memori), retry terbatas untuk galat sementara. */
export function createAppQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: (failureCount, error) => failureCount < 2 && isRetryable(error),
      },
      mutations: { retry: 0 },
    },
  });
}

export interface ProvidersProps {
  /** Daftar modul frontend (`web/src/app/modules.ts`). */
  modules: readonly FrontendModule[];
  /** Untuk test: klien API, QueryClient, riwayat router, sumber data, dan penyetelan bootstrap. */
  client?: TypedClient<PlatformPaths>;
  queryClient?: QueryClient;
  history?: RouterHistory;
  notificationSource?: NotificationSource;
  currentUserSource?: CurrentUserSource;
  bootstrapOptions?: BootstrapQueryTuning;
}

/**
 * Urutan provider (design 6.1): QueryClient tunggal, tema (tak bergantung bootstrap sehingga layar boot ikut tema),
 * bootstrap (kerangka dan layar galat dengan kamus bawaan), registry modul, runtime i18n, label `ui`, tooltip, sumber
 * notifikasi dan pengguna, aksi halaman, dan router.
 */
export function Providers(props: ProvidersProps) {
  const [queryClient] = useState(() => props.queryClient ?? createAppQueryClient());
  const [client] = useState(
    () => props.client ?? createPlatformClient({ getLocale: getActiveLocale }),
  );
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <BootstrapProvider
          client={client}
          loading={<BootLoading />}
          error={(retry) => <BootErrorScreen onRetry={retry} />}
          options={props.bootstrapOptions}
        >
          <ModuleRegistryProvider modules={props.modules}>
            <I18nProvider client={client}>
              <Suspense fallback={<BootLoading />}>
                <UiLabelsBridge>
                  <TooltipProvider>
                    <NotificationSourceProvider
                      source={props.notificationSource ?? emptyNotificationSource}
                    >
                      <CurrentUserSourceProvider
                        source={props.currentUserSource ?? anonymousUserSource}
                      >
                        <PageChromeProvider>
                          <AppRouter history={props.history} />
                          <Toaster />
                        </PageChromeProvider>
                      </CurrentUserSourceProvider>
                    </NotificationSourceProvider>
                  </TooltipProvider>
                </UiLabelsBridge>
              </Suspense>
            </I18nProvider>
          </ModuleRegistryProvider>
        </BootstrapProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

import type { FrontendModule } from "@erpora/module-sdk";
import type { PlatformPaths, TypedClient } from "@erpora/platform-contracts";
import {
  type AuthController,
  AuthProvider,
  BootstrapProvider,
  type BootstrapQueryTuning,
  type CurrentUserSource,
  CurrentUserSourceProvider,
  createAuthController,
  createBootstrapUserSource,
  createPlatformClient,
  DeskProvider,
  emptyNotificationSource,
  getActiveLocale,
  I18nProvider,
  ModuleRegistryProvider,
  type NotificationSource,
  NotificationSourceProvider,
  PageChromeProvider,
  ThemeProvider,
  useBootstrap,
} from "@erpora/platform-web";
import { TooltipProvider } from "@erpora/ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RouterHistory } from "@tanstack/react-router";
import { lazy, type ReactNode, Suspense, useMemo, useState } from "react";
import { AppRouter } from "../router/app-router";
import { AuthGate, PUBLIC_GATE_PATHS, useSignOut } from "./auth-gate";
import { BootErrorScreen, BootLoading } from "./boot-screens";
import { UiLabelsBridge } from "./ui-labels-bridge";

// Toaster (sonner, ±35 KB gzip) dimuat malas dari entry sekunder `@erpora/ui/toaster`: tidak ada di bundel awal (K-12).
const LazyToaster = lazy(() => import("@erpora/ui/toaster").then((m) => ({ default: m.Toaster })));

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
  /** Menggantikan sumber pengguna dari bootstrap (test). */
  currentUserSource?: CurrentUserSource;
  /** Klien token (test); bawaan dibuat per instance dan melakukan refresh senyap saat boot. */
  auth?: AuthController;
  bootstrapOptions?: BootstrapQueryTuning;
}

/** Path halaman Ubah sandi modul core: tautan "Ubah sandi" di menu pengguna (PRD-001 Q33 butir 5). */
const CHANGE_PASSWORD_PATH = "/core/change-password";

/**
 * Sumber pengguna sungguhan dari `bootstrap.user` (menggantikan `anonymousUserSource`); Keluar memanggil logout klien
 * token lewat gerbang (membersihkan cache dan membuka `/login`).
 */
function BootstrapUserSource({
  override,
  children,
}: {
  override: CurrentUserSource | undefined;
  children: ReactNode;
}) {
  const { user } = useBootstrap();
  const signOut = useSignOut();
  const source = useMemo(
    () =>
      override ??
      createBootstrapUserSource({ user, signOut, manageAccountPath: CHANGE_PASSWORD_PATH }),
    [override, user, signOut],
  );
  return <CurrentUserSourceProvider source={source}>{children}</CurrentUserSourceProvider>;
}

/**
 * Urutan provider (design 6.1): QueryClient tunggal, tema (tak bergantung bootstrap sehingga layar boot dan gerbang ikut
 * tema), klien token dan gerbang autentikasi (PRD-001 Q33: login/MFA/lupa/reset dirender SEBELUM bootstrap), bootstrap
 * (kerangka dan layar galat dengan kamus bawaan), registry modul, runtime i18n, label `ui`, tooltip, sumber notifikasi
 * dan pengguna, daftar Desk (`DeskProvider`), aksi halaman, dan router.
 */
export function Providers(props: ProvidersProps) {
  const [queryClient] = useState(() => props.queryClient ?? createAppQueryClient());
  const [client] = useState(
    () => props.client ?? createPlatformClient({ getLocale: getActiveLocale }),
  );
  const [auth] = useState(() => props.auth ?? createAuthController());
  // Tautan lupa/reset sandi tidak butuh sesi: tanpa refresh senyap (yang akan sia-sia dan menahan layar).
  const [boot] = useState<"refresh" | "skip">(() =>
    PUBLIC_GATE_PATHS.has(window.location.pathname) ? "skip" : "refresh",
  );
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <AuthProvider controller={auth} boot={boot}>
          <AuthGate>
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
                          <BootstrapUserSource override={props.currentUserSource}>
                            <DeskProvider client={client}>
                              <PageChromeProvider>
                                <AppRouter history={props.history} />
                                <Suspense fallback={null}>
                                  <LazyToaster />
                                </Suspense>
                              </PageChromeProvider>
                            </DeskProvider>
                          </BootstrapUserSource>
                        </NotificationSourceProvider>
                      </TooltipProvider>
                    </UiLabelsBridge>
                  </Suspense>
                </I18nProvider>
              </ModuleRegistryProvider>
            </BootstrapProvider>
          </AuthGate>
        </AuthProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

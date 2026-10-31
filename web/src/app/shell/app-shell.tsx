import { Message, useDockMode, usePageChrome, useTheme } from "@erpora/platform-web";
import { Sidebar, SidebarProvider, Skeleton } from "@erpora/ui";
import { CatchBoundary, Outlet, useRouterState } from "@tanstack/react-router";
import { type CSSProperties, lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { RouteErrorPage } from "../pages/pages";
import { CollapseEdge } from "./collapse-edge";
import { useCommandPaletteHotkey } from "./command-palette-hotkey";
import { DeskPanel, PanelSheet } from "./desk-panel";
import { useGlobalShortcuts } from "./global-shortcuts";
import { LocalBoundary } from "./local-boundary";
import { Navbar, NavbarFallback } from "./navbar";
import { useRouteTitle } from "./route-crumbs";
import { ShellBoundary } from "./shell-boundary";
import { type ShellChrome, ShellChromeProvider } from "./shell-chrome";
import { type SettingsSection, type ShellActions, ShellActionsProvider } from "./shell-context";
import { readSidebarOpen, writeSidebarOpen } from "./sidebar-state";
import { useIsCompact } from "./use-compact";

// Dock Pinned dimuat malas (NFR-04); sementara memuat, rel kosong selebar Dock menjaga tata letak (tanpa lompatan).
const LazyDock = lazy(() => import("./dock").then((m) => ({ default: m.Dock })));

function DockPlaceholder() {
  return (
    <div
      aria-hidden="true"
      className="relative z-20 h-full w-(--dock-width) shrink-0 border-r border-sidebar-border bg-sidebar"
    />
  );
}

// Dock Floating (pil, hot-edge, pintasan) dimuat malas: bagian berat di luar bundel awal (NFR-04).
const LazyDockFloating = lazy(() =>
  import("./dock-floating").then((m) => ({ default: m.DockFloating })),
);

// Palet perintah (cmdk) hanya ada di chunk malas ini; pintasan Ctrl+K hidup di bundel awal dan memuatnya saat pertama dipakai.
const LazyCommandPalette = lazy(() =>
  import("./command-palette").then((m) => ({ default: m.CommandPalette })),
);

// Navbar halaman Desks (K1) hanya ada di chunk malas ini; fallback setinggi navbar agar tidak ada lompatan tata letak.
const LazyDesksNavbar = lazy(() =>
  import("./desks-navbar").then((m) => ({ default: m.DesksNavbar })),
);

// Panel Notifikasi dan dialog Settings dimuat malas saat pertama dibuka (bagian berat di luar bundel awal).
const LazyNotificationPanel = lazy(() =>
  import("./notification-panel").then((m) => ({ default: m.NotificationPanel })),
);
const LazySettingsDialog = lazy(() =>
  import("./settings/settings-dialog").then((m) => ({ default: m.SettingsDialog })),
);

// Lebar panel K-15 (14,25 rem / 3,25 rem) dan Dock 3,5 rem; bawaan registry (16 rem / 3 rem) ditimpa lewat gaya Provider.
const SHELL_STYLE = {
  "--sidebar-width": "14.25rem",
  "--sidebar-width-icon": "3.25rem",
  "--dock-width": "3.5rem",
} as CSSProperties;

/**
 * Panel Pinned (K3): berdiri di samping Dock. Ciut, tepi kirinya berhenti di balik Dock (`dock - lebar panel`, bukan
 * `-lebar panel` bawaan shadcn yang membuat panel meluncur melintasi Dock); Dock berada di atas panel (z-20 vs z-10).
 * Saat ciut panel `invisible` (tidak dapat difokus); visibilitas ikut bertransisi sehingga tetap terlihat selama
 * meluncur. `motion-reduce` mematikan transisi; lebar `gap` mengikuti transisi shadcn yang sama.
 */
const PINNED_PANEL_CLASS = [
  "left-(--dock-width)",
  "group-data-[collapsible=offcanvas]:left-[calc(var(--dock-width)-var(--sidebar-width))]",
  "group-data-[collapsible=offcanvas]:invisible",
  "transition-[left,right,width,visibility]",
  "motion-reduce:transition-none",
].join(" ");

/** Kerangka navbar halaman Desks saat chunk dimuat atau galat: setinggi navbar, tanpa konten. */
function DesksNavbarFallback() {
  return <header className="h-(--navbar-height) shrink-0 border-b bg-background" />;
}

function ContentSkeleton() {
  return (
    <div className="flex flex-col gap-3 p-4" aria-hidden="true">
      <Skeleton className="h-6 w-48" />
      <Skeleton className="h-4 w-full" />
      <Skeleton className="h-4 w-2/3" />
    </div>
  );
}

/**
 * Menyinkronkan `document.title` dan mengumumkan perubahan halaman ke pembaca layar (wilayah `aria-live`), lalu
 * memindahkan fokus ke `<main>` pada navigasi berikutnya (bukan pemuatan awal). TanStack Router tidak melakukannya.
 */
function RouteAnnouncer({ main }: { main: React.RefObject<HTMLElement | null> }) {
  const title = useRouteTitle();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  // Lokasi router berubah lebih dulu daripada route yang cocok (`useMatches`) selesai dimuat: mengumumkan saat itu
  // akan membacakan judul halaman sebelumnya. Pengumuman menunggu router `idle` (judul sudah milik halaman baru).
  const idle = useRouterState({ select: (s) => s.status === "idle" });
  const [announcement, setAnnouncement] = useState("");
  const previous = useRef<string | null>(null);

  useEffect(() => {
    document.title = title;
  }, [title]);

  useEffect(() => {
    if (!idle) return;
    if (previous.current !== null && previous.current !== pathname) {
      setAnnouncement(title);
      main.current?.focus();
    }
    previous.current = pathname;
  }, [idle, pathname, title, main]);

  return (
    <div aria-live="polite" role="status" data-testid="route-announcer" className="sr-only">
      {announcement}
    </div>
  );
}

/**
 * Kerangka aplikasi (R4, R5): Dock + panel menu (Pinned) atau panel menu + Dock Floating (pil), navbar, dan area
 * konten bergulir (`usePageChrome().scroll`) dengan batas Suspense dan galat sehingga galat satu halaman tidak
 * merobohkan panel dan navbar (R14.5). Di bawah 1024 px panel menjadi `Sheet` (tombol menu) tanpa memandang mode Dock.
 * Keadaan ciut panel disimpan di `localStorage` (`erpora.sidebar`); keadaan Sheet tidak disimpan.
 */
export function AppShell() {
  const { scroll } = usePageChrome();
  const main = useRef<HTMLElement>(null);
  const compact = useIsCompact();
  const dockMode = useDockMode();
  const [panelOpen, setPanelOpen] = useState(readSidebarOpen);
  const [sheetOpen, setSheetOpen] = useState(false);
  // Varian kerangka (K1): halaman Desks memintanya lewat `useShellChrome("desks")`.
  const [chrome, setChrome] = useState<ShellChrome>("default");
  const desksChrome = chrome === "desks";
  const resetKey = useRouterState({ select: (s) => s.location.href });

  // Kembali ke lebar besar: Sheet yang masih terbuka ditutup agar tidak muncul lagi saat menyempit.
  useEffect(() => {
    if (!compact) setSheetOpen(false);
  }, [compact]);

  // Overlay dipasang saat pertama dibuka lalu dipertahankan (animasi tutup tetap berjalan, fokus kembali ke pemicu).
  const [palette, setPalette] = useState({ mounted: false, open: false });
  const [notifications, setNotifications] = useState({ mounted: false, open: false });
  const [settings, setSettings] = useState<{
    mounted: boolean;
    open: boolean;
    section?: SettingsSection;
  }>({ mounted: false, open: false });

  const actions = useMemo<ShellActions>(
    () => ({
      openPalette: () => setPalette({ mounted: true, open: true }),
      openNotifications: () => setNotifications({ mounted: true, open: true }),
      openSettings: (section) => setSettings({ mounted: true, open: true, section }),
      reload: () => window.location.reload(),
    }),
    [],
  );

  useCommandPaletteHotkey(actions.openPalette);
  const { resolved, setPreference } = useTheme();
  useGlobalShortcuts({
    toggleTheme: () => setPreference(resolved === "dark" ? "light" : "dark"),
    showShortcuts: () => actions.openSettings("shortcuts"),
  });

  return (
    <SidebarProvider
      className="h-svh overflow-hidden"
      style={SHELL_STYLE}
      data-shell-chrome={chrome}
      open={compact ? sheetOpen : panelOpen}
      onOpenChange={(open) => {
        if (compact) {
          setSheetOpen(open);
          return;
        }
        setPanelOpen(open);
        writeSidebarOpen(open);
      }}
    >
      <ShellActionsProvider value={actions}>
        <ShellChromeProvider setChrome={setChrome}>
          {/* biome-ignore lint/a11y/useValidAnchor: skip link (K-33): tautan ke #content dengan fokus langsung ke <main> tanpa mengubah hash router */}
          <a
            href="#content"
            className="sr-only z-50 rounded-md bg-background px-3 py-2 text-sm focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
            onClick={(e) => {
              // Tanpa mengubah hash URL (router klien): fokus langsung ke <main>.
              e.preventDefault();
              main.current?.focus();
            }}
          >
            <Message message={{ ns: "platform", key: "shell.skipToContent" }} />
          </a>
          {desksChrome ? null : compact ? (
            <PanelSheet />
          ) : (
            <>
              {dockMode === "pinned" ? (
                <ShellBoundary label="dock" fallback={<DockPlaceholder />}>
                  <Suspense fallback={<DockPlaceholder />}>
                    <LazyDock />
                  </Suspense>
                </ShellBoundary>
              ) : null}
              <Sidebar
                collapsible={dockMode === "pinned" ? "offcanvas" : "icon"}
                className={dockMode === "pinned" ? PINNED_PANEL_CLASS : undefined}
              >
                <DeskPanel variant={dockMode} />
              </Sidebar>
              <CollapseEdge mode={dockMode} />
              {dockMode === "floating" ? (
                <ShellBoundary label="dock floating" fallback={null}>
                  <Suspense fallback={null}>
                    <LazyDockFloating />
                  </Suspense>
                </ShellBoundary>
              ) : null}
            </>
          )}
          <div className="relative flex min-w-0 flex-1 flex-col bg-background">
            {/* Navbar dan pengumuman rute punya batas lokal: galat atau pemuatan teks di sana tidak merobohkan shell. */}
            {desksChrome ? (
              <LocalBoundary label="navbar" resetKey={resetKey} fallback={<DesksNavbarFallback />}>
                <Suspense fallback={<DesksNavbarFallback />}>
                  <LazyDesksNavbar />
                </Suspense>
              </LocalBoundary>
            ) : (
              <LocalBoundary label="navbar" resetKey={resetKey} fallback={<NavbarFallback />}>
                <Suspense fallback={<NavbarFallback />}>
                  <Navbar />
                </Suspense>
              </LocalBoundary>
            )}
            <main
              id="content"
              ref={main}
              tabIndex={-1}
              className={
                scroll
                  ? "min-h-0 flex-1 overflow-auto outline-none"
                  : "min-h-0 flex-1 overflow-hidden outline-none"
              }
            >
              <CatchBoundary
                getResetKey={() => resetKey}
                errorComponent={({ error, reset }) => (
                  <RouteErrorPage error={error} reset={reset} />
                )}
              >
                <Suspense fallback={<ContentSkeleton />}>
                  <Outlet />
                </Suspense>
              </CatchBoundary>
            </main>
          </div>
          {palette.mounted ? (
            <ShellBoundary label="palet perintah" fallback={null}>
              <Suspense fallback={null}>
                <LazyCommandPalette
                  open={palette.open}
                  onOpenChange={(open) => setPalette({ mounted: true, open })}
                />
              </Suspense>
            </ShellBoundary>
          ) : null}
          {notifications.mounted ? (
            <ShellBoundary label="panel notifikasi" fallback={null}>
              <Suspense fallback={null}>
                <LazyNotificationPanel
                  open={notifications.open}
                  onOpenChange={(open) => setNotifications({ mounted: true, open })}
                />
              </Suspense>
            </ShellBoundary>
          ) : null}
          {settings.mounted ? (
            <ShellBoundary label="pengaturan" fallback={null}>
              <Suspense fallback={null}>
                <LazySettingsDialog
                  open={settings.open}
                  initialSection={settings.section}
                  onOpenChange={(open) => setSettings((s) => ({ ...s, open }))}
                />
              </Suspense>
            </ShellBoundary>
          ) : null}
          <LocalBoundary label="pengumuman rute" resetKey={resetKey} fallback={null}>
            <Suspense fallback={null}>
              <RouteAnnouncer main={main} />
            </Suspense>
          </LocalBoundary>
        </ShellChromeProvider>
      </ShellActionsProvider>
    </SidebarProvider>
  );
}

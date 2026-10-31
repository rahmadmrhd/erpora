import { Message, useDockMode, usePageChrome } from "@erpora/platform-web";
import { Sidebar, SidebarProvider, Skeleton } from "@erpora/ui";
import { CatchBoundary, Outlet, useRouterState } from "@tanstack/react-router";
import { type CSSProperties, lazy, Suspense, useEffect, useMemo, useRef, useState } from "react";
import { RouteErrorPage } from "../pages/pages";
import { CollapseEdge } from "./collapse-edge";
import { DeskPanel, PanelSheet } from "./desk-panel";
import { Dock } from "./dock";
import { LocalBoundary } from "./local-boundary";
import { Navbar, NavbarFallback } from "./navbar";
import { useRouteTitle } from "./route-crumbs";
import { type ShellActions, ShellActionsProvider } from "./shell-context";
import { readSidebarOpen, writeSidebarOpen } from "./sidebar-state";
import { useIsCompact } from "./use-compact";

// Dock Floating (pil, hot-edge, pintasan) dimuat malas: bagian berat di luar bundel awal (NFR-04).
const LazyDockFloating = lazy(() =>
  import("./dock-floating").then((m) => ({ default: m.DockFloating })),
);

// Lebar panel K-15 (14,25 rem / 3,25 rem) dan Dock 3,5 rem; bawaan registry (16 rem / 3 rem) ditimpa lewat gaya Provider.
const SHELL_STYLE = {
  "--sidebar-width": "14.25rem",
  "--sidebar-width-icon": "3.25rem",
  "--dock-width": "3.5rem",
} as CSSProperties;

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
  const [announcement, setAnnouncement] = useState("");
  const previous = useRef<string | null>(null);

  useEffect(() => {
    document.title = title;
  }, [title]);

  useEffect(() => {
    if (previous.current !== null && previous.current !== pathname) {
      setAnnouncement(title);
      main.current?.focus();
    }
    previous.current = pathname;
  }, [pathname, title, main]);

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
  const resetKey = useRouterState({ select: (s) => s.location.href });

  // Kembali ke lebar besar: Sheet yang masih terbuka ditutup agar tidak muncul lagi saat menyempit.
  useEffect(() => {
    if (!compact) setSheetOpen(false);
  }, [compact]);

  const actions = useMemo<ShellActions>(
    () => ({
      openPalette: () => {},
      openNotifications: () => {},
      openSettings: () => {},
      reload: () => window.location.reload(),
    }),
    [],
  );

  return (
    <SidebarProvider
      className="h-svh overflow-hidden"
      style={SHELL_STYLE}
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
        {compact ? (
          <PanelSheet />
        ) : (
          <>
            {dockMode === "pinned" ? <Dock /> : null}
            <Sidebar
              collapsible={dockMode === "pinned" ? "offcanvas" : "icon"}
              className={dockMode === "pinned" ? "left-(--dock-width)" : undefined}
            >
              <DeskPanel variant={dockMode} />
            </Sidebar>
            <CollapseEdge mode={dockMode} />
            {dockMode === "floating" ? (
              <Suspense fallback={null}>
                <LazyDockFloating />
              </Suspense>
            ) : null}
          </>
        )}
        <div className="relative flex min-w-0 flex-1 flex-col bg-background">
          {/* Navbar dan pengumuman rute punya batas lokal: galat atau pemuatan teks di sana tidak merobohkan shell. */}
          <LocalBoundary label="navbar" resetKey={resetKey} fallback={<NavbarFallback />}>
            <Suspense fallback={<NavbarFallback />}>
              <Navbar />
            </Suspense>
          </LocalBoundary>
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
              errorComponent={({ error, reset }) => <RouteErrorPage error={error} reset={reset} />}
            >
              <Suspense fallback={<ContentSkeleton />}>
                <Outlet />
              </Suspense>
            </CatchBoundary>
          </main>
        </div>
        <LocalBoundary label="pengumuman rute" resetKey={resetKey} fallback={null}>
          <Suspense fallback={null}>
            <RouteAnnouncer main={main} />
          </Suspense>
        </LocalBoundary>
      </ShellActionsProvider>
    </SidebarProvider>
  );
}

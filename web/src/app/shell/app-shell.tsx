import { Message, usePageChrome } from "@erpora/platform-web";
import { SidebarProvider, Skeleton } from "@erpora/ui";
import { CatchBoundary, Outlet, useRouterState } from "@tanstack/react-router";
import { type CSSProperties, Suspense, useEffect, useRef, useState } from "react";
import { RouteErrorPage } from "../pages/pages";
import { AppSidebar } from "./app-sidebar";
import { LocalBoundary } from "./local-boundary";
import { Navbar, NavbarFallback } from "./navbar";
import { useRouteTitle } from "./route-crumbs";

// Persistensi keadaan sidebar (K-17) dikerjakan shell: Sidebar shadcn persis registry menyimpan di cookie, sehingga
// shell mengendalikan `open` dan menyimpannya di localStorage (kunci dan nilai sama seperti sebelum resync).
const SIDEBAR_STORAGE_KEY = "erpora.sidebar";

function readSidebarOpen(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_STORAGE_KEY) !== "collapsed";
  } catch {
    return true;
  }
}

function writeSidebarOpen(open: boolean): void {
  try {
    localStorage.setItem(SIDEBAR_STORAGE_KEY, open ? "expanded" : "collapsed");
  } catch {
    // penyimpanan diblokir: keadaan tetap berlaku untuk sesi ini
  }
}

// Lebar sidebar K-15 (14,25 rem / 3,25 rem); bawaan registry (16 rem / 3 rem) ditimpa lewat gaya Provider.
const SIDEBAR_STYLE = {
  "--sidebar-width": "14.25rem",
  "--sidebar-width-icon": "3.25rem",
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
 * Kerangka aplikasi: sidebar, navbar, dan area konten bergulir (`usePageChrome().scroll`) dengan batas Suspense dan
 * galat sehingga galat satu halaman tidak merobohkan sidebar dan navbar (R14.5).
 */
export function AppShell() {
  const { scroll } = usePageChrome();
  const main = useRef<HTMLElement>(null);
  const [sidebarOpen, setSidebarOpen] = useState(readSidebarOpen);
  const resetKey = useRouterState({ select: (s) => s.location.href });
  return (
    <SidebarProvider
      className="h-svh overflow-hidden"
      style={SIDEBAR_STYLE}
      open={sidebarOpen}
      onOpenChange={(open) => {
        setSidebarOpen(open);
        writeSidebarOpen(open);
      }}
    >
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
      <AppSidebar />
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
    </SidebarProvider>
  );
}

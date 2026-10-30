import {
  Kbd,
  KbdGroup,
  Separator,
  SidebarTrigger,
  Tooltip,
  TooltipContent,
  TooltipTrigger,
  useIsMobile,
  useUiLabel,
} from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { Breadcrumbs } from "./breadcrumbs";
import { PageActionsView } from "./page-actions-view";
import { useRouteCrumbs } from "./route-crumbs";

/** Navbar 48 px lengket di atas area konten: tombol collapse (dengan petunjuk Ctrl B), jejak halaman, aksi halaman. */
export function Navbar() {
  const t = useUiLabel();
  const compact = useIsMobile();
  const { crumbs } = useRouteCrumbs();
  return (
    <header className="sticky top-0 z-10 flex h-(--navbar-height) shrink-0 items-center gap-2.5 border-b bg-background px-3.5">
      <Tooltip>
        <TooltipTrigger asChild>
          <SidebarTrigger aria-label={t("sidebar.toggle")} />
        </TooltipTrigger>
        <TooltipContent>
          <span className="flex items-center gap-2">
            {t("sidebar.toggle")}
            <KbdGroup>
              <Kbd>Ctrl</Kbd>
              <Kbd>B</Kbd>
            </KbdGroup>
          </span>
        </TooltipContent>
      </Tooltip>
      <Separator orientation="vertical" className="h-4" />
      <Breadcrumbs crumbs={crumbs} compact={compact} />
      <PageActionsView />
    </header>
  );
}

const reported = new WeakSet<object>();

/** Mencatat galat render Navbar sekali per objek galat (nama saja: tanpa pesan/stack/PII); dipakai `onCatch` batas lokal. */
export function reportNavbarError(error: unknown): void {
  if (typeof error === "object" && error !== null) {
    if (reported.has(error)) return;
    reported.add(error);
  }
  const name = error instanceof Error ? error.name : "Error";
  console.error(`[erpora] navbar gagal dirender: ${name}`);
}

/**
 * Cadangan Navbar saat memuat atau galat: tombol collapse tetap ada (di layar kecil sidebar hanya dibuka lewat tombol ini)
 * dan jejak minimal berupa path.
 */
export function NavbarFallback() {
  const t = useUiLabel();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  return (
    <header className="sticky top-0 z-10 flex h-(--navbar-height) shrink-0 items-center gap-2.5 border-b bg-background px-3.5">
      <SidebarTrigger aria-label={t("sidebar.toggle")} />
      <Separator orientation="vertical" className="h-4" />
      <span className="truncate text-sm text-muted-foreground">{pathname}</span>
    </header>
  );
}

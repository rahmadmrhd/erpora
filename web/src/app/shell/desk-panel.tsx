import { Message, useCurrentDesk, useDesks, useTranslation } from "@erpora/platform-web";
import {
  Button,
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarSeparator,
  useSidebar,
  useUiLabel,
} from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { useEffect, useRef } from "react";
import { DESK_PANEL_ID } from "./collapse-edge";
import { DeskMenuTree, resetDeskMenuChunks } from "./desk-panel-menu";
import { STRIP_CENTER_CLASS } from "./dock-sizing";
import { FixedGroup } from "./fixed-group";
import { LogoPopover } from "./logo-popover";
import { ShellBoundary } from "./shell-boundary";
import { UserMenu } from "./user-menu";

export { findActiveItem } from "./desk-panel-menu";

/** `pinned`: panel di sebelah Dock; `floating`: satu-satunya sidebar (logo di kepala, pengguna di dasar); `sheet`: < 1024 px. */
export type PanelVariant = "pinned" | "floating" | "sheet";

/** Pengganti menu Desk bila katalog atau chunk-nya gagal dimuat: pesan kecil dan tombol coba lagi. */
function DeskMenuLoadError({ retry }: { retry: () => void }) {
  const { t } = useTranslation("platform");
  return (
    <SidebarGroup className="group-data-[collapsible=icon]:hidden">
      <p role="alert" className="px-2 text-sm text-muted-foreground">
        {t("panel.loadError")}
      </p>
      <Button type="button" variant="outline" size="sm" className="mx-2 mt-2" onClick={retry}>
        {t("shell.retry")}
      </Button>
    </SidebarGroup>
  );
}

/** Label kecil nama Desk dan menu Desk saat ini (R5.2 butir ii); Dashboard selalu item pertama (urutan server). */
function DeskMenu({ variant }: { variant: PanelVariant }) {
  const { t } = useTranslation("platform");
  const { status, refetch } = useDesks();
  const { desk } = useCurrentDesk();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const { state, isMobile } = useSidebar();
  const iconStrip = variant === "floating" && state === "collapsed" && !isMobile;

  if (status === "loading") {
    return (
      <SidebarGroup aria-busy="true">
        <SidebarMenu className={STRIP_CENTER_CLASS}>
          {[0, 1, 2].map((i) => (
            <SidebarMenuItem key={i}>
              <SidebarMenuSkeleton showIcon />
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarGroup>
    );
  }
  if (status === "error") {
    return (
      <SidebarGroup className="group-data-[collapsible=icon]:hidden">
        <p role="alert" className="px-2 text-sm text-muted-foreground">
          {t("panel.loadError")}
        </p>
        <Button type="button" variant="outline" size="sm" className="mx-2 mt-2" onClick={refetch}>
          {t("shell.retry")}
        </Button>
      </SidebarGroup>
    );
  }
  if (desk === null) {
    return (
      <SidebarGroup className="group-data-[collapsible=icon]:hidden">
        <p className="px-2 text-sm text-muted-foreground">{t("panel.noDesk")}</p>
      </SidebarGroup>
    );
  }

  return (
    <ShellBoundary
      label="menu Desk"
      onRetry={resetDeskMenuChunks}
      fallback={(retry) => <DeskMenuLoadError retry={retry} />}
    >
      <SidebarGroup>
        <SidebarGroupLabel>
          <Message message={desk.label} />
        </SidebarGroupLabel>
        <SidebarGroupContent>
          <DeskMenuTree desk={desk} pathname={pathname} iconStrip={iconStrip} />
        </SidebarGroupContent>
      </SidebarGroup>
    </ShellBoundary>
  );
}

/**
 * Isi panel menu (R5): tanpa header Desk. `floating` dan `sheet`: grup tetap, pemisah, lalu menu Desk saat ini, dengan
 * logo (popover) di kepala dan pengguna di dasar. `pinned` (K6): hanya label dan menu Desk saat ini; logo, Cari,
 * Notifikasi, ToDo, dan pengguna ada di Dock. Menu dibungkus `<nav aria-label>` sendiri karena `Sidebar` shadcn
 * tidak merender landmark.
 */
export function DeskPanel({ variant }: { variant: PanelVariant }) {
  const ui = useUiLabel();
  const withChrome = variant !== "pinned";
  return (
    <>
      {withChrome ? (
        <SidebarHeader>
          <LogoPopover variant="panel" />
        </SidebarHeader>
      ) : null}
      <SidebarContent className="overflow-x-hidden">
        <nav
          aria-label={ui("sidebar.label")}
          id={DESK_PANEL_ID}
          className="flex min-w-0 flex-col gap-2"
        >
          {withChrome ? (
            <>
              <FixedGroup />
              <SidebarSeparator className="data-[orientation=horizontal]:w-auto" />
            </>
          ) : null}
          <DeskMenu variant={variant} />
        </nav>
      </SidebarContent>
      {withChrome ? (
        <SidebarFooter>
          <UserMenu variant="panel" />
        </SidebarFooter>
      ) : null}
    </>
  );
}

/**
 * Panel sebagai `Sheet` di bawah 1024 px (R4.6, R5.8), dibuka tombol ☰ (`SidebarTrigger`/Ctrl+B) tanpa memandang
 * mode Dock; tanpa pembatas. Di bawah 768 px `SidebarProvider` memakai `openMobile`; antara 768 dan 1023 px memakai
 * `open` terkendali (AppShell memetakannya ke keadaan sheet). Menutup otomatis setelah navigasi.
 */
export function PanelSheet() {
  const { isMobile, openMobile, setOpenMobile, open, setOpen } = useSidebar();
  const ui = useUiLabel();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const sheetOpen = isMobile ? openMobile : open;
  const setSheetOpen = isMobile ? setOpenMobile : setOpen;

  const previous = useRef(pathname);
  useEffect(() => {
    if (previous.current === pathname) return;
    previous.current = pathname;
    setSheetOpen(false);
  }, [pathname]);

  return (
    <Sheet open={sheetOpen} onOpenChange={setSheetOpen}>
      <SheetContent
        side="left"
        data-slot="panel-sheet"
        className="w-(--sidebar-width) bg-sidebar p-0 text-sidebar-foreground [&>button]:hidden"
        style={{ "--sidebar-width": "18rem" } as React.CSSProperties}
      >
        <SheetHeader className="sr-only">
          <SheetTitle>{ui("sidebar.title")}</SheetTitle>
          <SheetDescription>{ui("sidebar.description")}</SheetDescription>
        </SheetHeader>
        <div className="flex h-full w-full flex-col">
          <DeskPanel variant="sheet" />
        </div>
      </SheetContent>
    </Sheet>
  );
}

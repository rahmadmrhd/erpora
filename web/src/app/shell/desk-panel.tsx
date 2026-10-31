import { Link } from "@erpora/module-sdk";
import {
  type Desk,
  type DeskMenuGroup,
  type DeskMenuItem,
  Message,
  useCurrentDesk,
  useDesks,
  useMessage,
  useTranslation,
} from "@erpora/platform-web";
import {
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Popover,
  PopoverContent,
  PopoverTrigger,
  resolveIcon,
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
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarSeparator,
  useSidebar,
  useUiLabel,
} from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import { DESK_PANEL_ID } from "./collapse-edge";
import { FixedGroup } from "./fixed-group";
import { LogoPopover } from "./logo-popover";
import { NavIcon } from "./nav-icon";
import { UserMenu } from "./user-menu";

/** `pinned`: panel di sebelah Dock; `floating`: satu-satunya sidebar (logo di kepala, pengguna di dasar); `sheet`: < 1024 px. */
export type PanelVariant = "pinned" | "floating" | "sheet";

const trim = (p: string): string => (p.length > 1 && p.endsWith("/") ? p.slice(0, -1) : p);

function leavesOf(desk: Desk): DeskMenuItem[] {
  return desk.menus.flatMap((m) => (m.type === "item" ? [m] : m.children));
}

/**
 * Item menu yang aktif untuk `pathname`: Dashboard (terkunci) hanya cocok persis; item lain cocok pada `path === to`
 * atau awalan `to + "/"` (segmen-aman); `to` terpanjang menang. `null` bila tidak ada yang cocok.
 */
export function findActiveItem(desk: Desk, pathname: string): DeskMenuItem | null {
  const path = trim(pathname);
  let best: DeskMenuItem | null = null;
  for (const item of leavesOf(desk)) {
    const to = trim(item.to);
    const matches = item.locked === true ? path === to : path === to || path.startsWith(`${to}/`);
    if (matches && (best === null || to.length > trim(best.to).length)) best = item;
  }
  return best;
}

function Chevron() {
  const Icon = resolveIcon("chevron-right");
  return Icon ? (
    <Icon
      aria-hidden="true"
      className="ml-auto transition-transform group-data-[state=open]/collapsible:rotate-90 motion-reduce:transition-none"
    />
  ) : null;
}

function ItemLink({
  node,
  active,
  exact,
}: {
  node: DeskMenuItem;
  active: boolean;
  exact: boolean;
}) {
  const text = useMessage(node.label);
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={text}>
        <Link
          to={node.to}
          activeOptions={{ exact: true }}
          aria-current={exact ? "page" : undefined}
        >
          <NavIcon name={node.icon} label={text} />
          <span>{text}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function SubLink({ node, active, exact }: { node: DeskMenuItem; active: boolean; exact: boolean }) {
  return (
    <SidebarMenuSubItem>
      <SidebarMenuSubButton asChild isActive={active}>
        <Link
          to={node.to}
          activeOptions={{ exact: true }}
          aria-current={exact ? "page" : undefined}
        >
          <span>
            <Message message={node.label} />
          </span>
        </Link>
      </SidebarMenuSubButton>
    </SidebarMenuSubItem>
  );
}

function GroupNode({
  node,
  activeId,
  pathname,
  iconStrip,
}: {
  node: DeskMenuGroup;
  activeId: string | null;
  pathname: string;
  iconStrip: boolean;
}) {
  const text = useMessage(node.label);
  const hasActive = node.children.some((c) => c.id === activeId);
  const [open, setOpen] = useState(hasActive);
  useEffect(() => {
    if (hasActive) setOpen(true);
  }, [hasActive]);

  const trigger = (
    <SidebarMenuButton
      tooltip={text}
      className={hasActive ? "font-medium" : undefined}
      data-group-active={hasActive || undefined}
    >
      <NavIcon name={node.icon} label={text} />
      <span>{text}</span>
      <Chevron />
    </SidebarMenuButton>
  );

  // Strip ikon (Floating terciut): anak tampil di popover di samping ikon grup (nama grup sebagai tooltip).
  if (iconStrip) {
    return (
      <SidebarMenuItem>
        <Popover>
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
          <PopoverContent side="right" align="start" className="w-48 p-1">
            <ul className="flex flex-col gap-0.5">
              {node.children.map((c) => (
                <li key={c.id}>
                  <Link
                    to={c.to}
                    activeOptions={{ exact: true }}
                    aria-current={trim(pathname) === trim(c.to) ? "page" : undefined}
                    className="flex h-8 items-center rounded-md px-2 text-sm hover:bg-accent aria-[current=page]:bg-accent aria-[current=page]:font-medium"
                  >
                    <Message message={c.label} />
                  </Link>
                </li>
              ))}
            </ul>
          </PopoverContent>
        </Popover>
      </SidebarMenuItem>
    );
  }

  return (
    <Collapsible asChild open={open} onOpenChange={setOpen} className="group/collapsible">
      <SidebarMenuItem>
        <CollapsibleTrigger asChild>{trigger}</CollapsibleTrigger>
        <CollapsibleContent>
          <SidebarMenuSub>
            {node.children.map((c) => (
              <SubLink
                key={c.id}
                node={c}
                active={c.id === activeId}
                exact={trim(pathname) === trim(c.to)}
              />
            ))}
          </SidebarMenuSub>
        </CollapsibleContent>
      </SidebarMenuItem>
    </Collapsible>
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
        <SidebarMenu>
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

  const active = findActiveItem(desk, pathname);
  const activeId = active?.id ?? null;
  return (
    <SidebarGroup>
      <SidebarGroupLabel>
        <Message message={desk.label} />
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {desk.menus.map((m) =>
            m.type === "item" ? (
              <ItemLink
                key={m.id}
                node={m}
                active={m.id === activeId}
                exact={trim(pathname) === trim(m.to)}
              />
            ) : (
              <GroupNode
                key={m.id}
                node={m}
                activeId={activeId}
                pathname={pathname}
                iconStrip={iconStrip}
              />
            ),
          )}
        </SidebarMenu>
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

/**
 * Isi panel menu (R5): tanpa header Desk; berurutan grup tetap, pemisah, menu Desk saat ini. `floating` dan `sheet`
 * menambah logo (popover) di kepala dan pengguna di dasar; pada `pinned` keduanya ada di Dock. Menu dibungkus
 * `<nav aria-label>` sendiri karena `Sidebar` shadcn tidak merender landmark.
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
      <SidebarContent>
        <nav aria-label={ui("sidebar.label")} id={DESK_PANEL_ID} className="flex flex-col gap-2">
          <FixedGroup />
          <SidebarSeparator />
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

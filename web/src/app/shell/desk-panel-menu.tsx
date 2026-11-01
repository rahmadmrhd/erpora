import { Link } from "@erpora/module-sdk";
import {
  type Desk,
  type DeskMenuGroup,
  type DeskMenuItem,
  deskContains,
  Message,
  useMessage,
} from "@erpora/platform-web";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  resolveIcon,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
} from "@erpora/ui";
import { Suspense, useEffect, useState } from "react";
import { STRIP_CENTER_CLASS } from "./dock-sizing";
import { NavIcon } from "./nav-icon";
import { retryableLazy } from "./retryable-lazy";

const LazyIconStripGroup = retryableLazy(() =>
  import("./icon-strip-group").then((m) => ({ default: m.IconStripGroup })),
);

/** Memuat ulang chunk grup strip ikon setelah gagal (dipanggil batas galat menu Desk saat mencoba lagi). */
export function resetDeskMenuChunks(): void {
  LazyIconStripGroup.reset();
}

const trim = (p: string): string => (p.length > 1 && p.endsWith("/") ? p.slice(0, -1) : p);

function leavesOf(desk: Desk): DeskMenuItem[] {
  return desk.menus.flatMap((m) => (m.type === "item" ? [m] : m.children));
}

/**
 * Item menu yang aktif untuk `pathname`: item yang cocok menurut aturan `@erpora/platform-web` (`deskContains`:
 * Dashboard terkunci hanya persis, item lain persis atau awalan segmen-aman) dengan `to` terpanjang menang (seri:
 * yang pertama). `null` bila tidak ada yang cocok. Aturan pencocokan tidak diduplikasi di sini: tiap item diuji
 * sebagai Desk satu-item lewat `deskContains`.
 */
export function findActiveItem(desk: Desk, pathname: string): DeskMenuItem | null {
  let best: DeskMenuItem | null = null;
  for (const item of leavesOf(desk)) {
    if (!deskContains({ ...desk, menus: [item] }, pathname)) continue;
    if (best === null || trim(item.to).length > trim(best.to).length) best = item;
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
  tooltip,
  onSelect,
}: {
  node: DeskMenuItem;
  active: boolean;
  exact: boolean;
  tooltip: boolean;
  onSelect?: () => void;
}) {
  const text = useMessage(node.label);
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={tooltip ? text : undefined}>
        <Link
          to={node.to}
          activeOptions={{ exact: true }}
          aria-current={exact ? "page" : undefined}
          onClick={onSelect}
        >
          <NavIcon name={node.icon} label={text} />
          <span>{text}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function SubLink({
  node,
  active,
  exact,
  onSelect,
}: {
  node: DeskMenuItem;
  active: boolean;
  exact: boolean;
  onSelect?: () => void;
}) {
  return (
    <SidebarMenuSubItem>
      <SidebarMenuSubButton asChild isActive={active}>
        <Link
          to={node.to}
          activeOptions={{ exact: true }}
          aria-current={exact ? "page" : undefined}
          onClick={onSelect}
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
  tooltip,
  onSelect,
}: {
  node: DeskMenuGroup;
  activeId: string | null;
  pathname: string;
  iconStrip: boolean;
  tooltip: boolean;
  onSelect?: () => void;
}) {
  const text = useMessage(node.label);
  const hasActive = node.children.some((c) => c.id === activeId);
  const [open, setOpen] = useState(hasActive);
  useEffect(() => {
    if (hasActive) setOpen(true);
  }, [hasActive]);

  const trigger = (
    <SidebarMenuButton
      tooltip={tooltip ? text : undefined}
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
      <Suspense fallback={<SidebarMenuItem>{trigger}</SidebarMenuItem>}>
        <LazyIconStripGroup node={node} pathname={pathname} trigger={trigger} />
      </Suspense>
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
                onSelect={onSelect}
              />
            ))}
          </SidebarMenuSub>
        </CollapsibleContent>
      </SidebarMenuItem>
    </Collapsible>
  );
}

/**
 * Pohon menu sebuah Desk bergaya item sidebar (Dashboard, item, dan grup yang dapat dibuka). Dipakai panel menu
 * (Pinned, Floating, Sheet) dan popover Desk di Dock Pinned (K8). `onSelect` dipanggil saat sebuah item dipilih
 * (popover memakainya untuk menetapkan Desk dan menutup diri); `tooltip` memberi tooltip pada mode strip ikon.
 */
export function DeskMenuTree({
  desk,
  pathname,
  iconStrip = false,
  tooltip = true,
  onSelect,
}: {
  desk: Desk;
  pathname: string;
  iconStrip?: boolean;
  tooltip?: boolean;
  onSelect?: () => void;
}) {
  const activeId = findActiveItem(desk, pathname)?.id ?? null;
  return (
    <SidebarMenu className={STRIP_CENTER_CLASS}>
      {desk.menus.map((m) =>
        m.type === "item" ? (
          <ItemLink
            key={m.id}
            node={m}
            active={m.id === activeId}
            exact={trim(pathname) === trim(m.to)}
            tooltip={tooltip}
            onSelect={onSelect}
          />
        ) : (
          <GroupNode
            key={m.id}
            node={m}
            activeId={activeId}
            pathname={pathname}
            iconStrip={iconStrip}
            tooltip={tooltip}
            onSelect={onSelect}
          />
        ),
      )}
    </SidebarMenu>
  );
}

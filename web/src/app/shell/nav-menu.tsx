import { Link } from "@erpora/module-sdk";
import {
  activeLeaf,
  groupHasActive,
  Message,
  type NavNode,
  useMessage,
} from "@erpora/platform-web";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  EmptyState,
  Popover,
  PopoverContent,
  PopoverTrigger,
  resolveIcon,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from "@erpora/ui";
import { useEffect, useState } from "react";
import { NavIcon } from "./nav-icon";

type NavItem = Extract<NavNode, { type: "item" }>;
type NavGroup = Extract<NavNode, { type: "group" }>;

const PLATFORM = "platform";

/** Navigasi bawaan platform (K-27): Beranda di atas modul, Pengaturan > Modul di bawahnya. */
export function buildShellNav(moduleNodes: readonly NavNode[]): readonly NavNode[] {
  return [
    {
      type: "item",
      id: "home",
      label: { ns: "platform", key: "nav.home" },
      to: "/",
      icon: "home",
      moduleKey: PLATFORM,
    },
    ...moduleNodes,
    {
      type: "group",
      id: "settings",
      label: { ns: "platform", key: "nav.settings" },
      icon: "settings",
      moduleKey: PLATFORM,
      children: [
        {
          type: "item",
          id: "settings-modules",
          label: { ns: "platform", key: "nav.settingsModules" },
          to: "/settings/modules",
          icon: "blocks",
          moduleKey: PLATFORM,
        },
      ],
    },
  ];
}

function ItemLink({ node, active }: { node: NavItem; active: boolean }) {
  const text = useMessage(node.label);
  return (
    <SidebarMenuItem>
      <SidebarMenuButton asChild isActive={active} tooltip={text}>
        <Link to={node.to} activeOptions={{ exact: true }}>
          <NavIcon name={node.icon} label={text} />
          <span>{text}</span>
        </Link>
      </SidebarMenuButton>
    </SidebarMenuItem>
  );
}

function SubLink({ node, active }: { node: NavItem; active: boolean }) {
  return (
    <SidebarMenuSubItem>
      <SidebarMenuSubButton asChild isActive={active}>
        <Link to={node.to} activeOptions={{ exact: true }}>
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
  pathname,
  nodes,
}: {
  node: NavGroup;
  pathname: string;
  nodes: readonly NavNode[];
}) {
  const text = useMessage(node.label);
  const { state, isMobile } = useSidebar();
  const hasActive = groupHasActive(node, pathname, nodes);
  const active = activeLeaf(nodes, pathname);
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

  // Terlipat ke ikon: anak tampil di popover di samping ikon grup (nama grup sebagai tooltip).
  if (state === "collapsed" && !isMobile) {
    return (
      <SidebarMenuItem>
        <Popover>
          <PopoverTrigger asChild>{trigger}</PopoverTrigger>
          <PopoverContent side="right" align="start" className="w-48 p-1">
            <ul className="flex flex-col gap-0.5">
              {node.children.map((c) => (
                <li key={`${c.moduleKey}/${c.id}`}>
                  <Link
                    to={c.to}
                    activeOptions={{ exact: true }}
                    aria-current={active === c ? "page" : undefined}
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
              <SubLink key={`${c.moduleKey}/${c.id}`} node={c} active={active === c} />
            ))}
          </SidebarMenuSub>
        </CollapsibleContent>
      </SidebarMenuItem>
    </Collapsible>
  );
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

/**
 * Daftar menu: Beranda, node modul aktif dari registry, dan Pengaturan. Item aktif unik (P3); grup terbuka bila
 * memuat anak aktif. Tanpa modul: keadaan kosong yang jelas (R9.6).
 */
export function NavMenu({
  moduleNodes,
  pathname,
}: {
  moduleNodes: readonly NavNode[];
  pathname: string;
}) {
  const nodes = buildShellNav(moduleNodes);
  const active = activeLeaf(nodes, pathname);
  const emptyTitle = useMessage({ ns: "platform", key: "nav.empty" });
  return (
    <SidebarGroup>
      <SidebarGroupLabel>
        <Message message={{ ns: "platform", key: "nav.menu" }} />
      </SidebarGroupLabel>
      <SidebarGroupContent>
        <SidebarMenu>
          {nodes.map((n) =>
            n.type === "item" ? (
              <ItemLink key={`${n.moduleKey}/${n.id}`} node={n} active={active === n} />
            ) : (
              <GroupNode
                key={`${n.moduleKey}/${n.id}`}
                node={n}
                pathname={pathname}
                nodes={nodes}
              />
            ),
          )}
        </SidebarMenu>
        {moduleNodes.length === 0 ? (
          <EmptyState className="p-3 group-data-[collapsible=icon]:hidden" title={emptyTitle} />
        ) : null}
      </SidebarGroupContent>
    </SidebarGroup>
  );
}

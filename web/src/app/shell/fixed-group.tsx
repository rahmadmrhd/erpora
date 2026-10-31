import { Link } from "@erpora/module-sdk";
import { type FixedItem, formatBadgeCount, useFixedItems, useMessage } from "@erpora/platform-web";
import {
  Kbd,
  KbdGroup,
  SidebarGroup,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { useId } from "react";
import { NavIcon } from "./nav-icon";
import { type ShellActions, useShellActions } from "./shell-context";

/** Perintah yang dikenal shell untuk item tetap berjenis `command`; perintah lain diabaikan (tanpa galat). */
export function runCommand(actions: ShellActions, command: string): void {
  if (command === "search") actions.openPalette();
  else if (command === "notifications") actions.openNotifications();
}

function Row({ item }: { item: FixedItem }) {
  const text = useMessage(item.label);
  const actions = useShellActions();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const count = item.useBadge();
  const badge = formatBadgeCount(count);
  const badgeId = useId();
  const describedBy = badge === "" ? undefined : badgeId;

  const content = (
    <>
      <NavIcon name={item.icon} label={text} />
      <span>{text}</span>
      {item.shortcut ? (
        <KbdGroup className="ml-auto group-data-[collapsible=icon]:hidden">
          {item.shortcut.map((key) => (
            <Kbd key={key}>{key}</Kbd>
          ))}
        </KbdGroup>
      ) : null}
    </>
  );

  return (
    <SidebarMenuItem>
      {item.action.kind === "link" ? (
        <SidebarMenuButton
          asChild
          tooltip={text}
          isActive={pathname === item.action.to || pathname.startsWith(`${item.action.to}/`)}
        >
          <Link to={item.action.to} activeOptions={{ exact: true }} aria-describedby={describedBy}>
            {content}
          </Link>
        </SidebarMenuButton>
      ) : (
        <SidebarMenuButton
          type="button"
          tooltip={text}
          aria-describedby={describedBy}
          onClick={() => {
            if (item.action.kind === "command") runCommand(actions, item.action.command);
          }}
        >
          {content}
        </SidebarMenuButton>
      )}
      {badge === "" ? null : <SidebarMenuBadge id={badgeId}>{badge}</SidebarMenuBadge>}
    </SidebarMenuItem>
  );
}

/**
 * Grup tetap di puncak panel menu (R5.2, R5.3): Cari, Notifikasi (lencana), ToDo (lencana, tautan) dan item lain yang
 * didaftarkan lewat `registerFixedItem`. Cari dan Notifikasi tidak punya rute: membuka palet dan panel.
 */
export function FixedGroup() {
  const items = useFixedItems();
  return (
    <SidebarGroup>
      <SidebarMenu>
        {items.map((item) => (
          <Row key={item.id} item={item} />
        ))}
      </SidebarMenu>
    </SidebarGroup>
  );
}

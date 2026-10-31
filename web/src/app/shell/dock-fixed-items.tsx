import { Link } from "@erpora/module-sdk";
import { type FixedItem, formatBadgeCount, useFixedItems, useMessage } from "@erpora/platform-web";
import { Button, Kbd, KbdGroup, Tooltip, TooltipContent, TooltipTrigger } from "@erpora/ui";
import { useRouterState } from "@tanstack/react-router";
import { useId } from "react";
import { DOCK_BUTTON_CLASS } from "./dock-sizing";
import { runCommand } from "./fixed-group";
import { NavIcon } from "./nav-icon";
import { useShellActions } from "./shell-context";

const BUTTON_CLASS = `relative ${DOCK_BUTTON_CLASS}`;

/** Satu item tetap sebagai tombol ikon di Dock Pinned: tooltip (label dan pintasan) dan lencana jumlah. */
function FixedIconButton({ item }: { item: FixedItem }) {
  const text = useMessage(item.label);
  const actions = useShellActions();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const badge = formatBadgeCount(item.useBadge());
  const badgeId = useId();
  const describedBy = badge === "" ? undefined : badgeId;
  const icon = <NavIcon name={item.icon} label={text} />;
  const action = item.action;
  const active =
    action.kind === "link" && (pathname === action.to || pathname.startsWith(`${action.to}/`));

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        {action.kind === "link" ? (
          <Button
            asChild
            variant="ghost"
            size="icon"
            className={BUTTON_CLASS}
            data-active={active ? "true" : undefined}
          >
            <Link
              to={action.to}
              activeOptions={{ exact: true }}
              aria-label={text}
              aria-describedby={describedBy}
            >
              {icon}
              {badge === "" ? null : <DockBadge id={badgeId}>{badge}</DockBadge>}
            </Link>
          </Button>
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className={BUTTON_CLASS}
            aria-label={text}
            aria-describedby={describedBy}
            onClick={() => runCommand(actions, action.command)}
          >
            {icon}
            {badge === "" ? null : <DockBadge id={badgeId}>{badge}</DockBadge>}
          </Button>
        )}
      </TooltipTrigger>
      <TooltipContent side="right">
        {text}
        {item.shortcut ? (
          <KbdGroup className="ml-2">
            {item.shortcut.map((key) => (
              <Kbd key={key}>{key}</Kbd>
            ))}
          </KbdGroup>
        ) : null}
      </TooltipContent>
    </Tooltip>
  );
}

/** Lencana jumlah di pojok kanan atas tombol ikon (dekoratif; jumlahnya dibacakan lewat `aria-describedby`). */
export function DockBadge({ id, children }: { id: string; children: string }) {
  return (
    <span
      id={id}
      data-slot="dock-badge"
      className="pointer-events-none absolute -top-0.5 -right-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-sidebar-primary px-1 text-[10px] font-medium text-sidebar-primary-foreground tabular-nums"
    >
      {children}
    </span>
  );
}

/**
 * Item tetap (Cari, Notifikasi, ToDo, dan yang didaftarkan lewat `registerFixedItem`) sebagai rel ikon tepat di bawah
 * logo Dock Pinned (K6). Sumbernya registri yang sama dengan `FixedGroup` di panel (Floating dan Sheet).
 */
export function DockFixedItems() {
  const items = useFixedItems();
  return (
    <ul data-slot="dock-fixed-items" className="flex flex-col items-center gap-1">
      {items.map((item) => (
        <li key={item.id}>
          <FixedIconButton item={item} />
        </li>
      ))}
    </ul>
  );
}

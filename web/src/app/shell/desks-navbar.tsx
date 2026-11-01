import {
  type FixedItem,
  formatBadgeCount,
  useFixedItems,
  useMessage,
  useTranslation,
} from "@erpora/platform-web";
import { Button, Kbd, KbdGroup, resolveIcon } from "@erpora/ui";
import { useId } from "react";
import { DockBadge } from "./dock-fixed-items";
import { LogoPopover } from "./logo-popover";
import { useShellActions } from "./shell-context";
import { UserMenu } from "./user-menu";

function SearchIcon() {
  const Icon = resolveIcon("search");
  return Icon ? <Icon aria-hidden="true" className="size-4 shrink-0" /> : null;
}

/** Tombol Cari: ikon saja di layar kecil, mulai `lg` selebar 14 rem (16 rem di `xl`) berlatar muted dengan petunjuk Ctrl K. */
function SearchButton() {
  const { t } = useTranslation("platform");
  const actions = useShellActions();
  return (
    <Button
      type="button"
      variant="outline"
      aria-keyshortcuts="Control+K"
      onClick={actions.openPalette}
      className="h-9 w-9 justify-center bg-muted/50 px-0 text-muted-foreground lg:w-56 lg:justify-between lg:px-3 xl:w-64"
    >
      <span className="flex items-center gap-2">
        <SearchIcon />
        <span className="sr-only lg:not-sr-only">{t("desksNavbar.search")}</span>
      </span>
      <KbdGroup className="hidden lg:inline-flex">
        <Kbd>Ctrl</Kbd>
        <Kbd>K</Kbd>
      </KbdGroup>
    </Button>
  );
}

function BellButton({ item }: { item: FixedItem }) {
  const text = useMessage(item.label);
  const actions = useShellActions();
  const badge = formatBadgeCount(item.useBadge());
  const badgeId = useId();
  const Icon = resolveIcon(item.icon);
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      data-slot="desks-navbar-notifications"
      aria-label={text}
      aria-describedby={badge === "" ? undefined : badgeId}
      onClick={actions.openNotifications}
      className="relative size-9 [&_svg]:size-5"
    >
      {Icon ? <Icon aria-hidden="true" /> : null}
      {badge === "" ? null : <DockBadge id={badgeId}>{badge}</DockBadge>}
    </Button>
  );
}

/** Lonceng notifikasi dari item tetap bawaan `notifications` (lencana dan label sama dengan Dock dan panel). */
function Notifications() {
  const item = useFixedItems().find((i) => i.id === "notifications");
  return item ? <BellButton item={item} /> : null;
}

/**
 * Navbar halaman Desks (K1): kiri logo ERPora (popover logo), tengah tombol Cari (membuka palet), kanan lonceng
 * notifikasi (membuka panel Notifikasi) dan avatar (menu pengguna). Tanpa breadcrumb, tombol sidebar, dan pemisah:
 * halaman Desks tidak punya Dock maupun panel. Dirender `AppShell` menggantikan `Navbar` selama sebuah halaman
 * meminta `useShellChrome("desks")`. Dimuat malas (bukan bagian bundel awal).
 */
export function DesksNavbar() {
  return (
    <header
      data-slot="desks-navbar"
      className="sticky top-0 z-10 grid h-(--navbar-height) shrink-0 grid-cols-[1fr_auto_1fr] items-center gap-2.5 border-b bg-background px-3.5"
    >
      <div className="flex min-w-0 items-center justify-self-start">
        <LogoPopover variant="navbar" />
      </div>
      <SearchButton />
      <div className="flex items-center gap-1 justify-self-end">
        <Notifications />
        <UserMenu variant="navbar" />
      </div>
    </header>
  );
}

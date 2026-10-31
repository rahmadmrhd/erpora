import { useNavigate } from "@erpora/module-sdk";
import {
  formatRelativeTime,
  type NotificationsState,
  safeInternalHref,
  useNotifications,
  useTranslation,
} from "@erpora/platform-web";
import {
  Alert,
  AlertDescription,
  Button,
  cn,
  resolveIcon,
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  Spinner,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@erpora/ui";
import { type CSSProperties, type ReactNode, useState } from "react";
import { EmptyNotice } from "../components/notices";
import { type NotificationPlacement, useNotificationAnchor } from "./notification-anchor";
import { useReturnFocus } from "./overlay-focus";

function Rows({
  n,
  onNavigate,
  onFailure,
}: {
  n: NotificationsState;
  onNavigate: (href: string) => void;
  onFailure: (action: Promise<void>) => void;
}) {
  const { locale } = useTranslation("platform");
  return (
    <ul className="divide-y">
      {n.items.map((item) => {
        const href = safeInternalHref(item.href);
        return (
          <li key={item.id}>
            <button
              type="button"
              className="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-accent focus-visible:bg-accent focus-visible:outline-none"
              onClick={() => {
                onFailure(n.markRead(item.id));
                if (href !== null) onNavigate(href);
              }}
            >
              <span
                aria-hidden="true"
                data-unread={!item.read}
                className="mt-1.5 size-2 shrink-0 rounded-full bg-transparent data-[unread=true]:bg-info"
              />
              <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                <span className="text-sm font-medium">{item.title}</span>
                <span className="line-clamp-2 text-sm text-muted-foreground">{item.message}</span>
                <time dateTime={item.createdAt} className="text-xs text-muted-foreground">
                  {formatRelativeTime(item.createdAt, locale)}
                </time>
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

/** Geser pendek panel menu (bukan seluruh lebar) agar panel tidak meluncur melintasi Dock atau panel menu. */
const SLIDE = "-0.75rem";

/**
 * Gaya per penempatan. Variabel `--tw-enter-*`/`--tw-exit-*` (tw-animate-css) di gaya inline menang atas utilitas
 * `slide-*` bawaan `SheetContent` (geser selebar penuh dari tepi layar) tanpa menyentuh berkas shadcn.
 */
function placementStyle(anchor: { placement: NotificationPlacement; left: number; top: number }) {
  if (anchor.placement === "desks") {
    return {
      top: anchor.top,
      right: "1rem",
      bottom: "auto",
      left: "auto",
      height: "auto",
      maxHeight: `calc(100svh - ${anchor.top + 16}px)`,
      "--tw-enter-translate-x": "0",
      "--tw-exit-translate-x": "0",
      "--tw-enter-translate-y": "-0.5rem",
      "--tw-exit-translate-y": "-0.5rem",
      "--tw-enter-scale": "0.98",
      "--tw-exit-scale": "0.98",
    } as CSSProperties;
  }
  return {
    left: anchor.left,
    "--tw-enter-translate-x": SLIDE,
    "--tw-exit-translate-x": SLIDE,
  } as CSSProperties;
}

/**
 * Panel Notifikasi: `Sheet` non-modal tanpa overlay penuh. Penempatan `sidebar` (bawaan): lembar setinggi layar yang
 * dimulai tepat di tepi kanan Dock/panel menu (Pinned terbuka atau diciutkan, Floating, strip ikon; lihat
 * `useNotificationAnchor`) dan muncul dengan geser pendek + pudar. Penempatan `desks` (halaman Desks, tanpa Dock dan
 * panel menu): kartu mengambang di kanan atas di bawah navbar. `placement` bawaan `auto` mendeteksi penanda
 * `data-shell-chrome="desks"` pada kerangka.
 *
 * Fokus: mode non-modal Radix memutar Tab di dalam panel (`FocusScope` loop) sehingga fokus tidak keluar lewat
 * keyboard; `Escape`, klik di luar, dan fokus yang berpindah keluar menutup panel (tanpa overlay, sehingga konten di
 * luar tetap dapat dibaca). Fokus dikembalikan ke elemen yang memicu pembukaan (Radix tidak melakukannya karena
 * pemicunya berada di luar komponen ini). Sumber data = `NotificationSource` M3a lewat `useNotifications` (kunci
 * query sama dengan lencana jumlah belum dibaca). Konten notifikasi dirender sebagai teks.
 */
export function NotificationPanel({
  open,
  onOpenChange,
  placement = "auto",
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  placement?: NotificationPlacement | "auto";
}) {
  const { t } = useTranslation("platform");
  const { t: tUi } = useTranslation("ui");
  const n = useNotifications({ enabled: open });
  const navigate = useNavigate();
  const anchor = useNotificationAnchor(open, placement);
  const card = anchor.placement === "desks";
  const [actionFailed, setActionFailed] = useState(false);

  const focus = useReturnFocus(open);

  const run = (action: Promise<void>) => {
    setActionFailed(false);
    action.catch(() => setActionFailed(true));
  };

  let body: ReactNode;
  if (n.status === "loading") {
    body = (
      <div role="status" className="flex justify-center p-6">
        <Spinner aria-label={tUi("status.loading")} />
      </div>
    );
  } else if (n.status === "error") {
    body = (
      <div className="p-4">
        <Alert variant="destructive">
          <AlertDescription>{t("notifications.loadError")}</AlertDescription>
        </Alert>
        <Button type="button" variant="outline" size="sm" className="mt-3" onClick={n.refetch}>
          {tUi("actions.retry")}
        </Button>
      </div>
    );
  } else if (n.items.length === 0) {
    body = <EmptyNotice icon="bell" title={t("notifications.empty")} />;
  } else {
    body = (
      <Rows
        n={n}
        onFailure={run}
        onNavigate={(href) => {
          onOpenChange(false);
          void navigate({ to: href });
        }}
      />
    );
  }

  const Check = resolveIcon("check");
  const Close = resolveIcon("x");
  return (
    <Sheet open={open} onOpenChange={onOpenChange} modal={false}>
      <SheetContent
        side={card ? "right" : "left"}
        showCloseButton={false}
        data-placement={anchor.placement}
        style={placementStyle(anchor)}
        className={cn(
          "w-96 max-w-full gap-0 ease-out data-[state=closed]:duration-150 data-[state=closed]:fade-out-0 data-[state=open]:duration-200 data-[state=open]:fade-in-0 sm:max-w-96",
          card && "overflow-hidden rounded-lg border",
        )}
        onInteractOutside={focus.onInteractOutside}
        onCloseAutoFocus={focus.onCloseAutoFocus}
      >
        <SheetHeader className="flex-row items-center justify-between gap-2 border-b px-4 py-3">
          <SheetTitle className="text-sm">{t("notifications.title")}</SheetTitle>
          <SheetDescription className="sr-only">{t("notifications.description")}</SheetDescription>
          <div className="flex items-center gap-1">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="size-7"
              aria-label={t("notifications.markAllRead")}
              disabled={n.unreadCount === 0}
              onClick={() => run(n.markAllRead())}
            >
              {Check ? <Check aria-hidden="true" /> : null}
            </Button>
            <SheetClose asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon"
                className="size-7"
                aria-label={tUi("actions.close")}
              >
                {Close ? <Close aria-hidden="true" /> : null}
              </Button>
            </SheetClose>
          </div>
        </SheetHeader>
        <Tabs defaultValue="all" className="min-h-0 flex-1 gap-0">
          <TabsList
            variant="line"
            aria-label={t("notifications.tabsLabel")}
            className="w-full px-2"
          >
            <TabsTrigger value="all">{t("notifications.tabs.all")}</TabsTrigger>
            <TabsTrigger value="events">{t("notifications.tabs.events")}</TabsTrigger>
          </TabsList>
          {actionFailed ? (
            <div className="border-b p-3">
              <Alert variant="destructive">
                <AlertDescription>{t("notifications.actionError")}</AlertDescription>
              </Alert>
            </div>
          ) : null}
          <TabsContent value="all" className="min-h-0 overflow-auto">
            {body}
          </TabsContent>
          <TabsContent value="events" className="min-h-0 overflow-auto">
            <EmptyNotice icon="calendar" title={t("notifications.eventsEmpty")} />
          </TabsContent>
        </Tabs>
      </SheetContent>
    </Sheet>
  );
}

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
import { type ReactNode, useLayoutEffect, useState } from "react";
import { EmptyNotice } from "../components/notices";
import { useReturnFocus } from "./overlay-focus";

/**
 * Penanda kontainer panel menu (sidebar shadcn). Tepi kanannya diukur langsung agar panel Notifikasi (yang dirender
 * di portal, di luar `SidebarProvider` sehingga tidak mewarisi `--sidebar-width`) selalu dimulai tepat di sebelahnya,
 * juga saat panel menu diciutkan atau Dock Pinned memperlebar kolom kiri.
 */
const MENU_PANEL = '[data-slot="sidebar-container"]';

/** Offset kiri (px) panel Notifikasi: tepi kanan panel menu; 0 bila tidak ada panel menu (mis. ponsel). */
function useMenuPanelOffset(open: boolean): number {
  const [offset, setOffset] = useState(0);
  useLayoutEffect(() => {
    if (!open) return;
    const el = document.querySelector(MENU_PANEL);
    if (el === null) {
      setOffset(0);
      return;
    }
    const measure = () => setOffset(Math.max(0, Math.round(el.getBoundingClientRect().right)));
    measure();
    window.addEventListener("resize", measure);
    let observer: ResizeObserver | undefined;
    if (typeof ResizeObserver !== "undefined") {
      observer = new ResizeObserver(measure);
      observer.observe(el);
    }
    return () => {
      window.removeEventListener("resize", measure);
      observer?.disconnect();
    };
  }, [open]);
  return offset;
}

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

/**
 * Panel Notifikasi: `Sheet` sisi kiri yang dimulai di tepi kanan panel menu, tanpa overlay penuh (mode non-modal
 * Radix: tanpa overlay dan tanpa kunci fokus; `Escape`, klik di luar, dan fokus keluar menutup). Fokus dikembalikan
 * ke elemen yang memicu pembukaan (Radix tidak melakukannya karena pemicunya berada di luar komponen ini). Sumber
 * data = `NotificationSource` M3a lewat `useNotifications` (kunci query sama dengan lencana jumlah belum dibaca).
 * Konten notifikasi dirender sebagai teks.
 */
export function NotificationPanel({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useTranslation("platform");
  const { t: tUi } = useTranslation("ui");
  const n = useNotifications({ enabled: open });
  const navigate = useNavigate();
  const offset = useMenuPanelOffset(open);
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
        side="left"
        showCloseButton={false}
        style={{ left: offset }}
        className="w-96 gap-0 sm:max-w-96"
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

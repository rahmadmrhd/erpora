import { useNavigate } from "@erpora/module-sdk";
import {
  formatRelativeTime,
  Message,
  type NotificationsState,
  safeInternalHref,
  useMessage,
  useNotifications,
  useTranslation,
} from "@erpora/platform-web";
import {
  Alert,
  AlertDescription,
  Button,
  EmptyState,
  LoadingIcon,
  Popover,
  PopoverContent,
  PopoverTrigger,
  resolveIcon,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@erpora/ui";
import { type ReactNode, useState } from "react";
import { NavIcon } from "./nav-icon";

const TITLE = { ns: "platform", key: "notifications.title" } as const;

/** `99+` bila lebih dari 99. */
export function badgeText(count: number): string {
  return count > 99 ? "99+" : String(count);
}

function Header({ n, onMarkAll }: { n: NotificationsState; onMarkAll: () => void }) {
  const { t } = useTranslation("platform");
  const Check = resolveIcon("check");
  return (
    <div className="flex items-center justify-between border-b px-4 py-3">
      <h2 className="text-sm font-semibold">
        <Message message={TITLE} />
      </h2>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="size-7"
        aria-label={t("notifications.markAllRead")}
        disabled={n.unreadCount === 0}
        onClick={onMarkAll}
      >
        {Check ? <Check aria-hidden="true" /> : null}
      </Button>
    </div>
  );
}

function Rows({ n, onNavigate }: { n: NotificationsState; onNavigate: (href: string) => void }) {
  const { locale } = useTranslation("platform");
  return (
    <ul className="divide-y">
      {n.items.map((item) => {
        const href = safeInternalHref(item.href);
        return (
          <li key={item.id}>
            <button
              type="button"
              className="flex w-full items-start gap-3 px-4 py-3 text-left hover:bg-accent"
              onClick={() => {
                void n.markRead(item.id);
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

/** Isi panel notifikasi: memuat, kosong, galat, atau daftar. Konten notifikasi dirender sebagai teks. */
export function NotificationsPanel({
  n,
  onNavigate,
}: {
  n: NotificationsState;
  onNavigate: (href: string) => void;
}) {
  const { t } = useTranslation("platform");
  let body: ReactNode;
  if (n.status === "loading") {
    body = (
      <div role="status" className="flex justify-center p-6">
        <LoadingIcon aria-label={t("shell.loading")} />
      </div>
    );
  } else if (n.status === "error") {
    body = (
      <div className="p-4">
        <Alert variant="destructive">
          <AlertDescription>{t("notifications.loadError")}</AlertDescription>
        </Alert>
        <Button type="button" variant="outline" size="sm" className="mt-3" onClick={n.refetch}>
          {t("shell.retry")}
        </Button>
      </div>
    );
  } else if (n.items.length === 0) {
    body = <EmptyState icon="bell" title={t("notifications.empty")} />;
  } else {
    body = <Rows n={n} onNavigate={onNavigate} />;
  }
  return (
    <div className="flex h-[460px] flex-col">
      <Header n={n} onMarkAll={() => void n.markAllRead()} />
      <div className="min-h-0 flex-1 overflow-auto">{body}</div>
    </div>
  );
}

/** Item Notifikasi di sidebar: lencana jumlah belum dibaca (titik saat terlipat) dan panel di sampingnya. */
export function NotificationsMenuItem() {
  const [open, setOpen] = useState(false);
  const n = useNotifications({ enabled: open });
  const navigate = useNavigate();
  const { t } = useTranslation("platform");
  const title = useMessage(TITLE);
  return (
    <SidebarMenuItem>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <SidebarMenuButton
            tooltip={title}
            aria-label={t("notifications.unreadLabel", { count: n.unreadCount })}
          >
            <NavIcon name="bell" label={title} />
            <span>{title}</span>
          </SidebarMenuButton>
        </PopoverTrigger>
        <PopoverContent side="right" align="start" className="w-96 p-0">
          <NotificationsPanel
            n={n}
            onNavigate={(href) => {
              setOpen(false);
              void navigate({ to: href });
            }}
          />
        </PopoverContent>
      </Popover>
      {n.unreadCount > 0 ? (
        <SidebarMenuBadge
          data-testid="notification-badge"
          className="bg-notification-badge-bg text-destructive-foreground peer-data-[active=true]/menu-button:text-destructive-foreground"
        >
          {badgeText(n.unreadCount)}
        </SidebarMenuBadge>
      ) : null}
    </SidebarMenuItem>
  );
}

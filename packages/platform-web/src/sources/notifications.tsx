import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createContext, type ReactNode, useCallback, useContext, useEffect } from "react";
import { useTenantId } from "../bootstrap";
import { scopedKey } from "../tenancy";
import { emptyNotificationSource, notificationPollInterval } from "./empty";
import type { AppNotification, NotificationSource } from "./types";

const LIST_LIMIT = 20;
const NO_ITEMS: readonly AppNotification[] = Object.freeze([]);

const NotificationSourceContext = createContext<NotificationSource>(emptyNotificationSource);

/** Memasang sumber notifikasi; tanpa provider dipakai `emptyNotificationSource`. */
export function NotificationSourceProvider({
  source,
  children,
}: {
  source: NotificationSource;
  children: ReactNode;
}) {
  return (
    <NotificationSourceContext.Provider value={source}>
      {children}
    </NotificationSourceContext.Provider>
  );
}

export function useNotificationSource(): NotificationSource {
  return useContext(NotificationSourceContext);
}

export interface NotificationsState {
  items: readonly AppNotification[];
  unreadCount: number;
  /** Status daftar bila `enabled`, selain itu status jumlah belum dibaca. */
  status: "loading" | "ready" | "error";
  refetch(): void;
  markRead(id: string): Promise<void>;
  markAllRead(): Promise<void>;
}

/**
 * Notifikasi dari sumber terpasang. Jumlah belum dibaca selalu dimuat (lencana sidebar) dan dipoll tiap 60 detik
 * kecuali sumber kosong; daftar hanya dimuat saat `enabled` (panel terbuka). Data hanya di cache memori Query
 * (tanpa persister) dan key berawalan tenant.
 */
export function useNotifications(opts: { enabled: boolean }): NotificationsState {
  const source = useNotificationSource();
  const tenantId = useTenantId();
  const queryClient = useQueryClient();

  const unread = useQuery({
    queryKey: scopedKey(tenantId, "notifications", "unread"),
    queryFn: ({ signal }) => source.unreadCount(signal),
    refetchInterval: notificationPollInterval(source),
  });
  const list = useQuery({
    queryKey: scopedKey(tenantId, "notifications", "list"),
    queryFn: ({ signal }) => source.list({ limit: LIST_LIMIT, signal }),
    enabled: opts.enabled,
  });

  const invalidate = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: scopedKey(tenantId, "notifications", "unread") }),
      queryClient.invalidateQueries({ queryKey: scopedKey(tenantId, "notifications", "list") }),
    ]);
  }, [queryClient, tenantId]);

  useEffect(() => {
    if (!source.subscribe) return;
    return source.subscribe(() => {
      void invalidate();
    });
  }, [source, invalidate]);

  const markRead = useMutation({
    mutationFn: (id: string) => source.markRead(id),
    onSuccess: invalidate,
  });
  const markAllRead = useMutation({
    mutationFn: () => source.markAllRead(),
    onSuccess: invalidate,
  });

  const active = opts.enabled ? list : unread;
  const status: NotificationsState["status"] = active.isError
    ? "error"
    : active.isSuccess
      ? "ready"
      : "loading";

  return {
    items: list.data ?? NO_ITEMS,
    unreadCount: unread.isError ? 0 : (unread.data ?? 0),
    status,
    refetch: () => {
      void unread.refetch();
      if (opts.enabled) void list.refetch();
    },
    markRead: (id) => markRead.mutateAsync(id),
    markAllRead: () => markAllRead.mutateAsync(),
  };
}

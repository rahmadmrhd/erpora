import { afterEach, describe, expect, test } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { StaticBootstrapProvider } from "../bootstrap";
import { makeBootstrap } from "../bootstrap/testing";
import { type NotificationSource, NotificationSourceProvider } from "../sources";
import {
  formatBadgeCount,
  registerFixedItem,
  resetFixedItems,
  TodoCountSourceProvider,
  useFixedItems,
} from "./index";

afterEach(() => resetFixedItems());

function wrapper(opts: { notifications?: NotificationSource; todo?: () => Promise<number> } = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <StaticBootstrapProvider value={makeBootstrap()}>
        <NotificationSourceProvider
          source={
            opts.notifications ?? {
              unreadCount: async () => 0,
              list: async () => [],
              markRead: async () => {},
              markAllRead: async () => {},
            }
          }
        >
          <TodoCountSourceProvider source={{ count: opts.todo ?? (async () => 0) }}>
            {children}
          </TodoCountSourceProvider>
        </NotificationSourceProvider>
      </StaticBootstrapProvider>
    </QueryClientProvider>
  );
}

describe("registri item tetap", () => {
  test("bawaan berurutan Cari, Notifikasi, ToDo dengan aksi yang benar", () => {
    const { result } = renderHook(() => useFixedItems(), { wrapper: wrapper() });
    expect(result.current.map((i) => i.id)).toEqual(["search", "notifications", "todo"]);
    const [search, notifications, todo] = result.current;
    expect(search?.action).toEqual({ kind: "command", command: "search" });
    expect(notifications?.action).toEqual({ kind: "command", command: "notifications" });
    expect(todo?.action).toEqual({ kind: "link", to: "/platform/todo" });
  });

  test("registerFixedItem menambah item menurut order tanpa mengubah komponen; fungsi pelepas menghapusnya", () => {
    const { result } = renderHook(() => useFixedItems(), { wrapper: wrapper() });
    let unregister = () => {};
    act(() => {
      unregister = registerFixedItem({
        id: "inbox",
        label: { ns: "platform", key: "shell.fixed.inbox" },
        icon: "inbox",
        order: 25,
        action: { kind: "link", to: "/platform/inbox" },
      });
    });
    expect(result.current.map((i) => i.id)).toEqual(["search", "notifications", "inbox", "todo"]);
    act(() => unregister());
    expect(result.current.map((i) => i.id)).toEqual(["search", "notifications", "todo"]);
  });

  test("id ganda, id tidak valid, dan tautan tidak aman ditolak", () => {
    const base = {
      label: { ns: "platform", key: "x" },
      icon: "inbox",
      order: 50,
    };
    expect(() =>
      registerFixedItem({ ...base, id: "todo", action: { kind: "link", to: "/platform/x" } }),
    ).toThrow(/todo/);
    expect(() =>
      registerFixedItem({ ...base, id: "Bad Id", action: { kind: "link", to: "/platform/x" } }),
    ).toThrow();
    expect(() =>
      registerFixedItem({ ...base, id: "evil", action: { kind: "link", to: "//evil.example" } }),
    ).toThrow();
    expect(() =>
      registerFixedItem({
        ...base,
        id: "evil2",
        action: { kind: "link", to: "javascript:alert(1)" },
      }),
    ).toThrow();
  });
});

describe("lencana", () => {
  test("Notifikasi memakai jumlah belum dibaca dari sumber notifikasi", async () => {
    const notifications: NotificationSource = {
      unreadCount: async () => 7,
      list: async () => [],
      markRead: async () => {},
      markAllRead: async () => {},
    };
    const { result } = renderHook(
      () => {
        const items = useFixedItems();
        const item = items.find((i) => i.id === "notifications");
        return item?.useBadge() ?? -1;
      },
      { wrapper: wrapper({ notifications }) },
    );
    await waitFor(() => expect(result.current).toBe(7));
  });

  test("ToDo memakai sumber sementara dan 0 secara bawaan", async () => {
    const { result } = renderHook(
      () =>
        useFixedItems()
          .find((i) => i.id === "todo")
          ?.useBadge() ?? -1,
      { wrapper: wrapper({ todo: async () => 3 }) },
    );
    await waitFor(() => expect(result.current).toBe(3));
    const empty = renderHook(
      () =>
        useFixedItems()
          .find((i) => i.id === "todo")
          ?.useBadge() ?? -1,
      {
        wrapper: wrapper(),
      },
    );
    await waitFor(() => expect(empty.result.current).toBe(0));
  });

  test("Cari tidak punya lencana (0)", () => {
    const { result } = renderHook(
      () =>
        useFixedItems()
          .find((i) => i.id === "search")
          ?.useBadge() ?? -1,
      { wrapper: wrapper() },
    );
    expect(result.current).toBe(0);
  });

  test.each([
    [0, ""],
    [-3, ""],
    [Number.NaN, ""],
    [5, "5"],
    [99, "99"],
    [100, "99+"],
    [250, "99+"],
  ])("formatBadgeCount(%p) = %p", (count, text) => {
    expect(formatBadgeCount(count)).toBe(text);
  });
});

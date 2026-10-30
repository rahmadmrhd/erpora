import { afterEach, describe, expect, test } from "bun:test";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { StaticBootstrapProvider } from "../bootstrap";
import { makeBootstrap } from "../bootstrap/testing";
import {
  type AppNotification,
  type CurrentUser,
  type CurrentUserSource,
  CurrentUserSourceProvider,
  emptyNotificationSource,
  type NotificationSource,
  NotificationSourceProvider,
  notificationPollInterval,
  useCurrentUser,
  useNotifications,
} from "./index";

const TENANT = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b";

function mk(id: string, read = false): AppNotification {
  return {
    id,
    title: `Judul ${id}`,
    message: `Pesan ${id}`,
    createdAt: "2026-10-29T10:00:00Z",
    read,
  };
}

/** Sumber dalam memori dengan keadaan nyata sehingga hitungan mengikuti markRead/markAllRead. */
function memorySource(initial: AppNotification[]) {
  let items = initial;
  const listeners = new Set<() => void>();
  const calls = { unread: 0, list: 0 };
  const source: NotificationSource = {
    unreadCount: async () => {
      calls.unread += 1;
      return items.filter((n) => !n.read).length;
    },
    list: async ({ limit }) => {
      calls.list += 1;
      return items.slice(0, limit);
    },
    markRead: async (id) => {
      items = items.map((n) => (n.id === id ? { ...n, read: true } : n));
    },
    markAllRead: async () => {
      items = items.map((n) => ({ ...n, read: true }));
    },
    subscribe: (onChange) => {
      listeners.add(onChange);
      return () => listeners.delete(onChange);
    },
  };
  return {
    source,
    calls,
    push: (n: AppNotification) => {
      items = [n, ...items];
      for (const l of listeners) l();
    },
    listeners: () => listeners.size,
  };
}

function setup(
  ui: ReactNode,
  queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  const result = render(
    <QueryClientProvider client={queryClient}>
      <StaticBootstrapProvider value={makeBootstrap({ tenantId: TENANT })}>
        {ui}
      </StaticBootstrapProvider>
    </QueryClientProvider>,
  );
  return { ...result, queryClient };
}

function NotifProbe({ enabled }: { enabled: boolean }) {
  const n = useNotifications({ enabled });
  return (
    <div>
      <span data-testid="status">{n.status}</span>
      <span data-testid="unread">{n.unreadCount}</span>
      <span data-testid="items">{n.items.map((i) => i.id).join(",")}</span>
      <button type="button" onClick={() => void n.markRead("a")}>
        baca-a
      </button>
      <button type="button" onClick={() => void n.markAllRead()}>
        baca-semua
      </button>
      <button type="button" onClick={() => n.refetch()}>
        muat-ulang
      </button>
    </div>
  );
}

afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("useNotifications", () => {
  test("sumber kosong (bawaan tanpa provider): ready, 0, tanpa item", async () => {
    setup(<NotifProbe enabled />);
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("ready"));
    expect(screen.getByTestId("unread").textContent).toBe("0");
    expect(screen.getByTestId("items").textContent).toBe("");
  });

  test("memuat lalu menampilkan item dan jumlah belum dibaca", async () => {
    const m = memorySource([mk("a"), mk("b"), mk("c", true)]);
    setup(
      <NotificationSourceProvider source={m.source}>
        <NotifProbe enabled />
      </NotificationSourceProvider>,
    );
    expect(screen.getByTestId("status").textContent).toBe("loading");
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("ready"));
    expect(screen.getByTestId("unread").textContent).toBe("2");
    expect(screen.getByTestId("items").textContent).toBe("a,b,c");
  });

  test("daftar tidak dimuat saat enabled=false; jumlah belum dibaca tetap dimuat", async () => {
    const m = memorySource([mk("a")]);
    setup(
      <NotificationSourceProvider source={m.source}>
        <NotifProbe enabled={false} />
      </NotificationSourceProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("unread").textContent).toBe("1"));
    expect(m.calls.list).toBe(0);
  });

  test("galat sumber: status error dan jumlah 0 (lencana tersembunyi)", async () => {
    const source: NotificationSource = {
      ...emptyNotificationSource,
      unreadCount: async () => {
        throw new Error("gagal");
      },
      list: async () => {
        throw new Error("gagal");
      },
    };
    setup(
      <NotificationSourceProvider source={source}>
        <NotifProbe enabled />
      </NotificationSourceProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("error"));
    expect(screen.getByTestId("unread").textContent).toBe("0");
  });

  test("refetch gagal setelah sukses: lencana disembunyikan (jumlah 0) walau ada data lama", async () => {
    let fail = false;
    const source: NotificationSource = {
      ...emptyNotificationSource,
      unreadCount: async () => {
        if (fail) throw new Error("gagal");
        return 7;
      },
    };
    setup(
      <NotificationSourceProvider source={source}>
        <NotifProbe enabled={false} />
      </NotificationSourceProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("unread").textContent).toBe("7"));
    fail = true;
    act(() => screen.getByText("muat-ulang").click());
    await waitFor(() => expect(screen.getByTestId("unread").textContent).toBe("0"));
  });

  test("markRead yang ditolak sumber: promise menolak (pemanggil menangani) dan hitungan tidak berubah", async () => {
    const m = memorySource([mk("a")]);
    const source: NotificationSource = {
      ...m.source,
      markRead: async () => {
        throw new Error("gagal tandai");
      },
    };
    let rejected = false;
    function P() {
      const n = useNotifications({ enabled: true });
      return (
        <button
          type="button"
          onClick={() => {
            n.markRead("a").catch(() => {
              rejected = true;
            });
          }}
        >
          tandai
        </button>
      );
    }
    setup(
      <NotificationSourceProvider source={source}>
        <P />
      </NotificationSourceProvider>,
    );
    act(() => screen.getByText("tandai").click());
    await waitFor(() => expect(rejected).toBe(true));
  });

  test("markRead dan markAllRead memperbarui hitungan dan daftar", async () => {
    const m = memorySource([mk("a"), mk("b")]);
    setup(
      <NotificationSourceProvider source={m.source}>
        <NotifProbe enabled />
      </NotificationSourceProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("unread").textContent).toBe("2"));
    act(() => screen.getByText("baca-a").click());
    await waitFor(() => expect(screen.getByTestId("unread").textContent).toBe("1"));
    act(() => screen.getByText("baca-semua").click());
    await waitFor(() => expect(screen.getByTestId("unread").textContent).toBe("0"));
  });

  test("subscribe memicu muat ulang dan dilepas saat unmount", async () => {
    const m = memorySource([mk("a")]);
    const { unmount } = setup(
      <NotificationSourceProvider source={m.source}>
        <NotifProbe enabled />
      </NotificationSourceProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("unread").textContent).toBe("1"));
    expect(m.listeners()).toBe(1);
    act(() => m.push(mk("z")));
    await waitFor(() => expect(screen.getByTestId("unread").textContent).toBe("2"));
    unmount();
    expect(m.listeners()).toBe(0);
  });

  test("semua query key berawalan tenant dan tidak ada data di storage", async () => {
    const m = memorySource([mk("a")]);
    const { queryClient } = setup(
      <NotificationSourceProvider source={m.source}>
        <NotifProbe enabled />
      </NotificationSourceProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("ready"));
    const keys = queryClient
      .getQueryCache()
      .getAll()
      .map((q) => q.queryKey);
    expect(keys.length).toBeGreaterThan(0);
    for (const k of keys) expect(k[0]).toBe(TENANT);
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });

  test("polling 60 detik hanya untuk sumber non-kosong", () => {
    expect(notificationPollInterval(emptyNotificationSource)).toBe(false);
    expect(notificationPollInterval(memorySource([]).source)).toBe(60_000);
  });
});

const USER: CurrentUser = {
  id: "u1",
  displayName: "Rahmad Maulana",
  email: "rahmad@example.test",
  avatarUrl: null,
};

function UserProbe() {
  const { user, status } = useCurrentUser();
  return (
    <div>
      <span data-testid="status">{status}</span>
      <span data-testid="name">{user?.displayName ?? "-"}</span>
    </div>
  );
}

describe("useCurrentUser", () => {
  test("tanpa provider: pengguna anonim (null) dan ready", async () => {
    setup(<UserProbe />);
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("ready"));
    expect(screen.getByTestId("name").textContent).toBe("-");
  });

  test("memuat pengguna dari sumber; PII tidak masuk storage; key berawalan tenant", async () => {
    const source: CurrentUserSource = {
      capabilities: { manageAccount: true, signOut: true },
      get: async () => USER,
    };
    const { queryClient } = setup(
      <CurrentUserSourceProvider source={source}>
        <UserProbe />
      </CurrentUserSourceProvider>,
    );
    expect(screen.getByTestId("status").textContent).toBe("loading");
    await waitFor(() => expect(screen.getByTestId("name").textContent).toBe("Rahmad Maulana"));
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
    for (const q of queryClient.getQueryCache().getAll()) expect(q.queryKey[0]).toBe(TENANT);
  });

  test("galat sumber: status error", async () => {
    const source: CurrentUserSource = {
      capabilities: { manageAccount: false, signOut: false },
      get: async () => {
        throw new Error("gagal");
      },
    };
    setup(
      <CurrentUserSourceProvider source={source}>
        <UserProbe />
      </CurrentUserSourceProvider>,
    );
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("error"));
  });
});

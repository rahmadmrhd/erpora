import { afterEach, describe, expect, spyOn, test } from "bun:test";
import { createClient, type PlatformPaths } from "@erpora/platform-contracts";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { StaticBootstrapProvider } from "../bootstrap";
import { makeBootstrap } from "../bootstrap/testing";
import { anonymousUserSource, type CurrentUserSource, CurrentUserSourceProvider } from "../sources";
import {
  DeskProvider,
  isShowDesksState,
  SHOW_DESKS_STATE,
  useCurrentDesk,
  useDesks,
  useShowDesks,
} from "./provider";
import { makeDashboardItem, makeDesk, makeDeskItem } from "./testing";
import { useDeskValidation } from "./validation";

const desks = [
  makeDesk(
    "platform",
    [
      makeDashboardItem("platform", "platform"),
      makeDeskItem("platform", "modules", "/platform/modules"),
    ],
    { source: "platform" },
  ),
  makeDesk("example", [
    makeDashboardItem("example", "example"),
    makeDeskItem("example", "notes", "/example/notes"),
  ]),
];

afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
  history.replaceState(null, "");
});

interface Opts {
  historyState?: unknown;
  user?: CurrentUserSource;
  list?: unknown;
}

function setup(opts: Opts = {}) {
  const client = createClient<PlatformPaths>({
    fetch: (async () =>
      new Response(JSON.stringify({ data: opts.list ?? { desks, revision: "r1" } }), {
        status: 200,
        headers: { "Content-Type": "application/json", ETag: '"e"' },
      })) as unknown as typeof fetch,
  });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <StaticBootstrapProvider value={makeBootstrap()}>
        <CurrentUserSourceProvider source={opts.user ?? anonymousUserSource}>
          <DeskProvider client={client} historyState={opts.historyState}>
            {children}
          </DeskProvider>
        </CurrentUserSourceProvider>
      </StaticBootstrapProvider>
    </QueryClientProvider>
  );
  return { wrapper };
}

async function ready(result: { current: { list: { status: string } } }) {
  await waitFor(() => expect(result.current.list.status).toBe("ready"));
}

describe("DeskProvider: daftar dan Desk saat ini (tahap 1-3)", () => {
  test("memuat daftar; tanpa kandidat Desk saat ini null", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(() => ({ list: useDesks(), cur: useCurrentDesk() }), { wrapper });
    expect(result.current.list.status).toBe("loading");
    expect(result.current.cur.deskId).toBeNull();
    await ready(result);
    expect(result.current.list.desks.map((d) => d.id)).toEqual(["platform", "example"]);
    expect(result.current.list.revision).toBe("r1");
    expect(result.current.cur.deskId).toBeNull();
    expect(result.current.cur.desk).toBeNull();
  });

  test("tahap 1 tab, 2 localStorage, 3 default_desk pengguna", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    localStorage.setItem("erpora.desk", "platform");
    const user = { ...anonymousUserSource, defaultDesk: "platform" };
    const { wrapper } = setup({ user });
    const { result } = renderHook(() => ({ list: useDesks(), cur: useCurrentDesk() }), { wrapper });
    await ready(result);
    expect(result.current.cur.deskId).toBe("example");
    expect(result.current.cur.desk?.id).toBe("example");
  });

  test("localStorage dan default_desk bila tab kosong", async () => {
    localStorage.setItem("erpora.desk", "platform");
    const { wrapper } = setup();
    const a = renderHook(() => ({ list: useDesks(), cur: useCurrentDesk() }), { wrapper });
    await ready(a.result);
    expect(a.result.current.cur.deskId).toBe("platform");
    localStorage.clear();
    const { wrapper: w2 } = setup({ user: { ...anonymousUserSource, defaultDesk: "example" } });
    const b = renderHook(() => ({ list: useDesks(), cur: useCurrentDesk() }), { wrapper: w2 });
    await ready(b.result);
    expect(b.result.current.cur.deskId).toBe("example");
  });

  test("nilai storage berbahaya atau tidak ada di daftar server diabaikan", async () => {
    sessionStorage.setItem("erpora.desk.tab", "<img onerror=x>");
    localStorage.setItem("erpora.desk", "hilang");
    const { wrapper } = setup();
    const { result } = renderHook(() => ({ list: useDesks(), cur: useCurrentDesk() }), { wrapper });
    await ready(result);
    expect(result.current.cur.deskId).toBeNull();
  });

  test("storage melempar (mode privat): tanpa galat, Desk null", async () => {
    const get = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    const set = spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("quota", "QuotaExceededError");
    });
    try {
      const { wrapper } = setup();
      const { result } = renderHook(() => ({ list: useDesks(), cur: useCurrentDesk() }), {
        wrapper,
      });
      await ready(result);
      expect(result.current.cur.deskId).toBeNull();
      act(() => result.current.cur.setCurrentDesk("example"));
      expect(result.current.cur.deskId).toBe("example");
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });
});

describe("setCurrentDesk", () => {
  test("menyimpan ke sessionStorage dan localStorage serta memperbarui state", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(() => ({ list: useDesks(), cur: useCurrentDesk() }), { wrapper });
    await ready(result);
    act(() => result.current.cur.setCurrentDesk("example"));
    expect(result.current.cur.deskId).toBe("example");
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("example");
    expect(localStorage.getItem("erpora.desk")).toBe("example");
  });

  test("id yang tidak ada di daftar server atau tidak valid ditolak (tidak disimpan)", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(() => ({ list: useDesks(), cur: useCurrentDesk() }), { wrapper });
    await ready(result);
    act(() => result.current.cur.setCurrentDesk("hilang"));
    act(() => result.current.cur.setCurrentDesk("Bad Id"));
    expect(result.current.cur.deskId).toBeNull();
    expect(sessionStorage.getItem("erpora.desk.tab")).toBeNull();
    expect(localStorage.getItem("erpora.desk")).toBeNull();
  });
});

describe("showDesks (state riwayat)", () => {
  test("isShowDesksState: hanya objek dengan showDesks === true", () => {
    expect(isShowDesksState(SHOW_DESKS_STATE)).toBe(true);
    for (const v of [null, undefined, {}, { showDesks: "true" }, { showDesks: 1 }, "x", 1, []]) {
      expect(isShowDesksState(v)).toBe(false);
    }
  });

  test("dibaca dari prop historyState", async () => {
    const { wrapper } = setup({ historyState: SHOW_DESKS_STATE });
    const { result } = renderHook(() => useShowDesks(), { wrapper });
    expect(result.current).toBe(true);
  });

  test("tanpa prop: dibaca dari window.history.state; tidak ada penanda berarti false", async () => {
    const { wrapper } = setup();
    expect(renderHook(() => useShowDesks(), { wrapper }).result.current).toBe(false);
    history.replaceState({ ...SHOW_DESKS_STATE }, "");
    expect(renderHook(() => useShowDesks(), { wrapper }).result.current).toBe(true);
  });
});

describe("useDeskValidation", () => {
  function validate(initial: string, opts: Opts = {}) {
    const { wrapper } = setup(opts);
    return renderHook(
      ({ path }: { path: string }) => ({
        list: useDesks(),
        cur: useCurrentDesk(),
        page: useDeskValidation(path),
      }),
      { wrapper, initialProps: { path: initial } },
    );
  }

  test("Desk saat ini yang memuat halaman tidak diubah", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const { result } = validate("/example/notes");
    await ready(result);
    expect(result.current.cur.deskId).toBe("example");
    expect(result.current.page.deskId).toBe("example");
  });

  test("beralih ke Desk valid tanpa muat ulang saat halaman bukan milik Desk saat ini", async () => {
    sessionStorage.setItem("erpora.desk.tab", "platform");
    const { result, rerender } = validate("/platform/modules");
    await ready(result);
    expect(result.current.cur.deskId).toBe("platform");
    rerender({ path: "/example/notes/12" });
    await waitFor(() => expect(result.current.cur.deskId).toBe("example"));
    expect(result.current.page.deskId).toBe("example");
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("example");
    expect(localStorage.getItem("erpora.desk")).toBe("example");
  });

  test("tanpa Desk saat ini: halaman menentukan Desk (pemilik utama)", async () => {
    const { result } = validate("/example/notes");
    await ready(result);
    await waitFor(() => expect(result.current.cur.deskId).toBe("example"));
  });

  test("halaman di luar semua Desk: Desk saat ini tetap, pageDeskId null (breadcrumb tanpa Desk)", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const { result } = validate("/platform/todo");
    await ready(result);
    expect(result.current.cur.deskId).toBe("example");
    expect(result.current.page.deskId).toBeNull();
  });

  test("path / bukan halaman: tidak memvalidasi", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const { result } = validate("/");
    await ready(result);
    expect(result.current.cur.deskId).toBe("example");
    expect(result.current.page.deskId).toBeNull();
  });

  test("selama daftar belum siap tidak melakukan apa pun", () => {
    const { result } = validate("/example/notes");
    expect(result.current.cur.deskId).toBeNull();
    expect(result.current.page.deskId).toBeNull();
  });
});

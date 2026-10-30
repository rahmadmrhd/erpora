import { afterEach, describe, expect, test } from "bun:test";
import { createClient, type PlatformPaths } from "@erpora/platform-contracts";
import { focusManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import {
  BootstrapProvider,
  StaticBootstrapProvider,
  useBootstrap,
  useFeature,
  useTenantId,
} from "./index";
import { makeBootstrap } from "./testing";

function jsonResponse(data: unknown, etag: string, status = 200) {
  return new Response(JSON.stringify({ data }), {
    status,
    headers: { "Content-Type": "application/json", ETag: etag },
  });
}

function harness(handlers: (() => Response | Promise<Response>)[]) {
  let i = 0;
  const seen: (string | null)[] = [];
  const client = createClient<PlatformPaths>({
    fetch: (async (_u: unknown, init?: RequestInit) => {
      seen.push(new Headers(init?.headers).get("If-None-Match"));
      const h = handlers[Math.min(i, handlers.length - 1)];
      i += 1;
      if (!h) throw new Error("tanpa handler");
      return h();
    }) as unknown as typeof fetch,
  });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const wrap = (node: ReactNode) => (
    <QueryClientProvider client={queryClient}>{node}</QueryClientProvider>
  );
  return { client, queryClient, wrap, seen, count: () => i };
}

function Probe() {
  const b = useBootstrap();
  return (
    <div>
      <span data-testid="rev">{b.revision}</span>
      <span data-testid="tenant">{useTenantId()}</span>
      <span data-testid="admin">{String(useFeature("modulesAdminApi"))}</span>
    </div>
  );
}

afterEach(() => {
  focusManager.setFocused(undefined);
});

describe("BootstrapProvider", () => {
  test("memuat lalu siap: loading tampil dulu, kemudian anak", async () => {
    const h = harness([() => jsonResponse(makeBootstrap({ revision: 7 }), '"7"')]);
    render(
      h.wrap(
        <BootstrapProvider
          client={h.client}
          loading={<p>memuat</p>}
          error={() => <p>gagal</p>}
          options={{ retry: false }}
        >
          <Probe />
        </BootstrapProvider>,
      ),
    );
    expect(screen.getByText("memuat")).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId("rev").textContent).toBe("7"));
    expect(screen.queryByText("memuat")).toBeNull();
  });

  test("galat: render prop error dengan retry yang memuat ulang sampai berhasil", async () => {
    const h = harness([
      () =>
        new Response(JSON.stringify({ error: { code: "internal_error", message: "x" } }), {
          status: 500,
        }),
      () => jsonResponse(makeBootstrap({ revision: 3 }), '"3"'),
    ]);
    render(
      h.wrap(
        <BootstrapProvider
          client={h.client}
          loading={<p>memuat</p>}
          error={(retry) => (
            <button type="button" onClick={retry}>
              coba lagi
            </button>
          )}
          options={{ retry: false }}
        >
          <Probe />
        </BootstrapProvider>,
      ),
    );
    const retry = await screen.findByText("coba lagi");
    act(() => retry.click());
    await waitFor(() => expect(screen.getByTestId("rev").textContent).toBe("3"));
  });

  test("refetch saat window focus; 304 tidak mengganti state; revisi baru memperbarui", async () => {
    const h = harness([
      () => jsonResponse(makeBootstrap({ revision: 1 }), '"1"'),
      () => new Response(null, { status: 304, headers: { ETag: '"1"' } }),
      () => jsonResponse(makeBootstrap({ revision: 2 }), '"2"'),
    ]);
    render(
      h.wrap(
        <BootstrapProvider
          client={h.client}
          loading={<p>memuat</p>}
          error={() => <p>gagal</p>}
          options={{ retry: false, staleTime: 0 }}
        >
          <Probe />
        </BootstrapProvider>,
      ),
    );
    await waitFor(() => expect(screen.getByTestId("rev").textContent).toBe("1"));

    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });
    await waitFor(() => expect(h.count()).toBe(2));
    expect(screen.getByTestId("rev").textContent).toBe("1");
    expect(h.seen[1]).toBe('"1"');

    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });
    await waitFor(() => expect(screen.getByTestId("rev").textContent).toBe("2"));
  });

  test("galat refetch ketika data sudah ada tidak mengganti layar dengan galat", async () => {
    const h = harness([
      () => jsonResponse(makeBootstrap({ revision: 1 }), '"1"'),
      () => new Response("x", { status: 500 }),
    ]);
    render(
      h.wrap(
        <BootstrapProvider
          client={h.client}
          loading={<p>memuat</p>}
          error={() => <p>gagal</p>}
          options={{ retry: false, staleTime: 0 }}
        >
          <Probe />
        </BootstrapProvider>,
      ),
    );
    await waitFor(() => expect(screen.getByTestId("rev").textContent).toBe("1"));
    act(() => {
      focusManager.setFocused(false);
      focusManager.setFocused(true);
    });
    await waitFor(() => expect(h.count()).toBe(2));
    expect(screen.queryByText("gagal")).toBeNull();
    expect(screen.getByTestId("rev").textContent).toBe("1");
  });
});

describe("hooks bootstrap", () => {
  test("useTenantId dan useFeature: bidang features tidak ada berarti false", () => {
    render(
      <StaticBootstrapProvider
        value={makeBootstrap({ tenantId: "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b" })}
      >
        <Probe />
      </StaticBootstrapProvider>,
    );
    expect(screen.getByTestId("tenant").textContent).toBe("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b");
    expect(screen.getByTestId("admin").textContent).toBe("false");
  });

  test("useFeature membaca features.modulesAdminApi", () => {
    render(
      <StaticBootstrapProvider value={makeBootstrap({ features: { modulesAdminApi: true } })}>
        <Probe />
      </StaticBootstrapProvider>,
    );
    expect(screen.getByTestId("admin").textContent).toBe("true");
  });

  test("hook di luar provider melempar galat yang jelas", () => {
    const original = console.error;
    console.error = () => {};
    try {
      expect(() => render(<Probe />)).toThrow(/BootstrapProvider/);
    } finally {
      console.error = original;
    }
  });
});

import { afterEach, describe, expect, test } from "bun:test";
import { createClient, type PlatformPaths, setDefaultTransport } from "@erpora/platform-contracts";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import { createAuthController } from "./controller";
import { AuthProvider, useAuth } from "./provider";
import { apiError, fakeServer, json, session } from "./test-support";

afterEach(() => setDefaultTransport(undefined));

function Probe() {
  const { state } = useAuth();
  return <span data-testid="status">{state.status}</span>;
}

function setup(server: ReturnType<typeof fakeServer>, opts: { boot?: "refresh" | "skip" } = {}) {
  const queryClient = new QueryClient();
  const controller = createAuthController({ fetch: server.fetch });
  const view = render(
    <QueryClientProvider client={queryClient}>
      <AuthProvider controller={controller} boot={opts.boot}>
        <Probe />
      </AuthProvider>
    </QueryClientProvider>,
  );
  return { queryClient, controller, ...view };
}

describe("AuthProvider", () => {
  test("boot refresh: senyap menukar cookie; sukses -> authenticated", async () => {
    const server = fakeServer({ "/api/core/auth/refresh": () => session("t") });
    setup(server);
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("authenticated"));
    expect(server.count("/api/core/auth/refresh")).toBe(1);
  });

  test("boot skip: langsung anonymous tanpa jaringan", async () => {
    const server = fakeServer({});
    setup(server, { boot: "skip" });
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("anonymous"));
    expect(server.calls).toHaveLength(0);
  });

  test("transport bawaan klien kontrak dipasang: semua createClient membawa Bearer dan menyegarkan token", async () => {
    const server = fakeServer({
      "/api/core/auth/refresh": () => session("baru"),
      "/api/bootstrap": (c) =>
        c.authorization === "Bearer baru" ? json({ data: {} }) : apiError(401, "token_expired"),
    });
    const { controller } = setup(server, { boot: "skip" });
    await act(async () => {
      controller.acceptSession({ accessToken: "lama", expiresIn: 600, mfaState: "none" });
    });
    // Klien tanpa opsi fetch/headers, persis seperti `client` hasil generate modul.
    const res = await createClient<PlatformPaths>().get("/api/bootstrap");
    expect(res.ok).toBe(true);
    expect(
      server.calls.filter((c) => c.path === "/api/bootstrap").map((c) => c.authorization),
    ).toEqual(["Bearer lama", "Bearer baru"]);
  });

  test("transport dilepas saat provider dilepas", async () => {
    const server = fakeServer({ "/api/bootstrap": () => json({ data: {} }) });
    const { controller, unmount } = setup(server, { boot: "skip" });
    await act(async () => {
      controller.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    });
    unmount();
    const raw = globalThis.fetch;
    globalThis.fetch = server.fetch;
    try {
      await createClient<PlatformPaths>().get("/api/bootstrap");
    } finally {
      globalThis.fetch = raw;
    }
    expect(server.calls.at(-1)?.authorization).toBeNull();
  });

  test("refresh gagal saat request: ke anonymous DAN cache query dibersihkan", async () => {
    const server = fakeServer({
      "/api/example/notes": () => apiError(401, "token_expired"),
      "/api/core/auth/refresh": () => apiError(401, "unauthenticated"),
    });
    const { controller, queryClient } = setup(server, { boot: "skip" });
    await act(async () => {
      controller.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    });
    queryClient.setQueryData(["tenant-1", "current-user"], { displayName: "Ani" });
    queryClient.setQueryData(["bootstrap"], { tenantId: "tenant-1" });
    expect(queryClient.getQueryCache().getAll()).toHaveLength(2);

    await act(async () => {
      await controller.fetch("/api/example/notes");
    });
    await waitFor(() => expect(screen.getByTestId("status").textContent).toBe("anonymous"));
    expect(queryClient.getQueryCache().getAll()).toHaveLength(0);
  });

  test("logout membersihkan cache query; sesi yang tidak pernah login tidak membersihkan apa pun", async () => {
    const server = fakeServer({
      "/api/core/auth/logout": () => new Response(null, { status: 204 }),
    });
    const { controller, queryClient } = setup(server, { boot: "skip" });
    queryClient.setQueryData(["x"], 1);
    await act(async () => {
      await controller.logout();
    });
    expect(queryClient.getQueryData(["x"])).toBe(1); // sudah anonymous sebelumnya: tidak ada transisi

    await act(async () => {
      controller.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    });
    await act(async () => {
      await controller.logout();
    });
    expect(queryClient.getQueryData(["x"])).toBeUndefined();
  });
});

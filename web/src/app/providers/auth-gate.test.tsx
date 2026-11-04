import { beforeEach, describe, expect, test } from "bun:test";
import { createAuthController } from "@erpora/platform-web";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderApp } from "../test-utils";

const REFRESH = "/api/core/auth/refresh";
const LOGIN = "/api/core/auth/login";
const VERIFY = "/api/core/auth/mfa/verify";
const LOGOUT = "/api/core/auth/logout";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const session = (token: string, mfaState = "none") =>
  json({ data: { accessToken: token, expiresIn: 600, mfaState } });
const apiError = (status: number, code: string) => json({ error: { code, message: "x" } }, status);

type Route = (call: { method: string; body: unknown }) => Response | Promise<Response>;

/** Controller dengan fetch palsu untuk /api/core/auth/*; `calls` mencatat path yang dipanggil. */
function authWith(routes: Record<string, Route>, initial: "booting" | "anonymous" = "booting") {
  const calls: string[] = [];
  const fetchFn = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const path = new URL(String(input), "http://localhost").pathname;
    calls.push(path);
    const handler = routes[path];
    const raw = init?.body;
    return handler
      ? handler({
          method: (init?.method ?? "GET").toUpperCase(),
          body: typeof raw === "string" ? JSON.parse(raw) : undefined,
        })
      : apiError(404, "not_found");
  }) as typeof fetch;
  return { auth: createAuthController({ fetch: fetchFn, initial }), calls };
}

beforeEach(() => {
  localStorage.clear();
  // happy-dom: navigator.languages en-US; gerbang dan shell diuji berbahasa Indonesia.
  localStorage.setItem("erpora.locale", "id");
  window.history.replaceState(null, "", "/");
});

describe("gerbang autentikasi pra-bootstrap (Q33)", () => {
  test("boot: refresh senyap gagal menampilkan login TANPA memanggil bootstrap; deep link menjadi returnTo", async () => {
    window.history.replaceState(null, "", "/example/notes?page=2");
    const { auth, calls } = authWith({ [REFRESH]: () => apiError(401, "unauthenticated") });
    const app = renderApp({ auth });
    expect(await screen.findByRole("heading", { level: 1, name: "Masuk" })).toBeInTheDocument();
    expect(calls).toEqual([REFRESH]);
    expect(app.server.requests).not.toContain("/api/bootstrap");
    expect(window.location.pathname).toBe("/login");
    expect(new URLSearchParams(window.location.search).get("returnTo")).toBe(
      "/example/notes?page=2",
    );
    // Tanpa kerangka shell: tidak ada navigasi utama/Dock di layar login.
    expect(screen.queryByRole("main", { name: /./ })).toBeNull();
  });

  test("boot: refresh senyap berhasil langsung membuka aplikasi (bootstrap dimuat setelah login)", async () => {
    const { auth, calls } = authWith({ [REFRESH]: () => session("t-boot") });
    const app = renderApp({ auth });
    await screen.findByRole("main");
    expect(calls).toEqual([REFRESH]);
    expect(app.server.requests).toContain("/api/bootstrap");
  });

  test("login sukses mengarahkan ke returnTo yang aman, baru setelah itu bootstrap dimuat", async () => {
    window.history.replaceState(null, "", "/login?returnTo=%2Fexample%2Fnotes");
    const { auth } = authWith({
      [REFRESH]: () => apiError(401, "unauthenticated"),
      [LOGIN]: () => session("t-login"),
    });
    const app = renderApp({ auth });
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Email"), "ani@example.test");
    await user.type(screen.getByLabelText("Sandi"), "kata-sandi-panjang-2026");
    expect(app.server.requests).not.toContain("/api/bootstrap");
    await user.click(screen.getByRole("button", { name: "Masuk" }));
    await screen.findByRole("main");
    expect(window.location.pathname).toBe("/example/notes");
    expect(app.server.requests).toContain("/api/bootstrap");
  });

  test("returnTo berbahaya diabaikan: kembali ke /", async () => {
    window.history.replaceState(null, "", "/login?returnTo=%2F%2Fevil.com");
    const { auth } = authWith({
      [REFRESH]: () => apiError(401, "unauthenticated"),
      [LOGIN]: () => session("t"),
    });
    renderApp({ auth });
    const user = userEvent.setup();
    await user.type(await screen.findByLabelText("Email"), "ani@example.test");
    await user.type(screen.getByLabelText("Sandi"), "kata-sandi-panjang-2026");
    await user.click(screen.getByRole("button", { name: "Masuk" }));
    await screen.findByRole("main");
    expect(window.location.pathname).toBe("/");
    expect(window.location.host).toBe("localhost");
  });

  test("mfaState tertunda: layar MFA tampil dan aplikasi (bootstrap) belum dimuat sampai verifikasi", async () => {
    const { auth } = authWith({
      [REFRESH]: () => session("t-pra", "verify_required"),
      [VERIFY]: () => session("t-otp"),
    });
    const app = renderApp({ auth });
    expect(
      await screen.findByRole("heading", { name: "Verifikasi dua langkah" }),
    ).toBeInTheDocument();
    expect(app.server.requests).not.toContain("/api/bootstrap");
    await userEvent.setup().type(screen.getByLabelText("Kode verifikasi"), "123456");
    await screen.findByRole("main");
    expect(app.server.requests).toContain("/api/bootstrap");
  });

  test("/forgot-password dan /reset-password tidak memicu refresh senyap dan menampilkan layarnya", async () => {
    window.history.replaceState(null, "", "/forgot-password");
    const first = authWith({ [REFRESH]: () => session("t") });
    const a = renderApp({ auth: first.auth });
    expect(await screen.findByRole("heading", { name: "Lupa sandi" })).toBeInTheDocument();
    expect(first.calls).toEqual([]);
    expect(a.server.requests).not.toContain("/api/bootstrap");
    a.unmount();

    window.history.replaceState(null, "", "/reset-password#token=abc");
    const second = authWith({});
    renderApp({ auth: second.auth });
    expect(await screen.findByRole("heading", { name: "Atur ulang sandi" })).toBeInTheDocument();
    expect(second.calls).toEqual([]);
    expect(window.location.hash).toBe(""); // token dihapus dari URL
  });

  test("pengguna yang sudah login membuka /login diarahkan ke returnTo tanpa menampilkan form", async () => {
    window.history.replaceState(null, "", "/login?returnTo=%2Fexample");
    const { auth } = authWith({ [REFRESH]: () => session("t") });
    renderApp({ auth });
    await screen.findByRole("main");
    expect(window.location.pathname).toBe("/example");
    expect(screen.queryByLabelText("Sandi")).toBeNull();
  });
});

describe("sesi berakhir dan keluar", () => {
  test("refresh gagal saat aplikasi berjalan: kembali ke login dan cache query dibersihkan", async () => {
    const { auth } = authWith({ [REFRESH]: () => apiError(401, "unauthenticated") }, "anonymous");
    auth.acceptSession({ accessToken: "t", expiresIn: 600, mfaState: "none" });
    const app = renderApp({ auth });
    await screen.findByRole("main");
    expect(app.queryClient.getQueryCache().getAll().length).toBeGreaterThan(0);

    await act(async () => {
      // Klien lain (mis. halaman modul) menerima 401 token_expired lalu refresh ditolak.
      auth.endSession();
    });
    expect(await screen.findByRole("heading", { level: 1, name: "Masuk" })).toBeInTheDocument();
    expect(app.queryClient.getQueryCache().getAll()).toHaveLength(0);
  });

  test("Keluar dari menu pengguna: POST logout, token dibuang, cache bersih, /login tanpa returnTo", async () => {
    window.history.replaceState(null, "", "/");
    const { auth, calls } = authWith(
      { [REFRESH]: () => session("t"), [LOGOUT]: () => new Response(null, { status: 204 }) },
      "booting",
    );
    const app = renderApp({
      auth,
      bootstrap: { user: { id: "u1", email: "ani@example.test", displayName: "Ani" } },
    });
    await screen.findByRole("main");
    const user = userEvent.setup();
    // Menu pengguna dibuka dan dioperasikan lewat papan ketik: klik penunjuk pada komponen Radix di dalam shell penuh
    // tidak andal di happy-dom (menu kadang tidak terbuka bergantung pada urutan uji).
    const trigger = (
      await screen.findAllByRole("button", { name: "Menu akun Ani" }, { timeout: 8000 })
    )[0] as HTMLElement;
    trigger.focus();
    await user.keyboard("{Enter}");
    const out = await screen.findByRole("menuitem", { name: "Keluar" }, { timeout: 8000 });
    out.focus();
    await user.keyboard("{Enter}");

    expect(
      await screen.findByRole("heading", { level: 1, name: "Masuk" }, { timeout: 8000 }),
    ).toBeInTheDocument();
    // B-1: judul tab milik layar login, bukan judul halaman shell sebelumnya.
    expect(document.title).toBe("Masuk - ERPora");
    expect(calls).toContain(LOGOUT);
    expect(auth.getAccessToken()).toBeUndefined();
    await waitFor(() => expect(window.location.pathname).toBe("/login"));
    expect(window.location.search).toBe("");
    expect(app.queryClient.getQueryCache().getAll()).toHaveLength(0);
  }, 30_000);

  test("Keluar gagal (server 500 atau 429): sesi tetap, tetap di aplikasi, galat tampil, token dan cache utuh (I-1)", async () => {
    for (const failure of [
      () => apiError(500, "internal_error"),
      () =>
        new Response(JSON.stringify({ error: { code: "rate_limited", message: "x" } }), {
          status: 429,
          headers: { "Content-Type": "application/json", "Retry-After": "30" },
        }),
    ]) {
      window.history.replaceState(null, "", "/");
      const { auth, calls } = authWith({ [REFRESH]: () => session("t"), [LOGOUT]: failure });
      const app = renderApp({
        auth,
        bootstrap: { user: { id: "u1", email: "ani@example.test", displayName: "Ani" } },
      });
      await screen.findByRole("main");
      const user = userEvent.setup();
      const trigger = (
        await screen.findAllByRole("button", { name: "Menu akun Ani" }, { timeout: 8000 })
      )[0] as HTMLElement;
      trigger.focus();
      await user.keyboard("{Enter}");
      const out = await screen.findByRole("menuitem", { name: "Keluar" }, { timeout: 8000 });
      out.focus();
      await user.keyboard("{Enter}");

      await waitFor(() => expect(calls).toContain(LOGOUT), { timeout: 8000 });
      expect(
        await screen.findByText(
          "Gagal keluar. Sesi Anda masih aktif; coba lagi.",
          {},
          { timeout: 8000 },
        ),
      ).toBeInTheDocument();
      await waitFor(() => expect(auth.getAccessToken()).toBe("t"));
      expect(auth.getState()).toEqual({ status: "authenticated" });
      expect(window.location.pathname).toBe("/");
      expect(screen.queryByRole("heading", { level: 1, name: "Masuk" })).toBeNull();
      expect(app.queryClient.getQueryCache().getAll().length).toBeGreaterThan(0);
      app.unmount();
      (await import("@erpora/ui/toaster")).toast.dismiss();
    }
  }, 40_000);
});

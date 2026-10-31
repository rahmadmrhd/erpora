import { afterEach, beforeEach, describe, expect, test } from "bun:test";
import {
  type Desk,
  makeDashboardItem,
  makeDesk,
  makeDeskItem,
  SHOW_DESKS_STATE,
} from "@erpora/platform-web";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DashboardPage } from "./dashboard-page";
import { DeskEntry } from "./desk-entry";
import { DesksPage } from "./desks-page";
import { renderPage } from "./harness";
import { TodoPlaceholderPage } from "./todo-placeholder";

const platform = makeDesk(
  "platform",
  [
    makeDashboardItem("platform", "platform"),
    makeDeskItem("platform", "modules", "/platform/modules"),
  ],
  { source: "platform", label: { ns: "platform", key: "desk.title" } },
);
const example = makeDesk(
  "example",
  [makeDashboardItem("example", "example"), makeDeskItem("example", "notes", "/example/notes")],
  { moduleKey: "acme.example", icon: "file-text" },
);
const desks: Desk[] = [platform, example];

/** Permintaan yang tidak pernah selesai: menahan keadaan loading. */
const pending = (() => new Promise<Response>(() => {})) as unknown as typeof fetch;

const Marker = (name: string) => () => <p>{`halaman ${name}`}</p>;
const routes = [
  { path: "/", component: DeskEntry },
  { path: "/platform", component: Marker("platform") },
  { path: "/example", component: Marker("example") },
  { path: "/platform/todo", component: TodoPlaceholderPage },
];

beforeEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});
afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("DeskEntry (R10.3, R10.5)", () => {
  test("memori tab: mengarahkan ke Dashboard Desk itu dengan replace (tanpa entri riwayat tambahan)", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const app = renderPage({ desks, routes });
    expect(await screen.findByText("halaman example")).toBeInTheDocument();
    expect(app.router.state.location.pathname).toBe("/example");
    expect(app.history.length).toBe(1);
  });

  test("localStorage dipakai bila tab kosong; default_desk tidak tersedia (null)", async () => {
    localStorage.setItem("erpora.desk", "platform");
    renderPage({ desks, routes });
    expect(await screen.findByText("halaman platform")).toBeInTheDocument();
  });

  test("tidak ada kandidat valid: halaman Desks tampil, tanpa navigasi", async () => {
    const app = renderPage({ desks, routes });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Platform/ })).toHaveAttribute("href", "/platform");
    expect(screen.getByRole("link", { name: /Catatan Contoh/ })).toHaveAttribute(
      "href",
      "/example",
    );
    expect(app.router.state.location.pathname).toBe("/");
  });

  test("nilai storage berbahaya atau Desk yang tidak diizinkan diabaikan", async () => {
    sessionStorage.setItem("erpora.desk.tab", "<script>");
    localStorage.setItem("erpora.desk", "hilang");
    const app = renderPage({ desks, routes });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    expect(app.router.state.location.pathname).toBe("/");
  });

  test("state showDesks melewati penentuan Desk walau ada Desk tersimpan (Home)", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const app = renderPage({ desks, routes, state: { ...SHOW_DESKS_STATE } });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    expect(app.router.state.location.pathname).toBe("/");
    expect(screen.queryByText("halaman example")).toBeNull();
  });

  test("selama daftar dimuat: kerangka (Skeleton), bukan halaman Desks kosong", async () => {
    renderPage({ desks, routes, fetchImpl: pending });
    expect(await screen.findByTestId("desk-entry-loading")).toBeInTheDocument();
    expect(screen.queryByText("Belum ada Desk")).toBeNull();
  });

  test("galat memuat daftar: pesan galat dengan Coba lagi", async () => {
    let n = 0;
    const app = renderPage({
      desks,
      routes,
      fetchImpl: (async () => {
        n += 1;
        return new Response(JSON.stringify({ error: { code: "internal_error", message: "x" } }), {
          status: 500,
        });
      }) as unknown as typeof fetch,
    });
    expect(await screen.findByText("Daftar Desk tidak dapat dimuat")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Coba lagi" }));
    await waitFor(() => expect(n).toBeGreaterThan(1));
    expect(app.router.state.location.pathname).toBe("/");
  });
});

describe("penanda showDesks dikonsumsi (R10.5)", () => {
  test("tampilan Desks dipertahankan, penanda dihapus dari entri riwayat; reload mengikuti penentuan biasa", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const first = renderPage({ desks, routes, state: { ...SHOW_DESKS_STATE } });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    await waitFor(() => expect(first.history.location.state).not.toHaveProperty("showDesks"));
    // Tampilan tidak berubah walau penanda sudah dihapus dari entri.
    expect(screen.getByRole("heading", { name: "Desk" })).toBeInTheDocument();
    expect(first.history.location.pathname).toBe("/");
    // Muat ulang: aplikasi baru di atas riwayat yang sama (history.state dipertahankan peramban).
    first.unmount();
    const reloaded = renderPage({ desks, routes, history: first.history });
    expect(await screen.findByText("halaman example")).toBeInTheDocument();
    expect(reloaded.history.location.pathname).toBe("/example");
  });

  test("hanya penanda yang dihapus; state lain di entri tetap", async () => {
    const app = renderPage({ desks, routes, state: { ...SHOW_DESKS_STATE, lain: 1 } });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    await waitFor(() => expect(app.history.location.state).not.toHaveProperty("showDesks"));
    expect(app.history.location.state).toMatchObject({ lain: 1 });
  });

  test("Home lagi saat Desks tampil: tetap Desks (penanda baru dikonsumsi lagi), tanpa pengalihan", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const app = renderPage({ desks, routes, state: { ...SHOW_DESKS_STATE } });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    await waitFor(() => expect(app.history.location.state).not.toHaveProperty("showDesks"));
    await act(async () => {
      await app.router.navigate({ to: "/", state: SHOW_DESKS_STATE as never });
    });
    await waitFor(() => expect(app.history.location.state).not.toHaveProperty("showDesks"));
    expect(screen.getByRole("heading", { name: "Desk" })).toBeInTheDocument();
    expect(app.history.location.pathname).toBe("/");
  });

  test("navigasi ke / tanpa penanda saat Desks tampil: kembali ke penentuan biasa", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const app = renderPage({ desks, routes, state: { ...SHOW_DESKS_STATE } });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    await waitFor(() => expect(app.history.location.state).not.toHaveProperty("showDesks"));
    await act(async () => {
      await app.router.navigate({ to: "/", search: { a: "1" } as never });
    });
    expect(await screen.findByText("halaman example")).toBeInTheDocument();
  });

  test("Back ke entri Desks (penanda sudah dikonsumsi) mengikuti penentuan biasa", async () => {
    sessionStorage.setItem("erpora.desk.tab", "example");
    const app = renderPage({ desks, routes, state: { ...SHOW_DESKS_STATE } });
    expect(await screen.findByRole("heading", { name: "Desk" })).toBeInTheDocument();
    await waitFor(() => expect(app.history.location.state).not.toHaveProperty("showDesks"));
    await act(async () => {
      await app.router.navigate({ to: "/platform" });
    });
    expect(await screen.findByText("halaman platform")).toBeInTheDocument();
    await act(async () => {
      app.history.back();
    });
    expect(await screen.findByText("halaman example")).toBeInTheDocument();
  });
});

describe("DesksPage (R10.6)", () => {
  test("klik kartu menetapkan Desk (storage) dan menuju Dashboard-nya", async () => {
    const app = renderPage({ desks, routes });
    await userEvent.click(await screen.findByRole("link", { name: /Catatan Contoh/ }));
    expect(await screen.findByText("halaman example")).toBeInTheDocument();
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("example");
    expect(localStorage.getItem("erpora.desk")).toBe("example");
    expect(app.router.state.location.pathname).toBe("/example");
  });

  test("keadaan kosong bila tidak ada Desk yang diizinkan", async () => {
    renderPage({ desks: [], routes: [{ path: "/", component: DesksPage }] });
    expect(await screen.findByText("Belum ada Desk")).toBeInTheDocument();
    expect(screen.queryByRole("link")).toBeNull();
  });
});

describe("DashboardPage (R14)", () => {
  const dash = (deskId: string) => () => <DashboardPage deskId={deskId} />;

  test("Skeleton selama daftar Desk dimuat", async () => {
    renderPage({
      desks,
      routes: [{ path: "/", component: dash("example") }],
      fetchImpl: pending,
    });
    expect(await screen.findByTestId("dashboard-loading")).toBeInTheDocument();
  });

  test("Desk ada: judul Desk, wilayah slot widget, dan Empty karena belum ada widget", async () => {
    renderPage({ desks, routes: [{ path: "/", component: dash("example") }] });
    expect(await screen.findByRole("heading", { name: "Catatan Contoh" })).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Widget Dashboard" })).toBeInTheDocument();
    expect(screen.getByText("Dashboard Belum Tersedia")).toBeInTheDocument();
  });

  test("Desk tidak ada atau tidak diizinkan: pesan tidak tersedia tanpa membocorkan sebabnya", async () => {
    renderPage({ desks, routes: [{ path: "/", component: dash("rahasia") }] });
    expect(await screen.findByText("Dashboard tidak tersedia")).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: "Widget Dashboard" })).toBeNull();
  });
});

describe("TodoPlaceholderPage", () => {
  test("placeholder ToDo dengan keadaan kosong", async () => {
    renderPage({ desks, routes, path: "/platform/todo" });
    expect(await screen.findByText("ToDo")).toBeInTheDocument();
    expect(
      screen.getByText("Daftar tugas akan hadir pada pembaruan berikutnya."),
    ).toBeInTheDocument();
  });
});

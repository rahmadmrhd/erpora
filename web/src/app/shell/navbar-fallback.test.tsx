import { beforeEach, describe, expect, spyOn, test } from "bun:test";
import { makeBootstrapModule } from "@erpora/platform-web";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createFakeServer, renderApp } from "../test-utils";

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

function failingServer() {
  const server = createFakeServer([
    makeBootstrapModule("acme.alpha", "alpha", "enabled"),
    makeBootstrapModule("acme.beta", "beta", "enabled"),
  ]);
  server.failNamespaces.add("gamma");
  return server;
}

describe("fallback galat Navbar (F-6)", () => {
  test("galat render di Navbar: tombol collapse tetap ada, konten halaman tetap, galat dicatat sekali", async () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      renderApp({ path: "/alpha/crumbfail", server: failingServer() });
      expect(await screen.findByText("Isi Crumb Gagal")).toBeInTheDocument();
      const header = await screen.findByRole("banner");
      await waitFor(() =>
        expect(
          within(header).getByRole("button", { name: "Ciutkan atau perluas sidebar" }),
        ).toBeInTheDocument(),
      );
      // sidebar dan konten tetap; minimal breadcrumb berupa path
      expect(screen.getByRole("navigation", { name: "Navigasi utama" })).toBeInTheDocument();
      expect(within(header).getByText("/alpha/crumbfail")).toBeInTheDocument();
      const logged = err.mock.calls.filter((c) => String(c[0]).includes("[erpora] navbar"));
      expect(logged).toHaveLength(1);
      expect(String(logged[0]?.[0])).not.toContain("at ");
    } finally {
      err.mockRestore();
    }
  });

  test("tombol collapse pada fallback masih berfungsi (membalik keadaan sidebar)", async () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      const user = userEvent.setup();
      renderApp({ path: "/alpha/crumbfail", server: failingServer() });
      await screen.findByText("Isi Crumb Gagal");
      const header = await screen.findByRole("banner");
      const button = await within(header).findByRole("button", {
        name: "Ciutkan atau perluas sidebar",
      });
      const sidebar = document.querySelector('[data-slot="sidebar"]');
      expect(sidebar).toHaveAttribute("data-state", "expanded");
      await user.click(button);
      await waitFor(() => expect(sidebar).toHaveAttribute("data-state", "collapsed"));
    } finally {
      err.mockRestore();
    }
  });
});

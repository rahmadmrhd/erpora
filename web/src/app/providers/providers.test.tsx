import { beforeEach, describe, expect, test } from "bun:test";
import { screen } from "@testing-library/react";
import { renderApp } from "../test-utils";

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

describe("Providers", () => {
  test("Toaster dimuat malas dari @erpora/ui/toaster dan wilayah notifikasinya bernama dari label ui", async () => {
    renderApp();
    await screen.findByRole("navigation", { name: "Navigasi utama" });
    expect(await screen.findByRole("region", { name: /^Notifikasi/ })).toBeInTheDocument();
  });

  test("kerangka memuat tampil dulu (status sibuk) sebelum bootstrap siap", () => {
    renderApp();
    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
  });

  test("hanya satu QueryClient: key bootstrap dan katalog berada di cache yang sama", async () => {
    const app = renderApp();
    await screen.findByRole("navigation", { name: "Navigasi utama" });
    const keys = app.queryClient
      .getQueryCache()
      .getAll()
      .map((q) => q.queryKey);
    expect(keys).toContainEqual(["bootstrap"]);
    expect(keys.some((k) => k[1] === "i18n" && k[3] === "platform")).toBe(true);
    for (const k of keys) {
      if (k[0] !== "bootstrap") expect(k[0]).toBe("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b");
    }
  });
});

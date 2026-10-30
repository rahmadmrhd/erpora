import { describe, expect, spyOn, test } from "bun:test";
import { defaultOfflinePolicy, defineModule } from "@erpora/module-sdk";
import { render, screen } from "@testing-library/react";
import { StaticBootstrapProvider } from "../bootstrap";
import { makeBootstrap, makeBootstrapModule } from "../bootstrap/testing";
import { DuplicateModuleKeyError, ModuleRegistryProvider, useModuleRegistry } from "./index";

const mod = (key: string, slug: string) =>
  defineModule({
    key,
    slug,
    version: "1.0.0",
    navigation: [{ type: "item", id: "home", label: { ns: slug, key: "nav.home" }, to: "/" }],
    registerRoutes: () => [],
    offline: defaultOfflinePolicy,
  });

function Probe() {
  const r = useModuleRegistry();
  return (
    <div>
      <span data-testid="nav">{r.navigation.map((n) => n.id).join(",")}</span>
      <span data-testid="status">{r.statusOf("acme.one") ?? "-"}</span>
      <span data-testid="enabled">{String(r.isEnabled("acme.one"))}</span>
      <span data-testid="resolved">{r.resolved.map((x) => x.module.key).join(",")}</span>
    </div>
  );
}

const modules = [mod("acme.one", "one"), mod("acme.two", "two")];

function renderWith(status: "enabled" | "disabled", withTwo = true) {
  const entries = [makeBootstrapModule("acme.one", "one", status)];
  if (withTwo) entries.push(makeBootstrapModule("acme.two", "two", "enabled"));
  const value = makeBootstrap({ modules: entries });
  const ui = (v: typeof value) => (
    <StaticBootstrapProvider value={v}>
      <ModuleRegistryProvider modules={modules} warn={() => {}}>
        <Probe />
      </ModuleRegistryProvider>
    </StaticBootstrapProvider>
  );
  return { ...render(ui(value)), ui };
}

describe("useModuleRegistry", () => {
  test("navigasi hanya modul enabled; statusOf dan isEnabled", () => {
    renderWith("enabled");
    expect(screen.getByTestId("nav").textContent).toBe("home,home");
    expect(screen.getByTestId("status").textContent).toBe("enabled");
    expect(screen.getByTestId("enabled").textContent).toBe("true");
  });

  test("bereaksi terhadap perubahan bootstrap tanpa menghitung ulang resolved", () => {
    const { rerender, ui } = renderWith("enabled");
    const before = screen.getByTestId("resolved").textContent;
    rerender(
      ui(
        makeBootstrap({
          revision: 2,
          modules: [
            makeBootstrapModule("acme.one", "one", "disabled"),
            makeBootstrapModule("acme.two", "two", "enabled"),
          ],
        }),
      ),
    );
    expect(screen.getByTestId("status").textContent).toBe("disabled");
    expect(screen.getByTestId("enabled").textContent).toBe("false");
    expect(screen.getByTestId("nav").textContent).toBe("home");
    expect(screen.getByTestId("resolved").textContent).toBe(before);
    rerender(
      ui(
        makeBootstrap({
          revision: 3,
          modules: [
            makeBootstrapModule("acme.one", "one", "enabled"),
            makeBootstrapModule("acme.two", "two", "enabled"),
          ],
        }),
      ),
    );
    expect(screen.getByTestId("nav").textContent).toBe("home,home");
  });

  test("modul tidak ada di bootstrap: tidak resolved, statusOf undefined", () => {
    renderWith("enabled", false);
    expect(screen.getByTestId("resolved").textContent).toBe("acme.one");
    expect(screen.getByTestId("nav").textContent).toBe("home");
  });

  test("key ganda menggagalkan render dengan DuplicateModuleKeyError", () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      expect(() =>
        render(
          <StaticBootstrapProvider value={makeBootstrap()}>
            <ModuleRegistryProvider
              modules={[mod("acme.one", "one"), mod("acme.one", "uno")]}
              warn={() => {}}
            >
              <Probe />
            </ModuleRegistryProvider>
          </StaticBootstrapProvider>,
        ),
      ).toThrow(DuplicateModuleKeyError);
    } finally {
      err.mockRestore();
    }
  });

  test("hook di luar ModuleRegistryProvider melempar galat yang jelas", () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      expect(() =>
        render(
          <StaticBootstrapProvider value={makeBootstrap()}>
            <Probe />
          </StaticBootstrapProvider>,
        ),
      ).toThrow(/ModuleRegistryProvider/);
    } finally {
      err.mockRestore();
    }
  });
});

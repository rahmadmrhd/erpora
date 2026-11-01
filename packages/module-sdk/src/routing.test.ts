import { describe, expect, test } from "bun:test";
import { QueryClient } from "@tanstack/react-query";
import { createMemoryHistory, createRouter } from "@tanstack/react-router";
import { ModuleDefinitionError } from "./define-module";
import {
  attachModuleRoutes,
  createAppRootRoute,
  createModuleBaseRoute,
  createRoute,
} from "./routing";

function buildRouter(initial: string, beforeLoad?: () => void) {
  const root = createAppRootRoute();
  const base = createModuleBaseRoute(root, "example", { beforeLoad });
  const notes = createRoute({
    getParentRoute: () => base,
    path: "/notes",
    staticData: { crumb: { text: "Catatan" } },
  });
  const tree = root.addChildren([attachModuleRoutes(base, [notes])]);
  return createRouter({
    routeTree: tree,
    history: createMemoryHistory({ initialEntries: [initial] }),
    context: { queryClient: new QueryClient() },
  });
}

describe("route modul", () => {
  test("anak base modul terpasang di /<slug>/<path> dan membawa staticData.crumb", async () => {
    const router = buildRouter("/example/notes");
    await router.load();
    const ids = router.state.matches.map((m) => m.pathname);
    expect(ids).toContain("/example/notes");
    const last = router.state.matches.at(-1);
    expect(last?.staticData.crumb).toEqual({ text: "Catatan" });
  });

  test("beforeLoad base dijalankan sebelum anak dimuat (dasar guard)", async () => {
    let calls = 0;
    const router = buildRouter("/example/notes", () => {
      calls += 1;
    });
    await router.load();
    expect(calls).toBe(1);
  });

  test("path di luar modul tidak cocok dengan anak modul", async () => {
    const router = buildRouter("/lain/notes");
    await router.load();
    expect(router.state.matches.some((m) => m.pathname === "/lain/notes")).toBe(false);
  });
});

describe("createAppRootRoute dengan opsi", () => {
  test("component, notFoundComponent, dan errorComponent diteruskan ke route akar", () => {
    const component = () => null;
    const notFoundComponent = () => null;
    const errorComponent = () => null;
    const root = createAppRootRoute({ component, notFoundComponent, errorComponent });
    expect(root.options.component).toBe(component);
    expect(root.options.notFoundComponent).toBe(notFoundComponent);
    expect(root.options.errorComponent).toBe(errorComponent);
  });
});

describe("attachModuleRoutes menolak rute indeks (R10.8)", () => {
  const make = (path: string) => {
    const root = createAppRootRoute();
    const base = createModuleBaseRoute(root, "example");
    const child = createRoute({ getParentRoute: () => base, path });
    return { base, child };
  };

  test('path "/" ditolak dengan ModuleDefinitionError yang menyebut modul dan rute indeks', () => {
    const { base, child } = make("/");
    let err: unknown;
    try {
      attachModuleRoutes(base, [child]);
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(ModuleDefinitionError);
    expect((err as Error).message).toContain("/example");
    expect((err as Error).message).toContain("indeks");
  });

  test("anak lain tetap diterima, termasuk bersarang dan tanpa indeks di dalamnya", () => {
    const { base, child } = make("/notes");
    expect(() => attachModuleRoutes(base, [child])).not.toThrow();
  });

  test("indeks di antara anak lain tetap ditolak", () => {
    const { base, child } = make("/notes");
    const index = createRoute({ getParentRoute: () => base, path: "/" });
    expect(() => attachModuleRoutes(base, [child, index])).toThrow(ModuleDefinitionError);
  });
});

describe("attachModuleRoutes dengan indeks milik shell", () => {
  test("opsi index memasang Dashboard di /<slug> tanpa ditolak, di samping anak modul", async () => {
    const root = createAppRootRoute();
    const base = createModuleBaseRoute(root, "example");
    const notes = createRoute({ getParentRoute: () => base, path: "/notes" });
    const index = createRoute({ getParentRoute: () => base, path: "/" });
    const tree = root.addChildren([attachModuleRoutes(base, [notes], { index })]);
    const router = createRouter({
      routeTree: tree,
      history: createMemoryHistory({ initialEntries: ["/example"] }),
      context: { queryClient: new QueryClient() },
    });
    await router.load();
    expect(router.state.matches.at(-1)?.routeId).toBe(index.id);
  });

  test("opsi index harus berupa rute indeks", () => {
    const root = createAppRootRoute();
    const base = createModuleBaseRoute(root, "example");
    const notRoot = createRoute({ getParentRoute: () => base, path: "/notes" });
    expect(() => attachModuleRoutes(base, [], { index: notRoot })).toThrow(ModuleDefinitionError);
  });
});

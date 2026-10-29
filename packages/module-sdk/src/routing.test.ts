import { describe, expect, test } from "bun:test";
import { QueryClient } from "@tanstack/react-query";
import { createMemoryHistory, createRouter } from "@tanstack/react-router";
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

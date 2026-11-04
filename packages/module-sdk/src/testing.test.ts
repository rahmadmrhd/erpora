import { describe, expect, test } from "bun:test";
import { defaultOfflinePolicy, defineModule } from "./index";
import { createRoute } from "./routing";
import { assertDeskRoutes } from "./testing";

const mod = defineModule({
  key: "acme.example",
  slug: "example",
  version: "1.0.0",
  registerRoutes: (base) => {
    const notes = createRoute({ getParentRoute: () => base, path: "/notes" });
    const archived = createRoute({ getParentRoute: () => notes, path: "/archived" });
    const tags = createRoute({ getParentRoute: () => base, path: "/more/tags" });
    return [notes.addChildren([archived]) as never, tags];
  },
  offline: defaultOfflinePolicy,
});

const item = (id: string, to: string) => ({
  id,
  label: `nav.${id}`,
  to,
  permission: "example.note.read",
});

describe("assertDeskRoutes", () => {
  test("lolos bila setiap `to` ada di rute terdaftar (item, anak grup, rute bersarang)", () => {
    expect(() =>
      assertDeskRoutes(mod, {
        label: "desk.title",
        icon: "file-text",
        menus: [
          item("notes", "/notes"),
          { id: "g", label: "nav.g", children: [item("tags", "/more/tags")] },
          item("archived", "/notes/archived"),
        ],
      }),
    ).not.toThrow();
  });

  test("gagal dan menyebut `to` yang tidak punya rute", () => {
    expect(() =>
      assertDeskRoutes(mod, {
        label: "desk.title",
        icon: "file-text",
        menus: [item("notes", "/notes"), item("hilang", "/hilang")],
      }),
    ).toThrow(/\/hilang/);
  });

  test("gagal untuk `to` indeks (/) karena Dashboard dibangkitkan platform", () => {
    expect(() =>
      assertDeskRoutes(mod, {
        label: "desk.title",
        icon: "file-text",
        menus: [item("home", "/")],
      }),
    ).toThrow(/Dashboard|indeks/);
  });

  test("gagal bila manifest tidak punya desk", () => {
    expect(() => assertDeskRoutes(mod, undefined)).toThrow(/desk/);
  });

  test("kecocokan harus persis: awalan saja tidak cukup", () => {
    expect(() =>
      assertDeskRoutes(mod, {
        label: "desk.title",
        icon: "file-text",
        menus: [item("n", "/note")],
      }),
    ).toThrow(/\/note/);
  });
});

describe("listModuleRoutePaths", () => {
  test("mengembalikan path relatif base, termasuk rute bersarang, terurut", async () => {
    const { listModuleRoutePaths } = await import("./index");
    expect(listModuleRoutePaths(mod)).toEqual(["/more/tags", "/notes", "/notes/archived"]);
  });
});

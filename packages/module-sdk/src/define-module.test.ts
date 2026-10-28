import { describe, expect, test } from "bun:test";
import { defineModule, ModuleDefinitionError } from "./define-module";
import { defaultOfflinePolicy, type OfflinePolicy } from "./offline";
import type { FrontendModule, MessageRef } from "./types";

const label = (key: string): MessageRef => ({ ns: "example", key });

function valid(overrides: Partial<FrontendModule> = {}): FrontendModule {
  return {
    key: "acme.example",
    slug: "example",
    version: "1.0.0",
    navigation: [{ type: "item", id: "notes", label: label("nav.notes"), to: "/notes" }],
    registerRoutes: () => [],
    offline: defaultOfflinePolicy,
    ...overrides,
  };
}

function messageOf(fn: () => unknown): string {
  try {
    fn();
  } catch (err) {
    expect(err).toBeInstanceOf(ModuleDefinitionError);
    return (err as Error).message;
  }
  throw new Error("seharusnya melempar ModuleDefinitionError");
}

describe("defineModule: modul valid", () => {
  test("lolos dan hasilnya beku (modul, navigation, entri, offline)", () => {
    const m = defineModule(
      valid({
        navigation: [
          { type: "item", id: "home", label: label("nav.home"), to: "/", icon: "home" },
          {
            type: "group",
            id: "reports",
            label: label("nav.reports"),
            children: [{ type: "item", id: "sales", label: label("nav.sales"), to: "/reports/sales" }],
          },
        ],
      }),
    );
    expect(Object.isFrozen(m)).toBe(true);
    expect(Object.isFrozen(m.navigation)).toBe(true);
    for (const entry of m.navigation) {
      expect(Object.isFrozen(entry)).toBe(true);
      if (entry.type === "group") {
        expect(Object.isFrozen(entry.children)).toBe(true);
        for (const child of entry.children) expect(Object.isFrozen(child)).toBe(true);
      }
    }
    expect(Object.isFrozen(m.offline)).toBe(true);
    expect(() => {
      (m as { slug: string }).slug = "lain";
    }).toThrow();
  });

  test("mempertahankan field, termasuk title opsional", () => {
    const title = label("page.title");
    const m = defineModule(valid({ title }));
    expect(m.key).toBe("acme.example");
    expect(m.slug).toBe("example");
    expect(m.version).toBe("1.0.0");
    expect(m.title).toEqual(title);
  });

  test("tidak mengubah objek masukan (salinan beku)", () => {
    const input = valid();
    defineModule(input);
    expect(Object.isFrozen(input)).toBe(false);
    expect(Object.isFrozen(input.navigation)).toBe(false);
  });

  test("slug menyimpulkan tipe literal", () => {
    const m = defineModule({ ...valid(), slug: "example" });
    const slug: "example" = m.slug;
    // @ts-expect-error slug literal "example" bukan "lain"
    const wrong: "lain" = m.slug;
    expect([slug, wrong]).toEqual(["example", "example"]);
  });

  test("versi pra-rilis dan build metadata semver diterima", () => {
    expect(() => defineModule(valid({ version: "1.2.3-rc.1+build.5" }))).not.toThrow();
  });
});

describe("defineModule: ModuleDefinitionError menyebut bidang", () => {
  test.each(["Acme.example", "example", "acme.", "acme.example.x", "1acme.example", ""])(
    "key tidak valid %p",
    (key) => {
      expect(messageOf(() => defineModule(valid({ key })))).toContain("key");
    },
  );

  test.each(["Example", "1example", "-example", "ex_ample", "", "a".repeat(51), "ex--ample"])(
    "slug tidak valid %p",
    (slug) => {
      expect(messageOf(() => defineModule(valid({ slug })))).toContain("slug");
    },
  );

  test.each(["1.0", "v1.0.0", "01.0.0", "1.0.0.0", ""])("version tidak valid %p", (version) => {
    expect(messageOf(() => defineModule(valid({ version })))).toContain("version");
  });

  test("id navigasi ganda (juga lintas grup dan anak)", () => {
    const dup = valid({
      navigation: [
        { type: "item", id: "notes", label: label("a"), to: "/a" },
        { type: "item", id: "notes", label: label("b"), to: "/b" },
      ],
    });
    expect(messageOf(() => defineModule(dup))).toContain("navigation[1].id");

    const nested = valid({
      navigation: [
        { type: "item", id: "x", label: label("a"), to: "/a" },
        {
          type: "group",
          id: "g",
          label: label("g"),
          children: [{ type: "item", id: "x", label: label("b"), to: "/b" }],
        },
      ],
    });
    expect(messageOf(() => defineModule(nested))).toContain("navigation[1].children[0].id");
  });

  test("to tanpa awalan /", () => {
    const bad = valid({
      navigation: [{ type: "item", id: "notes", label: label("a"), to: "notes" }],
    });
    expect(messageOf(() => defineModule(bad))).toContain("navigation[0].to");
  });

  test("to tidak boleh berskema atau protocol-relative", () => {
    for (const to of ["//evil.example/x", "/a b", "https://evil.example"]) {
      const bad = valid({
        navigation: [{ type: "item", id: "notes", label: label("a"), to }],
      });
      expect(() => defineModule(bad)).toThrow(ModuleDefinitionError);
    }
  });

  test("grup tanpa anak", () => {
    const bad = valid({
      navigation: [{ type: "group", id: "g", label: label("g"), children: [] }],
    });
    expect(messageOf(() => defineModule(bad))).toContain("navigation[0].children");
  });

  test("anak grup tidak boleh grup lagi", () => {
    const bad = valid({
      navigation: [
        {
          type: "group",
          id: "g",
          label: label("g"),
          children: [{ type: "group", id: "h", label: label("h"), children: [] }],
        } as never,
      ],
    });
    expect(() => defineModule(bad)).toThrow(ModuleDefinitionError);
  });

  test("label tanpa ns/key", () => {
    const bad = valid({
      navigation: [{ type: "item", id: "a", label: { ns: "", key: "x" }, to: "/a" }],
    });
    expect(messageOf(() => defineModule(bad))).toContain("navigation[0].label");
  });

  test("registerRoutes bukan fungsi", () => {
    expect(messageOf(() => defineModule(valid({ registerRoutes: undefined as never })))).toContain(
      "registerRoutes",
    );
  });

  describe("OfflinePolicy tidak valid", () => {
    const base = defaultOfflinePolicy;
    const cases: [string, unknown, string][] = [
      ["mode asing", { ...base, mode: "kapan-saja" }, "offline.mode"],
      ["assets.strategy asing", { ...base, assets: { strategy: "eager" } }, "offline.assets.strategy"],
      ["collections bukan array", { ...base, data: { ...base.data, collections: "notes" } }, "offline.data.collections"],
      ["collections berisi bukan string", { ...base, data: { ...base.data, collections: [1] } }, "offline.data.collections"],
      ["maxAgeSeconds negatif", { ...base, data: { ...base.data, maxAgeSeconds: -1 } }, "offline.data.maxAgeSeconds"],
      ["maxAgeSeconds pecahan", { ...base, data: { ...base.data, maxAgeSeconds: 1.5 } }, "offline.data.maxAgeSeconds"],
      ["maxRecords nol", { ...base, data: { ...base.data, maxRecords: 0 } }, "offline.data.maxRecords"],
      ["sync.enabled bukan boolean", { ...base, sync: { ...base.sync, enabled: "ya" } }, "offline.sync.enabled"],
      ["sync.conflict asing", { ...base, sync: { ...base.sync, conflict: "merge" } }, "offline.sync.conflict"],
      ["bukan objek", null, "offline"],
    ];
    test.each(cases)("%s", (_name, offline, field) => {
      expect(messageOf(() => defineModule(valid({ offline: offline as OfflinePolicy })))).toContain(
        field,
      );
    });
  });
});

describe("defaultOfflinePolicy", () => {
  test("sama dengan docs/architecture/offline-strategy.md §3 (online-only, on-demand, tanpa data, tanpa sync)", () => {
    expect(defaultOfflinePolicy).toEqual({
      mode: "online-only",
      assets: { strategy: "on-demand" },
      data: { collections: [], maxAgeSeconds: 0 },
      sync: { enabled: false, conflict: "server-wins" },
    });
  });

  test("beku sehingga tidak dapat dimutasi bersama oleh modul", () => {
    expect(Object.isFrozen(defaultOfflinePolicy)).toBe(true);
    expect(Object.isFrozen(defaultOfflinePolicy.data.collections)).toBe(true);
  });

  test("lolos validasi dan modul yang memakainya tidak berbagi referensi mutabel", () => {
    const m = defineModule(valid());
    expect(m.offline).toEqual(defaultOfflinePolicy);
  });
});

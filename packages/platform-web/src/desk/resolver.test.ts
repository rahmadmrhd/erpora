import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { deskContains } from "./match-menu";
import { resolveDesk } from "./resolver";
import { makeDashboardItem, makeDesk, makeDeskItem } from "./testing";

const platform = makeDesk(
  "platform",
  [
    makeDashboardItem("platform", "platform"),
    makeDeskItem("platform", "modules", "/platform/modules"),
  ],
  { source: "platform" },
);
const example = makeDesk("example", [
  makeDashboardItem("example", "example"),
  makeDeskItem("example", "notes", "/example/notes"),
]);
const addon = makeDesk("example-addon", [
  makeDashboardItem("example-addon", "example-addon"),
  makeDeskItem("example-addon", "notes", "/example/notes", { ownerDesk: "example" }),
]);
const desks = [platform, example, addon];

describe("resolveDesk tanpa page (tahap 1-3)", () => {
  test("tahap 1: memori tab menang", () => {
    expect(
      resolveDesk({
        desks,
        tabDesk: "example",
        rememberedDesk: "platform",
        defaultDesk: "example-addon",
      }),
    ).toEqual({ deskId: "example", step: 1 });
  });

  test("tahap 2: localStorage bila tab kosong atau tidak valid", () => {
    expect(resolveDesk({ desks, tabDesk: null, rememberedDesk: "platform" })).toEqual({
      deskId: "platform",
      step: 2,
    });
    expect(resolveDesk({ desks, tabDesk: "hilang", rememberedDesk: "platform" })).toEqual({
      deskId: "platform",
      step: 2,
    });
  });

  test("tahap 3: default_desk pengguna; null melewati ke tahap berikutnya", () => {
    expect(resolveDesk({ desks, defaultDesk: "example-addon" })).toEqual({
      deskId: "example-addon",
      step: 3,
    });
    expect(resolveDesk({ desks, defaultDesk: null })).toEqual({ deskId: null, step: null });
  });

  test("tahap 4-5 tidak berjalan tanpa page", () => {
    expect(resolveDesk({ desks })).toEqual({ deskId: null, step: null });
    expect(resolveDesk({ desks, page: null })).toEqual({ deskId: null, step: null });
  });

  test("Desk kosong: tidak ada hasil", () => {
    expect(resolveDesk({ desks: [], tabDesk: "example" })).toEqual({ deskId: null, step: null });
  });
});

describe("resolveDesk dengan page", () => {
  test("kandidat yang tidak memuat page dilewati", () => {
    expect(
      resolveDesk({
        desks,
        tabDesk: "platform",
        rememberedDesk: "example",
        page: "/example/notes",
      }),
    ).toEqual({ deskId: "example", step: 2 });
    expect(
      resolveDesk({
        desks,
        tabDesk: "platform",
        defaultDesk: "example-addon",
        page: "/example/notes",
      }),
    ).toEqual({ deskId: "example-addon", step: 3 });
  });

  test("tahap 4: Desk pemilik utama menu, bahkan bila Desk lain yang lebih awal juga memuatnya", () => {
    expect(resolveDesk({ desks: [addon, example], page: "/example/notes" })).toEqual({
      deskId: "example",
      step: 4,
    });
  });

  test("tahap 5: Desk pertama yang memuat page bila pemilik tidak ada di daftar", () => {
    expect(resolveDesk({ desks: [addon], page: "/example/notes" })).toEqual({
      deskId: "example-addon",
      step: 5,
    });
  });

  test("page tanpa Desk yang memuat: tidak ada hasil (R9.6)", () => {
    expect(resolveDesk({ desks, tabDesk: "example", page: "/platform/todo" })).toEqual({
      deskId: null,
      step: null,
    });
  });
});

describe("resolveDesk masukan tidak tepercaya", () => {
  const bad: unknown[] = [
    "",
    "Example",
    "ex ample",
    "example\n",
    "../etc",
    "<script>",
    "a".repeat(65),
    "é",
    42,
    {},
    [],
    undefined,
    null,
  ];

  test("nilai berbahaya diabaikan tanpa galat", () => {
    for (const v of bad) {
      expect(resolveDesk({ desks, tabDesk: v, rememberedDesk: v, defaultDesk: v })).toEqual({
        deskId: null,
        step: null,
      });
    }
  });

  test("id panjang 64 yang valid diterima bila ada di daftar; 65 ditolak", () => {
    const long = makeDesk("a".repeat(64));
    expect(resolveDesk({ desks: [long], tabDesk: "a".repeat(64) })).toEqual({
      deskId: "a".repeat(64),
      step: 1,
    });
    const tooLong = makeDesk("a".repeat(65));
    expect(resolveDesk({ desks: [tooLong], tabDesk: "a".repeat(65) })).toEqual({
      deskId: null,
      step: null,
    });
  });

  test("property: kandidat yang tidak ada di daftar server tidak pernah dipilih", () => {
    fc.assert(
      fc.property(fc.anything(), fc.anything(), fc.anything(), (a, b, c) => {
        const r = resolveDesk({ desks, tabDesk: a, rememberedDesk: b, defaultDesk: c });
        if (r.deskId !== null) expect(desks.map((d) => d.id)).toContain(r.deskId);
      }),
    );
  });

  test("property: hasil konsisten (id ada, step sesuai kandidat, memuat page bila ada)", () => {
    const id = fc.constantFrom("platform", "example", "example-addon", "hilang", "X", "");
    const page = fc.option(
      fc.constantFrom(
        "/",
        "/example",
        "/example/notes",
        "/example/notes/1",
        "/platform/modules",
        "/x",
      ),
      { nil: undefined },
    );
    fc.assert(
      fc.property(id, id, id, page, (tab, rem, def, pg) => {
        const r = resolveDesk({
          desks,
          tabDesk: tab,
          rememberedDesk: rem,
          defaultDesk: def,
          page: pg,
        });
        if (r.deskId === null) {
          expect(r.step).toBeNull();
          return;
        }
        if (r.step === 1) expect(r.deskId).toBe(tab);
        if (r.step === 2) expect(r.deskId).toBe(rem);
        if (r.step === 3) expect(r.deskId).toBe(def);
        if (pg === undefined) {
          expect([1, 2, 3]).toContain(r.step as number);
        } else {
          expect([1, 2, 3, 4, 5]).toContain(r.step as number);
          const desk = desks.find((d) => d.id === r.deskId);
          expect(desk).toBeDefined();
          if (desk) expect(deskContains(desk, pg)).toBe(true);
        }
      }),
    );
  });

  test("property: tahap 4-5 hanya bila page ada", () => {
    fc.assert(
      fc.property(fc.anything(), fc.anything(), fc.anything(), (a, b, c) => {
        const r = resolveDesk({ desks, tabDesk: a, rememberedDesk: b, defaultDesk: c });
        expect(r.step === 4 || r.step === 5).toBe(false);
      }),
    );
  });
});

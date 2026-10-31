import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { deskContains, deskDashboardPath, matchMenu } from "./match-menu";
import { makeDashboardItem, makeDesk, makeDeskGroup, makeDeskItem } from "./testing";

const example = makeDesk("example", [
  makeDashboardItem("example", "example"),
  makeDeskItem("example", "notes", "/example/notes"),
  makeDeskGroup("example", "more", [makeDeskItem("example", "tags", "/example/more/tags")]),
]);
const addon = makeDesk("example-addon", [
  makeDashboardItem("example-addon", "example-addon"),
  makeDeskItem("example-addon", "summary", "/example-addon/summary"),
  // Menu milik example yang juga tampil di Desk addon (R11.7).
  makeDeskItem("example-addon", "notes", "/example/notes", { ownerDesk: "example" }),
]);

describe("matchMenu", () => {
  test("Dashboard (/<slug>) cocok hanya persis", () => {
    expect(matchMenu("/example", [example]).owner).toBe("example");
    expect(matchMenu("/example/", [example]).owner).toBe("example");
    expect(matchMenu("/example/unknown", [example])).toEqual({ owner: null, containing: [] });
  });

  test("item biasa cocok pada path === to atau awalan to + '/'", () => {
    expect(matchMenu("/example/notes", [example]).owner).toBe("example");
    expect(matchMenu("/example/notes/42/edit", [example]).owner).toBe("example");
  });

  test("batas segmen: /example2 tidak cocok dengan /example, /notes2 tidak dengan /notes", () => {
    expect(matchMenu("/example2", [example])).toEqual({ owner: null, containing: [] });
    expect(matchMenu("/example/notes2", [example])).toEqual({ owner: null, containing: [] });
  });

  test("anak grup ikut dicocokkan", () => {
    expect(matchMenu("/example/more/tags", [example])).toEqual({
      owner: "example",
      containing: ["example"],
    });
  });

  test("terpanjang menang untuk owner; containing memuat semua Desk yang cocok", () => {
    const wide = makeDesk("wide", [makeDeskItem("wide", "all", "/example")]);
    const narrow = makeDesk("narrow", [makeDeskItem("narrow", "n", "/example/notes")]);
    const r = matchMenu("/example/notes/1", [wide, narrow]);
    expect(r.owner).toBe("narrow");
    expect(r.containing).toEqual(["wide", "narrow"]);
  });

  test("menu yang sama di beberapa Desk: owner = ownerDesk, containing = semua Desk", () => {
    const r = matchMenu("/example/notes", [addon, example]);
    expect(r.owner).toBe("example");
    expect(r.containing).toEqual(["example-addon", "example"]);
  });

  test("path tanpa kecocokan atau Desk kosong", () => {
    expect(matchMenu("/", [example])).toEqual({ owner: null, containing: [] });
    expect(matchMenu("/example", [])).toEqual({ owner: null, containing: [] });
  });

  test("property: /<to>x (tanpa batas segmen) tidak pernah cocok dengan item itu", () => {
    const seg = fc.stringMatching(/^[a-z]{1,8}$/);
    fc.assert(
      fc.property(seg, seg, seg, (slug, page, extra) => {
        const to = `/${slug}/${page}`;
        const desk = makeDesk("d", [makeDeskItem("d", "i", to)]);
        expect(matchMenu(`${to}${extra}`, [desk]).owner).toBeNull();
        expect(matchMenu(`${to}/${extra}`, [desk]).owner).toBe("d");
        expect(matchMenu(to, [desk]).owner).toBe("d");
      }),
    );
  });

  test("property: Dashboard hanya cocok persis", () => {
    const seg = fc.stringMatching(/^[a-z]{1,8}$/);
    fc.assert(
      fc.property(seg, seg, (slug, extra) => {
        const desk = makeDesk("d", [makeDashboardItem("d", slug)]);
        expect(matchMenu(`/${slug}`, [desk]).owner).toBe("d");
        expect(matchMenu(`/${slug}/${extra}`, [desk]).owner).toBeNull();
      }),
    );
  });

  test("property: total untuk string apa pun, containing selalu bagian dari id Desk", () => {
    fc.assert(
      fc.property(fc.string({ maxLength: 40 }), (path) => {
        const r = matchMenu(path, [example, addon]);
        for (const id of r.containing) expect(["example", "example-addon"]).toContain(id);
        if (r.owner === null) expect(r.containing).toEqual([]);
      }),
    );
  });
});

describe("deskContains", () => {
  test("mengikuti aturan yang sama pada satu Desk", () => {
    expect(deskContains(example, "/example")).toBe(true);
    expect(deskContains(example, "/example/notes/9")).toBe(true);
    expect(deskContains(example, "/example/other")).toBe(false);
    expect(deskContains(addon, "/example/notes")).toBe(true);
    expect(deskContains(addon, "/example2")).toBe(false);
  });
});

describe("deskDashboardPath", () => {
  test("item terkunci (Dashboard) menang, bahkan bila bukan yang pertama", () => {
    const d = makeDesk("x", [
      makeDeskItem("x", "a", "/x/a"),
      makeDeskItem("x", "dash", "/x", { locked: true }),
    ]);
    expect(deskDashboardPath(d)).toBe("/x");
  });

  test("tanpa item terkunci: item pertama (dalam grup pun); Desk kosong: null", () => {
    expect(
      deskDashboardPath(makeDesk("x", [makeDeskGroup("x", "g", [makeDeskItem("x", "a", "/x/a")])])),
    ).toBe("/x/a");
    expect(deskDashboardPath(makeDesk("x"))).toBeNull();
  });
});

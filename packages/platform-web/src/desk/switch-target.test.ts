import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { deskContains } from "./match-menu";
import { resolveDeskSwitchTarget } from "./switch-target";
import { makeDashboardItem, makeDesk, makeDeskGroup, makeDeskItem } from "./testing";

const example = makeDesk("example", [
  makeDashboardItem("example", "example"),
  makeDeskItem("example", "notes", "/example/notes"),
  makeDeskGroup("example", "more", [makeDeskItem("example", "tags", "/example/more/tags")]),
]);
const addon = makeDesk("example-addon", [
  makeDashboardItem("example-addon", "example-addon"),
  makeDeskItem("example-addon", "summary", "/example-addon/summary"),
  makeDeskItem("example-addon", "notes", "/example/notes", { ownerDesk: "example" }),
]);

describe("resolveDeskSwitchTarget", () => {
  test("rute yang juga ada di Desk tujuan bertahan (hanya Desk berganti)", () => {
    expect(resolveDeskSwitchTarget("/example/notes", addon)).toBe("/example/notes");
    expect(resolveDeskSwitchTarget("/example/notes/42/edit", addon)).toBe("/example/notes/42/edit");
  });

  test("rute di luar Desk tujuan menuju Dashboard Desk tujuan", () => {
    expect(resolveDeskSwitchTarget("/example-addon/summary", example)).toBe("/example");
    expect(resolveDeskSwitchTarget("/settings", addon)).toBe("/example-addon");
  });

  test("Dashboard hanya cocok persis: anak Dashboard Desk lain tidak bertahan", () => {
    expect(resolveDeskSwitchTarget("/example", example)).toBe("/example");
    expect(resolveDeskSwitchTarget("/example/unknown", example)).toBe("/example");
  });

  test("batas segmen: /example2 tidak cocok dengan /example", () => {
    expect(resolveDeskSwitchTarget("/example2", example)).toBe("/example");
  });

  test("Desk tujuan kosong tanpa menu: kembalikan currentPath", () => {
    expect(resolveDeskSwitchTarget("/example/notes", makeDesk("empty"))).toBe("/example/notes");
  });

  test("property: hasil selalu berada di dalam Desk tujuan", () => {
    const seg = fc.stringMatching(/^[a-z]{1,6}$/);
    fc.assert(
      fc.property(fc.array(seg, { minLength: 0, maxLength: 3 }), (segs) => {
        const path = `/${segs.join("/")}`;
        for (const target of [example, addon]) {
          const out = resolveDeskSwitchTarget(path, target);
          expect(deskContains(target, out)).toBe(true);
          if (deskContains(target, path)) expect(out).toBe(path);
        }
      }),
    );
  });
});

import { describe, expect, test } from "bun:test";
import { makeDashboardItem, makeDesk, makeDeskGroup, makeDeskItem } from "@erpora/platform-web";
import { deskDashboardPath } from "./desk-navigation";

describe("deskDashboardPath", () => {
  test("memakai item terkunci (Dashboard) walau bukan yang pertama", () => {
    const desk = makeDesk("example", [
      makeDeskItem("example", "notes", "/example/notes"),
      makeDashboardItem("example", "example"),
    ]);
    expect(deskDashboardPath(desk)).toBe("/example");
  });

  test("tanpa item terkunci: item pertama, termasuk yang ada di dalam grup", () => {
    const desk = makeDesk("example", [
      makeDeskGroup("example", "reports", [makeDeskItem("example", "sales", "/example/sales")]),
    ]);
    expect(deskDashboardPath(desk)).toBe("/example/sales");
  });

  test("Desk tanpa menu: /<id>", () => {
    expect(deskDashboardPath(makeDesk("billing"))).toBe("/billing");
  });
});

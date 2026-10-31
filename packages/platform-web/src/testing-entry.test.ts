import { describe, expect, test } from "bun:test";
import * as barrel from "./index";
import * as testing from "./testing";

describe("entry pengujian (@erpora/platform-web/testing)", () => {
  test("helper fixture Desk hanya ada di entry pengujian, bukan di barrel produksi (Minor 6)", () => {
    for (const name of ["makeDesk", "makeDeskItem", "makeDashboardItem", "makeDeskGroup"]) {
      expect(testing).toHaveProperty(name);
      expect(barrel).not.toHaveProperty(name);
    }
  });

  test("fixture berfungsi", () => {
    const d = testing.makeDesk("x", [testing.makeDashboardItem("x", "x")]);
    expect(d.menus[0]).toMatchObject({ to: "/x", locked: true, ownerDesk: "x" });
  });
});

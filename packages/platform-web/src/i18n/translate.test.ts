import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { resetTranslateWarnings, translate, translateRich } from "./translate";

let warn: ReturnType<typeof spyOn<Console, "warn">>;
beforeEach(() => {
  resetTranslateWarnings();
  warn = spyOn(console, "warn").mockImplementation(() => {});
});
afterEach(() => {
  warn.mockRestore();
});

describe("translate: kunci hilang (R3.3, paritas Go)", () => {
  test("mengembalikan ns::kunci, juga untuk rich", () => {
    expect(translate("id", "example", {}, "note.title")).toBe("example::note.title");
    expect(translateRich("id", "example", {}, "note.title")).toBe("example::note.title");
  });

  test("pesan yang tidak dapat dikompilasi mengembalikan ns::kunci", () => {
    expect(translate("id", "example", { "note.x": "{broken" }, "note.x")).toBe("example::note.x");
  });

  test("pesan valid diformat seperti biasa", () => {
    expect(translate("id", "example", { "note.x": "Halo {n}" }, "note.x", { n: "Dunia" })).toBe(
      "Halo Dunia",
    );
  });

  test("peringatan dev satu kali per (bahasa, ns, kunci)", () => {
    translate("id", "example", {}, "note.title");
    translate("id", "example", {}, "note.title");
    translate("id", "example", { "note.x": "{broken" }, "note.x");
    expect(warn).toHaveBeenCalledTimes(2);
    translate("en", "example", {}, "note.title");
    translate("id", "lain", {}, "note.title");
    expect(warn).toHaveBeenCalledTimes(4);
    expect(String(warn.mock.calls[0]?.[0])).toContain("example::note.title");
  });

  test("tanpa peringatan di produksi", () => {
    const prev = process.env.NODE_ENV;
    process.env.NODE_ENV = "production";
    try {
      translate("id", "example", {}, "note.title");
      expect(warn).not.toHaveBeenCalled();
    } finally {
      process.env.NODE_ENV = prev;
    }
  });
});

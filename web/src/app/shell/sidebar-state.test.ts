import { afterEach, describe, expect, spyOn, test } from "bun:test";
import { readSidebarOpen, writeSidebarOpen } from "./sidebar-state";

afterEach(() => {
  localStorage.clear();
});

describe("keadaan ciut panel", () => {
  test("bawaan terbuka; menulis dan membaca kembali di erpora.sidebar", () => {
    expect(readSidebarOpen()).toBe(true);
    writeSidebarOpen(false);
    expect(localStorage.getItem("erpora.sidebar")).toBe("collapsed");
    expect(readSidebarOpen()).toBe(false);
    writeSidebarOpen(true);
    expect(localStorage.getItem("erpora.sidebar")).toBe("expanded");
    expect(readSidebarOpen()).toBe(true);
  });

  test("nilai tak dikenal berarti terbuka", () => {
    localStorage.setItem("erpora.sidebar", "<script>");
    expect(readSidebarOpen()).toBe(true);
  });

  test("storage yang melempar (mode privat) tidak merusak: terbuka dan tanpa galat", () => {
    const get = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("diblokir");
    });
    const set = spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("diblokir");
    });
    try {
      expect(readSidebarOpen()).toBe(true);
      expect(() => writeSidebarOpen(false)).not.toThrow();
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });
});

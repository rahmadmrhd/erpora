import { afterEach, describe, expect, spyOn, test } from "bun:test";
import {
  DESK_STORAGE_KEYS,
  isValidDeskId,
  readRememberedDesk,
  readTabDesk,
  writeDeskChoice,
} from "./storage";

afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

describe("isValidDeskId", () => {
  test("hanya string ^[a-z0-9:-]{1,64}$", () => {
    expect(isValidDeskId("example-addon")).toBe(true);
    expect(isValidDeskId("custom:abc-1")).toBe(true);
    for (const v of ["", "A", "a b", "a_b", "a".repeat(65), "a\n", 1, null, undefined, {}]) {
      expect(isValidDeskId(v)).toBe(false);
    }
  });
});

describe("penyimpanan Desk", () => {
  test("kunci tetap: sessionStorage erpora.desk.tab dan localStorage erpora.desk", () => {
    expect(DESK_STORAGE_KEYS).toEqual({ tab: "erpora.desk.tab", remembered: "erpora.desk" });
  });

  test("tulis menyimpan hanya id di kedua tempat; baca mengembalikannya", () => {
    expect(readTabDesk()).toBeNull();
    expect(readRememberedDesk()).toBeNull();
    writeDeskChoice("example");
    expect(sessionStorage.getItem("erpora.desk.tab")).toBe("example");
    expect(localStorage.getItem("erpora.desk")).toBe("example");
    expect(readTabDesk()).toBe("example");
    expect(readRememberedDesk()).toBe("example");
  });

  test("nilai storage berbahaya dibaca sebagai null", () => {
    for (const v of ["A B", "x".repeat(65), "<b>", ""]) {
      sessionStorage.setItem("erpora.desk.tab", v);
      localStorage.setItem("erpora.desk", v);
      expect(readTabDesk()).toBeNull();
      expect(readRememberedDesk()).toBeNull();
    }
  });

  test("menulis id tidak valid diabaikan (tidak ada yang tersimpan)", () => {
    writeDeskChoice("Bad Id");
    expect(sessionStorage.getItem("erpora.desk.tab")).toBeNull();
    expect(localStorage.getItem("erpora.desk")).toBeNull();
  });

  test("getItem/setItem melempar (mode privat/penuh): bawaan tanpa galat", () => {
    const get = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    const set = spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("quota", "QuotaExceededError");
    });
    try {
      expect(readTabDesk()).toBeNull();
      expect(readRememberedDesk()).toBeNull();
      expect(() => writeDeskChoice("example")).not.toThrow();
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });

  test("akses properti sessionStorage/localStorage melempar: bawaan tanpa galat", () => {
    const names = ["sessionStorage", "localStorage"] as const;
    const originals = names.map((n) => Object.getOwnPropertyDescriptor(globalThis, n));
    for (const n of names) {
      Object.defineProperty(globalThis, n, {
        configurable: true,
        get() {
          throw new DOMException("denied", "SecurityError");
        },
      });
    }
    try {
      expect(readTabDesk()).toBeNull();
      expect(readRememberedDesk()).toBeNull();
      expect(() => writeDeskChoice("example")).not.toThrow();
    } finally {
      names.forEach((n, i) => {
        const o = originals[i];
        if (o) Object.defineProperty(globalThis, n, o);
      });
    }
  });
});

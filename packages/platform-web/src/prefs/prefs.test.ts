import { afterEach, describe, expect, spyOn, test } from "bun:test";
import { PREF_KEYS, readPref, writePref } from "./index";

afterEach(() => {
  localStorage.clear();
});

describe("prefs", () => {
  test("kunci preferensi UI tetap tiga: tema, sidebar, bahasa", () => {
    expect(PREF_KEYS).toEqual({
      theme: "erpora.theme",
      sidebar: "erpora.sidebar",
      locale: "erpora.locale",
    });
  });

  test("tulis lalu baca; kunci yang tidak ada menghasilkan null", () => {
    expect(readPref("erpora.theme")).toBeNull();
    writePref("erpora.theme", "dark");
    expect(readPref("erpora.theme")).toBe("dark");
    expect(localStorage.getItem("erpora.theme")).toBe("dark");
  });

  test("localStorage.getItem melempar: null, tidak melempar", () => {
    const spy = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    try {
      expect(readPref("erpora.theme")).toBeNull();
    } finally {
      spy.mockRestore();
    }
  });

  test("localStorage.setItem melempar (diblokir/penuh): diam-diam gagal", () => {
    const spy = spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("quota", "QuotaExceededError");
    });
    try {
      expect(() => writePref("erpora.theme", "dark")).not.toThrow();
    } finally {
      spy.mockRestore();
    }
  });

  test("akses properti localStorage sendiri melempar (mode privat/diblokir)", () => {
    const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      get() {
        throw new DOMException("denied", "SecurityError");
      },
    });
    try {
      expect(readPref("erpora.theme")).toBeNull();
      expect(() => writePref("erpora.theme", "dark")).not.toThrow();
    } finally {
      if (original) Object.defineProperty(globalThis, "localStorage", original);
    }
  });
});

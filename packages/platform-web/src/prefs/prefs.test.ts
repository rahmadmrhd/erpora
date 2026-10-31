import { afterEach, describe, expect, spyOn, test } from "bun:test";
import {
  isValidTimezone,
  PREF_KEYS,
  readDockMode,
  readPref,
  readTimezone,
  removePref,
  resolveTimezone,
  subscribePrefs,
  writeDockMode,
  writePref,
  writeTimezone,
} from "./index";

afterEach(() => {
  localStorage.clear();
});

describe("prefs", () => {
  test("kunci preferensi UI: tema, sidebar, bahasa, mode Dock, Desk terakhir, dan zona waktu", () => {
    expect(PREF_KEYS).toEqual({
      theme: "erpora.theme",
      sidebar: "erpora.sidebar",
      locale: "erpora.locale",
      dock: "erpora.dock",
      desk: "erpora.desk",
      timezone: "erpora.timezone",
    });
  });

  test("mode Dock: bawaan floating; hanya pinned/floating diterima", () => {
    expect(readDockMode()).toBe("floating");
    writeDockMode("pinned");
    expect(localStorage.getItem("erpora.dock")).toBe("pinned");
    expect(readDockMode()).toBe("pinned");
    localStorage.setItem("erpora.dock", "<x>");
    expect(readDockMode()).toBe("floating");
    writeDockMode("floating");
    expect(readDockMode()).toBe("floating");
  });

  test("mode Dock: penyimpanan melempar menghasilkan bawaan tanpa galat", () => {
    const get = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    const set = spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("quota", "QuotaExceededError");
    });
    try {
      expect(readDockMode()).toBe("floating");
      expect(() => writeDockMode("pinned")).not.toThrow();
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
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

  test("removePref menghapus kunci dan tidak melempar bila penyimpanan diblokir", () => {
    writePref("erpora.theme", "dark");
    removePref("erpora.theme");
    expect(readPref("erpora.theme")).toBeNull();
    const spy = spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    try {
      expect(() => removePref("erpora.theme")).not.toThrow();
    } finally {
      spy.mockRestore();
    }
  });
});

describe("zona waktu", () => {
  test("isValidTimezone: hanya IANA yang dikenal Intl", () => {
    expect(isValidTimezone("Asia/Jakarta")).toBe(true);
    expect(isValidTimezone("UTC")).toBe(true);
    expect(isValidTimezone("Mars/Olympus")).toBe(false);
    expect(isValidTimezone("")).toBe(false);
    expect(isValidTimezone("<script>")).toBe(false);
    expect(isValidTimezone(undefined)).toBe(false);
    expect(isValidTimezone("A".repeat(200))).toBe(false);
  });

  test("tulis lalu baca; tanpa nilai (otomatis) atau nilai tak valid menghasilkan null", () => {
    expect(readTimezone()).toBeNull();
    writeTimezone("Asia/Makassar");
    expect(localStorage.getItem("erpora.timezone")).toBe("Asia/Makassar");
    expect(readTimezone()).toBe("Asia/Makassar");
    localStorage.setItem("erpora.timezone", "Mars/Olympus");
    expect(readTimezone()).toBeNull();
    writeTimezone("Asia/Jayapura");
    writeTimezone(null);
    expect(localStorage.getItem("erpora.timezone")).toBeNull();
    expect(readTimezone()).toBeNull();
  });

  test("writeTimezone menolak nilai tak valid tanpa menulis", () => {
    writeTimezone("Mars/Olympus");
    expect(localStorage.getItem("erpora.timezone")).toBeNull();
  });

  test("resolveTimezone: pilihan tersimpan, selain itu zona peramban (tidak pernah kosong)", () => {
    expect(resolveTimezone().length).toBeGreaterThan(0);
    writeTimezone("Asia/Jakarta");
    expect(resolveTimezone()).toBe("Asia/Jakarta");
  });

  test("penyimpanan melempar menghasilkan null tanpa galat", () => {
    const get = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    try {
      expect(readTimezone()).toBeNull();
      expect(resolveTimezone().length).toBeGreaterThan(0);
    } finally {
      get.mockRestore();
    }
  });
});

describe("subscribePrefs", () => {
  test("writePref, removePref, dan writeDockMode memberi tahu pelanggan dengan kunci yang berubah", () => {
    const seen: (string | null)[] = [];
    const off = subscribePrefs((key) => seen.push(key));
    writeDockMode("pinned");
    writePref("erpora.theme", "dark");
    removePref("erpora.theme");
    writeTimezone("Asia/Jakarta");
    expect(seen).toEqual(["erpora.dock", "erpora.theme", "erpora.theme", "erpora.timezone"]);
    off();
    writeDockMode("floating");
    expect(seen).toHaveLength(4);
  });

  test("event storage dari tab lain memberi tahu pelanggan (kunci atau null untuk clear)", () => {
    const seen: (string | null)[] = [];
    const off = subscribePrefs((key) => seen.push(key));
    window.dispatchEvent(new StorageEvent("storage", { key: "erpora.dock" }));
    window.dispatchEvent(new StorageEvent("storage", { key: null }));
    off();
    window.dispatchEvent(new StorageEvent("storage", { key: "erpora.dock" }));
    expect(seen).toEqual(["erpora.dock", null]);
  });

  test("pelanggan yang melempar tidak menghentikan pelanggan lain atau penulisan", () => {
    const seen: (string | null)[] = [];
    const off1 = subscribePrefs(() => {
      throw new Error("boom");
    });
    const off2 = subscribePrefs((key) => seen.push(key));
    expect(() => writeDockMode("pinned")).not.toThrow();
    expect(seen).toEqual(["erpora.dock"]);
    off1();
    off2();
  });
});

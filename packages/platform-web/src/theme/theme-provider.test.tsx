import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { act, render, screen } from "@testing-library/react";
import { ThemeProvider, useTheme } from "./provider";
import { THEME_STORAGE_KEY } from "./theme";

interface FakeMedia {
  setDark(v: boolean): void;
  listeners(): number;
  restore(): void;
}

function installMatchMedia(initialDark: boolean): FakeMedia {
  const original = window.matchMedia;
  let dark = initialDark;
  const listeners = new Set<(e: { matches: boolean }) => void>();
  const mql = {
    get matches() {
      return dark;
    },
    media: "(prefers-color-scheme: dark)",
    addEventListener: (_: string, l: (e: { matches: boolean }) => void) => listeners.add(l),
    removeEventListener: (_: string, l: (e: { matches: boolean }) => void) => listeners.delete(l),
    addListener: (l: (e: { matches: boolean }) => void) => listeners.add(l),
    removeListener: (l: (e: { matches: boolean }) => void) => listeners.delete(l),
  };
  window.matchMedia = (() => mql) as unknown as typeof window.matchMedia;
  return {
    setDark(v) {
      dark = v;
      for (const l of [...listeners]) l({ matches: v });
    },
    listeners: () => listeners.size,
    restore() {
      window.matchMedia = original;
    },
  };
}

function Probe() {
  const { preference, resolved, setPreference } = useTheme();
  return (
    <div>
      <span data-testid="pref">{preference}</span>
      <span data-testid="resolved">{resolved}</span>
      <button type="button" onClick={() => setPreference("dark")}>
        gelap
      </button>
      <button type="button" onClick={() => setPreference("system")}>
        sistem
      </button>
    </div>
  );
}

const root = document.documentElement;
let media: FakeMedia;

beforeEach(() => {
  media = installMatchMedia(false);
});

afterEach(() => {
  media.restore();
  localStorage.clear();
  root.className = "";
  root.style.colorScheme = "";
});

function renderProbe() {
  return render(
    <ThemeProvider>
      <Probe />
    </ThemeProvider>,
  );
}

describe("ThemeProvider", () => {
  test("tanpa pilihan tersimpan: system, mengikuti OS (terang)", () => {
    renderProbe();
    expect(screen.getByTestId("pref").textContent).toBe("system");
    expect(screen.getByTestId("resolved").textContent).toBe("light");
    expect(root.classList.contains("light")).toBe(true);
    expect(root.classList.contains("dark")).toBe(false);
  });

  test("membaca pilihan tersimpan dan menerapkannya ke <html>", () => {
    localStorage.setItem(THEME_STORAGE_KEY, "dark");
    renderProbe();
    expect(screen.getByTestId("pref").textContent).toBe("dark");
    expect(root.classList.contains("dark")).toBe(true);
    expect(root.style.colorScheme).toBe("dark");
  });

  test("nilai tersimpan yang rusak menjadi system", () => {
    localStorage.setItem(THEME_STORAGE_KEY, "<script>");
    renderProbe();
    expect(screen.getByTestId("pref").textContent).toBe("system");
  });

  test("setPreference menyimpan dan menerapkan tanpa reload", () => {
    renderProbe();
    act(() => screen.getByText("gelap").click());
    expect(localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");
    expect(root.classList.contains("dark")).toBe(true);
    expect(root.classList.contains("light")).toBe(false);
  });

  test("system mengikuti perubahan preferensi OS tanpa reload", () => {
    renderProbe();
    expect(screen.getByTestId("resolved").textContent).toBe("light");
    act(() => media.setDark(true));
    expect(screen.getByTestId("resolved").textContent).toBe("dark");
    expect(root.classList.contains("dark")).toBe(true);
    act(() => media.setDark(false));
    expect(root.classList.contains("light")).toBe(true);
  });

  test("pilihan eksplisit tidak berubah oleh perubahan OS", () => {
    localStorage.setItem(THEME_STORAGE_KEY, "light");
    renderProbe();
    act(() => media.setDark(true));
    expect(screen.getByTestId("resolved").textContent).toBe("light");
    expect(root.classList.contains("light")).toBe(true);
  });

  test("sinkron antar tab lewat event storage", () => {
    renderProbe();
    localStorage.setItem(THEME_STORAGE_KEY, "dark");
    act(() => {
      window.dispatchEvent(
        new StorageEvent("storage", { key: THEME_STORAGE_KEY, newValue: "dark" }),
      );
    });
    expect(screen.getByTestId("pref").textContent).toBe("dark");
    expect(root.classList.contains("dark")).toBe(true);
  });

  test("event storage untuk kunci lain diabaikan", () => {
    renderProbe();
    act(() => {
      window.dispatchEvent(new StorageEvent("storage", { key: "lain", newValue: "dark" }));
    });
    expect(screen.getByTestId("pref").textContent).toBe("system");
  });

  test("P6: localStorage melempar tidak merusak aplikasi (fallback system)", () => {
    const get = spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    const set = spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    try {
      renderProbe();
      expect(screen.getByTestId("pref").textContent).toBe("system");
      act(() => screen.getByText("gelap").click());
      expect(screen.getByTestId("pref").textContent).toBe("dark");
      expect(root.classList.contains("dark")).toBe(true);
    } finally {
      get.mockRestore();
      set.mockRestore();
    }
  });

  test("melepas listener matchMedia saat unmount", () => {
    const { unmount } = renderProbe();
    expect(media.listeners()).toBeGreaterThan(0);
    unmount();
    expect(media.listeners()).toBe(0);
  });

  test("useTheme di luar provider melempar galat yang jelas", () => {
    const err = spyOn(console, "error").mockImplementation(() => {});
    try {
      expect(() => render(<Probe />)).toThrow(/ThemeProvider/);
    } finally {
      err.mockRestore();
    }
  });
});

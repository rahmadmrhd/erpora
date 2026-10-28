import { afterEach, beforeEach, describe, expect, spyOn, test } from "bun:test";
import { type IconName, iconRegistry, resolveIcon } from "./icons";

describe("resolveIcon", () => {
  test("mengenali nama terdaftar dan mengembalikan komponen", () => {
    const Home = resolveIcon("home");
    expect(Home).toBeDefined();
    expect(Home).toBe(iconRegistry.home);
  });

  test("setiap entri registri adalah komponen React (fungsi atau forwardRef)", () => {
    for (const [name, icon] of Object.entries(iconRegistry)) {
      expect(["function", "object"]).toContain(typeof icon);
      expect(icon, name).toBeTruthy();
    }
  });

  test("registri berisi sekitar 40-70 ikon dengan nama kebab-case", () => {
    const names = Object.keys(iconRegistry);
    expect(names.length).toBeGreaterThanOrEqual(40);
    expect(names.length).toBeLessThanOrEqual(70);
    for (const name of names) expect(name).toMatch(/^[a-z][a-z0-9]*(-[a-z0-9]+)*$/);
    for (const required of [
      "home",
      "bell",
      "package",
      "settings",
      "users",
      "file-text",
      "folder",
      "chevron-right",
    ]) {
      expect(names).toContain(required);
    }
  });

  test("nama asing, kosong, dan prototipe Object mengembalikan undefined", () => {
    expect(resolveIcon("tidak-ada")).toBeUndefined();
    expect(resolveIcon("")).toBeUndefined();
    expect(resolveIcon(undefined)).toBeUndefined();
    expect(resolveIcon("constructor")).toBeUndefined();
    expect(resolveIcon("__proto__")).toBeUndefined();
  });

  test("tipe IconName mencakup nama registri", () => {
    const name: IconName = "home";
    expect(resolveIcon(name)).toBeDefined();
  });
});

describe("peringatan dev untuk nama tak dikenal", () => {
  let warn: ReturnType<typeof spyOn>;
  beforeEach(() => {
    warn = spyOn(console, "warn").mockImplementation(() => {});
  });
  afterEach(() => {
    warn.mockRestore();
  });

  test("console.warn sekali per nama", () => {
    resolveIcon("ikon-modul-x");
    resolveIcon("ikon-modul-x");
    resolveIcon("ikon-modul-y");
    const names = warn.mock.calls.map((c: unknown[]) => String(c[0]));
    expect(names.filter((m: string) => m.includes("ikon-modul-x"))).toHaveLength(1);
    expect(names.filter((m: string) => m.includes("ikon-modul-y"))).toHaveLength(1);
  });

  test("nama terdaftar tidak memicu peringatan", () => {
    resolveIcon("bell");
    expect(warn).not.toHaveBeenCalled();
  });
});

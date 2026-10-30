import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { initialsOf } from "./initials";

describe("initialsOf", () => {
  test("dua kata pertama menjadi dua huruf besar", () => {
    expect(initialsOf("Rahmad Maulana")).toBe("RM");
    expect(initialsOf("budi santoso wijaya")).toBe("BS");
  });

  test("satu kata menjadi satu huruf", () => {
    expect(initialsOf("Rahmad")).toBe("R");
  });

  test("kosong atau spasi saja menjadi '?'", () => {
    expect(initialsOf("")).toBe("?");
    expect(initialsOf("   ")).toBe("?");
  });

  test("spasi berlebih diabaikan dan kode poin di luar BMP tidak terpotong", () => {
    expect(initialsOf("  ana   bela ")).toBe("AB");
    expect(Array.from(initialsOf("𝒜lfa Beta"))).toEqual(["𝒜", "B"]);
  });

  test("properti: tidak pernah kosong dan paling banyak dua karakter tampil", () => {
    fc.assert(
      fc.property(fc.string(), (name) => {
        const out = initialsOf(name);
        expect(out.length).toBeGreaterThan(0);
        expect(Array.from(out).length).toBeLessThanOrEqual(2);
      }),
    );
  });
});

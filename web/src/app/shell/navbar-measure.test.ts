import { afterEach, describe, expect, test } from "bun:test";
import { CURRENT_PAGE_CLASS, measureTexts } from "./navbar-measure";

const original = Element.prototype.getBoundingClientRect;
afterEach(() => {
  Element.prototype.getBoundingClientRect = original;
});

describe("measureTexts (K9: lebar halaman saat ini diukur dengan huruf besar dan tebal)", () => {
  test("segmen terakhir diukur dengan kelas halaman saat ini, segmen lain dengan teks biasa", () => {
    const seen: { text: string; className: string }[] = [];
    Element.prototype.getBoundingClientRect = function (this: Element): DOMRect {
      if (this.hasAttribute("data-fit-probe")) {
        seen.push({ text: this.textContent ?? "", className: this.className });
      }
      return { width: 1 } as DOMRect;
    };
    const container = document.createElement("div");
    document.body.append(container);
    try {
      const widths = measureTexts(container, ["Home", "Desk", "Halaman"]);
      expect(widths).toEqual([1, 1, 1]);
    } finally {
      container.remove();
    }
    expect(seen.map((s) => s.text)).toEqual(["Home", "Desk", "Halaman"]);
    for (const s of seen.slice(0, 2)) {
      expect(s.className).toContain("text-sm");
      expect(s.className).toContain("font-normal");
    }
    const last = seen[2]?.className ?? "";
    for (const token of CURRENT_PAGE_CLASS.split(" ")) expect(last).toContain(token);
    expect(last).not.toContain("font-normal");
    expect(last).not.toContain("text-sm");
    // Probe sementara dibuang.
    expect(container.querySelector("[data-fit-probe]")).toBeNull();
  });

  test("daftar kosong dan satu teks (halaman saja) tidak melempar", () => {
    const container = document.createElement("div");
    expect(measureTexts(container, [])).toEqual([]);
    expect(measureTexts(container, ["Satu"])).toHaveLength(1);
  });
});

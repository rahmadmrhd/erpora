import { beforeEach, describe, expect, test } from "bun:test";
import { screen, within } from "@testing-library/react";
import { renderApp } from "../test-utils";
import { dedupeCrumbs, type ResolvedCrumb } from "./breadcrumbs";

beforeEach(() => {
  localStorage.clear();
  Object.defineProperty(navigator, "languages", { value: [], configurable: true });
});

const c = (key: string, text?: string, to?: string): ResolvedCrumb => ({
  key,
  label: text ?? key,
  text,
  to,
});

describe("dedupeCrumbs", () => {
  test("crumb berturut-turut berteks sama: yang lebih dalam (halaman) dipertahankan", () => {
    const out = dedupeCrumbs(
      [c("a", "Catatan", "/example"), c("b", "Catatan", "/example/notes")],
      {},
    );
    expect(out.map((x) => x.key)).toEqual(["b"]);
  });

  test("teks dari kamus probe untuk crumb berpesan; tidak sama: semua dipertahankan", () => {
    const out = dedupeCrumbs([c("a"), c("b"), c("c", "Lain")], { a: "X", b: "Y" });
    expect(out.map((x) => x.key)).toEqual(["a", "b", "c"]);
    const dup = dedupeCrumbs([c("a"), c("b")], { a: "X", b: "X" });
    expect(dup.map((x) => x.key)).toEqual(["b"]);
  });

  test("hanya yang berturut-turut: teks sama yang tidak bersebelahan dipertahankan", () => {
    const out = dedupeCrumbs([c("a", "X"), c("b", "Y"), c("c", "X")], {});
    expect(out.map((x) => x.key)).toEqual(["a", "b", "c"]);
  });

  test("teks belum diketahui (belum dimuat) tidak dianggap sama", () => {
    expect(dedupeCrumbs([c("a"), c("b")], {}).map((x) => x.key)).toEqual(["a", "b"]);
  });

  test("masukan kosong dan satu item", () => {
    expect(dedupeCrumbs([], {})).toEqual([]);
    expect(dedupeCrumbs([c("a", "X")], {}).map((x) => x.key)).toEqual(["a"]);
  });
});

describe("breadcrumb halaman modul tidak duplikat (Notes > Notes)", () => {
  test("dua crumb berbeda kunci tetapi teks sama hanya tampil sekali", async () => {
    renderApp({ path: "/alpha/dupe" });
    const bc = await screen.findByRole("navigation", { name: "Jejak halaman" });
    await within(bc).findByText("Alpha Beranda");
    expect(within(bc).getAllByText("Alpha Beranda")).toHaveLength(1);
    expect(within(bc).getByText("Alpha Beranda")).toHaveAttribute("aria-current", "page");
  });
});

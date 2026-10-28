import { describe, expect, test } from "bun:test";
import { compile, format, formatRich, I18nFormatError } from "./format";

describe("compile", () => {
  test("cache per (locale, source) mengembalikan objek yang sama", () => {
    expect(compile("id", "Halo {name}")).toBe(compile("id", "Halo {name}"));
    expect(compile("id", "Halo {name}")).not.toBe(compile("en", "Halo {name}"));
  });

  test("pesan tidak sah melempar I18nFormatError dan tidak di-cache", () => {
    expect(() => compile("id", "Halo {name")).toThrow(I18nFormatError);
    expect(() => compile("id", "Halo {name")).toThrow(I18nFormatError);
  });

  test("locale tidak sah melempar I18nFormatError", () => {
    expect(() => compile("bukan locale!!", "x")).toThrow(I18nFormatError);
  });

  test.each([
    ["number", "{x, number}"],
    ["date", "{x, date}"],
    ["time", "{x, time}"],
    ["selectordinal", "{n, selectordinal, one {#st} other {#th}}"],
    ["offset plural", "{n, plural, offset:1 one {x} other {y}}"],
  ])("konstruksi tak didukung ditolak: %s", (_name, src) => {
    expect(() => compile("en", src)).toThrow(I18nFormatError);
  });
});

describe("format", () => {
  test("argumen hilang tidak melempar, termasuk nama yang ada di prototipe Object", () => {
    expect(format(compile("en", "{constructor} dan {toString}"), {})).toBe(
      "{constructor} dan {toString}",
    );
  });

  test("args dihilangkan sama sekali", () => {
    expect(format(compile("en", "Halo {name}"))).toBe("Halo {name}");
  });

  test("panggilan berulang menghasilkan keluaran sama", () => {
    const c = compile("en", "{n, plural, one {# item} other {# items}}");
    expect(format(c, { n: 1 })).toBe("1 item");
    expect(format(c, { n: 2 })).toBe("2 items");
    expect(format(c, { n: 1 })).toBe("1 item");
    expect(format(c, {})).toBe("{n}");
  });
});

describe("formatRich", () => {
  type Node = { tag: string; children: Array<string | Node> };
  const tags = {
    b: (chunks: Array<string | Node>): Node => ({ tag: "b", children: chunks }),
    i: (chunks: Array<string | Node>): Node => ({ tag: "i", children: chunks }),
  };

  test("tag dipanggil dengan anak-anaknya", () => {
    const out = formatRich<Node>(compile("id", "Klik <b>di sini</b> sekarang"), {}, tags);
    expect(out).toEqual(["Klik ", { tag: "b", children: ["di sini"] }, " sekarang"]);
  });

  test("tag bersarang dan argumen di dalam tag", () => {
    const out = formatRich<Node>(compile("id", "<b><i>{name}</i></b>"), { name: "Budi" }, tags);
    expect(out).toEqual([{ tag: "b", children: [{ tag: "i", children: ["Budi"] }] }]);
  });

  test("tag tanpa pemanggil dilucuti", () => {
    const out = formatRich<Node>(compile("id", "<b>a</b> dan <u>b</u>"), {}, tags);
    expect(out).toEqual([{ tag: "b", children: ["a"] }, " dan b"]);
  });

  test("argumen hilang di dalam tag tampil apa adanya", () => {
    const out = formatRich<Node>(compile("id", "<b>{x}</b>"), {}, tags);
    expect(out).toEqual([{ tag: "b", children: ["{x}"] }]);
  });

  test("tanpa tag hasilnya satu potongan teks; pesan kosong menghasilkan array kosong", () => {
    expect(formatRich<Node>(compile("id", "Halo {n}"), { n: 1 }, tags)).toEqual(["Halo 1"]);
    expect(formatRich<Node>(compile("id", ""), {}, tags)).toEqual([]);
  });

  test("nama tag yang bentrok dengan argumen: argumen menang, tag dilucuti", () => {
    expect(formatRich<Node>(compile("id", "<b>x</b>{b}"), { b: "y" }, tags)).toEqual(["xy"]);
  });
});

describe("argumen bool", () => {
  test("bool pada {nama} polos menjadi teks true/false (sama dengan Go)", () => {
    const c = compile("en", "<b>{ok}</b>/{no}");
    expect(format(c, { ok: true, no: false })).toBe("true/false");
    expect(formatRich<string>(c, { ok: true, no: false }, { b: (x) => x.join("") }).join("")).toBe(
      "true/false",
    );
  });

  test("bool pada select memilih kunci true/false", () => {
    expect(format(compile("en", "{f, select, true {Y} other {N}}"), { f: true })).toBe("Y");
  });
});

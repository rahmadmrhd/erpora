// Test properti format.ts (tasks.md 12.8; design.md P4/P5; Keputusan D-9: fast-check).
import { describe, expect, test } from "bun:test";
import fc from "fast-check";
import { type Args, type ArgValue, compile, format, formatRich, I18nFormatError } from "./format";

// ---- Pembangkit ----

const names = ["a", "b", "c", "d", "e"] as const;
const richTags = ["b", "i", "u"] as const;

/** Teks literal aman ICU: tanpa `{ } < > '` agar tidak membentuk sintaks atau tag. `#` boleh (literal di luar plural). */
const literalChar = fc.constantFrom(..."abcXYZ0129 .,!?#-".split(""));
const literal = fc.string({ unit: literalChar, maxLength: 8 });

/** Pohon pesan; dirender dua kali: dengan tag dan tanpa tag (hasil lucutan yang diharapkan). */
type Node =
  | { k: "lit"; s: string }
  | { k: "arg"; n: string }
  | { k: "plural"; n: string; one: Node[]; other: Node[] }
  | { k: "select"; n: string; x: Node[]; other: Node[] }
  | { k: "tag"; t: string; kids: Node[] };

const tree = fc.letrec<{ node: Node; nodes: Node[] }>((tie) => ({
  nodes: fc.array(tie("node"), { maxLength: 3 }),
  node: fc.oneof(
    { depthSize: "small", maxDepth: 3 },
    fc.record({ k: fc.constant("lit" as const), s: literal }),
    fc.record({ k: fc.constant("arg" as const), n: fc.constantFrom(...names) }),
    fc.record({
      k: fc.constant("plural" as const),
      n: fc.constantFrom(...names),
      one: tie("nodes"),
      other: tie("nodes"),
    }),
    fc.record({
      k: fc.constant("select" as const),
      n: fc.constantFrom(...names),
      x: tie("nodes"),
      other: tie("nodes"),
    }),
    fc.record({
      k: fc.constant("tag" as const),
      t: fc.constantFrom(...richTags),
      kids: tie("nodes"),
    }),
  ),
})).nodes;

function render(nodes: Node[], withTags: boolean): string {
  return nodes
    .map((n): string => {
      switch (n.k) {
        case "lit":
          return n.s;
        case "arg":
          return `{${n.n}}`;
        case "plural":
          return `{${n.n}, plural, one {${render(n.one, withTags)}} other {${render(n.other, withTags)}}}`;
        case "select":
          return `{${n.n}, select, x {${render(n.x, withTags)}} other {${render(n.other, withTags)}}}`;
        default:
          return withTags
            ? `<${n.t}>${render(n.kids, withTags)}</${n.t}>`
            : render(n.kids, withTags);
      }
    })
    .join("");
}

const locale = fc.constantFrom("id", "en", "en-GB", "ms");

/** Nilai argumen acak; string tidak memuat `<` supaya keluaran tidak bisa berisi tag dari nilai. */
const argValue: fc.Arbitrary<ArgValue> = fc.oneof(
  fc.string({ unit: fc.constantFrom(..."abc xyz{}#0123'\"&".split("")), maxLength: 6 }),
  fc.integer(),
  fc.double(),
  fc.boolean(),
  fc.constant(null),
  fc.constant(undefined),
);
const args: fc.Arbitrary<Args> = fc.record(
  { a: argValue, b: argValue, c: argValue, d: argValue, e: argValue },
  { requiredKeys: [] },
);

/** Potongan sintaks ICU acak: banyak yang tidak sah, sebagian lolos compile. */
const fuzzMessage = fc
  .array(
    fc.constantFrom(
      "{",
      "}",
      "<",
      ">",
      "</",
      "/>",
      "a",
      "b",
      " ",
      ",",
      "#",
      "=1",
      "'",
      "plural",
      "select",
      "one",
      "other",
      "{a}",
      "{a, plural, other {#}}",
      "{a, select, other {x}}",
      "<b>",
      "</b>",
    ),
    { maxLength: 14 },
  )
  .map((parts) => parts.join(""));

const tagIdentity = (chunks: Array<string>): string => chunks.join("");
const identityTags = { b: tagIdentity, i: tagIdentity, u: tagIdentity };

describe("properti format", () => {
  test("format tidak melempar untuk pesan dan argumen acak yang lolos compile", () => {
    fc.assert(
      fc.property(locale, fuzzMessage, args, (loc, src, a) => {
        let compiled: ReturnType<typeof compile>;
        try {
          compiled = compile(loc, src);
        } catch (err) {
          // Menolak pesan tidak sah adalah perilaku sah; galat jenis lain berarti bug.
          expect(err).toBeInstanceOf(I18nFormatError);
          return;
        }
        expect(typeof format(compiled, a)).toBe("string");
        expect(Array.isArray(formatRich<string>(compiled, a, identityTags))).toBe(true);
      }),
      { numRuns: 500 },
    );
  });

  test("pesan terbangkit yang sah selalu lolos compile dan format tidak melempar", () => {
    fc.assert(
      fc.property(locale, tree, args, (loc, nodes, a) => {
        const out = format(compile(loc, render(nodes, true)), a);
        expect(typeof out).toBe("string");
      }),
      { numRuns: 300 },
    );
  });

  test("hasil identik untuk panggilan berulang (cache aman)", () => {
    fc.assert(
      fc.property(locale, tree, args, (loc, nodes, a) => {
        const src = render(nodes, true);
        const c1 = compile(loc, src);
        const first = format(c1, a);
        const c2 = compile(loc, src);
        expect(c2).toBe(c1);
        expect(format(c2, a)).toBe(first);
        expect(format(c1, a)).toBe(first);
      }),
      { numRuns: 300 },
    );
  });

  test("hasil tetap benar setelah cache penuh dan entri tua terbuang", () => {
    const src = "{n, plural, one {# item} other {# items}} {g, select, x {X} other {O}}";
    const a: Args = { n: 3, g: "x" };
    const before = format(compile("en", src), a);
    for (let i = 0; i < 2100; i++) compile("en", `pesan unik ${i} {a}`);
    expect(format(compile("en", src), a)).toBe(before);
  });

  test("teks polos tidak berisi tag rich dan sama dengan pesan tanpa tag", () => {
    fc.assert(
      fc.property(locale, tree, args, (loc, nodes, a) => {
        const withTags = format(compile(loc, render(nodes, true)), a);
        const without = format(compile(loc, render(nodes, false)), a);
        expect(withTags).toBe(without);
        expect(withTags).not.toMatch(/<\/?[a-z][a-z0-9-]*\s*\/?>/);
      }),
      { numRuns: 300 },
    );
  });

  test("formatRich dengan pemanggil identitas, bila digabung, sama dengan teks polos", () => {
    fc.assert(
      fc.property(locale, tree, args, (loc, nodes, a) => {
        const c = compile(loc, render(nodes, true));
        expect(formatRich<string>(c, a, identityTags).join("")).toBe(format(c, a));
      }),
      { numRuns: 300 },
    );
  });

  test("argumen hilang atau null menghasilkan literal {nama} dan tidak pernah melempar", () => {
    fc.assert(
      fc.property(locale, fc.constantFrom(...names), (loc, n) => {
        const src = `[{${n}}|{${n}, plural, other {#}}|{${n}, select, other {x}}]`;
        const c = compile(loc, src);
        const want = `[{${n}}|{${n}}|{${n}}]`;
        expect(format(c, {})).toBe(want);
        expect(format(c, { [n]: null })).toBe(want);
        expect(format(c, { [n]: undefined })).toBe(want);
      }),
    );
  });
});

import { describe, expect, test } from "bun:test";
import { type ExprNode, evalExpr } from "./expr";
import { field, invoiceMeta } from "./fixtures";
import {
  divDecimal,
  evaluateCondition,
  evaluateExpr,
  formatDecimalString,
  parseDecimal,
  type RoundMode,
  roundDecimal,
} from "./index";
import { checkClientMeta, createResourceMetaLoader } from "./loader";
import type { FieldMeta, ResourceMeta } from "./meta";

// kunci "then" dibangun agar linter noThenProperty tidak menandai objek uji (bentuk JSON ekspresi `if`)
const THEN = ["th", "en"].join("");
const dec = (s: string) => parseDecimal(s) as NonNullable<ReturnType<typeof parseDecimal>>;

describe("M-4: mode pembulatan tak dikenal ditolak, bukan half_down diam-diam", () => {
  test("roundDecimal dan divDecimal mengembalikan null", () => {
    for (const mode of ["", "bankers", "HALF_UP", "half-down", "toString", "__proto__"]) {
      expect(roundDecimal(dec("2.5"), 0, mode as RoundMode)).toBeNull();
      expect(roundDecimal(dec("2.50"), 2, mode as RoundMode)).toBeNull(); // tetap ditolak walau tanpa pembulatan nyata
      expect(divDecimal(dec("1"), dec("3"), 2, mode as RoundMode)).toBeNull();
    }
  });
  test("skala negatif ditolak; mode sah tetap bekerja", () => {
    expect(roundDecimal(dec("1.5"), -1, "half_up")).toBeNull();
    expect(divDecimal(dec("1"), dec("3"), -1, "half_up")).toBeNull();
    expect(roundDecimal(dec("2.5"), 1, "half_down")).toEqual({ coef: 25n, scale: 1 });
    expect(roundDecimal(dec("2.5"), 0, "half_down")).toEqual({ coef: 2n, scale: 0 });
  });
  test("ekspresi bangunan sendiri dengan mode tak dikenal tidak membulatkan diam-diam", () => {
    const node = {
      kind: "add",
      args: [
        { kind: "dec", dec: dec("2.5") },
        { kind: "dec", dec: dec("0") },
      ],
      scale: 0,
      round: "bankers",
    } as unknown as ExprNode;
    const r = evalExpr(node, {}, {});
    expect(r.type).toBe("null");
    expect(r.reasons.map((x) => x.code)).toEqual(["invalid_expression"]);
  });
});

describe("M-5: literal tak berhingga ditolak seperti di Go", () => {
  const types = { i: "integer", d: "decimal" };
  for (const bad of [Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY]) {
    test(`literal ${bad}`, () => {
      const r = evaluateCondition({ field: "i", op: "eq", value: bad }, types, { i: 1 });
      expect(r.reasons.map((x) => x.code)).toEqual(["invalid_condition"]);
      const r2 = evaluateCondition({ field: "i", op: "in", value: [bad] }, types, { i: 1 });
      expect(r2.reasons.map((x) => x.code)).toEqual(["invalid_condition"]);
      expect(evaluateExpr({ lit: bad }, types, {}).reasons.map((x) => x.code)).toEqual([
        "invalid_expression",
      ]);
      const rec = evaluateCondition({ field: "i", op: "isnull", value: false }, types, { i: bad });
      expect(rec.reasons.map((x) => x.code)).toEqual(["type_mismatch"]);
    });
  }
});

describe("M-3: formatDecimalString benar-benar total", () => {
  test("mata uang tidak sah tidak melempar dan tetap memberi angka benar", () => {
    expect(formatDecimalString("1.00", "id-ID", { currency: "rupiah" })).toBe("1,00");
    expect(formatDecimalString("1234.5", "en", { currency: "" })).toBe("1,234.5");
    expect(formatDecimalString("1234.5", "en", { currency: "ZZZZ", scale: 2 })).toBe("1,234.50");
  });
  test("skala ekstrem dan opsi rusak", () => {
    const s64 = formatDecimalString("1.5", "en", { scale: 64 });
    expect(s64).toBe(`1.5${"0".repeat(63)}`);
    expect(formatDecimalString("1.5", "en", { scale: 1_000_000 })).toBe(`1.5${"0".repeat(63)}`);
    for (const scale of [Number.NaN, Number.POSITIVE_INFINITY, -5, 2.9]) {
      const r = formatDecimalString("12", "en", { scale });
      expect(typeof r).toBe("string");
    }
    expect(formatDecimalString("12", "en", { scale: 2.9 })).toBe("12.00");
    expect(formatDecimalString("12", "en", { scale: Number.NaN })).toBe("12");
  });
  test("bahasa rusak atau bukan string tidak melempar", () => {
    for (const locale of [
      "",
      "@@",
      "zz-INVALID-@@",
      123 as unknown as string,
      null as unknown as string,
    ]) {
      expect(formatDecimalString("1234.5", locale, { scale: 1 })).toBe("1,234.5");
    }
  });
});

const withAccessor = (
  name: string,
  accessor: FieldMeta["accessor"],
  extraFields: FieldMeta[] = [],
): ResourceMeta => ({
  ...invoiceMeta,
  fields: [
    ...invoiceMeta.fields,
    ...extraFields,
    field(name, "string", { readOnly: true, nullable: true, accessor }),
  ],
});

const costPrice = field("costPrice", "decimal", { classification: "confidential" });
const financeMargin = field("margin", "decimal", { fieldGroup: "finance" });
const join = (m: ResourceMeta) => checkClientMeta(m).join("\n");

describe("I-2: checkClientMeta menelusuri accessor.expr", () => {
  test("accessor expr aman: tanpa pelanggaran", () => {
    const m = withAccessor("calc", {
      kind: "expr",
      expr: { concat: [{ field: "number" }, { lit: "-" }, { field: "qty" }] },
    });
    expect(checkClientMeta(m)).toEqual([]);
  });
  test("rujukan field confidential ke atas", () => {
    const v = join(
      withAccessor("calc", { kind: "expr", expr: { concat: [{ field: "costPrice" }] } }, [
        costPrice,
      ]),
    );
    expect(v).toContain("costPrice");
    expect(v).toContain("accessor");
  });
  test("rujukan field grup non-default, bersarang di aritmetika", () => {
    const expr = {
      add: [{ dec: "1" }, { mul: [{ field: "margin" }, { dec: "2" }], scale: 2, round: "half_up" }],
      scale: 2,
      round: "half_up",
    };
    expect(join(withAccessor("calc", { kind: "expr", expr }, [financeMargin]))).toContain("margin");
  });
  test("rujukan field yang tidak ada di metadata klien (mis. clientVisible:false)", () => {
    expect(join(withAccessor("calc", { kind: "expr", expr: { field: "hiddenField" } }))).toContain(
      "hiddenField",
    );
  });
  test("kondisi if di dalam expr ikut ditelusuri (kondisi dan cabang)", () => {
    const viaCond = {
      if: { field: "costPrice", op: "gt", value: "0" },
      [THEN]: { lit: "a" },
      else: { lit: "b" },
    };
    expect(join(withAccessor("c1", { kind: "expr", expr: viaCond }, [costPrice]))).toContain(
      "costPrice",
    );
    const viaBranch = {
      if: { field: "qty", op: "gt", value: 0 },
      [THEN]: { field: "margin" },
      else: { lit: "b" },
    };
    expect(join(withAccessor("c2", { kind: "expr", expr: viaBranch }, [financeMargin]))).toContain(
      "margin",
    );
  });
  test("accessor stored dengan expr diperlakukan sama; func tidak punya rujukan", () => {
    expect(
      join(withAccessor("s", { kind: "stored", expr: { field: "costPrice" } }, [costPrice])),
    ).toContain("costPrice");
    expect(checkClientMeta(withAccessor("f", { kind: "func" }))).toEqual([]);
  });
  test("expr rusak dilaporkan, bukan dilewati", () => {
    expect(join(withAccessor("bad", { kind: "expr", expr: { concat: [] } }))).toContain("bad");
  });
  test("pemuat menolak chunk dengan accessor yang membocorkan rujukan", async () => {
    const loader = createResourceMetaLoader();
    const leaked = withAccessor("calc", { kind: "expr", expr: { field: "costPrice" } }, [
      costPrice,
    ]);
    loader.register("sales.invoice", async () => ({ resourceMeta: leaked }));
    await expect(loader.load("sales.invoice")).rejects.toMatchObject({ code: "unsafe_for_client" });
  });
});

const withPivot = (pivot: FieldMeta[], extra: FieldMeta[] = []): ResourceMeta => ({
  ...invoiceMeta,
  fields: [...invoiceMeta.fields, ...extra],
  relations: [
    ...invoiceMeta.relations,
    { name: "tags", type: "belongsToMany", to: "sales.tag", pivotFields: pivot },
  ],
});

describe("I-2: checkClientMeta menelusuri relation.pivotFields", () => {
  test("pivot aman: kondisi atas sesama pivot atau field resource default", () => {
    const m = withPivot([
      field("weight", "integer"),
      field("note", "string", {
        nullable: true,
        visibleWhen: { field: "weight", op: "gt", value: 0 },
      }),
      field("memo", "string", {
        nullable: true,
        visibleWhen: { field: "qty", op: "gt", value: 0 },
      }),
    ]);
    expect(checkClientMeta(m)).toEqual([]);
  });
  test("kondisi pivot merujuk field confidential atau grup non-default", () => {
    const m1 = withPivot(
      [
        field("n", "string", {
          nullable: true,
          visibleWhen: { field: "costPrice", op: "gt", value: "0" },
        }),
      ],
      [costPrice],
    );
    expect(join(m1)).toContain("costPrice");
    const m2 = withPivot(
      [
        field("n", "string", {
          requiredWhen: { field: "margin", op: "gt", value: "0" },
          nullable: true,
        }),
      ],
      [financeMargin],
    );
    expect(join(m2)).toContain("margin");
  });
  test("pivot yang merujuk field confidential milik pivot sendiri", () => {
    const secret = field("secret", "decimal", { classification: "confidential" });
    const m = withPivot([
      secret,
      field("n", "string", {
        nullable: true,
        readonlyWhen: { field: "secret", op: "gt", value: "0" },
      }),
    ]);
    expect(join(m)).toContain("secret");
  });
  test("accessor pada pivot ikut ditelusuri", () => {
    const m = withPivot(
      [
        field("calc", "string", {
          readOnly: true,
          nullable: true,
          accessor: { kind: "expr", expr: { field: "costPrice" } },
        }),
      ],
      [costPrice],
    );
    expect(join(m)).toContain("costPrice");
  });
  test("rujukan ke field yang tidak ada di pivot maupun resource", () => {
    const m = withPivot([
      field("n", "string", {
        nullable: true,
        visibleWhen: { field: "ghost", op: "eq", value: "x" },
      }),
    ]);
    expect(join(m)).toContain("ghost");
  });
  test("pivot dengan kunci terlarang dilaporkan", () => {
    const m = withPivot([
      { ...field("n", "string"), clientVisible: false } as unknown as FieldMeta,
    ]);
    expect(join(m)).toContain("clientVisible");
  });
});

describe("M-1: pemindaian kunci terlarang gagal-tertutup", () => {
  test("struktur melampaui batas pindai dianggap pelanggaran", () => {
    const huge = {
      ...invoiceMeta,
      junk: Array.from({ length: 300_000 }, () => ({ a: 1 })),
    } as unknown as ResourceMeta;
    expect(join(huge)).toContain("terlalu besar");
  });
  test("metadata normal tidak terkena batas", () => {
    expect(checkClientMeta(invoiceMeta)).toEqual([]);
  });
});

describe("M-2: chunk tak lengkap menjadi invalid_module, bukan TypeError", () => {
  const { relations: _r, rules: _u, sections: _s, ...base } = invoiceMeta;
  const cases: [string, unknown][] = [
    ["tanpa rules", { ...base, relations: [], sections: [] }],
    ["tanpa relations", { ...base, rules: [], sections: [] }],
    ["tanpa sections", { ...base, rules: [], relations: [] }],
    ["rules bukan larik", { ...base, rules: {}, relations: [], sections: [] }],
    ["relations null", { ...base, rules: [], relations: null, sections: [] }],
    ["tanpa defaultGroup", { ...invoiceMeta, defaultGroup: undefined }],
    ["elemen field bukan objek", { ...invoiceMeta, fields: [...invoiceMeta.fields, 5] }],
    ["elemen rule bukan objek", { ...invoiceMeta, rules: [null] }],
    [
      "pivotFields bukan larik",
      {
        ...invoiceMeta,
        relations: [{ name: "r", type: "belongsToMany", to: "a.b", pivotFields: {} }],
      },
    ],
  ];
  for (const [name, meta] of cases) {
    test(name, async () => {
      const loader = createResourceMetaLoader();
      loader.register("sales.invoice", async () => ({ resourceMeta: meta as ResourceMeta }));
      await expect(loader.load("sales.invoice")).rejects.toMatchObject({ code: "invalid_module" });
    });
    test(`${name}: checkClientMeta tidak melempar`, () => {
      expect(() => checkClientMeta(meta as ResourceMeta)).not.toThrow();
      expect(checkClientMeta(meta as ResourceMeta).length).toBeGreaterThan(0);
    });
  }
});

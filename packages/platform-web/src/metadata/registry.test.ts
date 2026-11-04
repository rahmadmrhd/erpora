import { describe, expect, test } from "bun:test";
import { field, invoiceMeta } from "./fixtures";
import { evaluateCondition } from "./index";
import {
  createFieldTypeRegistry,
  type EditorKind,
  FIELD_TYPES,
  fieldTypeRegistry,
  OPERATORS_BY_TYPE,
  type WidgetRegistration,
} from "./registry";
import type { FieldType } from "./types";

describe("operator sah per tipe (api-guidelines §5.1)", () => {
  test("tabel operator tiap tipe", () => {
    expect(OPERATORS_BY_TYPE.string).toEqual(["eq", "ne", "in", "contains", "isnull"]);
    expect(OPERATORS_BY_TYPE.text).toEqual(["eq", "ne", "in", "contains", "isnull"]);
    for (const t of ["integer", "decimal", "date", "datetime"] as FieldType[]) {
      expect(OPERATORS_BY_TYPE[t]).toEqual(["eq", "ne", "gt", "gte", "lt", "lte", "in", "isnull"]);
    }
    expect(OPERATORS_BY_TYPE.boolean).toEqual(["eq", "ne", "isnull"]);
    expect(OPERATORS_BY_TYPE.enum).toEqual(["eq", "ne", "in", "isnull"]);
    expect(OPERATORS_BY_TYPE.uuid).toEqual(["eq", "ne", "in", "isnull"]);
  });

  test("setiap operator registri didukung evaluator (tidak pernah unsupported_op)", () => {
    const sample: Record<FieldType, unknown> = {
      string: "a",
      text: "a",
      integer: 1,
      decimal: "1.5",
      boolean: true,
      date: "2026-01-01",
      datetime: "2026-01-01T00:00:00Z",
      enum: "draft",
      uuid: "123e4567-e89b-12d3-a456-426614174000",
    };
    for (const type of FIELD_TYPES) {
      for (const op of OPERATORS_BY_TYPE[type]) {
        const value = op === "isnull" ? true : op === "in" ? [sample[type]] : sample[type];
        const r = evaluateCondition({ field: "f", op, value }, { f: type }, { f: sample[type] });
        expect(r.reasons).toEqual([]);
      }
    }
  });

  test("operator yang tidak ada di registri memang tidak didukung evaluator pada tipe itu", () => {
    for (const type of FIELD_TYPES) {
      for (const op of ["gt", "contains"] as const) {
        if ((OPERATORS_BY_TYPE[type] as readonly string[]).includes(op)) continue;
        const r = evaluateCondition({ field: "f", op, value: "a" }, { f: type }, { f: null });
        // boolean/enum/uuid gt, dan contains pada non-teks: unsupported_op atau literal tak cocok; tidak pernah true
        expect(r.value).toBe(false);
      }
    }
  });

  test("operatorsFor: field tidak filterable tidak punya operator", () => {
    const f = field("x", "string", { filterable: false });
    expect(fieldTypeRegistry.operatorsFor(f)).toEqual([]);
  });

  test("operatorsFor: tipe domain mempersempit operator (irisan dengan tipe dasar)", () => {
    const email = invoiceMeta.fields.find((f) => f.name === "email");
    expect(fieldTypeRegistry.operatorsFor(email as NonNullable<typeof email>)).toEqual([
      "eq",
      "contains",
      "isnull",
    ]);
    const widened = field("w", "boolean", {
      filterable: true,
      operators: ["eq", "contains", "gt"],
    });
    expect(fieldTypeRegistry.operatorsFor(widened)).toEqual(["eq"]);
  });

  test("operatorsFor: tanpa operators domain memakai tabel tipe", () => {
    const f = field("t", "decimal", { filterable: true });
    expect(fieldTypeRegistry.operatorsFor(f)).toEqual(OPERATORS_BY_TYPE.decimal);
  });
});

describe("kontrak editor form dan filter", () => {
  const reg = createFieldTypeRegistry();
  test("jenis editor form bawaan per tipe", () => {
    const kinds: Record<FieldType, EditorKind> = {
      string: "text",
      text: "textarea",
      integer: "integer",
      decimal: "decimal",
      boolean: "boolean",
      date: "date",
      datetime: "datetime",
      enum: "enum",
      uuid: "text",
    };
    for (const type of FIELD_TYPES) expect(reg.formEditorKind(field("x", type))).toBe(kinds[type]);
  });

  test("editor filter bergantung pada operator: isnull boolean, in jamak", () => {
    const f = field("total", "decimal", { filterable: true });
    expect(reg.filterEditor(f, "gte")).toEqual({ kind: "decimal", multiple: false });
    expect(reg.filterEditor(f, "in")).toEqual({ kind: "decimal", multiple: true });
    expect(reg.filterEditor(f, "isnull")).toEqual({ kind: "boolean", multiple: false });
    expect(reg.filterEditor(field("s", "string", { filterable: true }), "contains")).toEqual({
      kind: "text",
      multiple: false,
    });
  });

  test("editor filter menolak operator yang tidak sah bagi field", () => {
    expect(reg.filterEditor(field("b", "boolean", { filterable: true }), "contains")).toBeNull();
    expect(reg.filterEditor(field("n", "string", { filterable: false }), "eq")).toBeNull();
  });
});

describe("widget kustom modul tanpa mengubah @erpora/ui (Requirement 3.13)", () => {
  const widget: WidgetRegistration = { form: () => null, filter: () => null };

  test("daftar dan resolve lewat kunci widget field", () => {
    const reg = createFieldTypeRegistry();
    reg.registerWidget("rupiah", widget);
    const f = field("price", "decimal", { widget: "rupiah", filterable: true });
    const r = reg.resolve(f);
    expect(r.widget).toBe(widget);
    expect(r.formKind).toBe("decimal");
    expect(r.operators).toEqual(OPERATORS_BY_TYPE.decimal);
  });

  test("tanpa registrasi: jatuh ke jenis bawaan tipe, widget tidak ada", () => {
    const reg = createFieldTypeRegistry();
    const r = reg.resolve(field("price", "decimal", { widget: "rupiah" }));
    expect(r.widget).toBeUndefined();
    expect(r.formKind).toBe("decimal");
  });

  test("registrasi ganda ditolak; kunci tidak sah ditolak", () => {
    const reg = createFieldTypeRegistry();
    reg.registerWidget("rupiah", widget);
    expect(() => reg.registerWidget("rupiah", widget)).toThrow();
    expect(() => reg.registerWidget("Bad Key", widget)).toThrow();
    expect(() => reg.registerWidget("__proto__", widget)).toThrow();
  });

  test("registri terisolasi antar instans", () => {
    const a = createFieldTypeRegistry();
    const b = createFieldTypeRegistry();
    a.registerWidget("rupiah", widget);
    expect(b.widget("rupiah")).toBeUndefined();
    expect(a.widget("toString")).toBeUndefined();
  });

  test("widget dapat mempersempit operator (irisan, tidak melebar)", () => {
    const reg = createFieldTypeRegistry();
    reg.registerWidget("narrow", { operators: ["eq", "contains"] });
    const f = field("p", "decimal", { widget: "narrow", filterable: true });
    expect(reg.resolve(f).operators).toEqual(["eq"]);
  });
});

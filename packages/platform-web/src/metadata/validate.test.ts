import { describe, expect, test } from "bun:test";
import { field, invoiceMeta, validDraft } from "./fixtures";
import {
  emptyRecord,
  fieldAvailability,
  isFieldAvailable,
  labelRef,
  type ResourceMeta,
  writablePayload,
} from "./meta";
import { type ValidationIssue, validateRecord } from "./validate";

const codes = (issues: readonly ValidationIssue[]) => issues.map((i) => `${i.field}:${i.code}`);
const draft = (patch: Record<string, unknown>) => ({ ...validDraft, ...patch });
const meta1 = (...fields: ReturnType<typeof field>[]): ResourceMeta => ({
  key: "sales.thing",
  defaultGroup: "default",
  fields,
  relations: [],
  rules: [],
  sections: [],
});

describe("validateRecord: draf valid", () => {
  test("draf lengkap tanpa isu", () => {
    expect(validateRecord(invoiceMeta, validDraft)).toEqual([]);
  });
  test("field readOnly dan accessor tidak divalidasi", () => {
    expect(
      validateRecord(invoiceMeta, draft({ id: "bukan-uuid", number: 5, subtotalLabel: 9 })),
    ).toEqual([]);
  });
});

describe("validateRecord: wajib dan nullable", () => {
  test("non-nullable tanpa nilai: required (create)", () => {
    const { customerName: _omit, ...rest } = validDraft;
    expect(codes(validateRecord(invoiceMeta, rest))).toEqual(["customerName:required"]);
  });
  test("null pada non-nullable: required", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ customerName: null })))).toEqual([
      "customerName:required",
    ]);
  });
  test("null pada nullable sah", () => {
    expect(validateRecord(invoiceMeta, draft({ dueOn: null, email: null }))).toEqual([]);
  });
  test("tidak ada tetapi punya default: sah di create", () => {
    const { qty: _q, status: _s, paid: _p, ...rest } = validDraft;
    expect(validateRecord(invoiceMeta, rest)).toEqual([]);
  });
  test("null pada field ber-default tetap required bila non-nullable", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ qty: null })))).toEqual(["qty:required"]);
  });
  test("update: field yang tidak ada dilewati (tidak berubah), tetapi null ditolak", () => {
    expect(validateRecord(invoiceMeta, { qty: 3 }, { mode: "update" })).toEqual([]);
    expect(codes(validateRecord(invoiceMeta, { customerName: null }, { mode: "update" }))).toEqual([
      "customerName:required",
    ]);
  });
  test("requiredWhen: wajib bila kondisi benar", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ total: "50000000.00" })))).toEqual([
      "approvalNote:required",
    ]);
    expect(validateRecord(invoiceMeta, draft({ total: "49999999.99" }))).toEqual([]);
    expect(
      validateRecord(invoiceMeta, draft({ total: "50000000.00", approvalNote: "disetujui" })),
    ).toEqual([]);
  });
  test("requiredWhen memakai desimal, bukan float", () => {
    // 50000000.001 > 50000000.00 secara desimal; float dengan pembulatan akan menyamakannya
    expect(codes(validateRecord(invoiceMeta, draft({ total: "50000000.01" })))).toEqual([
      "approvalNote:required",
    ]);
  });
  test("visibleWhen pasti salah: field tersembunyi tidak divalidasi", () => {
    const hidden = draft({ status: "cancelled", total: "50000000.00", approvalNote: null });
    expect(validateRecord(invoiceMeta, hidden)).toEqual([]);
  });
  test("readonlyWhen benar: nilai field terkunci tidak divalidasi", () => {
    expect(
      validateRecord(invoiceMeta, draft({ status: "submitted", lockedNote: "x".repeat(50) })),
    ).toEqual([]);
    expect(codes(validateRecord(invoiceMeta, draft({ lockedNote: "x".repeat(50) })))).toEqual([
      "lockedNote:too_long",
    ]);
  });
});

describe("validateRecord: tipe dan batasan", () => {
  test("panjang dihitung dalam code point", () => {
    const m = meta1(field("s", "string", { maxLength: 3, minLength: 2 }));
    expect(validateRecord(m, { s: "\u{1F600}\u{1F600}\u{1F600}" })).toEqual([]);
    expect(codes(validateRecord(m, { s: "\u{1F600}\u{1F600}\u{1F600}\u{1F600}" }))).toEqual([
      "s:too_long",
    ]);
    expect(codes(validateRecord(m, { s: "a" }))).toEqual(["s:too_short"]);
    expect(validateRecord(m, { s: "a" })[0]?.params).toEqual({ min: 2 });
    expect(validateRecord(m, { s: "abcd" })[0]?.params).toEqual({ max: 3 });
  });
  test("tipe salah: invalid_type", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ customerName: 5 })))).toEqual([
      "customerName:invalid_type",
    ]);
    expect(codes(validateRecord(invoiceMeta, draft({ qty: "2" })))).toEqual(["qty:invalid_type"]);
    expect(codes(validateRecord(invoiceMeta, draft({ qty: 2.5 })))).toEqual(["qty:invalid_type"]);
    expect(codes(validateRecord(invoiceMeta, draft({ total: 100.5 })))).toEqual([
      "total:invalid_type",
    ]);
    expect(codes(validateRecord(invoiceMeta, draft({ total: "1e3" })))).toEqual([
      "total:invalid_type",
    ]);
    expect(codes(validateRecord(invoiceMeta, draft({ paid: "true" })))).toEqual([
      "paid:invalid_type",
    ]);
    expect(codes(validateRecord(invoiceMeta, draft({ issuedOn: "2026-02-30" })))).toContain(
      "issuedOn:invalid_type",
    );
  });
  test("pola", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ customerName: "Acme 99" })))).toEqual([
      "customerName:pattern",
    ]);
  });
  test("pola: input terlalu panjang tidak diuji di klien (server otoritatif)", () => {
    const m = meta1(field("s", "string", { pattern: "^a+$" }));
    expect(validateRecord(m, { s: `${"a".repeat(1024)}b` })).toEqual([]);
    expect(codes(validateRecord(m, { s: "ab" }))).toEqual(["s:pattern"]);
  });
  test("pola tidak diuji pada nilai yang melebihi maxLength (hanya too_long)", () => {
    expect(
      codes(validateRecord(invoiceMeta, draft({ customerName: `${"x".repeat(60)}1` }))),
    ).toEqual(["customerName:too_long"]);
  });
  test("format email dan url", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ email: "bukan-email" })))).toEqual([
      "email:format",
    ]);
    expect(validateRecord(invoiceMeta, draft({ email: "a@b.co" }))).toEqual([]);
    const m = meta1(field("u", "string", { format: "url" }));
    expect(validateRecord(m, { u: "https://example.com/x" })).toEqual([]);
    expect(codes(validateRecord(m, { u: "javascript:alert(1)" }))).toEqual(["u:format"]);
    expect(codes(validateRecord(m, { u: "ftp://x" }))).toEqual(["u:format"]);
  });
  test("rentang integer dan desimal (batas inklusif)", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ qty: 0 })))).toEqual(["qty:below_minimum"]);
    expect(codes(validateRecord(invoiceMeta, draft({ qty: 101 })))).toEqual(["qty:above_maximum"]);
    expect(validateRecord(invoiceMeta, draft({ qty: 1 }))).toEqual([]);
    expect(validateRecord(invoiceMeta, draft({ qty: 100 }))).toEqual([]);
    expect(codes(validateRecord(invoiceMeta, draft({ total: "-0.01" })))).toEqual([
      "total:below_minimum",
    ]);
    expect(validateRecord(invoiceMeta, draft({ total: "-0.00" }))).toEqual([]);
    expect(
      codes(validateRecord(invoiceMeta, draft({ total: "10000000000.00", approvalNote: "ok" }))),
    ).toEqual(["total:decimal_precision", "total:above_maximum"]);
    expect(validateRecord(invoiceMeta, draft({ total: "-0.01" }))[0]?.params).toEqual({
      min: "0.00",
    });
  });
  test("presisi dan skala desimal", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ total: "1.005" })))).toEqual([
      "total:decimal_scale",
    ]);
    expect(
      codes(validateRecord(invoiceMeta, draft({ total: "12345678901.00", approvalNote: "ok" }))),
    ).toEqual(["total:decimal_precision", "total:above_maximum"]);
    expect(
      validateRecord(invoiceMeta, draft({ total: "9999999999.99", approvalNote: "ok" })),
    ).toEqual([]);
  });
  test("enum tertutup dan extensible", () => {
    expect(codes(validateRecord(invoiceMeta, draft({ status: "ghost" })))).toEqual([
      "status:not_in_values",
    ]);
    expect(codes(validateRecord(invoiceMeta, draft({ priority: "urgent" })))).toEqual([]);
    expect(codes(validateRecord(invoiceMeta, draft({ priority: "Urgent!" })))).toEqual([
      "priority:not_in_values",
    ]);
  });
  test("bukan field metadata: diabaikan (server menolak field tak dikenal)", () => {
    expect(validateRecord(invoiceMeta, draft({ rahasia: "x" }))).toEqual([]);
  });
});

describe("validateRecord: rules", () => {
  test("pelanggaran memberi isu rule dengan MessageRef i18n, bukan teks bebas", () => {
    const issues = validateRecord(invoiceMeta, draft({ dueOn: "2026-02-01" }));
    expect(issues).toHaveLength(1);
    const i = issues[0] as ValidationIssue;
    expect(i.code).toBe("rule");
    expect(i.field).toBe("dueOn");
    expect(i.ruleId).toBe("dueAfterIssue");
    expect(i.message).toEqual({ ns: "sales", key: "errors.invoice.dueBeforeIssued" });
    expect(Object.keys(i).sort()).toEqual(["code", "field", "message", "ruleId"]);
  });
  test("aturan tak tentu (field tidak tersedia atau null) dilewati; server memutuskan", () => {
    const { issuedOn: _i, ...noIssued } = validDraft;
    expect(codes(validateRecord(invoiceMeta, noIssued, { mode: "update" }))).toEqual([]);
    expect(validateRecord(invoiceMeta, draft({ dueOn: null }))).toEqual([]);
  });
  test("aturan yang merujuk field yang tidak ada di metadata klien tidak pernah melempar", () => {
    const m: ResourceMeta = {
      ...invoiceMeta,
      rules: [{ id: "r", assert: { field: "hidden", op: "eq", value: "x" }, message: "errors.x" }],
    };
    expect(validateRecord(m, validDraft)).toEqual([]);
  });
});

describe("field tidak ada di respons = tidak tersedia (bukan kosong)", () => {
  const response = { id: "123e4567-e89b-12d3-a456-426614174000", number: "INV-1", qty: 2 };
  test("isFieldAvailable dan fieldAvailability", () => {
    expect(isFieldAvailable(response, "qty")).toBe(true);
    expect(isFieldAvailable(response, "total")).toBe(false);
    expect(isFieldAvailable({ total: null }, "total")).toBe(true);
    expect(isFieldAvailable({ total: undefined }, "total")).toBe(false);
    const av = fieldAvailability(invoiceMeta, response);
    expect(av.qty).toBe("available");
    expect(av.total).toBe("unavailable");
  });
  test("update: field tidak tersedia tidak divalidasi, dan tidak ikut muatan tulis", () => {
    expect(validateRecord(invoiceMeta, response, { mode: "update" })).toEqual([]);
    expect(Object.keys(writablePayload(invoiceMeta, response))).toEqual(["qty"]);
  });
  test("kondisi atas field tidak tersedia menjadi tak tentu, bukan benar atau salah", () => {
    // approvalNote wajib bila total >= 50jt; total tidak tersedia: tidak boleh dianggap 0 atau kosong
    expect(validateRecord(invoiceMeta, { approvalNote: null }, { mode: "update" })).toEqual([]);
  });
  test("emptyRecord: bawaan atau null, tanpa readOnly dan accessor", () => {
    const e = emptyRecord(invoiceMeta);
    expect(e.qty).toBe(1);
    expect(e.customerName).toBeNull();
    expect("id" in e).toBe(false);
    expect("subtotalLabel" in e).toBe(false);
  });
  test("writablePayload membuang readOnly, accessor, dan field terkunci", () => {
    const rec = { ...validDraft, id: "x", number: "N", subtotalLabel: "s" };
    const keys = Object.keys(writablePayload(invoiceMeta, rec, (f) => f.name === "lockedNote"));
    expect(keys).not.toContain("id");
    expect(keys).not.toContain("number");
    expect(keys).not.toContain("subtotalLabel");
    expect(keys).not.toContain("lockedNote");
    expect(keys).toContain("customerName");
  });
});

describe("teks UI lewat i18n", () => {
  test("labelRef memakai namespace modul pemilik resource", () => {
    expect(labelRef(invoiceMeta, "fields.invoice.number")).toEqual({
      ns: "sales",
      key: "fields.invoice.number",
    });
    expect(labelRef({ key: "my-mod.thing" }, "a.b")).toEqual({ ns: "my-mod", key: "a.b" });
  });
  test("isu tidak pernah membawa teks bebas atau nilai field", () => {
    const issues = validateRecord(
      invoiceMeta,
      draft({ customerName: "Rahasia 123", email: "bocor" }),
    );
    const json = JSON.stringify(issues);
    expect(json).not.toContain("Rahasia");
    expect(json).not.toContain("bocor");
    for (const i of issues)
      expect(i.message === undefined || typeof i.message === "object").toBe(true);
  });
});

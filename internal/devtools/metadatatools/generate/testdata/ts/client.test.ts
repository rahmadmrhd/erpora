// Memeriksa keluaran golden pembangkit TypeScript (task 6.4) terhadap paket metadata frontend: pagar chunk publik
// (checkClientMeta), pemuat malas, dan validator. Dijalankan oleh `bun test` dan oleh TestTypeScriptPassesClientLoader.
// Impor relatif ke sumber paket agar tidak bergantung pada resolusi workspace.
import { describe, expect, test } from "bun:test";
import {
  checkClientMeta,
  createResourceMetaLoader,
} from "../../../../../../packages/platform-web/src/metadata/loader";
import { fieldTypes } from "../../../../../../packages/platform-web/src/metadata/meta";
import { validateRecord } from "../../../../../../packages/platform-web/src/metadata/validate";
import { resourceMeta as customer } from "../golden/fixture-web/modules/fixturecore/web/src/metadata/customer.meta.gen";
import { resourceMeta as invoice } from "../golden/fixture-web/modules/fixturesales/web/src/metadata/invoice.meta.gen";
import { resourceMeta as invoiceLine } from "../golden/fixture-web/modules/fixturesales/web/src/metadata/invoiceLine.meta.gen";
import { resourceMeta as note } from "../golden/fixture-web/modules/fixturesales/web/src/metadata/note.meta.gen";

const all = [customer, invoice, invoiceLine, note];

describe("keluaran pembangkit TS", () => {
  test("lolos checkClientMeta (aman untuk chunk publik)", () => {
    for (const meta of all) expect(checkClientMeta(meta)).toEqual([]);
  });

  test("tidak memuat clientVisible atau scope", () => {
    for (const meta of all) {
      const json = JSON.stringify(meta);
      expect(json).not.toContain("clientVisible");
      expect(json).not.toContain('"scope"');
    }
  });

  test("aturan scope server dan ambang confidential tidak ikut", () => {
    const ids = invoice.rules.map((r) => r.id);
    expect(ids).toContain("dueAfterIssue");
    expect(ids).not.toContain("discountWithinCap");
    expect(JSON.stringify(invoice)).not.toContain("50000000.00");
    const approval = invoice.fields.find((f) => f.name === "approvalNote");
    expect(approval?.requiredWhen).toBeUndefined();
    expect(approval?.visibleWhen).toBeUndefined();
  });

  test("struktur field dan relasi mencerminkan model datar", () => {
    const total = invoice.fields.find((f) => f.name === "total");
    expect(total).toMatchObject({
      type: "decimal",
      precision: 18,
      scale: 2,
      classification: "confidential",
      fieldGroup: "pricing",
    });
    const status = invoice.fields.find((f) => f.name === "status");
    expect(status?.values).toEqual(["draft", "deleted", "need_approval", "accepted", "rejected", "cancelled", "done"]);
    expect(status?.readOnly).toBe(true);
    expect(invoice.lockWhen).toEqual({ field: "status", op: "ne", value: "draft" });
    expect(invoice.relations.map((r) => r.name)).toEqual(["customer", "lines", "tags", "notes"]);
    expect(invoice.relations[0]).toMatchObject({ type: "belongsTo", to: "fixturecore.customer", required: true });
    expect(invoice.relations[1]).toMatchObject({ type: "hasMany", of: "fixturesales.invoiceLine", embedded: true });
    expect(invoice.relations[2].pivotFields?.[0]?.name).toBe("remark");
    expect(invoice.defaultGroup).toBe("general");
    expect(customer.fields.find((f) => f.name === "contactEmail")).toMatchObject({
      domainType: "fixturecore.email",
      widget: "email",
      format: "email",
    });
  });

  test("sumbu status dan lifecycle hanya field enum readOnly (tabel transisi tidak dikirim ke klien)", () => {
    const axis = (name: string) => invoice.fields.find((f) => f.name === name);
    expect(axis("billingStatus")).toMatchObject({
      type: "enum",
      readOnly: true,
      nullable: false,
      default: "to_bill",
      values: ["to_bill", "billed"],
    });
    expect(axis("deliveryStatus")).toMatchObject({
      type: "enum",
      readOnly: true,
      default: "to_deliver",
      values: ["to_deliver", "partially_delivered", "delivered"],
    });
    const json = JSON.stringify(invoice);
    expect(json).not.toContain("submitted");
    expect(json).not.toContain("transitions");
  });

  test("pasangan rename: kedua bentuk ada, bentuk lama bertanda deprecated (masa transisi)", () => {
    const phone = customer.fields.find((f) => f.name === "phone");
    const phoneE164 = customer.fields.find((f) => f.name === "phoneE164");
    expect(phone?.deprecated).toEqual({ since: "1.1.0", replacedBy: "phoneE164" });
    expect(phoneE164).toBeDefined();
    expect(phoneE164?.deprecated).toBeUndefined();
    expect(phone?.classification).toBe(phoneE164?.classification);
    expect(phone?.fieldGroup).toBe(phoneE164?.fieldGroup);
    expect(phoneE164?.maxLength).toBe(20);
  });

  test("tipe field membentuk peta untuk evaluator dan validator berjalan atasnya", () => {
    expect(fieldTypes(invoice).issuedOn).toBe("date");
    const issues = validateRecord(
      invoice,
      { issuedOn: "2026-02-01", dueOn: "2026-01-01", paymentTerm: "net_30", taxable: true },
      { mode: "update" },
    );
    expect(issues.some((i) => i.code === "rule" && i.ruleId === "dueAfterIssue")).toBe(true);
  });

  test("pemuat malas menerima chunk bangkitan", async () => {
    const loader = createResourceMetaLoader();
    loader.register("fixturesales.invoice", async () => ({ resourceMeta: invoice }));
    expect((await loader.load("fixturesales.invoice")).key).toBe("fixturesales.invoice");
  });
});

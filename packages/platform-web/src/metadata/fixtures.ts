// Fixture metadata untuk test (bukan kode produksi): bentuk yang akan dipancarkan pembangkit task 6.4.
import type { FieldMeta, ResourceMeta } from "./meta";

export function field(
  name: string,
  type: FieldMeta["type"],
  extra: Partial<FieldMeta> = {},
): FieldMeta {
  return {
    name,
    type,
    label: `fields.invoice.${name}`,
    nullable: false,
    readOnly: false,
    fieldGroup: "default",
    classification: "internal",
    filterable: false,
    sortable: false,
    groupable: false,
    aggregatable: false,
    ...extra,
  };
}

export const invoiceMeta: ResourceMeta = {
  key: "sales.invoice",
  labelTemplate: "{number} · {customerName}",
  defaultGroup: "default",
  fields: [
    field("id", "uuid", { readOnly: true }),
    field("number", "string", { maxLength: 32, readOnly: true, sortable: true, filterable: true }),
    field("customerName", "string", {
      minLength: 2,
      maxLength: 20,
      pattern: "^[A-Za-z ]+$",
      filterable: true,
    }),
    field("email", "string", {
      nullable: true,
      maxLength: 254,
      format: "email",
      classification: "confidential",
      domainType: "sdk:email",
      widget: "email",
      operators: ["eq", "contains", "isnull"],
      filterable: true,
    }),
    field("issuedOn", "date", { filterable: true, sortable: true }),
    field("dueOn", "date", { nullable: true }),
    field("qty", "integer", { minimum: 1, maximum: 100, default: 1, filterable: true }),
    field("total", "decimal", {
      precision: 12,
      scale: 2,
      minimum: "0.00",
      maximum: "9999999999.99",
      aggregatable: true,
      sortable: true,
      filterable: true,
    }),
    field("status", "enum", {
      values: ["draft", "submitted", "cancelled"],
      default: "draft",
      filterable: true,
    }),
    field("priority", "enum", { values: ["low", "high"], extensible: true, nullable: true }),
    field("paid", "boolean", { default: false }),
    field("approvalNote", "text", {
      nullable: true,
      requiredWhen: { field: "total", op: "gte", value: "50000000.00" },
      visibleWhen: { field: "status", op: "ne", value: "cancelled" },
    }),
    field("lockedNote", "string", {
      nullable: true,
      maxLength: 10,
      readonlyWhen: { field: "status", op: "ne", value: "draft" },
    }),
    field("subtotalLabel", "string", {
      readOnly: true,
      nullable: true,
      accessor: { kind: "expr", expr: { concat: [{ field: "number" }] } },
    }),
  ],
  relations: [
    {
      name: "customer",
      type: "belongsTo",
      to: "crm.customer",
      required: true,
      filter: { field: "status", op: "eq", value: "active" },
      snapshot: [{ from: "name", to: "customerName" }],
    },
    { name: "lines", type: "hasMany", of: "sales.invoiceLine", embedded: true },
  ],
  rules: [
    {
      id: "dueAfterIssue",
      assert: {
        any: [
          { field: "dueOn", op: "isnull", value: true },
          { field: "dueOn", op: "gte", ref: "issuedOn" },
        ],
      },
      field: "dueOn",
      message: "errors.invoice.dueBeforeIssued",
    },
  ],
  sections: [{ key: "main", columns: 2, order: ["number", "customerName", "issuedOn", "dueOn"] }],
  lockWhen: { field: "status", op: "ne", value: "draft" },
};

export const validDraft = {
  customerName: "Acme Corp",
  email: null,
  issuedOn: "2026-03-01",
  dueOn: "2026-03-31",
  qty: 2,
  total: "100.50",
  status: "draft",
  priority: null,
  paid: false,
  approvalNote: null,
  lockedNote: null,
};

// Registri tipe field (Requirement 18.1, 3.13; design Komponen 10): memetakan tiap tipe ke operator filter yang sah
// (api-guidelines §5.1), jenis editor nilai untuk form, dan jenis editor nilai untuk filter. Paket ini hanya
// mendefinisikan KONTRAK sebagai tipe TS; komponen editor dibuat di M3c (form) dan M3d (filter). Modul dapat
// mendaftarkan widget kustom (kunci widget dari tipe domain) tanpa mengubah `@erpora/ui`.

import type { ComponentType } from "react";
import type { ConditionOp } from "./condition";
import type { FieldMeta } from "./meta";
import type { FieldType } from "./types";

export const FIELD_TYPES: readonly FieldType[] = [
  "string",
  "text",
  "integer",
  "decimal",
  "boolean",
  "date",
  "datetime",
  "enum",
  "uuid",
];

const ORDERED: readonly ConditionOp[] = ["eq", "ne", "gt", "gte", "lt", "lte", "in", "isnull"];
const TEXTUAL: readonly ConditionOp[] = ["eq", "ne", "in", "contains", "isnull"];
const SET_ONLY: readonly ConditionOp[] = ["eq", "ne", "in", "isnull"];

/**
 * Operator filter yang sah per tipe dasar. Selalu himpunan bagian dari yang didukung evaluator kondisi
 * (diuji), dan selaras tabel api-guidelines §5.1: `contains` hanya teks, perbandingan urutan hanya
 * tipe terurut (bilangan, tanggal), `isnull` untuk semua.
 */
export const OPERATORS_BY_TYPE: Readonly<Record<FieldType, readonly ConditionOp[]>> = Object.freeze(
  {
    string: TEXTUAL,
    text: TEXTUAL,
    integer: ORDERED,
    decimal: ORDERED,
    date: ORDERED,
    datetime: ORDERED,
    boolean: Object.freeze(["eq", "ne", "isnull"] as const),
    enum: SET_ONLY,
    uuid: SET_ONLY,
  },
);

/** Jenis editor nilai tunggal; pemetaan ke komponen shadcn dilakukan M3c dan M3d. */
export type EditorKind =
  | "text"
  | "textarea"
  | "integer"
  | "decimal"
  | "boolean"
  | "date"
  | "datetime"
  | "enum";

const FORM_KIND: Readonly<Record<FieldType, EditorKind>> = Object.freeze({
  string: "text",
  text: "textarea",
  integer: "integer",
  decimal: "decimal",
  boolean: "boolean",
  date: "date",
  datetime: "datetime",
  enum: "enum",
  uuid: "text",
});

/** Editor nilai filter: jenis editor dan apakah menerima banyak nilai (operator `in`). */
export interface FilterEditorSpec {
  readonly kind: EditorKind;
  readonly multiple: boolean;
}

/**
 * Props editor form (kontrak untuk M3c). `value` mengikuti bentuk kawat: decimal string, integer number, dst.;
 * `undefined` berarti field tidak tersedia (api-guidelines §4.4), bukan kosong.
 */
export interface FieldEditorProps<V = unknown> {
  readonly field: FieldMeta;
  readonly id: string;
  readonly value: V | null | undefined;
  readonly onChange: (value: V | null) => void;
  readonly disabled?: boolean;
  readonly readOnly?: boolean;
  /** Ada isu validasi pada field ini (teks isu dirender lewat i18n oleh pemanggil). */
  readonly invalid?: boolean;
}

/** Props editor nilai filter (kontrak untuk M3d). */
export interface FilterEditorProps<V = unknown> {
  readonly field: FieldMeta;
  readonly operator: ConditionOp;
  readonly id: string;
  readonly value: V | readonly V[] | null | undefined;
  readonly onChange: (value: V | readonly V[] | null) => void;
}

export type FieldEditorComponent = ComponentType<FieldEditorProps>;
export type FilterEditorComponent = ComponentType<FilterEditorProps>;

/** Pendaftaran widget kustom modul; editor dapat berupa komponen langsung atau pemuat malas. */
export interface WidgetRegistration {
  readonly form?: FieldEditorComponent | (() => Promise<FieldEditorComponent>);
  readonly filter?: FilterEditorComponent | (() => Promise<FilterEditorComponent>);
  /** Mempersempit operator filter (irisan dengan tipe dasar dan tipe domain; tidak pernah melebarkan). */
  readonly operators?: readonly ConditionOp[];
}

export interface ResolvedFieldType {
  readonly operators: readonly ConditionOp[];
  readonly formKind: EditorKind;
  /** Widget kustom terdaftar untuk `field.widget`; undefined bila tidak ada. */
  readonly widget?: WidgetRegistration;
}

export interface FieldTypeRegistry {
  /** Operator filter yang sah bagi field; kosong bila field tidak `filterable`. */
  operatorsFor(field: FieldMeta): readonly ConditionOp[];
  formEditorKind(field: FieldMeta): EditorKind;
  /** Editor nilai filter untuk operator; null bila operator tidak sah bagi field. */
  filterEditor(field: FieldMeta, op: ConditionOp): FilterEditorSpec | null;
  registerWidget(key: string, registration: WidgetRegistration): void;
  widget(key: string): WidgetRegistration | undefined;
  resolve(field: FieldMeta): ResolvedFieldType;
}

const WIDGET_KEY_RE = /^[a-z][A-Za-z0-9]*$/;

function intersect(
  base: readonly ConditionOp[],
  narrow: readonly ConditionOp[] | undefined,
): readonly ConditionOp[] {
  return narrow === undefined ? base : base.filter((op) => narrow.includes(op));
}

/** Membuat registri baru (terisolasi); aplikasi memakai singleton `fieldTypeRegistry`. */
export function createFieldTypeRegistry(): FieldTypeRegistry {
  const widgets = new Map<string, WidgetRegistration>();
  const widgetOf = (field: FieldMeta): WidgetRegistration | undefined =>
    field.widget === undefined ? undefined : widgets.get(field.widget);

  const operatorsFor = (field: FieldMeta): readonly ConditionOp[] => {
    if (!field.filterable) return [];
    const base = OPERATORS_BY_TYPE[field.type] ?? [];
    return intersect(intersect(base, field.operators), widgetOf(field)?.operators);
  };

  return {
    operatorsFor,
    formEditorKind: (field) => FORM_KIND[field.type] ?? "text",
    filterEditor(field, op) {
      if (!operatorsFor(field).includes(op)) return null;
      if (op === "isnull") return { kind: "boolean", multiple: false };
      return {
        kind: FORM_KIND[field.type] === "textarea" ? "text" : (FORM_KIND[field.type] ?? "text"),
        multiple: op === "in",
      };
    },
    registerWidget(key, registration) {
      if (!WIDGET_KEY_RE.test(key) || key.length > 63)
        throw new Error(`kunci widget tidak sah: ${key}`);
      if (widgets.has(key)) throw new Error(`widget sudah terdaftar: ${key}`);
      widgets.set(key, registration);
    },
    widget: (key) => widgets.get(key),
    resolve(field) {
      return {
        operators: operatorsFor(field),
        formKind: FORM_KIND[field.type] ?? "text",
        widget: widgetOf(field),
      };
    },
  };
}

/** Registri aplikasi. Modul mendaftarkan widget kustom di sini saat registrasi modul. */
export const fieldTypeRegistry: FieldTypeRegistry = createFieldTypeRegistry();

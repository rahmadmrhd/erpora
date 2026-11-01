// Bentuk metadata resource untuk klien (Requirement 3.13, 18.1, 18.3; design Komponen 10, D-8, D-21).
// Bentuk ini dipancarkan oleh pembangkit `modules/<dir>/web/src/metadata/<nama>.meta.gen.ts` (task 6.4) dari
// model datar resolver: kunci camelCase, atribut seperti di berkas metadata, tanpa jejak asal. Chunk bersifat
// publik (SPA statis), sehingga tipe ini SENGAJA tidak memiliki `clientVisible`, `clientVisibleReason`, atau
// `scope`: field `clientVisible: false` dan aturan `scope: server` tidak pernah dipancarkan.

import type { MessageRef } from "@erpora/module-sdk";
import type { ConditionOp } from "./condition";
import type { FieldType } from "./types";

export type Classification = "public" | "internal" | "confidential";

/** Nilai literal pembanding pada daun kondisi. */
export type ConditionValue = string | number | boolean | readonly (string | number | boolean)[];

/**
 * Kondisi (Requirement 4): pohon `all`/`any`/`not` dengan daun `{field, op, value}` atau `{field, op, ref}`.
 * Bentuk yang sama dikonsumsi `evaluateCondition` dan dibangkitkan sebagai literal oleh pembangkit.
 */
export type Condition =
  | { readonly all: readonly Condition[] }
  | { readonly any: readonly Condition[] }
  | { readonly not: Condition }
  | {
      readonly field: string;
      readonly op: ConditionOp;
      readonly value?: ConditionValue | null;
      readonly ref?: string;
    };

export interface DeprecatedMeta {
  readonly since: string;
  readonly replacedBy?: string;
}

/**
 * Accessor (Requirement 5): `expr` dievaluasi di klien dengan `evaluateExpr`; `stored` dan `func` hanya
 * ditampilkan (nilainya datang dari server). Field accessor selalu hanya-baca.
 */
export type AccessorMeta =
  | { readonly kind: "expr"; readonly expr: unknown }
  | { readonly kind: "stored"; readonly expr?: unknown }
  | { readonly kind: "func" };

/** Satu field hasil resolusi (metadata datar). Atribut opsional yang tidak ada berarti tidak dinyatakan. */
export interface FieldMeta {
  readonly name: string;
  /** Tipe dasar (builtin); tipe domain sudah diselesaikan saat pembangkitan. */
  readonly type: FieldType;
  /** Kunci tipe domain asal (mis. "sdk:email"); hanya informasi (widget dan operator sudah digabung). */
  readonly domainType?: string;
  /** Kunci i18n pada katalog namespace modul pemilik resource (lihat `labelRef`). */
  readonly label: string;
  readonly nullable: boolean;
  readonly readOnly: boolean;
  readonly default?: string | number | boolean | null;
  /** Kunci grup field (bukan nilai akses; klien tidak pernah menyimpulkan akses dari metadata). */
  readonly fieldGroup: string;
  readonly classification: Classification;
  readonly filterable: boolean;
  readonly sortable: boolean;
  readonly groupable: boolean;
  readonly aggregatable: boolean;

  readonly minLength?: number;
  readonly maxLength?: number;
  /** Subset RE2/JS yang sudah lolos `metadata:check`. */
  readonly pattern?: string;
  readonly format?: "email" | "url";
  readonly precision?: number;
  readonly scale?: number;
  readonly currency?: string;
  /** Batas rentang inklusif untuk integer dan decimal; string desimal agar tidak kehilangan presisi. */
  readonly minimum?: number | string;
  readonly maximum?: number | string;
  /** Nilai enum (snake_case). */
  readonly values?: readonly string[];
  readonly extensible?: boolean;

  /** Kunci widget dan format tampilan dari tipe domain (registri tipe field, Requirement 3.13). */
  readonly widget?: string;
  readonly display?: string;
  /** Operator filter yang sah menurut tipe domain (subset operator tipe dasar). */
  readonly operators?: readonly ConditionOp[];

  readonly visibleWhen?: Condition;
  readonly requiredWhen?: Condition;
  readonly readonlyWhen?: Condition;
  readonly deprecated?: DeprecatedMeta;
  readonly accessor?: AccessorMeta;
}

export type RelationType =
  | "belongsTo"
  | "hasOne"
  | "hasMany"
  | "belongsToMany"
  | "morphTo"
  | "morphMany"
  | "morphToMany";

export interface SnapshotMapping {
  readonly from: string;
  readonly to: string;
}

/** Relasi (Requirement 6). Lintas modul hanya menyimpan ID; label datang dari kontrak seberang. */
export interface RelationMeta {
  readonly name: string;
  readonly type: RelationType;
  /** Kunci resource seberang (belongsTo, hasOne, belongsToMany, morphToMany). */
  readonly to?: string;
  /** Kunci resource anak (hasMany, morphMany). */
  readonly of?: string;
  /** Daftar putih tipe target (morphTo). */
  readonly types?: readonly string[];
  readonly as?: string;
  readonly onDelete?: "restrict" | "cascade" | "setNull";
  readonly required?: boolean;
  /** Baris dokumen yang diterima dan diubah bersama induk (hasMany). */
  readonly embedded?: boolean;
  /** Pembatas pilihan di form; klien hanya kenyamanan, server memvalidasi ulang. */
  readonly filter?: Condition;
  readonly snapshot?: readonly SnapshotMapping[];
  readonly pivotFields?: readonly FieldMeta[];
}

/** Aturan lintas field (Requirement 4.3). Hanya aturan `scope: both` yang dipancarkan ke klien. */
export interface RuleMeta {
  readonly id: string;
  /** Kondisi yang HARUS berlaku; pelanggaran memberi isu `rule`. */
  readonly assert: Condition;
  /** Field yang ditandai pada UI (opsional). */
  readonly field?: string;
  /** Kunci i18n pada katalog namespace modul. */
  readonly message: string;
}

export interface SectionMeta {
  readonly key: string;
  readonly columns: number;
  /** Nama field menurut urutan tampil. */
  readonly order: readonly string[];
}

/** Metadata satu resource untuk klien. */
export interface ResourceMeta {
  /** Kunci resource `<slug>.<nama>`. */
  readonly key: string;
  readonly labelTemplate?: string;
  /** Kunci grup default modul pemilik. */
  readonly defaultGroup: string;
  readonly fields: readonly FieldMeta[];
  readonly relations: readonly RelationMeta[];
  readonly rules: readonly RuleMeta[];
  readonly sections: readonly SectionMeta[];
  /** Kondisi penguncian dokumen; hanya bila dapat dipublikasikan ke klien. */
  readonly lockWhen?: Condition;
}

/** Bentuk modul `*.meta.gen.ts`: satu ekspor bernama `resourceMeta`. */
export interface ResourceMetaModule {
  readonly resourceMeta: ResourceMeta;
}

/** Nama namespace i18n modul pemilik resource: slug pada kunci `<slug>.<nama>`. */
export function moduleNamespace(resourceKey: string): string {
  const dot = resourceKey.indexOf(".");
  return dot < 0 ? resourceKey : resourceKey.slice(0, dot);
}

/**
 * Rujukan pesan i18n dari kunci label atau pesan di metadata: namespace = slug modul pemilik resource, kunci =
 * string apa adanya (aturan i18n proyek: tidak ada teks UI bebas di metadata). Hasilnya cocok dengan
 * `useTranslation(ns).t(key)`.
 */
export function labelRef(meta: Pick<ResourceMeta, "key">, key: string): MessageRef {
  return Object.freeze({ ns: moduleNamespace(meta.key), key });
}

const typeCache = new WeakMap<ResourceMeta, Readonly<Record<string, string>>>();

/** Peta nama field ke tipe (untuk evaluator); di-cache per objek metadata. */
export function fieldTypes(meta: ResourceMeta): Readonly<Record<string, string>> {
  let m = typeCache.get(meta);
  if (!m) {
    m = Object.freeze(Object.fromEntries(meta.fields.map((f) => [f.name, f.type])));
    typeCache.set(meta, m);
  }
  return m;
}

export function fieldOf(meta: ResourceMeta, name: string): FieldMeta | undefined {
  return meta.fields.find((f) => f.name === name);
}

/**
 * Field "tersedia" bila ada di respons (api-guidelines §4.4: field yang tidak boleh dilihat dihilangkan dari JSON,
 * bukan dikirim `null`). Klien memperlakukan field yang tidak ada sebagai "tidak tersedia", bukan "kosong".
 */
export function isFieldAvailable(record: Readonly<Record<string, unknown>>, name: string): boolean {
  return Object.hasOwn(record, name) && record[name] !== undefined;
}

export type Availability = "available" | "unavailable";

/** Ketersediaan tiap field metadata pada sebuah record respons. */
export function fieldAvailability(
  meta: ResourceMeta,
  record: Readonly<Record<string, unknown>>,
): Readonly<Record<string, Availability>> {
  const out: Record<string, Availability> = {};
  for (const f of meta.fields)
    out[f.name] = isFieldAvailable(record, f.name) ? "available" : "unavailable";
  return out;
}

/** Record kosong untuk form baru: setiap field bernilai bawaan atau `null` (readOnly dan accessor dilewati). */
export function emptyRecord(meta: ResourceMeta): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of meta.fields) {
    if (f.readOnly || f.accessor) continue;
    out[f.name] = f.default === undefined ? null : f.default;
  }
  return out;
}

/**
 * Muatan tulis dari draf form: hanya field yang tersedia, dapat ditulis (bukan readOnly atau accessor), dan
 * tidak sedang terkunci `readonlyWhen`. Server menolak field readOnly dan tak dikenal (mass assignment).
 */
export function writablePayload(
  meta: ResourceMeta,
  record: Readonly<Record<string, unknown>>,
  isLocked: (field: FieldMeta) => boolean = () => false,
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const f of meta.fields) {
    if (f.readOnly || f.accessor || !isFieldAvailable(record, f.name) || isLocked(f)) continue;
    out[f.name] = record[f.name];
  }
  return out;
}

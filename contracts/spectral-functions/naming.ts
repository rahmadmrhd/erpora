// Rule penamaan field dan tipe data (api-guidelines §4.1).
import { asArr, asObj, asStr, type Obj, type PathSeg, type Result, walkDoc } from "./refs";

const CAMEL = /^[a-z][a-zA-Z0-9]*$/;
const SNAKE = /^[a-z][a-z0-9]*(_[a-z0-9]+)*$/;
const MONEY_NAME = /(amount|total|subtotal|price|cost|qty|quantity|balance|tax|discount|rate)$/i;

/** Semua properti skema: [nama, skemaProperti, lokasi]. */
export function* schemaProperties(root: unknown): Generator<[string, unknown, PathSeg[]]> {
  for (const [path, node] of walkDoc(root)) {
    const o = asObj(node);
    const props = o ? asObj(o.properties) : undefined;
    if (!props || path[path.length - 1] === "properties") continue;
    for (const [name, schema] of Object.entries(props)) {
      yield [name, schema, [...path, "properties", name]];
    }
  }
}

export function propertyCamelCase(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [name, , path] of schemaProperties(root)) {
    if (!CAMEL.test(name)) {
      out.push({ message: `Properti "${name}" harus camelCase (api-guidelines §4.1)`, path });
    }
  }
  return out;
}

export function enumSnakeCase(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [path, node] of walkDoc(root)) {
    const o = asObj(node);
    if (!o || !Array.isArray(o.enum) || path[path.length - 1] === "properties") continue;
    for (const v of asArr(o.enum)) {
      if (typeof v === "string" && !SNAKE.test(v)) {
        out.push({
          message: `Nilai enum "${v}" harus snake_case (api-guidelines §4.1)`,
          path: [...path, "enum"],
        });
      }
    }
  }
  return out;
}

function typeIncludes(schema: Obj, t: string): boolean {
  return schema.type === t || (Array.isArray(schema.type) && schema.type.includes(t));
}

const DECIMAL_REF = /\/schemas\/Decimal$/;

/**
 * Desimal nullable: `oneOf` yang memuat `$ref` ke skema Decimal dan selain itu hanya `{type: 'null'}`
 * (keputusan pemilik produk K21; tambahan aditif atas `$ref` langsung). `oneOf` tanpa Decimal tidak lolos.
 */
function isNullableDecimal(schema: Obj): boolean {
  const alts = asArr(schema.oneOf);
  if (alts.length === 0) return false;
  const isDecimal = (x: unknown) => DECIMAL_REF.test(asStr(asObj(x)?.$ref) ?? "");
  const isNull = (x: unknown) => asObj(x)?.type === "null";
  return alts.some(isDecimal) && alts.every((a) => isDecimal(a) || isNull(a));
}

/**
 * erp-decimal-not-number: uang dan kuantitas adalah string desimal ($ref Decimal, atau oneOf [Decimal, null]),
 * bukan number (§4.1).
 */
export function decimalNotNumber(root: unknown): Result[] {
  const out: Result[] = [];
  for (const [name, raw, path] of schemaProperties(root)) {
    const s = asObj(raw) ?? {};
    if (typeIncludes(s, "number")) {
      out.push({
        message: `Properti "${name}" bertipe number; uang dan kuantitas wajib string desimal ($ref erp:common#/components/schemas/Decimal) (api-guidelines §4.1)`,
        path,
      });
      continue;
    }
    if (asArr(s.oneOf).some((a) => typeIncludes(asObj(a) ?? {}, "number"))) {
      out.push({
        message: `Properti "${name}" memuat alternatif bertipe number; uang dan kuantitas wajib string desimal ($ref erp:common#/components/schemas/Decimal) (api-guidelines §4.1)`,
        path,
      });
      continue;
    }
    const ref = asStr(s.$ref) ?? "";
    // Penghitung bertipe integer (mis. Page.total) bukan uang dan dikecualikan dari pemeriksaan nama.
    if (
      MONEY_NAME.test(name) &&
      !typeIncludes(s, "integer") &&
      !DECIMAL_REF.test(ref) &&
      !isNullableDecimal(s)
    ) {
      out.push({
        message: `Properti "${name}" tampak uang atau kuantitas tetapi tidak merujuk skema Decimal (api-guidelines §4.1)`,
        path,
      });
    }
  }
  return out;
}

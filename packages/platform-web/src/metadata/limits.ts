// Batas sumber daya evaluator metadata (Requirement 1.3, 19.1). Nilai identik dengan sdk/metadata/limits.go.

/** Jumlah simpul maksimum satu kondisi (grup dan daun). */
export const MAX_CONDITION_NODES = 32;
/** Kedalaman maksimum kondisi; simpul akar berkedalaman 1. */
export const MAX_CONDITION_DEPTH = 4;
/** Jumlah nilai maksimum pada operator `in`. */
export const MAX_IN_VALUES = 100;
/** Panjang maksimum pattern, dalam code point. */
export const MAX_PATTERN_LEN = 200;
/** Panjang maksimum nilai teks yang dievaluasi, dalam satuan UTF-16. */
export const MAX_STRING_LEN = 65536;
/** Jumlah digit maksimum nilai desimal masukan (tanpa tanda dan titik). */
export const MAX_DECIMAL_DIGITS = 64;
/** Skala maksimum hasil aritmetika ekspresi. */
export const MAX_SCALE = 30;
/** Jumlah simpul maksimum satu ekspresi, termasuk simpul kondisi pada `if`. */
export const MAX_EXPR_NODES = 32;
/** Kedalaman maksimum simpul ekspresi (kondisi pada `if` punya batas sendiri). */
export const MAX_EXPR_DEPTH = 8;
/** Jumlah argumen maksimum `concat` dan `coalesce`. */
export const MAX_CONCAT_ARGS = 16;
/** Jumlah argumen maksimum `add` dan `mul`. */
export const MAX_ARITH_ARGS = 8;
/** Panjang maksimum nama field. */
export const MAX_IDENTIFIER_LEN = 63;
/** Panjang maksimum templat label, dalam satuan UTF-16. */
export const MAX_TEMPLATE_LEN = 256;
/** Jumlah placeholder maksimum pada templat label. */
export const MAX_TEMPLATE_PLACEHOLDERS = 16;
/**
 * Panjang input maksimum (UTF-16) yang diuji regex di klien (Requirement 2.5): mesin JavaScript dapat
 * melakukan backtracking eksponensial, sehingga input lebih panjang tidak diuji (server tetap otoritatif).
 */
export const CLIENT_PATTERN_INPUT_LIMIT = 1024;

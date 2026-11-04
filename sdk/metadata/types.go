package metadata

import "fmt"

// FieldType adalah tipe field metadata yang dikenal evaluator. TypeNull hanya muncul sebagai tipe hasil
// ekspresi bernilai null.
type FieldType string

const (
	TypeString   FieldType = "string"
	TypeText     FieldType = "text"
	TypeInteger  FieldType = "integer"
	TypeDecimal  FieldType = "decimal"
	TypeBoolean  FieldType = "boolean"
	TypeDate     FieldType = "date"
	TypeDateTime FieldType = "datetime"
	TypeEnum     FieldType = "enum"
	TypeUUID     FieldType = "uuid"
	TypeNull     FieldType = "null"
)

// Types memetakan nama field ke tipenya (berasal dari metadata resource).
type Types map[string]FieldType

// Record memuat nilai field. Kunci yang tidak ada berarti "tidak tersedia" (api-guidelines §4.4), bukan
// kosong; nilai nil berarti null. Nilai yang dikenal: nil, bool, string, json.Number, bilangan bulat dan
// float Go, serta Decimal (untuk tipe decimal).
type Record map[string]any

// Kode alasan (stabil; dipakai korpus vektor dan klien).
const (
	ReasonInvalidCondition  = "invalid_condition"
	ReasonInvalidExpression = "invalid_expression"
	ReasonInvalidTemplate   = "invalid_template"
	ReasonTooDeep           = "too_deep"
	ReasonTooManyNodes      = "too_many_nodes"
	ReasonListTooLong       = "list_too_long"
	ReasonUnknownField      = "unknown_field"
	ReasonFieldUnavailable  = "field_unavailable"
	ReasonTypeMismatch      = "type_mismatch"
	ReasonUnsupportedOp     = "unsupported_op"
	ReasonValueTooLong      = "value_too_long"
	ReasonDivisionByZero    = "division_by_zero"
)

// Reason menjelaskan mengapa evaluasi tak tentu atau input tak sah. Path menunjuk simpul
// (mis. "$.all[1].not"; untuk label "{namaField}").
type Reason struct {
	Code string
	Path string
}

// String berbentuk "kode@path", dipakai korpus vektor.
func (r Reason) String() string { return r.Code + "@" + r.Path }

// Error adalah galat pemuatan struktur (kondisi, ekspresi, templat) yang tidak sah.
type Error struct{ Reason Reason }

func (e *Error) Error() string { return fmt.Sprintf("metadata: %s", e.Reason) }

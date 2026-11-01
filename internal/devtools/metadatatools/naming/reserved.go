package naming

import "strings"

// Target adalah bahasa atau sistem keluaran yang kata tercadangnya diperiksa.
type Target string

// Target yang diperiksa.
const (
	TargetSQL Target = "sql"
	TargetGo  Target = "go"
	TargetTS  Target = "typescript"
)

func set(words string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(words) {
		m[w] = true
	}
	return m
}

// sqlReserved memuat kata kunci PostgreSQL berkategori "reserved" dan "reserved (can be function or type)"
// (lampiran C dokumentasi PostgreSQL 17) yang tidak dapat dipakai sebagai nama kolom tanpa kutip. Dibandingkan
// dengan bentuk snake_case seluruh identifier (userName menjadi user_name tidak tercadang; user tercadang).
var sqlReserved = set(`all analyse analyze and any array as asc asymmetric both case cast check collate column
constraint create current_catalog current_date current_role current_time current_timestamp current_user default
deferrable desc distinct do else end except false fetch for foreign from grant group having in initially intersect
into lateral leading limit localtime localtimestamp not null offset on only or order placing primary references
returning select session_user some symmetric system_user table then to trailing true union unique user using
variadic when where window with
authorization binary collation concurrently cross current_schema freeze full ilike inner is isnull join left like
natural notnull outer overlaps right similar tablesample verbose`)

// goReserved memuat 25 kata kunci Go.
var goReserved = set(`break default func interface select case defer go map struct chan else goto package switch
const fallthrough if range type continue for import return var`)

// tsReserved memuat kata tercadang ECMAScript (termasuk yang tercadang pada strict mode).
var tsReserved = set(`break case catch class const continue debugger default delete do else enum export extends false
finally for function if import in instanceof new null return super switch this throw true try typeof var void while
with yield let static implements interface package private protected public await`)

// ReservedTargets mengembalikan target (urutan sql, go, typescript) yang menganggap name kata tercadang. SQL
// diperiksa pada bentuk snake_case; Go dan TypeScript pada bentuk camelCase apa adanya.
func ReservedTargets(name string) []Target {
	var out []Target
	snake := strings.ToLower(name)
	if s, err := ToSnake(name); err == nil {
		snake = s
	}
	if sqlReserved[snake] {
		out = append(out, TargetSQL)
	}
	if goReserved[name] {
		out = append(out, TargetGo)
	}
	if tsReserved[name] {
		out = append(out, TargetTS)
	}
	return out
}

func reservedError(name string, t Target) *Error {
	switch t {
	case TargetSQL:
		return errf(CodeReservedSQL, name, "kata kunci SQL tercadang; pilih nama lain")
	case TargetGo:
		return errf(CodeReservedGo, name, "kata kunci Go tercadang; pilih nama lain")
	default:
		return errf(CodeReservedTS, name, "kata tercadang TypeScript/JavaScript; pilih nama lain")
	}
}

// CheckReserved mengembalikan galat untuk target pertama yang menganggap name kata tercadang, atau nil.
func CheckReserved(name string) error {
	ts := ReservedTargets(name)
	if len(ts) == 0 {
		return nil
	}
	return reservedError(name, ts[0])
}

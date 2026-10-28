package i18n

import "errors"

// Args adalah argumen pesan. Nilai yang didukung: string, bool, bilangan bulat dan pecahan Go,
// json.Number, dan fmt.Stringer. Nilai nil dan nama yang tidak ada dianggap hilang.
type Args map[string]any

// ParamKind menggolongkan cara sebuah argumen dipakai oleh pesan.
type ParamKind int

// Jenis Param.
const (
	// KindArg adalah argumen sederhana {nama}.
	KindArg ParamKind = iota + 1
	// KindPlural adalah argumen plural: {n, plural, ...}.
	KindPlural
	// KindSelect adalah argumen select: {g, select, ...}.
	KindSelect
	// KindTag adalah tag rich: <b>...</b>.
	KindTag
)

// Param menjelaskan satu argumen (atau tag) yang dibutuhkan pesan; dipakai untuk memeriksa kesesuaian
// antar bahasa dan override serta untuk membangkitkan tipe TypeScript.
type Param struct {
	Name string
	Kind ParamKind
	// Options memuat kunci opsi (termasuk "other") untuk KindSelect, urutan penulisan.
	Options []string
}

// Message adalah pesan ICU yang sudah dikompilasi. Nilainya immutable dan aman dipakai banyak goroutine.
type Message interface {
	// Format menghasilkan teks polos untuk loc; tag rich dilucuti.
	Format(loc Locale, args Args) (string, error)
	// Params mengembalikan argumen dan tag pesan menurut urutan kemunculan pertama.
	Params() []Param
}

var errNotImplemented = errors.New("i18n: belum diimplementasi")

// Compile mengurai satu pesan ICU; pesan yang melebihi batas atau bersintaks tidak sah ditolak.
func Compile(src string, lim Limits) (Message, error) {
	return nil, errNotImplemented
}

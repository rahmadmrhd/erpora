package i18n

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

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

// String mengembalikan nama jenis untuk pesan galat dan laporan.
func (k ParamKind) String() string {
	switch k {
	case KindArg:
		return "arg"
	case KindPlural:
		return "plural"
	case KindSelect:
		return "select"
	case KindTag:
		return "tag"
	}
	return "unknown"
}

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
	// Format menghasilkan teks polos untuk loc; tag rich dilucuti. Argumen yang hilang tidak membuat
	// galat: elemen yang membutuhkannya tampil sebagai {nama} apa adanya.
	Format(loc Locale, args Args) (string, error)
	// Params mengembalikan argumen dan tag pesan menurut urutan kemunculan pertama.
	Params() []Param
}

// MissingReporter diimplementasi Message hasil Compile: FormatReport seperti Format tetapi juga mengembalikan nama
// argumen yang hilang (Requirement 2.3), sehingga pemanggil dapat mencatat peringatan.
type MissingReporter interface {
	FormatReport(loc Locale, args Args) (out string, missing []string, err error)
}

// CompileError adalah galat Compile. Code salah satu dari CodeInvalidICU, CodeUnsupportedConstruct,
// atau CodeTooLarge. Pos adalah indeks karakter (rune) dalam pesan.
type CompileError struct {
	Code   string
	Pos    int
	Detail string
}

// Error mengimplementasi error.
func (e *CompileError) Error() string {
	return fmt.Sprintf("%s: posisi %d: %s", e.Code, e.Pos, e.Detail)
}

// Issue mengubah galat menjadi Issue (File, Locale, dan Key diisi pemanggil).
func (e *CompileError) Issue() Issue {
	return Issue{Code: e.Code, Detail: fmt.Sprintf("posisi %d: %s", e.Pos, e.Detail)}
}

// message adalah implementasi Message.
type message struct {
	nodes  []node
	params []Param
	// htmlReason tidak kosong bila pesan memuat markup yang bukan tag rich sederhana (lihat
	// ValidateMessage); Compile sendiri tetap menerimanya agar paritas dengan FormatJS terjaga.
	htmlReason string
}

// Params mengimplementasi Message. Salinan dikembalikan agar pesan tetap immutable.
func (m *message) Params() []Param {
	out := make([]Param, len(m.params))
	for i, p := range m.params {
		out[i] = Param{Name: p.Name, Kind: p.Kind, Options: append([]string(nil), p.Options...)}
	}
	return out
}

// Compile mengurai satu pesan ICU. Sintaks yang didukung: argumen {nama}, plural (kategori CLDR,
// =N, #), select, tag rich <nama>...</nama>, dan kutip ICU. Pesan yang melebihi lim, bersintaks tidak
// sah, atau memakai konstruksi yang sengaja tidak didukung (selectordinal, number, date, time, offset)
// ditolak dengan *CompileError. Perilaku mengikuti intl-messageformat (FormatJS); perbedaannya (kunci
// plural bukan kategori CLDR, konstruksi tak didukung, batas ukuran) ditandai goOnly di fixture
// konformansi. Compile TIDAK menolak markup HTML: gunakan ValidateMessage untuk isi katalog.
func Compile(src string, lim Limits) (Message, error) {
	return compile(src, lim)
}

func compile(src string, lim Limits) (*message, error) {
	lim = lim.normalize()
	if !utf8.ValidString(src) {
		return nil, &CompileError{Code: CodeInvalidICU, Detail: "pesan bukan UTF-8 yang sah"}
	}
	if n := utf8.RuneCountInString(src); n > lim.MaxMessageLen {
		return nil, &CompileError{Code: CodeTooLarge, Detail: fmt.Sprintf("pesan %d karakter, batas %d", n, lim.MaxMessageLen)}
	}
	p := &parser{src: []rune(src), lim: lim, paramIdx: map[string]int{}, argNames: map[string]struct{}{}}
	nodes, err := p.parseMessage(0, false, false)
	if err != nil {
		return nil, err
	}
	return &message{nodes: nodes, params: p.params, htmlReason: p.html}, nil
}

// ValidateMessage mengompilasi src dan menerapkan aturan isi katalog (Requirement 2.4): pesan tidak
// boleh berisi HTML; satu-satunya `<` yang sah adalah tag rich bernama huruf kecil tanpa atribut
// (`<b>...</b>`, nama [a-z][a-z0-9]*) di luar nama elemen aktif (script, style, iframe, dst.) atau
// `<` yang dikutip ('<'). Galat berupa *CompileError, atau *CompileError berkode CodeHTMLNotAllowed.
func ValidateMessage(src string, lim Limits) (Message, error) {
	m, err := compile(src, lim)
	if err != nil {
		return nil, err
	}
	if m.htmlReason != "" {
		return nil, &CompileError{Code: CodeHTMLNotAllowed, Pos: 0, Detail: m.htmlReason}
	}
	return m, nil
}

// SameParamsStrict seperti SameParams tetapi jenis argumen harus SAMA PERSIS: KindArg dan KindPlural tidak
// saling menggantikan (argumen sederhana menerima string atau angka, plural hanya angka; mengubah yang pertama
// menjadi yang kedua membuat nilai string menjadi NaN). Options select tetap tidak dibandingkan. Dipakai
// validasi override (Requirement 12.3); SameParams longgar dipakai untuk paritas antar bahasa.
func SameParamsStrict(a, b []Param) bool { return sameParams(a, b, false) }

// SameParams melaporkan apakah dua daftar Param kompatibel: nama yang sama, dan jenis yang sama
// dengan satu kelonggaran: KindArg dan KindPlural saling menggantikan (keduanya menerima bilangan,
// mis. "{n} item" dan "{n, plural, ...}"). Options select tidak dibandingkan. Dipakai pemeriksa
// katalog dan validasi override agar tidak ada dua definisi "argumen sama".
func SameParams(a, b []Param) bool { return sameParams(a, b, true) }

func sameParams(a, b []Param, argPluralEquivalent bool) bool {
	norm := func(ps []Param) map[string]string {
		m := make(map[string]string, len(ps))
		for _, p := range ps {
			k := p.Kind
			if argPluralEquivalent && k == KindPlural {
				k = KindArg
			}
			m[p.Name+"\x00"+k.String()] = ""
		}
		return m
	}
	ma, mb := norm(a), norm(b)
	if len(ma) != len(mb) {
		return false
	}
	for k := range ma {
		if _, ok := mb[k]; !ok {
			return false
		}
	}
	return true
}

// htmlDenylist adalah nama elemen aktif yang tidak pernah sah sebagai tag rich walau nama
// sintaksnya valid, supaya `<script>` tidak lolos sebagai penanda teks kaya.
var htmlDenylist = map[string]struct{}{
	"script": {}, "style": {}, "iframe": {}, "frame": {}, "frameset": {}, "object": {}, "embed": {},
	"applet": {}, "link": {}, "meta": {}, "base": {}, "form": {}, "input": {}, "textarea": {},
	"button": {}, "select": {}, "option": {}, "img": {}, "picture": {}, "svg": {}, "math": {},
	"template": {}, "noscript": {}, "audio": {}, "video": {}, "source": {}, "track": {}, "canvas": {},
	"portal": {}, "slot": {}, "head": {}, "body": {}, "html": {}, "title": {},
}

// simpleTagName melaporkan nama tag rich yang sah: [a-z][a-z0-9]*.
func simpleTagName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// quoteForDetail memotong teks untuk Detail agar pesan tidak pernah dikutip utuh.
func quoteForDetail(s string) string {
	const max = 40
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return strings.ReplaceAll(s, "\n", " ")
}

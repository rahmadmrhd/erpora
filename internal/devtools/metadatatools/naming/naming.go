// Package naming berisi aturan penamaan berkas metadata (Requirement 1.4, 1.7, 1.8; K15; design Komponen 13).
//
// Semua fungsi murni (tanpa I/O). Berkas metadata memakai camelCase ketat: pola
// ^[a-z][a-z0-9]*([A-Z][a-z0-9]*)*$ tanpa huruf kapital berurutan, panjang paling banyak 63 byte. Dengan pola
// itu pemisahan kata deterministik: kata baru dimulai pada setiap huruf kapital, angka melekat pada kata
// sebelumnya. Paket ini mengonversi nama ke konvensi tiap target (SQL snake_case, Go PascalCase dengan
// inisialisme baku, URL kebab-case jamak) dan memeriksa kata tercadang, tabrakan, batas 63 byte, serta
// konversi bolak-balik yang identik.
//
// Kasus yang tidak tertangani aturan di sini sengaja ditolak dengan galat, bukan ditambal dengan opsi baru.
package naming

import (
	"fmt"
	"regexp"
	"strings"
)

// MaxIdentifierBytes adalah batas identifier PostgreSQL (NAMEDATALEN-1) yang dipakai untuk semua nama.
const MaxIdentifierBytes = 63

// MaxSlugBytes adalah batas panjang slug modul (module.schema.json: maxLength 50).
const MaxSlugBytes = 50

// Code adalah kode galat penamaan yang stabil (dipakai pelapor untuk membedakan pelanggaran).
type Code string

// Kode galat.
const (
	CodeNotCamelCase        Code = "not_camel_case"
	CodeConsecutiveCapitals Code = "consecutive_capitals"
	CodeTooLong             Code = "too_long"
	CodeReservedSQL         Code = "reserved_sql"
	CodeReservedGo          Code = "reserved_go"
	CodeReservedTS          Code = "reserved_typescript"
	CodeRoundTrip           Code = "round_trip"
	CodeCollision           Code = "collision"
	CodeNotSnakeCase        Code = "not_snake_case"
	CodeNotPascalCase       Code = "not_pascal_case"
	CodeInvalidSlug         Code = "invalid_slug"
	CodeInvalidKey          Code = "invalid_resource_key"
)

// Error adalah pelanggaran aturan penamaan.
type Error struct {
	Code   Code
	Name   string
	Detail string
}

func (e *Error) Error() string { return fmt.Sprintf("nama %q: %s", e.Name, e.Detail) }

func errf(code Code, name, format string, args ...any) *Error {
	return &Error{Code: code, Name: name, Detail: fmt.Sprintf(format, args...)}
}

// identifierPattern setara dengan pola Requirement 1.4 ditambah larangan huruf kapital berurutan: kata ke-2
// dan seterusnya wajib berekor huruf kecil atau angka, kecuali kata terakhir yang boleh berupa satu huruf
// kapital (mis. pointX). Pola longgar Requirement 1.4 saja menerima taxID.
const identifierPattern = `^[a-z][a-z0-9]*([A-Z][a-z0-9]+)*[A-Z]?$`

// loosePattern adalah pola camelCase longgar (menerima huruf kapital berurutan).
const loosePattern = `^[a-z][a-z0-9]*([A-Z][a-z0-9]*)*$`

var (
	identifierRe = regexp.MustCompile(identifierPattern)
	looseRe      = regexp.MustCompile(loosePattern)
	snakeRe      = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z][a-z0-9]*)*$`)
	kebabRe      = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)*$`)
	relationRe   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	slugRe       = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	keyRe        = regexp.MustCompile(`^([a-z][a-z0-9]*(?:-[a-z0-9]+)*)\.([a-z][a-zA-Z0-9]*)$`)
)

// IdentifierPattern mengembalikan pola regex identifier camelCase ketat (untuk JSON Schema dan pelapor).
func IdentifierPattern() string { return identifierPattern }

// IdentifierRegexp mengembalikan pola yang sudah dikompilasi.
func IdentifierRegexp() *regexp.Regexp { return identifierRe }

// shapeError memeriksa bentuk camelCase ketat saja (tanpa batas panjang).
func shapeError(s string) *Error {
	if identifierRe.MatchString(s) {
		return nil
	}
	if s == "" {
		return errf(CodeNotCamelCase, s, "tidak boleh kosong")
	}
	if looseRe.MatchString(s) {
		return errf(CodeConsecutiveCapitals, s,
			"huruf kapital berurutan tidak diizinkan; tulis akronim sebagai kata biasa (%s, bukan %s)", SuggestCamel(s), s)
	}
	return errf(CodeNotCamelCase, s,
		"harus camelCase ketat (huruf kecil di awal; hanya huruf ASCII dan angka; tanpa garis bawah, tanda hubung, atau spasi)")
}

// ValidateIdentifier memeriksa nama sebagai identifier camelCase ketat: bentuk dan panjang <= 63 byte.
func ValidateIdentifier(s string) error {
	if e := shapeError(s); e != nil {
		return e
	}
	if len(s) > MaxIdentifierBytes {
		return errf(CodeTooLong, s, "%d byte melebihi batas %d byte", len(s), MaxIdentifierBytes)
	}
	return nil
}

func isUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isLower(c byte) bool { return c >= 'a' && c <= 'z' }
func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// Words memecah identifier camelCase menjadi kata huruf kecil: kata baru dimulai pada tiap huruf kapital,
// angka melekat pada kata sebelumnya (address2Line menjadi address2, line). Hanya bentuk yang diperiksa;
// batas panjang diperiksa oleh fungsi konversi.
func Words(s string) ([]string, error) {
	if e := shapeError(s); e != nil {
		return nil, e
	}
	var words []string
	start := 0
	for i := 1; i < len(s); i++ {
		if isUpper(s[i]) {
			words = append(words, strings.ToLower(s[start:i]))
			start = i
		}
	}
	words = append(words, strings.ToLower(s[start:]))
	return words, nil
}

func checkLen(name string) error {
	if len(name) > MaxIdentifierBytes {
		return errf(CodeTooLong, name, "%d byte melebihi batas %d byte", len(name), MaxIdentifierBytes)
	}
	return nil
}

func joinWords(name string, sep string, words []string) (string, error) {
	if err := checkLen(name); err != nil {
		return "", err
	}
	out := strings.Join(words, sep)
	if err := checkLen(out); err != nil {
		return "", &Error{Code: CodeTooLong, Name: name, Detail: fmt.Sprintf("hasil konversi %q (%d byte) melebihi batas %d byte", out, len(out), MaxIdentifierBytes)}
	}
	return out, nil
}

// ToSnake mengonversi camelCase ke snake_case (SQL; kunci i18n tidak memakainya).
func ToSnake(name string) (string, error) {
	w, err := Words(name)
	if err != nil {
		return "", err
	}
	return joinWords(name, "_", w)
}

// ToKebab mengonversi camelCase ke kebab-case (segmen URL; tanpa jamak).
func ToKebab(name string) (string, error) {
	w, err := Words(name)
	if err != nil {
		return "", err
	}
	return joinWords(name, "-", w)
}

// initialisms adalah inisialisme baku Go yang ditulis kapital penuh pada PascalCase. Urutan tidak penting:
// pencocokan memakai yang terpanjang.
var initialisms = []string{"ID", "URL", "UUID", "URI", "API", "IP", "HTTP", "HTTPS", "HTML", "JSON", "XML", "SQL", "UI"}

var initialismSet = func() map[string]bool {
	m := make(map[string]bool, len(initialisms))
	for _, i := range initialisms {
		m[strings.ToLower(i)] = true
	}
	return m
}()

// ToPascal mengonversi camelCase ke PascalCase untuk Go (nama field struct dan tipe). Kata yang persis sama
// dengan inisialisme baku ditulis kapital penuh: customerId menjadi CustomerID, customerUrl menjadi CustomerURL.
func ToPascal(name string) (string, error) {
	w, err := Words(name)
	if err != nil {
		return "", err
	}
	if err := checkLen(name); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, word := range w {
		if initialismSet[word] {
			sb.WriteString(strings.ToUpper(word))
			continue
		}
		sb.WriteString(strings.ToUpper(word[:1]))
		sb.WriteString(word[1:])
	}
	return sb.String(), nil
}

func capitalize(w string) string { return strings.ToUpper(w[:1]) + w[1:] }

func camelFromWords(words []string) string {
	var sb strings.Builder
	for i, w := range words {
		if i == 0 {
			sb.WriteString(w)
			continue
		}
		sb.WriteString(capitalize(w))
	}
	return sb.String()
}

// FromSnake membalik ToSnake. Hasil harus identifier camelCase sah, bila tidak galat.
func FromSnake(s string) (string, error) {
	if !snakeRe.MatchString(s) {
		return "", errf(CodeNotSnakeCase, s, "bukan snake_case (huruf kecil dan angka dipisah garis bawah; tiap kata diawali huruf)")
	}
	out := camelFromWords(strings.Split(s, "_"))
	if err := ValidateIdentifier(out); err != nil {
		return "", err
	}
	return out, nil
}

// FromKebab membalik ToKebab.
func FromKebab(s string) (string, error) {
	if !kebabRe.MatchString(s) {
		return "", errf(CodeNotSnakeCase, s, "bukan kebab-case (huruf kecil dan angka dipisah tanda hubung; tiap kata diawali huruf)")
	}
	out := camelFromWords(strings.Split(s, "-"))
	if err := ValidateIdentifier(out); err != nil {
		return "", err
	}
	return out, nil
}

// initialismAt mengembalikan panjang inisialisme terpanjang pada s[i:], atau 0. Kandidat yang langsung
// diikuti huruf kecil atau angka ditolak karena huruf terakhirnya adalah awal kata berikutnya (HTTPServer
// dibaca HTTP + Server, bukan HTTPS + erver).
func initialismAt(s string, i int) int {
	best := 0
	for _, in := range initialisms {
		if len(in) <= best || !strings.HasPrefix(s[i:], in) {
			continue
		}
		if end := i + len(in); end < len(s) && (isLower(s[end]) || isDigit(s[end])) {
			continue
		}
		best = len(in)
	}
	return best
}

// FromPascal membalik ToPascal (pemisahan dengan pencocokan inisialisme terpanjang). Pemisahan ini bisa
// menyimpang dari nama asal untuk nama tertentu (mis. iD); itulah yang dideteksi CheckRoundTrip.
func FromPascal(s string) (string, error) {
	if s == "" || !isUpper(s[0]) {
		return "", errf(CodeNotPascalCase, s, "bukan PascalCase (harus diawali huruf kapital)")
	}
	var words []string
	for i := 0; i < len(s); {
		if !isUpper(s[i]) {
			return "", errf(CodeNotPascalCase, s, "bukan PascalCase pada posisi %d", i)
		}
		if n := initialismAt(s, i); n > 0 {
			words = append(words, strings.ToLower(s[i:i+n]))
			i += n
			continue
		}
		j := i + 1
		for j < len(s) && (isLower(s[j]) || isDigit(s[j])) {
			j++
		}
		words = append(words, strings.ToLower(s[i:j]))
		i = j
	}
	out := camelFromWords(words)
	if err := ValidateIdentifier(out); err != nil {
		return "", err
	}
	return out, nil
}

// CheckRoundTrip memastikan konversi bolak-balik identik untuk snake_case, kebab-case, dan PascalCase
// (Requirement 1.8). Nama yang gagal (mis. iD menjadi ID menjadi id) ditolak.
func CheckRoundTrip(name string) error {
	if err := ValidateIdentifier(name); err != nil {
		return err
	}
	type pair struct {
		target string
		to     func(string) (string, error)
		from   func(string) (string, error)
	}
	for _, p := range []pair{
		{"snake_case", ToSnake, FromSnake},
		{"kebab-case", ToKebab, FromKebab},
		{"PascalCase", ToPascal, FromPascal},
	} {
		conv, err := p.to(name)
		if err != nil {
			return err
		}
		back, err := p.from(conv)
		if err != nil || back != name {
			return errf(CodeRoundTrip, name, "konversi bolak-balik ke %s tidak identik (%q lalu %q); ubah penulisan kata", p.target, conv, back)
		}
	}
	return nil
}

// Violation adalah alias Error untuk hasil Validate.
type Violation = Error

// Validate menjalankan semua pemeriksaan satu identifier: bentuk, panjang, kata tercadang per target, dan
// konversi bolak-balik. Bila bentuk atau panjang salah, hanya pelanggaran itu yang dilaporkan.
func Validate(name string) []*Violation {
	if err := ValidateIdentifier(name); err != nil {
		var e *Error
		if asError(err, &e) {
			return []*Violation{e}
		}
	}
	var out []*Violation
	for _, t := range ReservedTargets(name) {
		out = append(out, reservedError(name, t))
	}
	if err := CheckRoundTrip(name); err != nil {
		var e *Error
		if asError(err, &e) {
			out = append(out, e)
		}
	}
	return out
}

func asError(err error, target **Error) bool {
	e, ok := err.(*Error) //nolint:errorlint // semua galat paket ini bertipe *Error tanpa pembungkus
	if ok {
		*target = e
	}
	return ok
}

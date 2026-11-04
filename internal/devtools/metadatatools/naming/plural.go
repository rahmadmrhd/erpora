package naming

import "strings"

// Aturan jamak nama tabel dan segmen URL (Requirement 1.8; tasks 2.4). Aturan eksplisit, bahasa Inggris
// sederhana, hanya pada kata terakhir nama resource (invoiceLine menjadi invoice_lines):
//
//  1. kata tak terhitung (uncountable) tetap;
//  2. kata tak beraturan (irregular) memakai tabel;
//  3. berakhir s, x, z, ch, atau sh: tambah "es";
//  4. berakhir konsonan + y: ganti "y" dengan "ies";
//  5. selain itu tambah "s" (termasuk kata berakhir angka dan vokal + y).
//
// Kata yang tidak tertangani (mis. berakhir f atau fe, atau kata berakhir o) mendapat "s" dan harus
// dilaporkan bila hasilnya salah; jangan menambal dengan opsi.
var uncountable = set(`data metadata information equipment series species news sheep fish money`)

var irregular = map[string]string{
	"person":    "people",
	"child":     "children",
	"man":       "men",
	"woman":     "women",
	"foot":      "feet",
	"tooth":     "teeth",
	"mouse":     "mice",
	"goose":     "geese",
	"datum":     "data",
	"criterion": "criteria",
	"analysis":  "analyses",
	"basis":     "bases",
	"matrix":    "matrices",
	"vertex":    "vertices",
	"medium":    "media",
}

func isVowel(c byte) bool {
	switch c {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	}
	return false
}

// Plural mengembalikan bentuk jamak satu kata huruf kecil.
func Plural(w string) string {
	if w == "" {
		return w
	}
	if uncountable[w] {
		return w
	}
	if p, ok := irregular[w]; ok {
		return p
	}
	switch {
	case strings.HasSuffix(w, "s"), strings.HasSuffix(w, "x"), strings.HasSuffix(w, "z"),
		strings.HasSuffix(w, "ch"), strings.HasSuffix(w, "sh"):
		return w + "es"
	case strings.HasSuffix(w, "y") && len(w) > 1 && !isVowel(w[len(w)-2]) && !isDigit(w[len(w)-2]):
		return w[:len(w)-1] + "ies"
	}
	return w + "s"
}

// pluralWords memecah name, menjamakkan kata terakhir, dan mengembalikan kata-katanya.
func pluralWords(name string) ([]string, error) {
	w, err := Words(name)
	if err != nil {
		return nil, err
	}
	w[len(w)-1] = Plural(w[len(w)-1])
	return w, nil
}

// PluralSnake mengonversi nama resource camelCase ke snake_case dengan kata terakhir jamak
// (invoiceLine menjadi invoice_lines).
func PluralSnake(name string) (string, error) {
	w, err := pluralWords(name)
	if err != nil {
		return "", err
	}
	return joinWords(name, "_", w)
}

// URLSegment mengonversi nama resource camelCase ke segmen URL kebab-case jamak (invoiceLine menjadi
// invoice-lines; api-guidelines §2).
func URLSegment(name string) (string, error) {
	w, err := pluralWords(name)
	if err != nil {
		return "", err
	}
	return joinWords(name, "-", w)
}

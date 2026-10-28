package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"golang.org/x/text/feature/plural"
	"golang.org/x/text/language"
	xmessage "golang.org/x/text/message"
	"golang.org/x/text/number"
)

// ErrUnsupportedArg dikembalikan Format untuk nilai argumen yang tipenya tidak didukung.
var ErrUnsupportedArg = errors.New("i18n: tipe nilai argumen tidak didukung")

// ErrOutputTooLarge dikembalikan Format bila keluaran melebihi MaxFormatOutput.
var ErrOutputTooLarge = errors.New("i18n: keluaran pesan melebihi batas")

// argString mengubah nilai argumen menjadi teks seperti String(nilai) di JavaScript (angka memakai
// representasi terpendek; eksponen untuk >= 1e21 dan < 1e-6), supaya `{n}` sama di Go dan TS.
// Nilai bool menjadi "true"/"false" (FormatJS mengubah false menjadi teks kosong dan true menjadi
// objek; Go sengaja memilih teks agar select pada bool bermakna).
func argString(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return x, nil
	case bool:
		return strconv.FormatBool(x), nil
	case json.Number:
		return x.String(), nil
	case int:
		return strconv.FormatInt(int64(x), 10), nil
	case int8:
		return strconv.FormatInt(int64(x), 10), nil
	case int16:
		return strconv.FormatInt(int64(x), 10), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint8:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint16:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint32:
		return strconv.FormatUint(uint64(x), 10), nil
	case uint64:
		return strconv.FormatUint(x, 10), nil
	case float64:
		return jsNumberString(x), nil
	case float32:
		f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(x), 'g', -1, 32), 64)
		return jsNumberString(f), nil
	case fmt.Stringer:
		return x.String(), nil
	}
	return "", ErrUnsupportedArg
}

// jsNumberString mengikuti Number.prototype.toString().
func jsNumberString(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	if a := math.Abs(f); a >= 1e21 || a < 1e-6 {
		s := strconv.FormatFloat(f, 'e', -1, 64) // mis. 1e+21, 2.5e-07
		mant, exp, _ := strings.Cut(s, "e")
		digits := strings.TrimLeft(exp[1:], "0")
		return mant + "e" + exp[:1] + digits
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// argNumber mengubah nilai argumen menjadi bilangan seperti `nilai - 0` di JavaScript: string
// dikonversi seperti Number(), bool menjadi 0/1, dan teks yang bukan bilangan menjadi NaN.
func argNumber(v any) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(x), 'g', -1, 32), 64)
		return f, nil
	case int:
		return float64(x), nil
	case int8:
		return float64(x), nil
	case int16:
		return float64(x), nil
	case int32:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case uint:
		return float64(x), nil
	case uint8:
		return float64(x), nil
	case uint16:
		return float64(x), nil
	case uint32:
		return float64(x), nil
	case uint64:
		return float64(x), nil
	case bool:
		if x {
			return 1, nil
		}
		return 0, nil
	}
	s, err := argString(v)
	if err != nil {
		return 0, err
	}
	return jsToNumber(s), nil
}

// jsToNumber mengikuti Number(string) untuk bentuk desimal dan eksponen (heksadesimal, biner, dan
// oktal tidak didukung dan menjadi NaN).
func jsToNumber(s string) float64 {
	t := strings.TrimSpace(s)
	switch t {
	case "":
		return 0
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if !isDecimalLiteral(t) {
		return math.NaN()
	}
	f, _ := strconv.ParseFloat(t, 64) // galat rentang tetap mengembalikan +-Inf
	return f
}

func isDecimalLiteral(t string) bool {
	i := 0
	if i < len(t) && (t[i] == '+' || t[i] == '-') {
		i++
	}
	digits := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
		digits++
	}
	if i < len(t) && t[i] == '.' {
		i++
		for i < len(t) && t[i] >= '0' && t[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return false
	}
	if i < len(t) && (t[i] == 'e' || t[i] == 'E') {
		i++
		if i < len(t) && (t[i] == '+' || t[i] == '-') {
			i++
		}
		exp := 0
		for i < len(t) && t[i] >= '0' && t[i] <= '9' {
			i++
			exp++
		}
		if exp == 0 {
			return false
		}
	}
	return i == len(t)
}

// decimal adalah bilangan yang sudah dibulatkan seperti Intl.NumberFormat/PluralRules bawaan: paling
// banyak 3 digit pecahan, half-expand pada representasi desimal terpendek, tanpa nol di belakang.
type decimal struct {
	neg      bool
	nan, inf bool
	ip, fp   string // bagian bulat (tanpa nol di depan) dan pecahan (tanpa nol di belakang)
}

const maxFractionDigits = 3

func newDecimal(f float64) decimal {
	switch {
	case math.IsNaN(f):
		return decimal{nan: true}
	case math.IsInf(f, 0):
		return decimal{inf: true, neg: f < 0}
	}
	d := decimal{neg: math.Signbit(f)}
	s := strconv.FormatFloat(math.Abs(f), 'f', -1, 64)
	d.ip, d.fp, _ = strings.Cut(s, ".")
	if len(d.fp) > maxFractionDigits {
		up := d.fp[maxFractionDigits] >= '5'
		d.fp = d.fp[:maxFractionDigits]
		if up {
			digits := []byte(d.ip + d.fp)
			i := len(digits) - 1
			for ; i >= 0; i-- {
				if digits[i] == '9' {
					digits[i] = '0'
					continue
				}
				digits[i]++
				break
			}
			if i < 0 {
				digits = append([]byte{'1'}, digits...)
			}
			n := len(digits) - len(d.fp)
			d.ip, d.fp = string(digits[:n]), string(digits[n:])
		}
	}
	d.fp = strings.TrimRight(d.fp, "0")
	return d
}

// form menentukan kategori plural kardinal CLDR untuk locale; NaN dan tak hingga selalu other.
func (d decimal) form(tag language.Tag) plural.Form {
	if d.nan || d.inf {
		return plural.Other
	}
	i, err := strconv.Atoi(d.ip)
	if err != nil {
		i = math.MaxInt32 // terlalu besar untuk operan; aturan CLDR bergantung pada sisa bagi
	}
	f, _ := strconv.Atoi(d.fp)
	v := len(d.fp)
	return plural.Cardinal.MatchPlural(tag, i, v, v, f, f)
}

// format menulis bilangan sesuai locale (pemisah ribuan dan desimal) lewat x/text/number.
func (d decimal) format(tag language.Tag) string {
	switch {
	case d.nan:
		return "NaN"
	case d.inf && d.neg:
		return "-∞"
	case d.inf:
		return "∞"
	}
	lit := d.ip
	if d.fp != "" {
		lit += "." + d.fp
	}
	x, _ := strconv.ParseFloat(lit, 64)
	if d.neg {
		x = math.Copysign(x, -1)
	}
	return xmessage.NewPrinter(tag).Sprint(number.Decimal(x, number.MaxFractionDigits(maxFractionDigits)))
}

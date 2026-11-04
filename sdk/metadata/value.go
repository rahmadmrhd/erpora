package metadata

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// kind adalah keluarga nilai; dua nilai hanya dapat dibandingkan bila keluarganya sama
// (integer dan decimal sama-sama kNum).
type kind uint8

const (
	kNull kind = iota
	kBool
	kNum
	kText
	kEnum
	kUUID
	kDate
	kDateTime
)

// value adalah nilai bertipe hasil koersi nilai record, literal, atau hasil ekspresi.
type value struct {
	k     kind
	ft    FieldType // tipe terdeklarasi (untuk tipe hasil ekspresi)
	b     bool
	num   Decimal
	isInt bool
	s     string // teks, enum, uuid, serta bentuk kanonik date dan datetime
	day   int64  // date: hari sejak 1970-01-01
	sec   int64  // datetime: detik sejak epoch
	nsec  int64  // datetime: nanodetik
}

func nullValue() value { return value{k: kNull, ft: TypeNull} }

const maxSafeInt = 1<<53 - 1

func familyOf(ft FieldType) (kind, bool) {
	switch ft {
	case TypeString, TypeText:
		return kText, true
	case TypeInteger, TypeDecimal:
		return kNum, true
	case TypeBoolean:
		return kBool, true
	case TypeDate:
		return kDate, true
	case TypeDateTime:
		return kDateTime, true
	case TypeEnum:
		return kEnum, true
	case TypeUUID:
		return kUUID, true
	}
	return kNull, false
}

// tooLong melaporkan apakah s melebihi MaxStringLen satuan UTF-16 (byte UTF-8 tidak sah dihitung 1,
// setara satu U+FFFD).
func tooLong(s string) bool {
	if len(s) <= MaxStringLen {
		return false // UTF-16 tidak pernah lebih panjang daripada byte UTF-8
	}
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n > MaxStringLen
}

// normText mengganti byte UTF-8 tidak sah dengan U+FFFD per byte (setara pemetaan surrogat tunggal di TS).
func normText(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var sb strings.Builder
	for _, r := range s {
		sb.WriteRune(r)
	}
	return sb.String()
}

// toSafeInt menerima bilangan bulat aman dari tipe numerik Go atau json.Number (lewat float64, sama seperti
// JSON.parse di JavaScript).
func toSafeInt(raw any) (int64, bool) {
	safe := func(f float64) (int64, bool) {
		if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || math.Abs(f) > maxSafeInt {
			return 0, false
		}
		return int64(f), true
	}
	switch v := raw.(type) {
	case json.Number:
		f, ok := parseJSONNumber(v)
		if !ok {
			return 0, false
		}
		return safe(f)
	case float64:
		return safe(v)
	case float32:
		return safe(float64(v))
	case int:
		return safeInt64(int64(v))
	case int8:
		return int64(v), true
	case int16:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return safeInt64(v)
	case uint:
		return safeUint64(uint64(v))
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		return safeUint64(v)
	}
	return 0, false
}

// jsonNumberRE adalah tata bahasa angka JSON (RFC 8259); json.Number adalah tipe string sehingga isinya dapat
// berupa apa saja ("NaN", "Infinity", "0x1p3", "1_0", "+1"), padahal JSON.parse di klien tidak pernah
// menghasilkannya.
var jsonNumberRE = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

// parseJSONNumber mengurai json.Number yang sah dan berhingga menjadi float64 (sama dengan JSON.parse).
func parseJSONNumber(n json.Number) (float64, bool) {
	if !jsonNumberRE.MatchString(string(n)) {
		return 0, false
	}
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func safeInt64(v int64) (int64, bool) {
	if v > maxSafeInt || v < -maxSafeInt {
		return 0, false
	}
	return v, true
}

func safeUint64(v uint64) (int64, bool) {
	if v > maxSafeInt {
		return 0, false
	}
	return int64(v), true
}

func isDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func isLeap(y int) bool { return y%4 == 0 && (y%100 != 0 || y%400 == 0) }

var monthDays = [...]int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}

// parseDate memvalidasi YYYY-MM-DD (tahun 0001..9999, kalender Gregorian) dan mengembalikan hari sejak epoch.
func parseDate(s string) (int64, bool) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' || !isDigits(s[:4]) || !isDigits(s[5:7]) || !isDigits(s[8:]) {
		return 0, false
	}
	y, m, d := atoi(s[:4]), atoi(s[5:7]), atoi(s[8:])
	if y < 1 || m < 1 || m > 12 || d < 1 {
		return 0, false
	}
	dim := monthDays[m-1]
	if m == 2 && isLeap(y) {
		dim = 29
	}
	if d > dim {
		return 0, false
	}
	return daysFromCivil(y, m, d), true
}

// daysFromCivil adalah algoritma days_from_civil (Howard Hinnant); tahun dibatasi 1..9999 oleh pemanggil.
func daysFromCivil(y, m, d int) int64 {
	if m <= 2 {
		y--
	}
	era := y / 400
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return int64(era)*146097 + int64(doe) - 719468
}

// parseDateTime memvalidasi YYYY-MM-DDTHH:MM:SS[.d{1,9}]Z (UTC) dan mengembalikan detik dan nanodetik.
func parseDateTime(s string) (sec, nsec int64, ok bool) {
	if len(s) < 20 || s[len(s)-1] != 'Z' || s[10] != 'T' {
		return 0, 0, false
	}
	day, dok := parseDate(s[:10])
	if !dok {
		return 0, 0, false
	}
	t := s[11 : len(s)-1]
	if len(t) < 8 || t[2] != ':' || t[5] != ':' || !isDigits(t[:2]) || !isDigits(t[3:5]) || !isDigits(t[6:8]) {
		return 0, 0, false
	}
	h, mi, se := atoi(t[:2]), atoi(t[3:5]), atoi(t[6:8])
	if h > 23 || mi > 59 || se > 59 {
		return 0, 0, false
	}
	var ns int64
	if len(t) > 8 {
		if t[8] != '.' {
			return 0, 0, false
		}
		frac := t[9:]
		if len(frac) < 1 || len(frac) > 9 || !isDigits(frac) {
			return 0, 0, false
		}
		ns = int64(atoi(frac + strings.Repeat("0", 9-len(frac))))
	}
	return day*86400 + int64(h*3600+mi*60+se), ns, true
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i := 0; i < 36; i++ {
		c := s[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
			continue
		}
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// coerce mengubah nilai mentah menjadi value menurut tipe terdeklarasi. Mengembalikan kode alasan bila
// nilai tidak sah ("" bila sah).
func coerce(ft FieldType, raw any) (value, string) {
	fam, known := familyOf(ft)
	if !known {
		return value{}, ReasonTypeMismatch
	}
	if raw == nil {
		return nullValue(), ""
	}
	v := value{k: fam, ft: ft}
	switch ft {
	case TypeString, TypeText, TypeEnum:
		s, ok := raw.(string)
		if !ok {
			return value{}, ReasonTypeMismatch
		}
		if tooLong(s) {
			return value{}, ReasonValueTooLong
		}
		v.s = normText(s)
	case TypeUUID:
		s, ok := raw.(string)
		if !ok || !isUUID(s) {
			return value{}, ReasonTypeMismatch
		}
		v.s = s
	case TypeInteger:
		n, ok := toSafeInt(raw)
		if !ok {
			return value{}, ReasonTypeMismatch
		}
		v.num, v.isInt = DecimalFromInt(n), true
	case TypeDecimal:
		switch x := raw.(type) {
		case string:
			d, ok := ParseDecimal(x)
			if !ok {
				return value{}, ReasonTypeMismatch
			}
			v.num = d
		case Decimal:
			v.num = x
		default:
			return value{}, ReasonTypeMismatch
		}
	case TypeBoolean:
		b, ok := raw.(bool)
		if !ok {
			return value{}, ReasonTypeMismatch
		}
		v.b = b
	case TypeDate:
		s, ok := raw.(string)
		if !ok {
			return value{}, ReasonTypeMismatch
		}
		day, ok := parseDate(s)
		if !ok {
			return value{}, ReasonTypeMismatch
		}
		v.s, v.day = s, day
	case TypeDateTime:
		s, ok := raw.(string)
		if !ok {
			return value{}, ReasonTypeMismatch
		}
		sec, ns, ok := parseDateTime(s)
		if !ok {
			return value{}, ReasonTypeMismatch
		}
		v.s, v.sec, v.nsec = s, sec, ns
	}
	return v, ""
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// cmpValues membandingkan dua nilai bukan-null dari keluarga yang sama.
func cmpValues(a, b value) int {
	switch a.k {
	case kNum:
		return a.num.Cmp(b.num)
	case kBool:
		switch {
		case a.b == b.b:
			return 0
		case !a.b:
			return -1
		}
		return 1
	case kDate:
		return cmpInt64(a.day, b.day)
	case kDateTime:
		if c := cmpInt64(a.sec, b.sec); c != 0 {
			return c
		}
		return cmpInt64(a.nsec, b.nsec)
	}
	// teks, enum, uuid: urutan byte UTF-8 = urutan code point (string sudah dinormalkan).
	return strings.Compare(a.s, b.s)
}

// asciiFold mengubah hanya huruf ASCII A-Z menjadi huruf kecil (paritas deterministik dengan klien).
func asciiFold(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if b[j] >= 'A' && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// valueText menulis nilai sebagai teks kanonik (untuk concat dan templat label); null menjadi "".
func valueText(v value) string {
	switch v.k {
	case kBool:
		return strconv.FormatBool(v.b)
	case kNum:
		if v.isInt {
			return strconv.FormatInt(v.num.IntPart(), 10)
		}
		return v.num.String()
	case kText, kEnum, kUUID, kDate, kDateTime:
		return v.s
	}
	return ""
}

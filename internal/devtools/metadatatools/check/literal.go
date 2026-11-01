package check

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"erpora/sdk/metadata"
)

var reUUID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const maxSafeInt = 1<<53 - 1

// constraints adalah batasan field yang relevan untuk memvalidasi literal.
type constraints struct {
	typ        metadata.FieldType
	nullable   bool
	minLength  int
	maxLength  int
	hasMinLen  bool
	hasMaxLen  bool
	pattern    string
	precision  int
	scale      int
	hasPrec    bool
	minimum    string // teks asli (integer atau desimal)
	maximum    string
	enumValues []string
	hasEnum    bool
}

func numStr(v any) (string, bool) {
	switch n := v.(type) {
	case json.Number:
		return n.String(), true
	case string:
		return n, true
	}
	return "", false
}

func atoiAttr(v any) (int, bool) {
	s, ok := numStr(v)
	if !ok {
		return 0, false
	}
	i, err := strconv.Atoi(s)
	return i, err == nil
}

func isInteger(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := strconv.ParseInt(n.String(), 10, 64)
	if err != nil || i > maxSafeInt || i < -maxSafeInt {
		return 0, false
	}
	return i, true
}

// checkLiteral memvalidasi literal terhadap tipe dan batasan field; mengembalikan pesan galat atau "".
func (c constraints) checkLiteral(v any) string {
	if v == nil {
		if !c.nullable {
			return "nilai null hanya sah untuk field nullable"
		}
		return ""
	}
	switch c.typ {
	case metadata.TypeString, metadata.TypeText:
		s, ok := v.(string)
		if !ok {
			return "harus teks"
		}
		n := utf8.RuneCountInString(s)
		if c.hasMaxLen && n > c.maxLength {
			return fmt.Sprintf("panjang %d melebihi maxLength %d", n, c.maxLength)
		}
		if c.hasMinLen && n < c.minLength {
			return fmt.Sprintf("panjang %d kurang dari minLength %d", n, c.minLength)
		}
		if c.pattern != "" {
			if p, err := metadata.CompilePattern(c.pattern); err == nil && !p.Match(s) {
				return "tidak cocok dengan pattern field"
			}
		}
	case metadata.TypeInteger:
		i, ok := isInteger(v)
		if !ok {
			return "harus bilangan bulat aman (tanpa pecahan)"
		}
		if c.minimum != "" {
			if m, err := strconv.ParseInt(c.minimum, 10, 64); err == nil && i < m {
				return fmt.Sprintf("kurang dari minimum %s", c.minimum)
			}
		}
		if c.maximum != "" {
			if m, err := strconv.ParseInt(c.maximum, 10, 64); err == nil && i > m {
				return fmt.Sprintf("melebihi maximum %s", c.maximum)
			}
		}
	case metadata.TypeDecimal:
		s, ok := v.(string)
		if !ok {
			return "desimal harus ditulis sebagai teks (\"1.50\"), bukan bilangan"
		}
		d, ok := metadata.ParseDecimal(s)
		if !ok {
			return "bukan desimal yang sah"
		}
		digits := strings.TrimPrefix(s, "-")
		intPart, frac, _ := strings.Cut(digits, ".")
		intPart = strings.TrimLeft(intPart, "0")
		if c.hasPrec {
			if len(frac) > c.scale {
				return fmt.Sprintf("skala %d melebihi scale %d", len(frac), c.scale)
			}
			if len(intPart) > c.precision-c.scale {
				return fmt.Sprintf("%d digit bulat melebihi precision %d dan scale %d", len(intPart), c.precision, c.scale)
			}
		}
		if c.minimum != "" {
			if m, ok := metadata.ParseDecimal(c.minimum); ok && d.Cmp(m) < 0 {
				return fmt.Sprintf("kurang dari minimum %s", c.minimum)
			}
		}
		if c.maximum != "" {
			if m, ok := metadata.ParseDecimal(c.maximum); ok && d.Cmp(m) > 0 {
				return fmt.Sprintf("melebihi maximum %s", c.maximum)
			}
		}
	case metadata.TypeBoolean:
		if _, ok := v.(bool); !ok {
			return "harus boolean (true atau false)"
		}
	case metadata.TypeDate:
		s, ok := v.(string)
		if !ok || len(s) != 10 {
			return "harus tanggal YYYY-MM-DD"
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return "bukan tanggal kalender yang sah (YYYY-MM-DD)"
		}
	case metadata.TypeDateTime:
		s, ok := v.(string)
		if !ok || !strings.HasSuffix(s, "Z") {
			return "harus datetime RFC 3339 UTC berakhiran Z"
		}
		if _, err := time.Parse(time.RFC3339Nano, s); err != nil {
			return "bukan datetime RFC 3339 yang sah"
		}
	case metadata.TypeUUID:
		s, ok := v.(string)
		if !ok || !reUUID.MatchString(s) {
			return "harus UUID huruf kecil kanonik"
		}
	case metadata.TypeEnum:
		s, ok := v.(string)
		if !ok {
			return "harus teks anggota values"
		}
		if c.hasEnum {
			found := false
			for _, e := range c.enumValues {
				if e == s {
					found = true
					break
				}
			}
			if !found {
				return fmt.Sprintf("%q bukan anggota values (%s)", s, strings.Join(c.enumValues, ", "))
			}
		}
	default:
		return ""
	}
	return ""
}

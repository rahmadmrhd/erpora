package migrate

import (
	"regexp"
	"strconv"
	"strings"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/sdk/metadata"
)

// isLoosening menentukan apakah mengganti CHECK lama (definisi hasil pg_get_constraintdef) dengan CHECK model c
// hanya melonggarkan batasan (Requirement 11.2: pelonggaran batasan dan nilai enum extensible baru). Hanya
// bentuk yang dibangkitkan pembangkit ini yang dikenali; definisi lain dianggap tidak dikenali dan hasilnya
// false (kerangka TODO), tidak pernah ditebak.
func isLoosening(c *dbmodel.Check, oldDef string) bool {
	switch c.Kind {
	case dbmodel.CheckEnum:
		if !c.Extensible {
			return false
		}
		old, ok := parseEnum(oldDef, c.Column)
		if !ok {
			return false
		}
		in := make(map[string]bool, len(c.Values))
		for _, v := range c.Values {
			in[v] = true
		}
		for _, v := range old {
			if !in[v] {
				return false
			}
		}
		return true
	case dbmodel.CheckLength:
		b, ok := parseBounds(oldDef, c.Column, true)
		if !ok {
			return false
		}
		return looser(c, b, func(a, b string) int {
			x, _ := strconv.ParseInt(a, 10, 64)
			y, _ := strconv.ParseInt(b, 10, 64)
			switch {
			case x < y:
				return -1
			case x > y:
				return 1
			}
			return 0
		})
	case dbmodel.CheckRange:
		b, ok := parseBounds(oldDef, c.Column, false)
		if !ok {
			return false
		}
		return looser(c, b, func(a, b string) int {
			x, ok1 := metadata.ParseDecimal(a)
			y, ok2 := metadata.ParseDecimal(b)
			if !ok1 || !ok2 {
				return 2 // tak terbandingkan
			}
			return x.Cmp(y)
		})
	}
	return false
}

type bounds struct {
	min, max       string
	hasMin, hasMax bool
}

// looser: batas baru tidak lebih ketat dari batas lama pada kedua sisi.
func looser(c *dbmodel.Check, old bounds, cmp func(a, b string) int) bool {
	if c.Min != "" {
		if !old.hasMin {
			return false
		}
		if r := cmp(c.Min, old.min); r > 0 {
			return false
		}
	}
	if c.Max != "" {
		if !old.hasMax {
			return false
		}
		if r := cmp(c.Max, old.max); r < 0 || r == 2 {
			return false
		}
	}
	return true
}

var (
	enumLitRe  = regexp.MustCompile(`'((?:[^']|'')*)'::text`)
	castRe     = regexp.MustCompile(`::(?:text|numeric|bigint|integer)`)
	keywordsRe = regexp.MustCompile(`\b(?:CHECK|AND|ANY|ARRAY)\b`)
)

func colRe(column string) string { return `"?` + regexp.QuoteMeta(column) + `"?` }

// parseEnum membaca daftar nilai dari CHECK (kolom = ANY (ARRAY['a'::text, ...])); ok false bila bentuknya lain.
func parseEnum(def, column string) ([]string, bool) {
	if !strings.Contains(def, "= ANY (ARRAY[") {
		return nil, false
	}
	var vals []string
	for _, m := range enumLitRe.FindAllStringSubmatch(def, -1) {
		vals = append(vals, strings.ReplaceAll(m[1], "''", "'"))
	}
	if len(vals) == 0 {
		return nil, false
	}
	rest := enumLitRe.ReplaceAllString(def, "")
	rest = keywordsRe.ReplaceAllString(castRe.ReplaceAllString(rest, ""), "")
	rest = regexp.MustCompile(colRe(column)).ReplaceAllString(rest, "")
	rest = strings.Map(func(r rune) rune {
		if strings.ContainsRune("=()[], \t\n", r) {
			return -1
		}
		return r
	}, rest)
	return vals, rest == ""
}

// parseBounds membaca batas bawah dan atas dari CHECK length(kolom) >= n AND length(kolom) <= m (length true)
// atau kolom >= x AND kolom <= y; ok false bila ada klausa lain atau batas ganda.
func parseBounds(def, column string, length bool) (bounds, bool) {
	var re *regexp.Regexp
	c := colRe(column)
	if length {
		re = regexp.MustCompile(`length\(\(?` + c + `\)?(?:::text)?\) (>=|<=) (\d+)`)
	} else {
		re = regexp.MustCompile(`\(?\(?` + c + `\)?(?:::(?:numeric|bigint|integer))? (>=|<=) \(?(-?\d+(?:\.\d+)?)\)?(?:::(?:numeric|bigint|integer))?`)
	}
	var b bounds
	for _, m := range re.FindAllStringSubmatch(def, -1) {
		switch m[1] {
		case ">=":
			if b.hasMin {
				return bounds{}, false
			}
			b.min, b.hasMin = m[2], true
		default:
			if b.hasMax {
				return bounds{}, false
			}
			b.max, b.hasMax = m[2], true
		}
	}
	if !b.hasMin && !b.hasMax {
		return bounds{}, false
	}
	rest := re.ReplaceAllString(def, "")
	rest = keywordsRe.ReplaceAllString(castRe.ReplaceAllString(rest, ""), "")
	rest = strings.Map(func(r rune) rune {
		if r == '(' || r == ')' || r == ' ' || r == '\t' || r == '\n' {
			return -1
		}
		return r
	}, rest)
	return b, rest == ""
}

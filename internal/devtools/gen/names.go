package gen

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"erpora/internal/platform/modules/registry"
)

var (
	slugRe      = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	typeNameRe  = regexp.MustCompile(`^[A-Z][A-Za-z0-9]{0,63}$`)
	snakeNameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
)

// ModuleName adalah turunan nama modul (keputusan rencana 13): Dir, nama package, dan akhiran module path memakai
// slug tanpa "-"; tabel dan goose memakai Snake.
type ModuleName struct {
	Slug, Dir, Key, Snake, Camel, Pascal, NPM, GoPath string
	Publisher, Title                                  string
}

// ParseModuleName memvalidasi slug (kebab-case, 2..40 karakter, bukan slug cadangan registry) dan publisher.
func ParseModuleName(arg, publisher string) (ModuleName, error) {
	if len(arg) < 2 || len(arg) > 40 || !slugRe.MatchString(arg) {
		return ModuleName{}, fmt.Errorf("nama modul %q tidak valid: kebab-case huruf kecil (a-z, 0-9, tanda hubung), 2 sampai 40 karakter, diawali huruf", arg)
	}
	if registry.IsReservedSlug(arg) {
		return ModuleName{}, fmt.Errorf("nama modul %q dicadangkan untuk platform", arg)
	}
	if !slugRe.MatchString(publisher) {
		return ModuleName{}, fmt.Errorf("publisher %q tidak valid: kebab-case huruf kecil", publisher)
	}
	words := strings.Split(arg, "-")
	var pascal, title strings.Builder
	for i, w := range words {
		cap := strings.ToUpper(w[:1]) + w[1:]
		pascal.WriteString(cap)
		if i > 0 {
			title.WriteByte(' ')
		}
		title.WriteString(cap)
	}
	dir := strings.ReplaceAll(arg, "-", "")
	p := pascal.String()
	return ModuleName{
		Slug: arg, Dir: dir, Key: publisher + "." + arg, Snake: strings.ReplaceAll(arg, "-", "_"),
		Camel: strings.ToLower(p[:1]) + p[1:], Pascal: p, NPM: "@erpora/module-" + dir, GoPath: "erpora/modules/" + dir,
		Publisher: publisher, Title: title.String(),
	}, nil
}

// TypeName adalah turunan nama tipe PascalCase.
type TypeName struct{ Pascal, Camel, Snake, Kebab, PluralKebab, PluralSnake string }

// ParseTypeName memvalidasi nama tipe (^[A-Z][A-Za-z0-9]{0,63}$) dan menurunkan bentuk lain beserta plural
// (+s; konsonan+y -> ies; s|x|z|ch|sh -> es).
func ParseTypeName(arg string) (TypeName, error) {
	if !typeNameRe.MatchString(arg) {
		return TypeName{}, fmt.Errorf("nama tipe %q tidak valid: PascalCase, huruf dan angka, diawali huruf besar", arg)
	}
	snake := toSnake(arg)
	kebab := strings.ReplaceAll(snake, "_", "-")
	return TypeName{
		Pascal: arg, Camel: strings.ToLower(arg[:1]) + arg[1:], Snake: snake, Kebab: kebab,
		PluralKebab: pluralize(kebab), PluralSnake: pluralize(snake),
	}, nil
}

// ParseSnakeName memvalidasi nama snake_case (^[a-z][a-z0-9_]{0,62}$).
func ParseSnakeName(arg string) (string, error) {
	if !snakeNameRe.MatchString(arg) {
		return "", fmt.Errorf("nama %q tidak valid: snake_case huruf kecil, angka, dan garis bawah, diawali huruf", arg)
	}
	return arg, nil
}

func toSnake(pascal string) string {
	rs := []rune(pascal)
	var b strings.Builder
	for i, r := range rs {
		if unicode.IsUpper(r) && i > 0 {
			prev := rs[i-1]
			nextLower := i+1 < len(rs) && unicode.IsLower(rs[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextLower) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func pluralize(s string) string {
	switch {
	case strings.HasSuffix(s, "y") && len(s) > 1 && !strings.ContainsRune("aeiou", rune(s[len(s)-2])):
		return s[:len(s)-1] + "ies"
	case strings.HasSuffix(s, "s"), strings.HasSuffix(s, "x"), strings.HasSuffix(s, "z"),
		strings.HasSuffix(s, "ch"), strings.HasSuffix(s, "sh"):
		return s + "es"
	}
	return s + "s"
}

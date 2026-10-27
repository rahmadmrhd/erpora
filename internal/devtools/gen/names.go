package gen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"regexp"
	"strconv"
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
	// Dir (slug tanpa "-") menjadi nama package Go sekaligus alias impor di internal/app/modules.go.
	if dir := strings.ReplaceAll(arg, "-", ""); token.IsKeyword(dir) || dir == "main" || types.Universe.Lookup(dir) != nil {
		return ModuleName{}, fmt.Errorf("nama modul %q menghasilkan nama package Go %q yang tidak dapat dipakai (keyword, main, atau identifier predeclared)", arg, dir)
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

// CheckModuleDir memastikan dir (nama package Go modul baru) tidak bentrok dengan identifier mana pun yang sudah ada
// di internal/app/modules.go (variabel lokal seperti ex, alias impor, atau nama paket yang diimpor): modul baru diimpor
// sebagai paket bernama dir, sehingga bentrokan membuat file itu tidak dapat dikompilasi.
func CheckModuleDir(dir string, modulesGo []byte) error {
	f, err := parser.ParseFile(token.NewFileSet(), "modules.go", modulesGo, 0)
	if err != nil {
		return fmt.Errorf("internal/app/modules.go tidak terbaca: %w", err)
	}
	used := map[string]bool{}
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		used[path.Base(p)] = true
		if imp.Name != nil {
			used[imp.Name.Name] = true
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			used[id.Name] = true
		}
		return true
	})
	if used[dir] {
		return fmt.Errorf("nama package %q bentrok dengan identifier yang sudah dipakai di internal/app/modules.go; pilih nama modul lain", dir)
	}
	return nil
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
	if err := CheckGoFileStem(snake); err != nil {
		return TypeName{}, fmt.Errorf("nama tipe %q: %w", arg, err)
	}
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

// Daftar GOOS dan GOARCH yang dikenal go/build (go/build.knownOS/knownArch tidak diekspor). Nama berkas Go
// berakhiran _<GOOS>, _<GOARCH>, atau _<GOOS>_<GOARCH> dikecualikan dari build di platform lain.
var (
	knownOS = map[string]bool{
		"aix": true, "android": true, "darwin": true, "dragonfly": true, "freebsd": true, "hurd": true, "illumos": true,
		"ios": true, "js": true, "linux": true, "nacl": true, "netbsd": true, "openbsd": true, "plan9": true,
		"solaris": true, "wasip1": true, "windows": true, "zos": true,
	}
	knownArch = map[string]bool{
		"386": true, "amd64": true, "amd64p32": true, "arm": true, "armbe": true, "arm64": true, "arm64be": true,
		"loong64": true, "mips": true, "mipsle": true, "mips64": true, "mips64le": true, "mips64p32": true,
		"mips64p32le": true, "ppc": true, "ppc64": true, "ppc64le": true, "riscv": true, "riscv64": true,
		"s390": true, "s390x": true, "sparc": true, "sparc64": true, "wasm": true,
	}
)

// CheckGoFileStem menolak stem nama berkas Go (tanpa .go) yang bermakna khusus bagi toolchain: akhiran _test
// (berkas test), _<GOOS>, atau _<GOARCH> (build constraint implisit). Segmen pertama tidak dihitung, seperti go/build
// (stem "linux" atau "test" tanpa garis bawah aman).
func CheckGoFileStem(stem string) error {
	parts := strings.Split(stem, "_")
	if len(parts) < 2 {
		return nil
	}
	last := parts[len(parts)-1]
	switch {
	case last == "test":
		return fmt.Errorf("nama menghasilkan berkas %s.go yang dianggap berkas test oleh Go (akhiran _test); pilih nama lain", stem)
	case knownOS[last] || knownArch[last]:
		return fmt.Errorf("nama menghasilkan berkas %s.go dengan build constraint implisit (akhiran _%s); pilih nama lain", stem, last)
	}
	return nil
}

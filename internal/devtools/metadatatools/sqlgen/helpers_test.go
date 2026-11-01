package sqlgen

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"erpora/internal/devtools/metadatatools/dbmodel"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
)

var update = flag.Bool("update", false, "tulis ulang berkas golden")

// golden membandingkan got dengan testdata/<name>; dengan -update menulis ulang.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path) //nolint:gosec // golden uji
	if err != nil {
		t.Fatalf("golden %s: %v (jalankan dengan -update)", path, err)
	}
	if string(want) != got {
		t.Errorf("golden %s berbeda.\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// fixtureSchema menurunkan model dari modul fixture (testdata/repo) hanya-baca.
func fixtureSchema(t *testing.T) *dbmodel.Schema {
	t.Helper()
	c, ps := resolve.NewCatalogWithBuiltin()
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	root := os.DirFS("../testdata/repo/modules")
	for _, m := range []struct {
		mod  resolve.Module
		path string
	}{
		{resolve.Module{Slug: "fixturecore"}, "fixturecore/metadata"},
		{resolve.Module{Slug: "fixturesales", Dependencies: []string{"fixturecore"}}, "fixturesales/metadata"},
	} {
		docs, lps := load.LoadFS(root, m.path)
		if len(lps) != 0 {
			t.Fatal(lps)
		}
		if aps := c.AddModule(m.mod, docs...); len(aps) != 0 {
			t.Fatal(aps)
		}
	}
	res := c.ResolveAll()
	if len(res.Problems) != 0 {
		t.Fatal(res.Problems)
	}
	s, ps := dbmodel.Derive(res.Resources)
	if len(ps) != 0 {
		t.Fatal(ps)
	}
	return s
}

// splitTopLevel memecah teks SQL menjadi pernyataan pada titik koma tingkat atas, dengan memperhatikan
// literal '...' (dan E'...'), identifier "...", kutip dolar $tag$...$tag$, dan komentar. Dipakai untuk
// membuktikan bahwa keluaran bangkitan tidak dapat "keluar" dari literalnya (Requirement 19.10): satu
// pernyataan bangkitan harus tetap satu pernyataan, apa pun isi masukan.
func splitTopLevel(t *testing.T, sql string) []string {
	t.Helper()
	var out []string
	start := 0
	i := 0
	n := len(sql)
	for i < n {
		c := sql[i]
		switch {
		case c == '\'':
			esc := i > 0 && (sql[i-1] == 'E' || sql[i-1] == 'e') && (i < 2 || !isIdentByte(sql[i-2]))
			i++
			for {
				if i >= n {
					t.Fatalf("literal tidak tertutup: %q", sql)
				}
				if esc && sql[i] == '\\' {
					i += 2
					continue
				}
				if sql[i] == '\'' {
					if i+1 < n && sql[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case c == '"':
			i++
			for {
				if i >= n {
					t.Fatalf("identifier tidak tertutup: %q", sql)
				}
				if sql[i] == '"' {
					if i+1 < n && sql[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				i++
			}
		case c == '$':
			j := i + 1
			for j < n && isIdentByte(sql[j]) && (sql[j] < '0' || sql[j] > '9' || j != i+1) {
				j++
			}
			if j < n && sql[j] == '$' {
				tag := sql[i : j+1]
				end := strings.Index(sql[j+1:], tag)
				if end < 0 {
					t.Fatalf("kutip dolar %s tidak tertutup: %q", tag, sql)
				}
				i = j + 1 + end + len(tag)
			} else {
				i++
			}
		case c == '-' && i+1 < n && sql[i+1] == '-':
			for i < n && sql[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && sql[i+1] == '*':
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				t.Fatalf("komentar blok tidak tertutup: %q", sql)
			}
			i += 2 + end + 2
		case c == ';':
			out = append(out, strings.TrimSpace(sql[start:i]))
			i++
			start = i
		default:
			i++
		}
	}
	if rest := strings.TrimSpace(sql[start:]); rest != "" {
		out = append(out, rest)
	}
	return out
}

func isIdentByte(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b >= 0x80
}

// decodeLiteral adalah pembaca acuan literal string PostgreSQL (standard_conforming_strings=on): '...' tanpa
// backslash khusus, E'...' dengan escape backslash. Hanya untuk uji.
func decodeLiteral(t *testing.T, lit string) string {
	t.Helper()
	esc := false
	body := lit
	if strings.HasPrefix(lit, "E'") {
		esc = true
		body = lit[1:]
	}
	if len(body) < 2 || body[0] != '\'' || body[len(body)-1] != '\'' {
		t.Fatalf("bukan literal string: %q", lit)
	}
	body = body[1 : len(body)-1]
	var sb strings.Builder
	for i := 0; i < len(body); i++ {
		switch {
		case body[i] == '\'':
			if i+1 >= len(body) || body[i+1] != '\'' {
				t.Fatalf("kutip tunggal tidak di-escape pada %q", lit)
			}
			sb.WriteByte('\'')
			i++
		case esc && body[i] == '\\':
			if i+1 >= len(body) {
				t.Fatalf("backslash menggantung pada %q", lit)
			}
			switch body[i+1] {
			case '\\':
				sb.WriteByte('\\')
			default:
				t.Fatalf("escape tak dikenal \\%c pada %q", body[i+1], lit)
			}
			i++
		default:
			sb.WriteByte(body[i])
		}
	}
	if !utf8.ValidString(sb.String()) {
		t.Fatalf("hasil dekode bukan UTF-8 sah")
	}
	return sb.String()
}

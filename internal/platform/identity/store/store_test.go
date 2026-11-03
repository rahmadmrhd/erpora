package store

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/tenancy"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestNormalizeEmail(t *testing.T) {
	for in, want := range map[string]string{
		"Admin@Example.COM":     "admin@example.com",
		"  a@b.co \t":           "a@b.co",
		"\nMixed.Case@X.io\r\n": "mixed.case@x.io",
		"":                      "",
	} {
		if got := NormalizeEmail(in); got != want {
			t.Errorf("NormalizeEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidKeyTransition(t *testing.T) {
	ok := map[[2]string]bool{
		{KeyPending, KeyActive}:    true,
		{KeyActive, KeyRetiring}:   true,
		{KeyRetiring, KeyRetired}:  true,
		{KeyPending, KeyRetired}:   true, // rotasi dibatalkan: kunci pending dibuang
		{KeyActive, KeyActive}:     false,
		{KeyActive, KeyPending}:    false,
		{KeyRetiring, KeyActive}:   false,
		{KeyRetired, KeyActive}:    false,
		{KeyRetired, KeyRetiring}:  false,
		{KeyActive, KeyRetired}:    false,
		{"bogus", KeyActive}:       false,
		{KeyPending, "bogus"}:      false,
		{"", ""}:                   false,
		{KeyRetiring, KeyPending}:  false,
		{KeyPending, KeyRetiring}:  false,
		{KeyRetired, KeyRetired}:   false,
		{KeyRetiring, KeyRetiring}: false,
	}
	for pair, want := range ok {
		if got := ValidKeyTransition(pair[0], pair[1]); got != want {
			t.Errorf("ValidKeyTransition(%q, %q) = %v, want %v", pair[0], pair[1], got, want)
		}
	}
}

// ID yang bukan UUID tidak boleh mencapai database: diperlakukan sebagai tidak ditemukan (bukan galat 500 dan
// bukan celah injeksi). Store nol tanpa DB membuktikan penolakan terjadi sebelum query.
func TestMalformedIDsAreNotFoundWithoutTouchingDB(t *testing.T) {
	s := &Store{}
	ctx := context.Background()
	tn := tenancy.TenantID{}
	for _, bad := range []string{"", "not-a-uuid", "1' OR '1'='1", "../../etc/passwd"} {
		if _, err := s.GetUser(ctx, tn, bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetUser(%q) err = %v, want ErrNotFound", bad, err)
		}
		if _, err := s.GetSession(ctx, tn, bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetSession(%q) err = %v, want ErrNotFound", bad, err)
		}
		if _, err := s.GetMFAFactor(ctx, tn, bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetMFAFactor(%q) err = %v, want ErrNotFound", bad, err)
		}
	}
}

func TestSigningKeyDefaultsFillRequiredTimestamps(t *testing.T) {
	created := mustTime(t, "2026-11-03T10:00:00Z")
	k := SigningKey{ID: "k1", Status: KeyActive, CreatedAt: created}
	normalizeKey(&k)
	if k.ActivatedAt == nil || !k.ActivatedAt.Equal(created) {
		t.Errorf("ActivatedAt = %v, want %v (bawaan CreatedAt untuk kunci active)", k.ActivatedAt, created)
	}
	p := SigningKey{ID: "k2", Status: KeyPending, CreatedAt: created}
	normalizeKey(&p)
	if p.ActivatedAt != nil || p.RetiringAt != nil || p.RetiredAt != nil {
		t.Errorf("kunci pending tidak boleh membawa cap waktu aktivasi: %+v", p)
	}
}

// Skema hanya lewat migrasi goose (CLAUDE.md): paket ini tidak boleh memanggil AutoMigrate (komentar boleh
// menyebutnya; yang diperiksa adalah identifier pada kode, bukan teks).
func TestNoAutoMigrate(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, e.Name(), nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		checked++
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && id.Name == "AutoMigrate" {
				t.Errorf("%s memakai AutoMigrate (dilarang; skema hanya lewat migrasi goose)", fset.Position(id.Pos()))
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("tidak ada berkas non-test yang diperiksa")
	}
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"erpora/internal/platform/crypto"
	"erpora/internal/platform/tenancy"
	"erpora/internal/platform/testsupport/secretkey"
)

func testSealer(t *testing.T) *crypto.Sealer {
	t.Helper()
	s, err := crypto.NewSealerFromMaterial(secretkey.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestValidate(t *testing.T) {
	ok := Setting{Scope: ScopePlatform, Key: "platform.public_url", Value: json.RawMessage(`"https://x"`)}
	if err := validate(ok, "admin"); err != nil {
		t.Fatalf("sah ditolak: %v", err)
	}
	bad := map[string]Setting{
		"scope ilegal":               {Scope: "tenant", Key: "a.b", Value: json.RawMessage(`1`)},
		"platform dengan module":     {Scope: ScopePlatform, ModuleKey: "erp.core", Key: "a.b", Value: json.RawMessage(`1`)},
		"module tanpa module":        {Scope: ScopeModule, Key: "a.b", Value: json.RawMessage(`1`)},
		"key huruf besar":            {Scope: ScopePlatform, Key: "A.b", Value: json.RawMessage(`1`)},
		"key kosong":                 {Scope: ScopePlatform, Key: "", Value: json.RawMessage(`1`)},
		"value bukan JSON":           {Scope: ScopePlatform, Key: "a.b", Value: json.RawMessage(`{nope`)},
		"value kosong":               {Scope: ScopePlatform, Key: "a.b"},
		"value melebihi batas":       {Scope: ScopePlatform, Key: "a.b", Value: json.RawMessage(`"` + strings.Repeat("x", MaxValueBytes) + `"`)},
		"key berisi pemisah AAD":     {Scope: ScopePlatform, Key: "a|b", Value: json.RawMessage(`1`)},
		"module_key berisi pemisah":  {Scope: ScopeModule, ModuleKey: "a|b", Key: "a.b", Value: json.RawMessage(`1`)},
		"module_key huruf besar":     {Scope: ScopeModule, ModuleKey: "Erp", Key: "a.b", Value: json.RawMessage(`1`)},
		"key terlalu panjang":        {Scope: ScopePlatform, Key: "a" + strings.Repeat("b", 128), Value: json.RawMessage(`1`)},
		"module_key terlalu panjang": {Scope: ScopeModule, ModuleKey: "a" + strings.Repeat("b", 64), Key: "a.b", Value: json.RawMessage(`1`)},
	}
	for name, s := range bad {
		if err := validate(s, "admin"); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument", name, err)
		}
	}
	if err := validate(ok, strings.Repeat("u", 129)); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("updatedBy 129 karakter: err = %v", err)
	}
}

func TestSealRoundTripAndBinding(t *testing.T) {
	sl := testSealer(t)
	s := Setting{Scope: ScopeModule, ModuleKey: "erp.core", Key: "smtp.password", Value: json.RawMessage(`"rahasia-sekali"`), Encrypted: true}
	stored, err := encodeValue(sl, s)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), "rahasia-sekali") {
		t.Fatalf("plaintext bocor di nilai tersimpan: %s", stored)
	}
	var asString string
	if err := json.Unmarshal(stored, &asString); err != nil {
		t.Fatalf("nilai tersegel harus string JSON: %v", err)
	}
	got, err := decodeValue(sl, s.Scope, s.ModuleKey, s.Key, true, stored)
	if err != nil || string(got) != `"rahasia-sekali"` {
		t.Fatalf("decode = %s, %v", got, err)
	}
	// AAD mengikat (scope, module, key): salin ke kunci lain gagal
	for name, args := range map[string][3]string{
		"key lain":   {s.Scope, s.ModuleKey, "smtp.other"},
		"modul lain": {s.Scope, "erp.other", s.Key},
		"scope lain": {ScopePlatform, "", s.Key},
	} {
		if _, err := decodeValue(sl, args[0], args[1], args[2], true, stored); err == nil {
			t.Errorf("%s: pembukaan harus gagal (AAD terikat)", name)
		}
	}
	// tanpa Sealer: gagal tertutup di kedua arah
	if _, err := encodeValue(nil, s); !errors.Is(err, ErrSealerRequired) {
		t.Errorf("encode tanpa sealer: err = %v, want ErrSealerRequired", err)
	}
	if _, err := decodeValue(nil, s.Scope, s.ModuleKey, s.Key, true, stored); !errors.Is(err, ErrSealerRequired) {
		t.Errorf("decode tanpa sealer: err = %v, want ErrSealerRequired", err)
	}
	// nilai tak terenkripsi lewat apa adanya
	plain := Setting{Scope: ScopePlatform, Key: "a.b", Value: json.RawMessage(`{"x":1}`)}
	raw, err := encodeValue(nil, plain)
	if err != nil || string(raw) != `{"x":1}` {
		t.Fatalf("encode polos = %s, %v", raw, err)
	}
	if back, err := decodeValue(nil, plain.Scope, plain.ModuleKey, plain.Key, false, raw); err != nil || string(back) != `{"x":1}` {
		t.Fatalf("decode polos = %s, %v", back, err)
	}
	// blob rusak
	if _, err := decodeValue(sl, s.Scope, s.ModuleKey, s.Key, true, json.RawMessage(`"!!bukan-base64!!"`)); err == nil {
		t.Error("base64 rusak harus ditolak")
	}
	if _, err := decodeValue(sl, s.Scope, s.ModuleKey, s.Key, true, json.RawMessage(`{"a":1}`)); err == nil {
		t.Error("nilai terenkripsi non-string harus ditolak")
	}
}

// Tanpa tenant di context, store gagal sebelum menyentuh DB (store nol aman).
func TestNoTenantInContext(t *testing.T) {
	s := &Store{}
	if _, err := s.Get(context.Background(), ScopePlatform, "", "a.b"); !errors.Is(err, ErrNoTenant) {
		t.Errorf("Get tanpa tenant: err = %v, want ErrNoTenant", err)
	}
	if err := s.Upsert(context.Background(), Setting{Scope: ScopePlatform, Key: "a.b", Value: json.RawMessage(`1`)}, "x"); !errors.Is(err, ErrNoTenant) {
		t.Errorf("Upsert tanpa tenant: err = %v, want ErrNoTenant", err)
	}
	// validasi lebih dulu daripada akses tenant/DB
	ctx := tenancy.WithTenant(context.Background(), tenancy.TenantID{})
	if err := s.Upsert(ctx, Setting{Scope: "x", Key: "a.b", Value: json.RawMessage(`1`)}, "x"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("Upsert tidak valid: err = %v, want ErrInvalidArgument", err)
	}
}

// Skema hanya lewat migrasi goose: tidak ada AutoMigrate pada kode paket ini.
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
				t.Errorf("%s memakai AutoMigrate", fset.Position(id.Pos()))
			}
			return true
		})
	}
	if checked == 0 {
		t.Fatal("tidak ada berkas diperiksa")
	}
}

package store

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateScope(t *testing.T) {
	ok := [][]string{
		{"all"},
		{"owner"},
		{"owner", "assigned:inventory.warehouse"},
		{"custom:example.mine"},
		{"assigned:core.org_unit", "custom:example-addon.team", "owner"},
	}
	for _, s := range ok {
		if err := ValidateScope(s); err != nil {
			t.Errorf("ValidateScope(%v) = %v, want nil", s, err)
		}
	}
	bad := map[string][]string{
		"nil":                nil,
		"kosong":             {},
		"token tak dikenal":  {"everything"},
		"assigned kosong":    {"assigned:"},
		"custom kosong":      {"custom:"},
		"assigned huruf":     {"assigned:Warehouse"},
		"token kosong":       {""},
		"duplikat":           {"owner", "owner"},
		"spasi":              {"all "},
		"terlalu banyak":     manyTokens(17),
		"sisipan SQL":        {"assigned:x' OR 1=1"},
		"assigned dengan :":  {"assigned:a:b"},
		"huruf besar":        {"ALL"},
		"campuran valid+bad": {"all", "bad"},
	}
	for name, s := range bad {
		if err := ValidateScope(s); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: ValidateScope(%v) = %v, want ErrInvalidArgument", name, s, err)
		}
	}
}

func manyTokens(n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, "custom:k"+strings.Repeat("a", i+1))
	}
	return out
}

func TestValidatePermissionAndGrants(t *testing.T) {
	good := PermissionRow{Resource: "example.note", Actions: []string{"read", "update"}, Scope: []string{"all"}}
	if err := validatePermission(good); err != nil {
		t.Fatalf("baris sah ditolak: %v", err)
	}
	cases := map[string]PermissionRow{
		"resource kosong":   {Resource: "", Actions: []string{"read"}, Scope: []string{"all"}},
		"resource besar":    {Resource: "Example.Note", Actions: []string{"read"}, Scope: []string{"all"}},
		"tanpa aksi":        {Resource: "example.note", Scope: []string{"all"}},
		"aksi huruf besar":  {Resource: "example.note", Actions: []string{"Read"}, Scope: []string{"all"}},
		"aksi berkoma":      {Resource: "example.note", Actions: []string{"read,update"}, Scope: []string{"all"}},
		"aksi kosong":       {Resource: "example.note", Actions: []string{""}, Scope: []string{"all"}},
		"scope kosong":      {Resource: "example.note", Actions: []string{"read"}, Scope: []string{}},
		"scope nil":         {Resource: "example.note", Actions: []string{"read"}},
		"scope token rusak": {Resource: "example.note", Actions: []string{"read"}, Scope: []string{"x"}},
	}
	for name, p := range cases {
		if err := validatePermission(p); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument", name, err)
		}
	}

	dup := RoleGrants{FieldAccess: []FieldAccessRow{
		{Resource: "example.note", FieldGroup: "profit", Access: AccessNone},
		{Resource: "example.note", FieldGroup: "profit", Access: AccessRead},
	}}
	if err := validateGrants(dup); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("field access ganda: err = %v", err)
	}
	if err := validateGrants(RoleGrants{FieldAccess: []FieldAccessRow{{Resource: "example.note", FieldGroup: "profit", Access: "all"}}}); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("access all: err = %v", err)
	}
}

func TestValidateNewRole(t *testing.T) {
	ok := []NewRole{
		{Key: "kasir.bandung", Label: "Kasir Bandung", Kind: KindCustom},
		{Key: "inventory.manager", Label: "Manajer", Kind: KindModuleDefault, SourceModule: "inventory"},
	}
	for _, n := range ok {
		if err := validateNewRole(n); err != nil {
			t.Errorf("validateNewRole(%+v) = %v", n, err)
		}
	}
	bad := map[string]NewRole{
		"kind sistem":           {Key: "x", Label: "X", Kind: KindSystem},
		"kind kosong":           {Key: "x", Label: "X"},
		"custom dengan modul":   {Key: "x", Label: "X", Kind: KindCustom, SourceModule: "m"},
		"bawaan tanpa modul":    {Key: "x", Label: "X", Kind: KindModuleDefault},
		"key huruf besar":       {Key: "Kasir", Label: "X", Kind: KindCustom},
		"label kosong":          {Key: "x", Label: "", Kind: KindCustom},
		"deskripsi kepanjangan": {Key: "x", Label: "X", Kind: KindCustom, Description: strings.Repeat("d", 1001)},
	}
	for name, n := range bad {
		if err := validateNewRole(n); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("%s: err = %v, want ErrInvalidArgument", name, err)
		}
	}
	if validLabel("   ") || validLabel(strings.Repeat("a", 121)) || !validLabel("Ünïcode") {
		t.Error("validLabel salah")
	}
}

// Nilai tersimpan yang tidak dapat diuraikan menjadi aksi dan scope sah ditandai ScopeInvalid dan Scope dikosongkan:
// pemakai wajib menolak baris itu (gagal tertutup), tidak pernah menganggapnya "all".
func TestDecodePermissionMarksInvalidStoredValues(t *testing.T) {
	good := decodePermission(permRow{ID: "i", RoleID: "r", Resource: "a.b", Actions: `["read"]`, Scope: `["owner","all"]`})
	if good.ScopeInvalid || len(good.Scope) != 2 || good.Actions[0] != "read" || good.RoleID != "r" {
		t.Errorf("baris sah: %+v", good)
	}
	// duplikat token tidak mengubah arti (AND idempoten) dan tidak ditolak pembaca
	if dup := decodePermission(permRow{Actions: `["read"]`, Scope: `["all","all"]`}); dup.ScopeInvalid {
		t.Errorf("duplikat token: %+v", dup)
	}
	invalid := map[string]permRow{
		"scope bukan JSON":    {Actions: `["read"]`, Scope: `not json`},
		"scope objek":         {Actions: `["read"]`, Scope: `{"all":true}`},
		"scope string":        {Actions: `["read"]`, Scope: `"all"`},
		"scope larik kosong":  {Actions: `["read"]`, Scope: `[]`},
		"scope elemen angka":  {Actions: `["read"]`, Scope: `["all",1]`},
		"scope token tak sah": {Actions: `["read"]`, Scope: `["bogus"]`},
		"scope null":          {Actions: `["read"]`, Scope: `null`},
		"aksi kosong":         {Actions: `[]`, Scope: `["all"]`},
		"aksi bukan JSON":     {Actions: `{read}`, Scope: `["all"]`},
	}
	for name, r := range invalid {
		p := decodePermission(r)
		if !p.ScopeInvalid || p.Scope != nil {
			t.Errorf("%s: %+v, want ScopeInvalid dengan Scope nil", name, p)
		}
	}
}

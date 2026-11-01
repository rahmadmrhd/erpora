package domainrt

import (
	"encoding/json"
	"strings"
	"testing"

	"erpora/sdk/metadata"
)

func spec() *ResourceSpec {
	return &ResourceSpec{
		Key: "x.invoice",
		Fields: []FieldSpec{
			{Name: "id", Type: metadata.TypeUUID, ReadOnly: true},
			{Name: "status", Type: metadata.TypeEnum, ReadOnly: true, HasDefault: true, Default: "draft", Enum: []string{"draft", "submitted"}},
			{Name: "number", Type: metadata.TypeString, Required: true, MinLength: 2, MaxLength: 5, Pattern: "^[A-Z0-9]+$"},
			{Name: "total", Type: metadata.TypeDecimal, Required: true, Precision: 6, Scale: 2, Minimum: "0", Maximum: "9999.99"},
			{Name: "qty", Type: metadata.TypeInteger, Nullable: true, Minimum: "1", Maximum: "10"},
			{Name: "kind", Type: metadata.TypeEnum, Nullable: true, Enum: []string{"a", "b_c"}},
			{Name: "email", Type: metadata.TypeString, Nullable: true, Format: "email"},
			{Name: "site", Type: metadata.TypeString, Nullable: true, Format: "url"},
			{Name: "on", Type: metadata.TypeDate, Nullable: true},
			{Name: "at", Type: metadata.TypeDateTime, Nullable: true},
			{Name: "ref", Type: metadata.TypeUUID, Nullable: true},
			{Name: "paid", Type: metadata.TypeBoolean, HasDefault: true, Default: false},
			{Name: "note", Type: metadata.TypeText, Nullable: true,
				RequiredWhen: metadata.Cmp("total", metadata.OpGte, "1000.00")},
			{Name: "locked", Type: metadata.TypeString, Nullable: true,
				ReadonlyWhen: metadata.Cmp("status", metadata.OpNe, "draft")},
			{Name: "phone", Type: metadata.TypeString, Nullable: true},
			{Name: "phoneE164", Type: metadata.TypeString, Nullable: true, MigrateFrom: "phone"},
			{Name: "label", Type: metadata.TypeString, ReadOnly: true, Computed: true},
		},
		Rules: []RuleSpec{{
			ID: "capTotal", Code: "cap_total", Field: "total", MessageKey: "x.error.cap",
			Assert: metadata.Cmp("total", metadata.OpLte, "5000.00"),
		}},
		LockWhen: metadata.Cmp("status", metadata.OpNe, "draft"),
	}
}

func codes(vs []Violation) map[string]string {
	m := map[string]string{}
	for _, v := range vs {
		m[v.Field] = v.Code
	}
	return m
}

func present(rec metadata.Record) map[string]bool {
	p := map[string]bool{}
	for k := range rec {
		p[k] = true
	}
	return p
}

func TestCreateValid(t *testing.T) {
	rec := metadata.Record{"number": "AB1", "total": "10.50"}
	if vs := spec().Validate(ModeCreate, rec, present(rec)); len(vs) != 0 {
		t.Fatalf("seharusnya sah: %v", vs)
	}
}

func TestCreateRequiredAndNull(t *testing.T) {
	rec := metadata.Record{"total": nil}
	got := codes(spec().Validate(ModeCreate, rec, present(rec)))
	if got["number"] != CodeRequired || got["total"] != CodeNullNotAllowed {
		t.Errorf("got %v", got)
	}
}

func TestFieldChecks(t *testing.T) {
	base := func(extra metadata.Record) metadata.Record {
		r := metadata.Record{"number": "AB", "total": "1"}
		for k, v := range extra {
			r[k] = v
		}
		return r
	}
	cases := []struct {
		name string
		rec  metadata.Record
		want map[string]string
	}{
		{"short", base(metadata.Record{"number": "A"}), map[string]string{"number": CodeTooShort}},
		{"long", base(metadata.Record{"number": "ABCDEF"}), map[string]string{"number": CodeTooLong}},
		{"pattern", base(metadata.Record{"number": "ab1"}), map[string]string{"number": CodePatternMismatch}},
		{"nul", base(metadata.Record{"number": "A\x00B"}), map[string]string{"number": CodeInvalidCharacters}},
		{"type", base(metadata.Record{"number": 5}), map[string]string{"number": CodeInvalidType}},
		{"decnumber", base(metadata.Record{"total": 1.5}), map[string]string{"total": CodeInvalidType}},
		{"decbad", base(metadata.Record{"total": "1e3"}), map[string]string{"total": CodeInvalidDecimal}},
		{"decleading", base(metadata.Record{"total": "01"}), map[string]string{"total": CodeInvalidDecimal}},
		{"decscale", base(metadata.Record{"total": "1.234"}), map[string]string{"total": CodeDecimalScaleExceeded}},
		{"decscaleTrailingZero", base(metadata.Record{"total": "1.500"}), nil},
		{"decprec", base(metadata.Record{"total": "12345.1", "note": "x"}), map[string]string{"total": CodeDecimalPrecisionExceeds}},
		{"decmin", base(metadata.Record{"total": "-1"}), map[string]string{"total": CodeBelowMinimum}},
		{"qtymax", base(metadata.Record{"qty": 11}), map[string]string{"qty": CodeAboveMaximum}},
		{"qtymin", base(metadata.Record{"qty": 0}), map[string]string{"qty": CodeBelowMinimum}},
		{"qtyjson", base(metadata.Record{"qty": json.Number("3")}), nil},
		{"qtystring", base(metadata.Record{"qty": "3"}), map[string]string{"qty": CodeInvalidType}},
		{"qtyunsafe", base(metadata.Record{"qty": int64(1) << 60}), map[string]string{"qty": CodeInvalidType}},
		{"enum", base(metadata.Record{"kind": "zzz"}), map[string]string{"kind": CodeInvalidEnumValue}},
		{"enumok", base(metadata.Record{"kind": "b_c"}), nil},
		{"email", base(metadata.Record{"email": "a b@c"}), map[string]string{"email": CodeInvalidFormat}},
		{"emailok", base(metadata.Record{"email": "a@b.co"}), nil},
		{"url", base(metadata.Record{"site": "ftp://x.y"}), map[string]string{"site": CodeInvalidFormat}},
		{"urlok", base(metadata.Record{"site": "https://x.y/z"}), nil},
		{"date", base(metadata.Record{"on": "2026-02-30"}), map[string]string{"on": CodeInvalidDate}},
		{"dateok", base(metadata.Record{"on": "2024-02-29"}), nil},
		{"datetime", base(metadata.Record{"at": "2026-01-01T00:00:00+07:00"}), map[string]string{"at": CodeInvalidDateTime}},
		{"datetimeok", base(metadata.Record{"at": "2026-01-01T00:00:00.5Z"}), nil},
		{"uuid", base(metadata.Record{"ref": "ABC"}), map[string]string{"ref": CodeInvalidUUID}},
		{"readonly", base(metadata.Record{"id": "x"}), map[string]string{"id": CodeReadOnly}},
		{"computed", base(metadata.Record{"label": "x"}), map[string]string{"label": CodeReadOnly}},
		{"bool", base(metadata.Record{"paid": "yes"}), map[string]string{"paid": CodeInvalidType}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := codes(spec().Validate(ModeCreate, c.rec, present(c.rec)))
			if len(got) != len(c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("%s: got %q want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestViolationsNeverContainValues(t *testing.T) {
	secret := "RAHASIA-BESAR-123"
	rec := metadata.Record{"number": secret, "total": secret, "email": secret}
	for _, v := range spec().Validate(ModeCreate, rec, present(rec)) {
		if strings.Contains(v.Field+v.Code+v.MessageKey, secret) {
			t.Fatalf("nilai bocor: %+v", v)
		}
	}
}

func TestRequiredWhen(t *testing.T) {
	rec := metadata.Record{"number": "AB", "total": "2000.00"}
	if got := codes(spec().Validate(ModeCreate, rec, present(rec))); got["note"] != CodeRequired {
		t.Errorf("requiredWhen tidak ditegakkan: %v", got)
	}
	rec = metadata.Record{"number": "AB", "total": "2000.00", "note": "ok"}
	if got := spec().Validate(ModeCreate, rec, present(rec)); len(got) != 0 {
		t.Errorf("seharusnya sah: %v", got)
	}
	rec = metadata.Record{"number": "AB", "total": "2000.00", "note": nil}
	if got := codes(spec().Validate(ModeCreate, rec, present(rec))); got["note"] != CodeRequired {
		t.Errorf("null eksplisit pada requiredWhen: %v", got)
	}
	rec = metadata.Record{"number": "AB", "total": "10.00"}
	if got := spec().Validate(ModeCreate, rec, present(rec)); len(got) != 0 {
		t.Errorf("requiredWhen salah: %v", got)
	}
}

func TestBusinessRule(t *testing.T) {
	rec := metadata.Record{"number": "AB", "total": "6000.00", "note": "ok"}
	vs := spec().Validate(ModeCreate, rec, present(rec))
	if len(vs) != 1 || !vs[0].Business || vs[0].Code != "cap_total" || vs[0].Field != "total" || vs[0].MessageKey != "x.error.cap" {
		t.Fatalf("got %+v", vs)
	}
	// Pelanggaran struktur menekan evaluasi aturan.
	rec = metadata.Record{"number": "ab", "total": "6000.00", "note": "ok"}
	for _, v := range spec().Validate(ModeCreate, rec, present(rec)) {
		if v.Business {
			t.Fatalf("aturan tidak boleh dievaluasi saat struktur salah: %+v", v)
		}
	}
}

func TestUpdate(t *testing.T) {
	current := metadata.Record{"number": "AB", "total": "10.00", "status": "draft", "locked": nil, "note": nil}
	merged := func(p metadata.Record) (metadata.Record, map[string]bool) {
		m := metadata.Record{}
		for k, v := range current {
			m[k] = v
		}
		for k, v := range p {
			m[k] = v
		}
		return m, present(p)
	}
	m, p := merged(metadata.Record{"qty": 5})
	if vs := spec().Validate(ModeUpdate, m, p); len(vs) != 0 {
		t.Fatalf("patch sah: %v", vs)
	}
	m, p = merged(metadata.Record{"number": nil})
	if got := codes(spec().Validate(ModeUpdate, m, p)); got["number"] != CodeNullNotAllowed {
		t.Errorf("got %v", got)
	}
	m, p = merged(metadata.Record{"total": "2000.00"})
	if got := codes(spec().Validate(ModeUpdate, m, p)); got["note"] != CodeRequired {
		t.Errorf("requiredWhen pada update: %v", got)
	}
	current["status"] = "submitted"
	m, p = merged(metadata.Record{"locked": "x"})
	if got := codes(spec().Validate(ModeUpdate, m, p)); got["locked"] != CodeReadOnly {
		t.Errorf("readonlyWhen: %v", got)
	}
}

func TestDeprecatedPairConflict(t *testing.T) {
	rec := metadata.Record{"number": "AB", "total": "1", "phone": "1", "phoneE164": "2"}
	if got := codes(spec().Validate(ModeCreate, rec, present(rec))); got["phone"] != CodeConflictingDeprecated {
		t.Errorf("got %v", got)
	}
	rec = metadata.Record{"number": "AB", "total": "1", "phone": "1", "phoneE164": "1"}
	if vs := spec().Validate(ModeCreate, rec, present(rec)); len(vs) != 0 {
		t.Errorf("nilai sama sah: %v", vs)
	}
}

func TestLockedAndHelpers(t *testing.T) {
	s := spec()
	if s.Locked(metadata.Record{"status": "draft"}) || !s.Locked(metadata.Record{"status": "submitted"}) {
		t.Error("Locked salah")
	}
	if s.Locked(metadata.Record{}) {
		t.Error("tak tentu bukan terkunci")
	}
	ro := s.ReadOnlyNames()
	for _, n := range []string{"id", "status", "label"} {
		if _, ok := ro[n]; !ok {
			t.Errorf("%s harus read-only", n)
		}
	}
	if _, ok := ro["number"]; ok {
		t.Error("number tidak read-only")
	}
	if f, ok := s.Field("qty"); !ok || f.Type != metadata.TypeInteger {
		t.Error("Field")
	}
	if _, ok := s.Field("nope"); ok {
		t.Error("Field palsu")
	}
	if s.Types()["total"] != metadata.TypeDecimal {
		t.Error("Types")
	}
}

func TestInvalidPatternFailsClosed(t *testing.T) {
	s := &ResourceSpec{Fields: []FieldSpec{{Name: "a", Type: metadata.TypeString, Pattern: "(?=x)"}}}
	rec := metadata.Record{"a": "x"}
	if got := codes(s.Validate(ModeCreate, rec, present(rec))); got["a"] != CodePatternMismatch {
		t.Errorf("got %v", got)
	}
}

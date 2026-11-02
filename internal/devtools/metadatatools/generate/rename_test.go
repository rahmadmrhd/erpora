package generate

import (
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"erpora/internal/devtools/metadatatools/internal/testrepo"
)

// Task 12.4 (Requirement 12.2, 12.3, 12.6; D-33): pasangan rename `phone` (deprecated) dan `phoneE164`
// (migrateFrom: phone) pada fixturecore.customer. Selama masa transisi DTO menerima kedua bentuk, kontrak menandai
// bentuk lama deprecated, server mengisi keduanya, dan nilai berbeda ditolak.

func TestRenamePairOutput(t *testing.T) {
	root, cat := fixtureRepo(t)
	res, _ := runGen(t, root, Options{Check: true}, cat)

	dom := fileContent(t, res, "modules/fixturecore/internal/domain/customer_meta.gen.go")
	for _, want := range []string{`Name:           "phone"`, "Deprecated:     true", `Name:           "phoneE164"`, `MigrateFrom:    "phone"`} {
		if !strings.Contains(dom, want) {
			t.Errorf("domain tidak memuat %q", want)
		}
	}
	if !strings.Contains(dom, "func (s *ResourceSpec) RenamePairs") && !strings.Contains(fileContent(t, res, "modules/fixturecore/internal/domain/meta_runtime.gen.go"), "RenamePairs") {
		t.Error("runtime domain harus menyediakan RenamePairs")
	}

	dto := fileContent(t, res, "modules/fixturecore/internal/presentation/http/customer_dto.gen.go")
	for _, want := range []string{"Phone ", "PhoneE164 ", "func (d *CustomerCreate) FillRenamePairs()", "func (d *CustomerUpdate) FillRenamePairs()"} {
		if !strings.Contains(dto, want) {
			t.Errorf("DTO tidak memuat %q", want)
		}
	}
	// resource tanpa pasangan tidak punya FillRenamePairs
	if other := fileContent(t, res, "modules/fixturecore/internal/presentation/http/branch_dto.gen.go"); strings.Contains(other, "FillRenamePairs") {
		t.Error("resource tanpa pasangan tidak boleh punya FillRenamePairs")
	}

	spec := fileContent(t, res, "modules/fixturecore/openapi.yaml")
	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Deprecated bool `yaml:"deprecated"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal([]byte(spec), &doc); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{"Customer", "CustomerCreate", "CustomerUpdate"} {
		props := doc.Components.Schemas[schema].Properties
		old, okOld := props["phone"]
		nw, okNew := props["phoneE164"]
		if !okOld || !okNew {
			t.Fatalf("%s harus memuat kedua bentuk: %v", schema, props)
		}
		if !old.Deprecated || nw.Deprecated {
			t.Errorf("%s: hanya bentuk lama yang deprecated (lama=%v baru=%v)", schema, old.Deprecated, nw.Deprecated)
		}
	}
}

const coreBehaviorTest = `package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/modules/fixturecore/internal/domain"
	"erpora/sdk/httpx"
)

func post(body string) (*httptest.ResponseRecorder, *http.Request) {
	r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return httptest.NewRecorder(), r
}

func base(extra string) string { return "{\"code\":\"C1\",\"name\":\"N\"" + extra + "}" }

func decodeCreate(t *testing.T, extra string) CustomerCreate {
	t.Helper()
	w, r := post(base(extra))
	d, err := DecodeCustomerCreate(w, r)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func codeOf(t *testing.T, err error) (string, map[string]string) {
	t.Helper()
	var he *httpx.Error
	if !errors.As(err, &he) || he.Status != 422 {
		t.Fatalf("bukan 422: %v", err)
	}
	m := map[string]string{}
	for _, d := range he.Details {
		m[d.Field] = d.Code
	}
	return he.Code, m
}

func TestBothFormsSameValueAccepted(t *testing.T) {
	d := decodeCreate(t, ",\"phone\":\"+62811\",\"phoneE164\":\"+62811\"")
	if err := d.Validate(context.Background()); err != nil {
		t.Fatalf("nilai sama harus diterima: %v", err)
	}
	d.FillRenamePairs()
	if *d.Phone != "+62811" || *d.PhoneE164 != "+62811" {
		t.Errorf("phone=%v phoneE164=%v", d.Phone, d.PhoneE164)
	}
}

func TestBothFormsDifferentValueRejected(t *testing.T) {
	d := decodeCreate(t, ",\"phone\":\"+62811\",\"phoneE164\":\"+62822\"")
	code, m := codeOf(t, d.Validate(context.Background()))
	if code != "validation_failed" || m["phone"] != "conflicting_deprecated_field" {
		t.Errorf("%s %v", code, m)
	}
	// salah satu null dan yang lain berisi juga berbeda
	d = decodeCreate(t, ",\"phone\":null,\"phoneE164\":\"+62822\"")
	if code, m = codeOf(t, d.Validate(context.Background())); code != "validation_failed" || m["phone"] != "conflicting_deprecated_field" {
		t.Errorf("%s %v", code, m)
	}
	// Update: nilai berbeda ditolak walau record tersimpan berisi nilai lain
	w, r := post("{\"phone\":\"1\",\"phoneE164\":\"2\"}")
	u, err := DecodeCustomerUpdate(w, r)
	if err != nil {
		t.Fatal(err)
	}
	if code, m = codeOf(t, u.Validate(context.Background(), nil)); code != "validation_failed" || m["phone"] != "conflicting_deprecated_field" {
		t.Errorf("update: %s %v", code, m)
	}
}

func TestOneFormFillsBoth(t *testing.T) {
	d := decodeCreate(t, ",\"phone\":\"+62811\"")
	if err := d.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	d.FillRenamePairs()
	if d.PhoneE164 == nil || *d.PhoneE164 != "+62811" || !d.Present["phoneE164"] || !d.Present["phone"] {
		t.Errorf("bentuk baru harus terisi dari lama: %+v", d)
	}
	d = decodeCreate(t, ",\"phoneE164\":\"+62822\"")
	d.FillRenamePairs()
	if d.Phone == nil || *d.Phone != "+62822" || !d.Present["phone"] {
		t.Errorf("bentuk lama harus terisi dari baru: %+v", d)
	}
	// null eksplisit pada satu bentuk menyalin null
	d = decodeCreate(t, ",\"phone\":null")
	d.FillRenamePairs()
	if d.PhoneE164 != nil || !d.Present["phoneE164"] {
		t.Errorf("null harus tersalin dan tercatat present: %+v", d)
	}
	// tidak ada yang dikirim: tidak ada yang diisi
	d = decodeCreate(t, "")
	d.FillRenamePairs()
	if d.Present["phone"] || d.Present["phoneE164"] || d.Phone != nil || d.PhoneE164 != nil {
		t.Errorf("tidak boleh mengisi bila keduanya tidak dikirim: %+v", d)
	}
	// salinan tidak berbagi pointer dengan sumbernya
	d = decodeCreate(t, ",\"phone\":\"a\"")
	d.FillRenamePairs()
	*d.PhoneE164 = "b"
	if *d.Phone != "a" {
		t.Error("salinan harus independen")
	}
}

func TestFillDoesNotMutateSharedPresence(t *testing.T) {
	d := decodeCreate(t, ",\"phone\":\"x\"")
	shared := d.Present
	d.FillRenamePairs()
	if shared["phoneE164"] {
		t.Error("Present asli tidak boleh dimutasi")
	}
}

func TestUpdateFillsBoth(t *testing.T) {
	w, r := post("{\"phoneE164\":\"+62833\"}")
	u, err := DecodeCustomerUpdate(w, r)
	if err != nil {
		t.Fatal(err)
	}
	u.FillRenamePairs()
	if u.Phone == nil || *u.Phone != "+62833" || !u.Present["phone"] {
		t.Errorf("%+v", u)
	}
}

func TestRenamePairsMetadata(t *testing.T) {
	pairs := domain.CustomerSpec.RenamePairs()
	if len(pairs) != 1 || pairs[0][0] != "phoneE164" || pairs[0][1] != "phone" {
		t.Errorf("pairs = %v", pairs)
	}
	old, _ := domain.CustomerSpec.Field("phone")
	nw, _ := domain.CustomerSpec.Field("phoneE164")
	if !old.Deprecated || nw.Deprecated || nw.MigrateFrom != "phone" || old.Classification != nw.Classification || old.Group != nw.Group {
		t.Errorf("old=%+v nw=%+v", old, nw)
	}
}
`

// TestRenamePairCompilesAndBehaves membuktikan perilaku dua bentuk pada modul fixturecore sungguhan.
func TestRenamePairCompilesAndBehaves(t *testing.T) {
	if testing.Short() {
		t.Skip("kompilasi modul fixture (lambat)")
	}
	root := testrepo.Copy(t, true)
	cat := fixtureCatalog(t, root)
	if _, err := Generate(root, cat, Options{Checker: okChecker{}}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "modules/fixturecore/internal/presentation/http/behavior_test.go", coreBehaviorTest)
	coreDir := filepath.Join(root, "modules", "fixturecore")
	goTool(t, coreDir, "vet", "./...")
	goTool(t, coreDir, "test", "./...")
}

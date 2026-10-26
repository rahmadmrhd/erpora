package manifest

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Masterminds/semver/v3"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"erpora/contracts"
	"erpora/sdk/module"
)

// Kode problem hasil validasi manifest.
const (
	CodeSchemaInvalid       = "schema_invalid"
	CodeTrustNotSupported   = "trust_not_supported"
	CodeInvalidVersion      = "invalid_version"
	CodeInvalidConstraint   = "invalid_constraint"
	CodeDuplicateDependency = "duplicate_dependency"
	CodeRawMissing          = "raw_missing"
	CodeManifestMismatch    = "manifest_mismatch"
	moduleSchemaResourceID  = "https://erpora.dev/contracts/module.schema.json"
)

// Problem adalah satu temuan validasi manifest. Field berupa JSON pointer ke
// lokasi di module.json (mis. "/dependencies/0/version").
type Problem struct {
	Module  string
	Field   string
	Code    string
	Message string
}

var messagePrinter = message.NewPrinter(language.English)

var compiledSchema = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(contracts.ModuleSchema))
	if err != nil {
		return nil, fmt.Errorf("manifest: schema tidak terbaca: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(moduleSchemaResourceID, doc); err != nil {
		return nil, fmt.Errorf("manifest: schema ditolak: %w", err)
	}
	return c.Compile(moduleSchemaResourceID)
})

// Validate memeriksa manifest terhadap JSON Schema (atas Raw) lalu menjalankan
// pemeriksaan tambahan: trust, SemVer versi, dan constraint SemVer. Semua temuan
// dikumpulkan sekaligus; hasil kosong berarti valid.
func Validate(m module.Manifest) []Problem {
	if len(m.Raw) == 0 {
		return []Problem{{Module: m.Key, Code: CodeRawMissing, Message: "module.json asli (Raw) tidak tersedia untuk divalidasi"}}
	}

	problems := validateSchema(m)
	problems = append(problems, checkMatchesRaw(m)...)
	schemaFields := make(map[string]bool, len(problems))
	for _, p := range problems {
		schemaFields[p.Field] = true
	}

	if m.Trust != "builtin" && !schemaFields["/trust"] {
		problems = append(problems, Problem{
			Module: m.Key, Field: "/trust", Code: CodeTrustNotSupported,
			Message: fmt.Sprintf("trust %q belum didukung; hanya \"builtin\" yang diizinkan", m.Trust),
		})
	}
	if _, err := semver.StrictNewVersion(m.Version); err != nil && !schemaFields["/version"] {
		problems = append(problems, Problem{
			Module: m.Key, Field: "/version", Code: CodeInvalidVersion,
			Message: fmt.Sprintf("versi %q bukan SemVer yang valid", m.Version),
		})
	}
	problems = appendConstraint(problems, m.Key, "/compatibility/platform", m.Compatibility.Platform, schemaFields)
	problems = appendConstraint(problems, m.Key, "/compatibility/sdk", m.Compatibility.SDK, schemaFields)
	seenDeps := map[string]bool{}
	for i, d := range m.Dependencies {
		problems = appendConstraint(problems, m.Key, "/dependencies/"+strconv.Itoa(i)+"/version", d.Version, schemaFields)
		if seenDeps[d.Module] {
			problems = append(problems, Problem{
				Module: m.Key, Field: "/dependencies/" + strconv.Itoa(i) + "/module", Code: CodeDuplicateDependency,
				Message: fmt.Sprintf("dependensi %q dideklarasikan lebih dari sekali", d.Module),
			})
		}
		seenDeps[d.Module] = true
	}
	return problems
}

// checkMatchesRaw memastikan field struct sama dengan hasil decode Raw, sehingga
// registry tidak memakai nilai yang lolos schema hanya karena Raw-nya valid.
func checkMatchesRaw(m module.Manifest) []Problem {
	decoded, err := module.ParseManifest(m.Raw)
	if err != nil {
		return nil // JSON rusak sudah dilaporkan sebagai schema_invalid
	}
	a, b := m, decoded
	a.Raw, b.Raw = nil, nil
	if reflect.DeepEqual(a, b) {
		return nil
	}
	return []Problem{{Module: m.Key, Code: CodeManifestMismatch,
		Message: "field Manifest berbeda dari isi Raw (module.json); gunakan hasil ParseManifest tanpa modifikasi"}}
}

func appendConstraint(problems []Problem, key, field, value string, schemaFields map[string]bool) []Problem {
	if value == "" || schemaFields[field] {
		return problems
	}
	if _, err := semver.NewConstraint(value); err != nil {
		problems = append(problems, Problem{
			Module: key, Field: field, Code: CodeInvalidConstraint,
			Message: fmt.Sprintf("constraint %q tidak valid: %v", value, err),
		})
	}
	return problems
}

func validateSchema(m module.Manifest) []Problem {
	schema, err := compiledSchema()
	if err != nil {
		return []Problem{{Module: m.Key, Code: CodeSchemaInvalid, Message: err.Error()}}
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(m.Raw))
	if err != nil {
		return []Problem{{Module: m.Key, Code: CodeSchemaInvalid, Message: "module.json bukan JSON valid: " + err.Error()}}
	}
	verr := schema.Validate(inst)
	if verr == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(verr, &ve) {
		return []Problem{{Module: m.Key, Code: CodeSchemaInvalid, Message: verr.Error()}}
	}

	var out []Problem
	seen := map[string]bool{}
	add := func(field, msg string) {
		k := field + "\x00" + msg
		if seen[k] {
			return
		}
		seen[k] = true
		out = append(out, Problem{Module: m.Key, Field: field, Code: CodeSchemaInvalid, Message: msg})
	}
	collectLeaves(ve, add)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}

// collectLeaves menelusuri pohon ValidationError dan melaporkan hanya error daun.
// Error "required" dan "additionalProperties" dipetakan ke JSON pointer properti terkait.
func collectLeaves(ve *jsonschema.ValidationError, add func(field, msg string)) {
	if len(ve.Causes) > 0 {
		for _, c := range ve.Causes {
			collectLeaves(c, add)
		}
		return
	}
	base := pointer(ve.InstanceLocation)
	switch k := ve.ErrorKind.(type) {
	case *kind.Required:
		for _, name := range k.Missing {
			add(base+"/"+escape(name), fmt.Sprintf("properti wajib %q tidak ada", name))
		}
	case *kind.AdditionalProperties:
		for _, name := range k.Properties {
			add(base+"/"+escape(name), fmt.Sprintf("properti %q tidak dikenal", name))
		}
	default:
		add(base, messageOf(ve))
	}
}

func messageOf(ve *jsonschema.ValidationError) string {
	return ve.ErrorKind.LocalizedString(messagePrinter)
}

func pointer(loc []string) string {
	var sb strings.Builder
	for _, p := range loc {
		sb.WriteByte('/')
		sb.WriteString(escape(p))
	}
	return sb.String()
}

func escape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

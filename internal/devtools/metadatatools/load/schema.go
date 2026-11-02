package load

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"erpora/contracts"
	"erpora/internal/devtools/metadatatools/naming"
)

const resourceSchemaID = "https://erpora.dev/contracts/resource.schema.json"

var printer = message.NewPrinter(language.English)

var compiled = sync.OnceValues(func() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(contracts.ResourceSchema))
	if err != nil {
		return nil, fmt.Errorf("resource.schema.json tidak terbaca: %w", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(resourceSchemaID, doc); err != nil {
		return nil, fmt.Errorf("resource.schema.json ditolak: %w", err)
	}
	return c.Compile(resourceSchemaID)
})

// validateSchema memvalidasi nilai terhadap skema dan mengembalikan temuan tanpa posisi (posisi diisi pemanggil
// dari JSON pointer).
func validateSchema(value map[string]any) []Problem {
	schema, err := compiled()
	if err != nil {
		return []Problem{{Code: CodeSchema, Message: err.Error()}}
	}
	verr := schema.Validate(value)
	if verr == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(verr, &ve) {
		return []Problem{{Code: CodeSchema, Message: verr.Error()}}
	}
	var out []Problem
	seen := map[string]bool{}
	add := func(ptr, msg string) {
		key := ptr + "\x00" + msg
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Problem{Pointer: ptr, Code: CodeSchema, Message: msg})
	}
	collect(ve, add)
	return out
}

func pointer(loc []string) string {
	var sb strings.Builder
	for _, p := range loc {
		sb.WriteByte('/')
		sb.WriteString(escapePointer(p))
	}
	return sb.String()
}

// collect menelusuri pohon galat dan melaporkan hanya daun. Galat "required" dipetakan ke pointer objek
// induk; "properti tidak dikenal" ke pointer properti itu. Bila kind tidak sah atau hilang, tidak ada badan
// skema yang diterapkan sehingga hanya masalah kind yang dilaporkan.
func collect(ve *jsonschema.ValidationError, add func(ptr, msg string)) {
	base := pointer(ve.InstanceLocation)
	switch k := ve.ErrorKind.(type) {
	case *kind.Required:
		for _, name := range k.Missing {
			add(base, fmt.Sprintf("properti wajib %q tidak ada", name))
		}
		return
	case *kind.AdditionalProperties:
		for _, name := range k.Properties {
			msg := fmt.Sprintf("properti %q tidak dikenal", name)
			if !naming.IdentifierRegexp().MatchString(name) && !strings.Contains(name, ".") {
				msg += " (atau bukan nama camelCase ketat)"
			}
			add(base+"/"+escapePointer(name), msg)
		}
		return
	case *kind.Not:
		add(base, "kombinasi properti ini tidak diizinkan (mis. value dan ref sekaligus)")
		return
	}
	if len(ve.Causes) > 0 {
		for _, c := range ve.Causes {
			collect(c, add)
		}
		return
	}
	add(base, describe(ve, base))
}

// patternHints menjelaskan pola yang dilanggar menurut nama properti terakhir pada pointer.
var patternHints = map[string]string{
	"type":                "tipe harus string, text, integer, decimal, boolean, date, datetime, enum, uuid, sdk:<nama>, atau <slug>.<nama>",
	"label":               "harus kunci i18n bertitik (mis. sales.invoice.number)",
	"message":             "harus kunci i18n bertitik (mis. sales.invoice.error.dueBeforeIssued)",
	"key":                 "harus <slug>.<nama> dengan slug kebab-case dan nama camelCase ketat",
	"to":                  "harus kunci resource <slug>.<nama>",
	"of":                  "harus kunci resource <slug>.<nama>",
	"source":              "harus kunci metadata (sdk:<nama> atau <slug>.<nama>), bukan jalur berkas",
	"labelTemplate":       "hanya placeholder {field} dan teks tetap berupa tanda baca atau spasi; kata harus dari kunci i18n",
	"description":         "teks tidak boleh memuat karakter kontrol, backtick, titik koma, $, <, >, garis miring terbalik, atau kurung kurawal",
	"reason":              "teks tidak boleh memuat karakter kontrol, backtick, titik koma, $, <, >, garis miring terbalik, atau kurung kurawal",
	"clientVisibleReason": "teks tidak boleh memuat karakter kontrol, backtick, titik koma, $, <, >, garis miring terbalik, atau kurung kurawal",
	"since":               "harus versi SemVer MAJOR.MINOR.PATCH",
	"removeAfter":         "harus versi SemVer MAJOR.MINOR.PATCH",
	"currency":            "harus kode ISO 4217 tiga huruf kapital",
}

func lastSegment(ptr string) string {
	if i := strings.LastIndexByte(ptr, '/'); i >= 0 {
		return ptr[i+1:]
	}
	return ptr
}

// describe menerjemahkan galat daun skema ke pesan Indonesia; yang tidak dikenal memakai teks pustaka.
func describe(ve *jsonschema.ValidationError, base string) string {
	switch k := ve.ErrorKind.(type) {
	case *kind.Pattern:
		hint := patternHints[lastSegment(base)]
		if hint == "" {
			if len(k.Want) > 80 {
				hint = "format tidak sesuai pola yang diizinkan"
			} else {
				hint = "harus sesuai pola " + k.Want
			}
		}
		return "nilai tidak valid: " + hint
	case *kind.Enum:
		return strings.Replace(k.LocalizedString(printer), "value must be one of", "nilai harus salah satu dari", 1)
	case *kind.Const:
		return strings.Replace(k.LocalizedString(printer), "value must be", "nilai harus", 1)
	case *kind.Type:
		return fmt.Sprintf("tipe nilai %s tidak sesuai; harus %s", k.Got, strings.Join(k.Want, " atau "))
	case *kind.MaxItems:
		return fmt.Sprintf("memuat %d butir, melebihi batas %d", k.Got, k.Want)
	case *kind.MinItems:
		return fmt.Sprintf("memuat %d butir, kurang dari minimum %d", k.Got, k.Want)
	case *kind.MaxProperties:
		return fmt.Sprintf("memuat %d properti, melebihi batas %d", k.Got, k.Want)
	case *kind.MinProperties:
		return fmt.Sprintf("memuat %d properti, kurang dari minimum %d", k.Got, k.Want)
	case *kind.MaxLength:
		return fmt.Sprintf("panjang %d karakter melebihi batas %d", k.Got, k.Want)
	case *kind.MinLength:
		return fmt.Sprintf("panjang %d karakter kurang dari minimum %d", k.Got, k.Want)
	case *kind.UniqueItems:
		return "butir tidak boleh ganda"
	}
	return ve.ErrorKind.LocalizedString(printer)
}

func sortProblems(ps []Problem) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Pointer != b.Pointer {
			return a.Pointer < b.Pointer
		}
		return a.Message < b.Message
	})
}

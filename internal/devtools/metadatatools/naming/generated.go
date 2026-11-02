package naming

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Identifier Go yang dibangkitkan (keputusan pemilik produk, 2 Nov 2026): satu sumber untuk metadata:check dan
// metadata:generate agar bentrok yang pasti gagal kompilasi ditolak sebelum kode ditulis.
//
// Dua jenis bentrok nyata:
//  1. Anggota struct DTO (XCreate dan XUpdate): hanya field tulis (GoField.Writable) menjadi field struct ber-PascalCase yang berdampingan
//     dengan Present (field) serta metode Validate dan FillRenamePairs.
//  2. Identifier tingkat paket: konstanta dan variabel per resource (<P>Key, <P>LabelTemplate, <P>Spec,
//     <P>Field<F>, konstanta enum <P><F><Nilai>) pada paket domain, serta <P>Create, <P>Update, Decode<P>Create,
//     Decode<P>Update pada paket HTTP, harus unik di seluruh resource satu modul dan tidak boleh sama dengan
//     tipe runtime yang disalin ke tiap modul (mis. resource field menghasilkan FieldSpec).

// GoDTOMembers adalah anggota struct DTO bangkitan (bandingkan dengan bentuk PascalCase nama field).
var GoDTOMembers = set(`Present Validate FillRenamePairs`)

// GoDomainRuntimeNames adalah identifier eksported tingkat paket pada runtime domain yang disalin ke tiap modul
// (templates/resource/domainrt). Tes generate memastikan daftar ini sama dengan isi berkas runtime.
var GoDomainRuntimeNames = set(`Mode ModeCreate ModeUpdate CodeRequired CodeNullNotAllowed CodeReadOnly CodeInvalidType
CodeInvalidCharacters CodeTooShort CodeTooLong CodePatternMismatch CodeInvalidFormat CodeInvalidEnumValue CodeInvalidDecimal
CodeDecimalScaleExceeded CodeDecimalPrecisionExceeds CodeBelowMinimum CodeAboveMaximum CodeInvalidUUID CodeInvalidDate
CodeInvalidDateTime CodeConflictingDeprecated FieldSpec RuleSpec Violation Transition StatusSpec ResourceSpec`)

// GoHTTPRuntimeNames adalah identifier eksported tingkat paket pada runtime HTTP (httprt); saat ini kosong.
var GoHTTPRuntimeNames = set(``)

// GoField adalah satu field (termasuk kolom relasi) beserta nilai enum-nya.
type GoField struct {
	Name string
	Enum []string
	// Writable menandai field yang menjadi masukan tulis (masuk struct DTO XCreate dan XUpdate): bukan readOnly dan bukan
	// terhitung (accessor). Hanya field tulis yang dapat bentrok dengan anggota struct DTO; field baca-saja tidak ada
	// di DTO sehingga bebas memakai nama seperti validate.
	Writable bool
}

// GoResource adalah masukan pemeriksaan identifier Go satu resource.
type GoResource struct {
	// Key adalah kunci resource (untuk pesan); Name adalah segmen nama camelCase.
	Key, Name string
	Fields    []GoField
}

// GoConflict adalah satu bentrok identifier Go; Field kosong bila bentroknya pada resource.
type GoConflict struct {
	Resource, Field string
	Err             *Error
}

var enumValueRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)

// PascalEnumValue mengubah nilai enum snake_case menjadi PascalCase ("net_15" menjadi "Net15"); "" bila nilai
// tidak memenuhi pola enum (tidak ada konstanta dibuat untuk nilai seperti itu).
func PascalEnumValue(v string) string {
	if !enumValueRe.MatchString(v) {
		return ""
	}
	var sb strings.Builder
	for _, w := range strings.Split(v, "_") {
		if w == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return sb.String()
}

type goIdent struct{ name, what, field string }

// GoDomainIdentifiers mengembalikan identifier tingkat paket domain yang dibangkitkan untuk satu resource.
func GoDomainIdentifiers(pascal string, fields []GoField) []string {
	var out []string
	for _, id := range domainIdents(pascal, fields) {
		out = append(out, id.name)
	}
	return out
}

func domainIdents(pascal string, fields []GoField) []goIdent {
	ids := []goIdent{{pascal + "Key", "kunci resource", ""}, {pascal + "LabelTemplate", "templat label", ""}, {pascal + "Spec", "spesifikasi", ""}}
	for _, f := range fields {
		gn, err := ToPascal(f.Name)
		if err != nil {
			continue
		}
		ids = append(ids, goIdent{pascal + "Field" + gn, "field " + f.Name, f.Name})
		for _, v := range f.Enum {
			if pv := PascalEnumValue(v); pv != "" {
				ids = append(ids, goIdent{pascal + gn + pv, "nilai enum " + f.Name + "=" + v, f.Name})
			}
		}
	}
	return ids
}

func httpIdents(pascal string) []goIdent {
	return []goIdent{
		{pascal + "Create", "body pembuatan", ""}, {pascal + "Update", "body perubahan", ""},
		{"Decode" + pascal + "Create", "decoder pembuatan", ""}, {"Decode" + pascal + "Update", "decoder perubahan", ""},
	}
}

// CheckGoPackage memeriksa seluruh resource satu modul: anggota DTO, bentrok dengan runtime, dan bentrok antar
// identifier tingkat paket. Semua pelanggaran memakai kode CodeReservedGo (name_reserved_go) dan terurut stabil.
func CheckGoPackage(rs []GoResource) []GoConflict {
	var out []GoConflict
	add := func(r GoResource, field string, detail string, args ...any) {
		name := r.Name
		if field != "" {
			name = field
		}
		out = append(out, GoConflict{Resource: r.Key, Field: field, Err: errf(CodeReservedGo, name, detail, args...)})
	}
	sorted := append([]GoResource(nil), rs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })
	domain := map[string]string{} // identifier -> asal
	http := map[string]string{}
	for _, r := range sorted {
		pascal, err := ToPascal(r.Name)
		if err != nil {
			continue // galat penamaan dilaporkan pemeriksa lain
		}
		for _, f := range r.Fields {
			if gn, err := ToPascal(f.Name); err == nil && f.Writable && GoDTOMembers[gn] {
				add(r, f.Name, "bentrok dengan anggota struct DTO bangkitan %s (%s); pilih nama lain", gn, joinKeys(GoDTOMembers))
			}
		}
		claim := func(pkg map[string]string, runtime map[string]bool, ids []goIdent) {
			for _, id := range ids {
				if runtime[id.name] {
					add(r, id.field, "%s menghasilkan identifier Go %s yang bentrok dengan tipe atau konstanta runtime bangkitan; pilih nama lain", id.what, id.name)
					continue
				}
				if prev, dup := pkg[id.name]; dup {
					add(r, id.field, "%s menghasilkan identifier Go %s yang sama dengan %s; pilih nama lain", id.what, id.name, prev)
					continue
				}
				pkg[id.name] = fmt.Sprintf("%s (%s)", id.what, r.Key)
			}
		}
		claim(domain, GoDomainRuntimeNames, domainIdents(pascal, r.Fields))
		claim(http, GoHTTPRuntimeNames, httpIdents(pascal))
	}
	return out
}

func joinKeys(m map[string]bool) string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, ", ")
}

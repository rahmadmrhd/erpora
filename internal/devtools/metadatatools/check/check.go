// Package check adalah validasi semantik metadata:check (spec resource-metadata, task 5.x): tipe dan batasan
// field, kondisi, accessor, templat label, relasi, integrasi manifest, serta aturan deprecated, salinan, dan
// penguncian. Paket ini hanya membaca (tidak menulis apa pun) dan memakai hasil resolusi (resolve), aturan
// penamaan (naming), pemuat (load), serta AST kondisi, ekspresi, templat label, dan pola dari sdk/metadata.
//
// Semua temuan memuat berkas dan baris sumber; pesan berbahasa Indonesia.
package check

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
)

// Severity membedakan galat (menggagalkan) dari peringatan.
type Severity string

// Tingkat keparahan temuan.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Problem adalah satu temuan. Pointer adalah JSON pointer ke nilai yang bermasalah pada dokumen sumber
// (untuk field hasil resolusi memakai nama field hasil resolusi, mis. /fields/billingStreet/maxLength).
type Problem struct {
	Severity Severity
	File     string
	Line     int
	Column   int
	Pointer  string
	Code     string
	Message  string
	// Module adalah slug modul dan Resource kunci metadata (<slug>.<nama>) yang sedang diperiksa.
	Module   string
	Resource string
}

func (p Problem) String() string {
	loc := p.File
	if p.Line > 0 {
		loc = fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
	}
	ptr := ""
	if p.Pointer != "" {
		ptr = " " + p.Pointer + ":"
	}
	return fmt.Sprintf("%s: %s [%s]%s %s", loc, p.Severity, p.Code, ptr, p.Message)
}

// ManifestGroup adalah satu field group pada manifest (PRD-001 FR-110).
type ManifestGroup struct {
	Key            string // snake_case
	Default        bool
	Classification string
	// Fields adalah daftar nama field (snake_case) bila manifest memuatnya; opsional (P-1).
	Fields []string
}

// ManifestResource adalah satu entri resources[] pada manifest.
type ManifestResource struct {
	Key           string // snake_case pada segmen nama, mis. sales.invoice_line
	Actions       []string
	CustomActions []string
	OwnerField    string            // snake_case
	ScopeFields   map[string]string // kunci dimensi -> nama field (snake_case)
	FieldGroups   []ManifestGroup
}

// Manifest adalah bagian module.json yang dibutuhkan pemeriksaan metadata.
type Manifest struct {
	Key  string
	Slug string
	// Version adalah versi modul yang sedang disiapkan (manifest version, SemVer).
	Version string
	// Dependencies adalah kunci modul pada dependencies manifest.
	Dependencies []string
	Capabilities []string
	// HasResources menandai manifest memuat "resources" (PRD-001); false = keadaan sebelum PRD-001.
	HasResources bool
	Resources    []ManifestResource
}

// ParseManifest membaca bagian yang relevan dari module.json.
func ParseManifest(raw []byte) (Manifest, error) {
	var in struct {
		Key          string `json:"key"`
		Slug         string `json:"slug"`
		Version      string `json:"version"`
		Dependencies []struct {
			Module string `json:"module"`
		} `json:"dependencies"`
		Capabilities []string `json:"capabilities"`
		Resources    *[]struct {
			Key           string   `json:"key"`
			Actions       []string `json:"actions"`
			CustomActions []struct {
				Key string `json:"key"`
			} `json:"custom_actions"`
			OwnerField  string            `json:"owner_field"`
			ScopeFields map[string]string `json:"scope_fields"`
			FieldGroups []struct {
				Key            string   `json:"key"`
				Default        bool     `json:"default"`
				Classification string   `json:"classification"`
				Fields         []string `json:"fields"`
			} `json:"field_groups"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return Manifest{}, err
	}
	m := Manifest{Key: in.Key, Slug: in.Slug, Version: in.Version, Capabilities: in.Capabilities}
	for _, d := range in.Dependencies {
		m.Dependencies = append(m.Dependencies, d.Module)
	}
	if in.Resources != nil {
		m.HasResources = true
		for _, r := range *in.Resources {
			mr := ManifestResource{Key: r.Key, Actions: r.Actions, OwnerField: r.OwnerField, ScopeFields: r.ScopeFields}
			for _, c := range r.CustomActions {
				mr.CustomActions = append(mr.CustomActions, c.Key)
			}
			for _, g := range r.FieldGroups {
				mr.FieldGroups = append(mr.FieldGroups, ManifestGroup{Key: g.Key, Default: g.Default, Classification: g.Classification, Fields: g.Fields})
			}
			m.Resources = append(m.Resources, mr)
		}
	}
	return m, nil
}

// Module adalah satu modul yang diperiksa: manifest dan dokumen metadatanya (boleh kosong).
type Module struct {
	// Dir adalah direktori modul relatif root repo (untuk pesan), mis. modules/sales.
	Dir      string
	Manifest Manifest
	Docs     []*load.Document
	// MigrationsFS adalah isi direktori migrations modul (berkas .sql di akar); nil bila modul tanpa migrasi.
	// Dipakai untuk menggagalkan pemeriksaan bila masih ada kerangka migrasi bertanda TODO(metadata:migrate).
	MigrationsFS fs.FS
}

// Input adalah semua modul yang menjadi konteks pemeriksaan (relasi dan include lintas modul diselesaikan
// terhadap kumpulan ini). Metadata bawaan SDK ditambahkan otomatis.
type Input struct {
	Modules []Module
	// Git (opsional) membaca metadata pada tag rilis terakhir tiap modul untuk aturan deprecate-before-remove
	// (Requirement 12.1); nil melewati perbandingan itu.
	Git GitReader
}

// ResourceInfo memuat hasil turunan pemeriksaan untuk satu resource, dipakai pembangkit (6.x).
type ResourceInfo struct {
	Key    string
	Module string
	// Document menandai resource dokumen (menyertakan sdk.document).
	Document bool
	// RuleScope memetakan id aturan ke scope efektif (both atau server; Requirement 4.6).
	RuleScope map[string]string
	// ConditionScope memetakan pointer kondisi tingkat field (/fields/<f>/visibleWhen dst.) dan /lockWhen ke
	// scope efektif; kondisi yang merujuk field sensitif atau di luar grup default hanya server.
	ConditionScope map[string]string
	// Class dan Group adalah klasifikasi dan grup efektif tiap field (accessor mewarisi yang tertinggi dan
	// paling ketat dari field masukannya; Requirement 5.8). Grup memakai kunci manifest (snake_case).
	Class map[string]string
	Group map[string]string
}

// Report adalah hasil Run.
type Report struct {
	Problems  []Problem
	Resolved  *resolve.Result
	Resources map[string]*ResourceInfo
}

// Resource mengembalikan info turunan resource menurut kunci, nil bila tidak ada.
func (r *Report) Resource(key string) *ResourceInfo { return r.Resources[key] }

// ErrorCount menghitung galat (peringatan tidak dihitung).
func (r *Report) ErrorCount() int {
	n := 0
	for _, p := range r.Problems {
		if p.Severity == SeverityError {
			n++
		}
	}
	return n
}

// HasErrors menyatakan ada galat.
func (r *Report) HasErrors() bool { return r.ErrorCount() > 0 }

// LoadWorkspace membaca semua modules/*/module.json dan modules/*/metadata/** di bawah root (hanya-baca).
// Temuan pemuatan (YAML/skema/nama berkas) dikembalikan sebagai Problem; berkas yang gagal dimuat tidak
// disertakan pada Input.
func LoadWorkspace(root string) (Input, []Problem) {
	var in Input
	var probs []Problem
	manifests, _ := filepath.Glob(filepath.Join(root, "modules", "*", "module.json"))
	sort.Strings(manifests)
	fsys := os.DirFS(root)
	for _, mf := range manifests {
		dir := filepath.Base(filepath.Dir(mf))
		rel := "modules/" + dir
		raw, err := os.ReadFile(mf) //nolint:gosec // path dari glob di root repo
		if err != nil {
			probs = append(probs, Problem{Severity: SeverityError, File: rel + "/module.json", Code: CodeManifestParse, Message: "module.json tidak dapat dibaca: " + err.Error()})
			continue
		}
		m, err := ParseManifest(raw)
		if err != nil {
			probs = append(probs, Problem{Severity: SeverityError, File: rel + "/module.json", Code: CodeManifestParse, Message: "module.json tidak valid: " + err.Error()})
			continue
		}
		mod := Module{Dir: rel, Manifest: m}
		if st, err := os.Stat(filepath.Join(root, "modules", dir, "migrations")); err == nil && st.IsDir() {
			mod.MigrationsFS = os.DirFS(filepath.Join(root, "modules", dir, "migrations"))
		}
		docs, lps := load.LoadFS(fsys, rel+"/metadata")
		mod.Docs = docs
		for _, p := range lps {
			probs = append(probs, fromLoad(p, m.Slug, ""))
		}
		in.Modules = append(in.Modules, mod)
	}
	return in, probs
}

func fromLoad(p load.Problem, module, resource string) Problem {
	return Problem{Severity: SeverityError, File: p.File, Line: p.Line, Column: p.Column, Pointer: p.Pointer, Code: p.Code,
		Message: p.Message, Module: module, Resource: resource}
}

func sortProblems(ps []Problem) []Problem {
	seen := map[string]bool{}
	out := ps[:0:0]
	for _, p := range ps {
		// Dedupe lintas resource: temuan di berkas sumber yang sama (mis. abstrak yang disertakan beberapa
		// resource) dilaporkan sekali.
		k := fmt.Sprintf("%s|%d|%d|%s|%s|%s", p.File, p.Line, p.Column, p.Code, p.Severity, strings.TrimSpace(p.Message))
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Column != b.Column {
			return a.Column < b.Column
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
	return out
}

// Package i18ntools berisi pemeriksa, pelapor kelengkapan, pembuat kerangka bahasa, dan generator kunci terketik
// untuk katalog terjemahan (PRD-000 FR-25, spec i18n-platform Requirement 7 dan 8). Katalog dimuat dari
// FILESYSTEM repo (bukan embed) dengan sdk/i18n.ParseCatalog dan Compile yang sama dengan runtime, sehingga
// tidak ada dua definisi "valid".
package i18ntools

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Tata letak repo yang dikenal pemeriksa (relatif terhadap root repo).
const (
	// RegistryPath adalah registri bahasa platform.
	RegistryPath = "internal/platform/i18n/locales.json"
	// PlatformCatalogDir menyimpan katalog namespace "platform" (<locale>.json).
	PlatformCatalogDir = "internal/platform/i18n/catalog"
	// UICatalogDir menyimpan katalog namespace "ui" (<locale>.json).
	UICatalogDir = "packages/ui/i18n"
)

// Severity adalah tingkat temuan.
type Severity string

// Tingkat temuan: galat menggagalkan build, peringatan hanya dicetak (kecuali --strict).
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Kode temuan di luar kode Issue sdk/i18n (invalid_json, duplicate_key, flatten_collision, invalid_segment,
// invalid_icu, html_not_allowed, too_large, unsupported_construct).
const (
	// CodeInvalidRegistry: registri bahasa tidak lolos validasi (kode, fallback, siklus, kedalaman, default).
	CodeInvalidRegistry = "invalid_registry"
	// CodeInvalidNamespace: kunci bernamespace pemilik lain ("ns::kunci") atau namespace modul tidak sah.
	CodeInvalidNamespace = "invalid_namespace"
	// CodeInvalidManifest: bagian i18n manifest tidak konsisten (defaultLocale di luar locales).
	CodeInvalidManifest = "invalid_manifest"
	// CodeUnregisteredLocale: modul mendeklarasikan bahasa yang tidak ada di registri.
	CodeUnregisteredLocale = "unregistered_locale"
	// CodeMissingFile: berkas katalog bahasa yang wajib ada tidak ditemukan.
	CodeMissingFile = "missing_file"
	// CodeMissingKey (peringatan): kunci bahasa default tidak ada di bahasa ini.
	CodeMissingKey = "missing_key"
	// CodeExtraKey (peringatan): kunci ada di bahasa ini tetapi tidak di bahasa default.
	CodeExtraKey = "extra_key"
	// CodeParamMismatch (peringatan): argumen ICU berbeda dari bahasa default.
	CodeParamMismatch = "param_mismatch"
	// CodeUndeclaredI18n (peringatan): modul membawa direktori i18n/ tanpa bagian i18n di manifest.
	CodeUndeclaredI18n = "undeclared_i18n"
	// CodeUndeclaredFile (peringatan): berkas <bahasa>.json di direktori katalog yang tidak dideklarasikan.
	CodeUndeclaredFile = "undeclared_file"
)

// Finding adalah satu temuan pemeriksa. File memakai pemisah "/" relatif terhadap root repo.
type Finding struct {
	Severity  Severity `json:"severity"`
	Code      string   `json:"code"`
	Namespace string   `json:"namespace,omitempty"`
	Locale    string   `json:"locale,omitempty"`
	File      string   `json:"file,omitempty"`
	Key       string   `json:"key,omitempty"`
	Detail    string   `json:"detail"`
}

// Report adalah hasil Check. Findings terurut stabil (galat dulu, lalu berkas, bahasa, kunci).
type Report struct {
	Findings []Finding `json:"findings"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
}

// Failed melaporkan apakah ada galat (peringatan hanya menggagalkan bila --strict mengubahnya menjadi galat).
func (r *Report) Failed() bool { return r.Errors > 0 }

// WriteJSON menulis laporan sebagai JSON (--json).
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// WriteText menulis laporan sebagai teks: satu baris per temuan dan ringkasan.
func (r *Report) WriteText(w io.Writer) {
	for _, f := range r.Findings {
		label := "peringatan"
		if f.Severity == SeverityError {
			label = "galat"
		}
		loc := f.File
		if loc == "" {
			loc = f.Namespace
		}
		var parts []string
		if f.Locale != "" {
			parts = append(parts, "bahasa "+f.Locale)
		}
		if f.Key != "" {
			parts = append(parts, "kunci "+f.Key)
		}
		extra := ""
		if len(parts) > 0 {
			extra = " (" + strings.Join(parts, ", ") + ")"
		}
		fmt.Fprintf(w, "%s %s [%s]%s: %s\n", label, loc, f.Code, extra, f.Detail)
	}
	fmt.Fprintf(w, "%d galat, %d peringatan\n", r.Errors, r.Warnings)
}

func (r *Report) add(f Finding) { r.Findings = append(r.Findings, f) }

// finalize menghitung ringkasan; strict mengubah semua peringatan menjadi galat. Urutan stabil.
func (r *Report) finalize(strict bool) {
	r.Errors, r.Warnings = 0, 0
	for i := range r.Findings {
		if strict && r.Findings[i].Severity == SeverityWarning {
			r.Findings[i].Severity = SeverityError
		}
	}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if a.Severity != b.Severity {
			return a.Severity == SeverityError
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Locale != b.Locale {
			return a.Locale < b.Locale
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		return a.Code < b.Code
	})
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			r.Errors++
		} else {
			r.Warnings++
		}
	}
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
}

// ConfigError adalah kegagalan prasyarat atau konfigurasi (registri tidak ada, manifest tidak terbaca, root
// repo tidak ditemukan), dibedakan dari kegagalan validasi katalog (Requirement 8.6): exit code 3.
type ConfigError struct{ Err error }

// Error mengimplementasi error.
func (e *ConfigError) Error() string { return e.Err.Error() }

// Unwrap mengembalikan galat dasar.
func (e *ConfigError) Unwrap() error { return e.Err }

func configErr(format string, a ...any) error {
	return &ConfigError{Err: fmt.Errorf(format, a...)}
}

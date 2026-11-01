// Package load memuat berkas metadata resource (modules/<dir>/metadata/*.yaml) secara aman dan
// memvalidasinya terhadap contracts/resource.schema.json (spec resource-metadata Requirement 1).
//
// Urutan: batas ukuran berkas, parse YAML ke pohon node (tanpa ekspansi alias), pemeriksaan tag, alias,
// kedalaman, dan ukuran ekspansi sebelum nilai apa pun dibentuk, konversi ke nilai generik (alias
// diekspansi, kunci ganda ditolak), validasi JSON Schema, lalu batas struktural yang tidak dapat dinyatakan
// JSON Schema (node dan kedalaman kondisi) serta penolakan URL. Semua temuan dikumpulkan dengan berkas, baris,
// dan kolom; paket ini tidak menulis apa pun.
package load

import (
	"fmt"
	"strings"
)

// Kode temuan.
const (
	CodeRead             = "read_error"
	CodeSymlink          = "symlink"
	CodeFileTooLarge     = "file_too_large"
	CodeFileName         = "file_name"
	CodeKindMismatch     = "kind_mismatch"
	CodeYAMLSyntax       = "yaml_syntax"
	CodeYAMLShape        = "yaml_shape"
	CodeYAMLMultiDoc     = "yaml_multi_document"
	CodeYAMLTag          = "yaml_tag"
	CodeYAMLAlias        = "yaml_alias"
	CodeYAMLDepth        = "yaml_depth"
	CodeYAMLKey          = "yaml_key"
	CodeYAMLDuplicateKey = "yaml_duplicate_key"
	CodeYAMLMerge        = "yaml_merge_key"
	CodeSchema           = "schema"
	CodeConditionNodes   = "condition_nodes"
	CodeConditionDepth   = "condition_depth"
	CodeForbiddenValue   = "forbidden_value"
)

// Problem adalah satu temuan pemuatan, dengan lokasi di berkas sumber. Line dan Column berbasis 1; nol berarti
// temuan tidak terikat pada baris tertentu (mis. nama berkas).
type Problem struct {
	File    string
	Line    int
	Column  int
	Pointer string // JSON pointer ke nilai yang bermasalah; kosong = akar dokumen
	Code    string
	Message string
}

func (p Problem) String() string {
	var sb strings.Builder
	sb.WriteString(p.File)
	if p.Line > 0 {
		fmt.Fprintf(&sb, ":%d:%d", p.Line, p.Column)
	}
	fmt.Fprintf(&sb, ": [%s]", p.Code)
	if p.Pointer != "" {
		sb.WriteString(" " + p.Pointer + ":")
	}
	sb.WriteString(" " + p.Message)
	return sb.String()
}

// Pos adalah posisi di berkas sumber (berbasis 1).
type Pos struct{ Line, Column int }

// Limits adalah batas pemuat YAML aman (Requirement 1.2).
type Limits struct {
	// MaxFileBytes adalah ukuran berkas maksimum (256 KB).
	MaxFileBytes int
	// MaxDepth adalah kedalaman bersarang maksimum node YAML (setelah ekspansi alias).
	MaxDepth int
	// MaxAliases adalah jumlah node alias maksimum per berkas.
	MaxAliases int
	// MaxAliasExpansion adalah jumlah node tambahan maksimum akibat ekspansi alias.
	MaxAliasExpansion int
}

// DefaultLimits mengembalikan batas bawaan.
func DefaultLimits() Limits {
	return Limits{
		MaxFileBytes:      256 * 1024,
		MaxDepth:          32,
		MaxAliases:        16,
		MaxAliasExpansion: 10000,
	}
}

// Batas struktural kondisi (Requirement 1.3) yang tidak dapat dinyatakan JSON Schema.
const (
	MaxConditionNodes = 32
	MaxConditionDepth = 4
)

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

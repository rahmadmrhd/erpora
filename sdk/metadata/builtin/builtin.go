// Package builtin menyematkan berkas metadata bawaan SDK (abstrak sdk.audit dan sdk.document serta tipe domain
// sdk.email, sdk.phone, sdk.percentage, sdk.money). Paket ini hanya berisi berkas YAML dan embed; berkas
// dimuat dan diselesaikan oleh alat pengembang saat pembangkitan (internal/devtools/metadatatools), tidak
// pernah ditafsirkan saat runtime (design D-2, D-4).
package builtin

import "embed"

// FS memuat semua berkas metadata bawaan di akar: *.abstract.yaml dan *.type.yaml.
//
//go:embed *.yaml
var FS embed.FS

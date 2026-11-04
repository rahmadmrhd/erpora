// Package resource menyematkan templat dan sumber runtime pembangkit metadata resource (spec resource-metadata,
// task 6.2 sampai 6.4). Hanya dikompilasi ke binary dev.
//
// Templat Go memakai pembatas <% %> (bukan [[ ]] seperti templat make:*) karena kode Go bangkitan memuat
// generik seperti f[[T]] dan literal komposit. Semua nilai literal disiapkan pembangkit lewat satu fungsi
// kutip; templat tidak pernah menyisipkan teks masukan secara langsung (design D-32).
package resource

import "embed"

// FS berisi templat (*.tmpl) dan sumber runtime (domainrt/runtime.go, httprt/runtime.go).
//
//go:embed domain_meta.go.tmpl http_dto.go.tmpl domainrt/runtime.go httprt/runtime.go
var FS embed.FS

// Nama berkas pada FS.
const (
	DomainMetaTemplate = "domain_meta.go.tmpl"
	HTTPDTOTemplate    = "http_dto.go.tmpl"
	// DomainRuntimeSource dan HTTPRuntimeSource adalah sumber paket Go biasa; klausa package-nya
	// (domainrt dan httprt) diganti saat disalin ke modul.
	DomainRuntimeSource  = "domainrt/runtime.go"
	HTTPRuntimeSource    = "httprt/runtime.go"
	DomainRuntimePackage = "domainrt"
	HTTPRuntimePackage   = "httprt"
)

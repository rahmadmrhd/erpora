// Package export menulis ekspor CSV/XLSX yang aman untuk modul (security-guidelines §3.2, PRD-000 FR-24.2):
// sel CSV yang berpotensi formula diberi awalan kutip (SanitizeCell), XLSX hanya memakai sel teks
// (inlineStr) tanpa elemen formula, setiap baris melewati authz.MaskFields, kolom highly_confidential
// ditolak, jumlah baris dibatasi (DefaultMaxRows), dan setiap ekspor dicatat lewat Auditor yang dipasang
// platform. Nilai sel wajib skalar; nilai bersarang ditolak (ErrNonScalarValue).
//
// Ekspor ini sinkron; ekspor besar menunggu infrastruktur Jobs (api-guidelines §10).
package export

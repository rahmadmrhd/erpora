// Package authz adalah API otorisasi tiga lapis untuk modul (PRD-001 FR-110 sampai FR-119): lapis aksi
// (Require), lapis record (Filter, Check; scope owner/assigned/custom/all), dan lapis field (Fields,
// MaskFields, CheckWrite). Mesin yang mengevaluasi hak efektif diimplementasi platform dan dipasang di
// context lewat WithEngine; paket ini hanya mendelegasikan dan selalu gagal tertutup: tanpa principal
// (dan tanpa hak sistem), tanpa mesin, atau galat/panic mesin berarti ditolak (Filter menambah
// WHERE false). Hak sistem hanya lewat AsSystem dengan reason konstanta yang teraudit.
//
// Paket ini hanya mengimpor stdlib, gorm.io/gorm (tanda tangan Filter dan ScopeProvider; D3b), dan
// erpora/sdk/identity. Untuk test modul gunakan paket authztest.
package authz

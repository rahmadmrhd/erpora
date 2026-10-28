// Package store adalah persistensi lifecycle modul (PRD-000 FR-19): installations, operations, events,
// revision tenant, jejak migrasi modul, dan advisory lock per tenant. Satu-satunya pemakai GORM untuk
// tabel lifecycle (D3b). Semua query memfilter tenant_id; record milik tenant lain = ErrNotFound (FR-119).
package store

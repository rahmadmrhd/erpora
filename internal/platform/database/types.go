package database

import "gorm.io/gorm"

// ControlDB adalah handle ke control plane (platform_state, registry modul).
type ControlDB struct{ *gorm.DB }

// TenantDB adalah handle ke data tenant; modul tidak pernah memegangnya langsung (lihat sdk/data).
type TenantDB struct{ *gorm.DB }

// DB adalah alias *gorm.DB agar paket di luar daftar D3b (mis. migrate, cmd) tidak perlu
// mengimpor gorm.io. Tipenya identik dengan gorm.DB.
type DB = gorm.DB

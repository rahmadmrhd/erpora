package module

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
)

// Module adalah kontrak utama yang diimplementasi setiap modul.
type Module interface {
	// Manifest mengembalikan manifest modul (biasanya dari module.json yang di-embed).
	Manifest() Manifest
	// Register mendaftarkan route dan command modul ke platform. Dipanggil saat boot.
	Register(r Registrar) error
}

// MigrationSource diimplementasi modul yang membawa migrasi goose sendiri.
type MigrationSource interface {
	// Migrations mengembalikan filesystem berisi berkas migrasi modul.
	Migrations() fs.FS
}

// ContractSource diimplementasi modul yang membawa kontrak API-nya (PRD-000 FR-23.1): isi
// modules/<dir>/openapi.yaml yang di-embed. Registry menyimpannya di Entry.Contract dan memvalidasinya.
type ContractSource interface {
	// OpenAPI mengembalikan dokumen OpenAPI 3.1 (YAML) modul.
	OpenAPI() []byte
}

// Seluruh hook lifecycle di bawah WAJIB idempoten (PRD-000 FR-19.5): platform
// dapat memanggilnya ulang setelah kegagalan sebagian.

// Installer dipanggil saat modul pertama kali dipasang.
type Installer interface {
	Install(ctx context.Context, env Env) error
}

// Upgrader dipanggil saat modul dinaikkan dari versi from ke versi to.
type Upgrader interface {
	Upgrade(ctx context.Context, env Env, from, to string) error
}

// EnableHook dipanggil setelah modul diaktifkan.
type EnableHook interface {
	OnEnable(ctx context.Context, env Env) error
}

// DisableHook dipanggil sebelum modul dinonaktifkan.
type DisableHook interface {
	OnDisable(ctx context.Context, env Env) error
}

// HealthChecker melaporkan kesehatan modul; error berarti tidak sehat.
type HealthChecker interface {
	Health(ctx context.Context, env Env) error
}

// DevSeeder mengisi data contoh untuk lingkungan pengembangan saja.
type DevSeeder interface {
	SeedDev(ctx context.Context, env Env) error
}

// Env adalah konteks terbatas untuk hook dan command. Akses data lewat
// data.DB(ctx); platform menjamin ctx yang diberikan ke hook/command membawa
// data.Provider tenant yang sama dengan Env.TenantID.
type Env struct {
	TenantID string
	Logger   *slog.Logger
}

// Flag mendeskripsikan satu flag command.
type Flag struct {
	Name    string
	Usage   string
	Default string
}

// Invocation adalah masukan satu eksekusi Command.
type Invocation struct {
	Args  []string
	Flags map[string]string
	Out   io.Writer
	Env   Env
}

// Command adalah perintah CLI yang disumbang modul. Run wajib diisi; registry
// menolak Command dengan Run nil.
type Command struct {
	Name  string
	Short string
	Flags []Flag
	Run   func(ctx context.Context, inv Invocation) error
}

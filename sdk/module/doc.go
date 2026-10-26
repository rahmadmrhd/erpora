// Package module berisi kontrak SDK publik yang dipakai modul ERP.
// Paket sdk lain yang boleh diimpor modul: erpora/sdk/data (akses database tenant aktif; hanya dari
// internal/infrastructure modul, aturan D3b) dan erpora/sdk/module/moduletest (helper test).
//
// Ringkasan kontrak (PRD-000 FR-05): Module (Manifest + Register), Registrar
// (Handle/Public/Command, diimplementasi platform), dan hook lifecycle opsional
// (MigrationSource, Installer, Upgrader, EnableHook, DisableHook, HealthChecker,
// DevSeeder) yang semuanya wajib idempoten.
package module

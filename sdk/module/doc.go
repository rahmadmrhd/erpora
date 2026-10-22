// Package module berisi kontrak SDK publik yang dipakai modul ERP.
// Ini satu-satunya paket di modul sdk yang boleh diimpor modul.
//
// Ringkasan kontrak (PRD-000 FR-05): Module (Manifest + Register), Registrar
// (Handle/Public/Command, diimplementasi platform), dan hook lifecycle opsional
// (MigrationSource, Installer, Upgrader, EnableHook, DisableHook, HealthChecker,
// DevSeeder) yang semuanya wajib idempoten.
package module

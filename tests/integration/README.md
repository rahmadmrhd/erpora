# tests/integration

Test integrasi tingkat biner dengan PostgreSQL nyata (testcontainers), dijalankan lewat `task test:integration` dengan
tag `integration`: `serve` (graceful shutdown, route uji), dan manajemen modul lewat CLI `erpora modules` (Task M2b).

Test lifecycle yang memakai perakitan in-process (modul referensi `example` dan `exampleaddon` lewat
migrasi platform, upgrade/downgrade, dependensi, API) ada di `internal/app/*_integration_test.go`, karena hanya
`internal/app` yang boleh mengimpor `modules/**` (aturan D7). Test engine dan store ada di
`internal/platform/modules/{lifecycle,store}` dan `internal/platform/migrate`.

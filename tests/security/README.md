# tests/security

Test keamanan lintas modul.

- `public_routes_test.go` memastikan setiap route modul punya permission atau ditandai `Public`, dan route `Public` persis sama dengan allowlist `public_routes.txt` (selisih dua arah dilaporkan). Menambah route publik berarti sengaja mengubah allowlist dan melewati review keamanan (PRD-000 FR-04).
- Isolasi tenant ditambahkan pada PRD-001.

Jalankan: `go test ./tests/security/...` (tanpa database).

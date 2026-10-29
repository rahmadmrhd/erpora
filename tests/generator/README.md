# tests/generator

Bukti bahwa modul hasil `erpora make:module` dan `make:handler` lolos semua gate kualitas dan dapat dipakai (PRD-000 FR-22 AC, exit gate M2).

`TestMakeModule_PassesAllGates` menyalin repo ke direktori sementara (tanpa `.git`, `node_modules`, `bin`, `tmp`, `contracts/dist`), membangun binary dev
dari salinan itu, membuat modul `inventory` (`--frontend --depends acme.example`) dan `stock-opname`, lalu menjalankan `task lint arch check:gowork-off test`,
`module:check`, `contracts:lint`, `contracts:generate --check`, `i18n:check --strict`, `i18n:generate --check`, typecheck/lint/deps frontend, `bun test` dan build shell, dan terakhir install + enable + `serve` terhadap PostgreSQL
(testcontainers). Repo asli tidak disentuh.

Jalankan dengan `task test:generator` (butuh Docker, Bun, golangci-lint, dan Go module cache; berjalan lama, batas 45 menit). Tag `generator` memilih paket ini
dan tag `integration` dibutuhkan helper `pgtest`. Test ini sengaja tidak termasuk `task test` atau `task test:integration`.

// Package devtools adalah titik masuk developer tools CLI erpora flavor dev (PRD-000 FR-22): command make:*,
// db:*, contracts:*, module:check, doctor, dan release:*. Hanya cmd/erpora/devtools_on.go (build tag devtools)
// yang boleh mengimpor package ini (aturan arch DEVTOOLS); binary release tidak memuatnya.
package devtools

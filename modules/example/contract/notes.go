// Package contract adalah port publik modul example untuk modul lain; tidak mengimpor internal/.
package contract

import "context"

// Notes adalah kontrak baca atas notes modul example.
//
// Count tidak menerima ID dan hanya mengembalikan agregat tenant aktif dari
// context. Pemeriksaan permission di dalam kontrak ditambahkan saat sdk/authz
// tersedia (PRD-001 FR-119).
type Notes interface {
	Count(ctx context.Context) (int64, error)
}

package runtime

import (
	"maps"
	"sync"

	"erpora/internal/platform/tenancy"
)

// platformKey adalah modul bawaan platform; selalu enabled.
const platformKey = "platform.core"

type tenantState struct {
	statuses map[string]Status
	revision int64
}

// Snapshot menyimpan status modul per tenant dan aman dipakai konkuren.
type Snapshot struct {
	mu      sync.RWMutex
	tenants map[tenancy.TenantID]tenantState
}

// NewSnapshot membuat snapshot kosong.
func NewSnapshot() *Snapshot {
	return &Snapshot{tenants: map[tenancy.TenantID]tenantState{}}
}

// Apply mengganti status tenant hanya bila revision > revision saat ini, atau tenant belum punya state
// (compare-and-set monoton). Mengembalikan false untuk pembaruan basi.
func (s *Snapshot) Apply(t tenancy.TenantID, statuses map[string]Status, revision int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.tenants[t]; ok && revision <= cur.revision {
		return false
	}
	s.tenants[t] = tenantState{statuses: maps.Clone(statuses), revision: revision}
	return true
}

// Statuses mengembalikan salinan status modul tenant beserta revision-nya.
func (s *Snapshot) Statuses(t tenancy.TenantID) (map[string]Status, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cur := s.tenants[t]
	return maps.Clone(cur.statuses), cur.revision
}

// Status mengembalikan status modul; platform.core selalu enabled dan modul
// yang tidak dikenal berstatus available.
func (s *Snapshot) Status(t tenancy.TenantID, key string) Status {
	if key == platformKey {
		return StatusEnabled
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if st, ok := s.tenants[t].statuses[key]; ok {
		return st
	}
	return StatusAvailable
}

// Revision mengembalikan revision snapshot tenant (0 bila belum ada).
func (s *Snapshot) Revision(t tenancy.TenantID) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.tenants[t].revision
}

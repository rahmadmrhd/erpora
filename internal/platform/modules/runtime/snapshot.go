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

// Set mengganti seluruh status modul tenant beserta revision-nya.
func (s *Snapshot) Set(t tenancy.TenantID, statuses map[string]Status, revision int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenants[t] = tenantState{statuses: maps.Clone(statuses), revision: revision}
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

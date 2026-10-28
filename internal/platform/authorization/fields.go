package authorization

import (
	"context"
	"log/slog"
	"maps"
	"sync"

	"erpora/sdk/authz"
)

// NewStubFieldMasker membuat authz.FieldMasker sementara sampai field-level access PRD-001 (FR-112).
// Mode allow mengembalikan salinan apa adanya dan mencatat INFO sekali per resource; mode lain
// (deny atau tidak dikenal) membuang semua field dan mencatat WARN (gagal tertutup).
func NewStubFieldMasker(mode StubMode, log *slog.Logger) authz.FieldMasker {
	return &stubMasker{mode: mode, log: log, seen: &sync.Map{}}
}

type stubMasker struct {
	mode StubMode
	log  *slog.Logger
	seen *sync.Map
}

func (m *stubMasker) MaskFields(ctx context.Context, resource string, fields map[string]any) map[string]any {
	log := m.log
	if log == nil {
		log = slog.Default()
	}
	if m.mode != "allow" {
		// WARN sekali per resource: ekspor memanggil masker per baris, jadi log per pemanggilan membanjiri keluaran.
		if _, seen := m.seen.LoadOrStore(resource, struct{}{}); !seen {
			log.WarnContext(ctx, "masking field stub: deny", "resource", resource, "mode", string(m.mode))
		}
		return map[string]any{}
	}
	if _, seen := m.seen.LoadOrStore(resource, struct{}{}); !seen {
		log.InfoContext(ctx, "masking field stub: allow", "resource", resource, "mode", string(m.mode))
	}
	return maps.Clone(fields)
}

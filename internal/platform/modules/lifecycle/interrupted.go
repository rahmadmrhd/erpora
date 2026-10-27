package lifecycle

import (
	"context"

	"erpora/internal/platform/tenancy"
)

// markInterrupted menandai operasi running sebagai failed/interrupted (keputusan 11). Diisi penuh di Task 8.
func (s *Service) markInterrupted(_ context.Context, _ tenancy.TenantID) error { return nil }

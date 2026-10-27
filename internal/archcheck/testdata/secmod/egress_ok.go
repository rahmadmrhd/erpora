package secfixture

import (
	"context"
	"net/http"

	"erpora/sdk/egress"
)

// Fixture: pemakaian yang sah, 0 pelanggaran. Method Get milik http.Header bukan http.Get.
func egressOK(ctx context.Context) {
	_, _ = egress.New()
	_, _ = http.NewRequestWithContext(ctx, http.MethodGet, "https://x", nil)
	h := http.Header{}
	_ = h.Get("X-Test")
}

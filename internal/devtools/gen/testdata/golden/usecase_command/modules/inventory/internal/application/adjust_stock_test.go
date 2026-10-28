package application

import (
	"context"
	"testing"
)

// adjustStockNopRepo memenuhi Repository tanpa perilaku; method yang tidak di-override panik bila dipanggil,
// sehingga test ini membuktikan use case tidak menyentuh repo di luar yang diharapkan.
type adjustStockNopRepo struct{ Repository }

func TestAdjustStockHandler_Handle(t *testing.T) {
	h := NewAdjustStockHandler(adjustStockNopRepo{})
	if err := h.Handle(context.Background(), AdjustStockCommand{}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

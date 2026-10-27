package application

import (
	"context"
	"testing"
)

// stockLevelNopRepo memenuhi Repository tanpa perilaku; method yang tidak di-override panik bila dipanggil,
// sehingga test ini membuktikan use case tidak menyentuh repo di luar yang diharapkan.
type stockLevelNopRepo struct{ Repository }

func TestStockLevelHandler_Handle(t *testing.T) {
	h := NewStockLevelHandler(stockLevelNopRepo{})
	if _, err := h.Handle(context.Background(), StockLevelQuery{}); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

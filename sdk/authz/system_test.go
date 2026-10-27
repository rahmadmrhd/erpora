package authz

import (
	"context"
	"testing"
)

func TestAsSystem_RoundTrip(t *testing.T) {
	r, ok := SystemReason(AsSystem(context.Background(), "x"))
	if !ok || r != "x" {
		t.Fatalf("SystemReason = %q, %v", r, ok)
	}
}

func TestSystemReason_PlainContext(t *testing.T) {
	if _, ok := SystemReason(context.Background()); ok {
		t.Fatal("ctx biasa tidak boleh membawa hak sistem")
	}
}

func TestAsSystem_EmptyReasonPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("AsSystem dengan reason kosong harus panic")
		}
	}()
	AsSystem(context.Background(), "")
}

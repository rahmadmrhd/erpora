package httpx

import (
	"context"
	"testing"
)

func TestClientIPRoundTrip(t *testing.T) {
	if got := ClientIP(context.Background()); got != "" {
		t.Fatalf("tanpa nilai, ClientIP = %q", got)
	}
	if got := ClientIP(WithClientIP(context.Background(), "203.0.113.7")); got != "203.0.113.7" {
		t.Fatalf("ClientIP = %q", got)
	}
}

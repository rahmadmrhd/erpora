package egress

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"
)

// M-7: MaxResponseBytes = MaxInt64 tidak boleh overflow pada remaining+1 (sebelumnya slice dengan batas negatif -> panic).
func TestLimitedBody_MaxInt64DoesNotOverflow(t *testing.T) {
	b := &limitedBody{rc: io.NopCloser(strings.NewReader("abcdef")), remaining: math.MaxInt64}
	got, err := io.ReadAll(b)
	if err != nil || string(got) != "abcdef" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestLimitedBody_ExceedingLimitStillDetected(t *testing.T) {
	for _, limit := range []int64{0, 1, 5, 6} {
		b := &limitedBody{rc: io.NopCloser(strings.NewReader("abcdef!")), remaining: limit}
		got, err := io.ReadAll(b)
		if !errors.Is(err, ErrResponseTooLarge) || int64(len(got)) != limit {
			t.Errorf("limit %d: got %q, %v", limit, got, err)
		}
	}
	// Tepat di batas: tidak melebihi.
	b := &limitedBody{rc: io.NopCloser(strings.NewReader("abc")), remaining: 3}
	if got, err := io.ReadAll(b); err != nil || string(got) != "abc" {
		t.Errorf("tepat di batas: %q, %v", got, err)
	}
}

func TestNew_AcceptsMaxInt64ResponseLimit(t *testing.T) {
	if _, err := New(Config{MaxResponseBytes: math.MaxInt64}); err != nil {
		t.Fatalf("New: %v", err)
	}
}

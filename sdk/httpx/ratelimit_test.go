package httpx

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTooManyRequests_RetryAfterRoundsUpToWholeSeconds(t *testing.T) {
	cases := []struct {
		name string
		d    time.Duration
		want string
	}{
		{"nol menjadi minimal 1", 0, "1"},
		{"negatif menjadi minimal 1", -5 * time.Second, "1"},
		{"sub-detik dibulatkan ke atas", 100 * time.Millisecond, "1"},
		{"tepat satu detik", time.Second, "1"},
		{"1,001 detik menjadi 2", time.Second + time.Millisecond, "2"},
		{"30 detik", 30 * time.Second, "30"},
		{"15 menit", 15 * time.Minute, "900"},
		{"1 jam", time.Hour, "3600"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			TooManyRequests(w, httptest.NewRequest("POST", "/api/core/auth/login", nil), tc.d)
			if w.Code != 429 {
				t.Fatalf("status = %d, want 429", w.Code)
			}
			if got := w.Header().Get("Retry-After"); got != tc.want {
				t.Errorf("Retry-After = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTooManyRequests_EnvelopeAndCode(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set(HeaderRequestID, "req-9")
	TooManyRequests(w, httptest.NewRequest("POST", "/x", nil), 2*time.Second)
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	body := w.Body.String()
	for _, want := range []string{`"error":{`, `"code":"rate_limited"`, `"requestId":"req-9"`} {
		if !strings.Contains(body, want) {
			t.Errorf("body tidak memuat %s: %s", want, body)
		}
	}
	if strings.Contains(body, `"data"`) {
		t.Errorf("amplop galat tidak boleh memuat data: %s", body)
	}
}

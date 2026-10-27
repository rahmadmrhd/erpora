package database

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func traceN(l *slogLogger, ctx context.Context, sql string, n int) {
	for range n {
		l.Trace(ctx, time.Now(), func() (string, int64) { return sql, 1 }, nil)
	}
}

func newTestLogger(buf *bytes.Buffer) *slogLogger {
	return &slogLogger{log: slog.New(slog.NewTextHandler(buf, nil))}
}

func TestNPlusOne_WarnsOncePerShapeAtThresholdPlusOne(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(&buf)
	ctx := WithQueryCounter(context.Background(), 10)

	traceN(l, ctx, "SELECT * FROM notes WHERE id = $1", 10)
	if strings.Contains(buf.String(), "indikasi N+1") {
		t.Fatalf("10 query (== threshold) belum boleh memperingatkan: %s", buf.String())
	}
	traceN(l, ctx, "SELECT * FROM notes WHERE id = $1", 1)
	if n := strings.Count(buf.String(), "indikasi N+1"); n != 1 || !strings.Contains(buf.String(), "count=11") || !strings.Contains(buf.String(), "threshold=10") {
		t.Fatalf("setelah 11 query: %s", buf.String())
	}
	traceN(l, ctx, "SELECT * FROM notes WHERE id = $1", 14) // total 25
	if n := strings.Count(buf.String(), "indikasi N+1"); n != 1 {
		t.Fatalf("25 query harus tetap satu WARN, dapat %d", n)
	}
}

func TestNPlusOne_TwoShapesTwoWarnings(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(&buf)
	ctx := WithQueryCounter(context.Background(), 10)
	traceN(l, ctx, "SELECT a FROM x WHERE id = $1", 11)
	traceN(l, ctx, "SELECT b FROM y WHERE id = $1", 11)
	if n := strings.Count(buf.String(), "indikasi N+1"); n != 2 {
		t.Fatalf("warnings = %d, want 2: %s", n, buf.String())
	}
}

func TestNPlusOne_NoCounterNoWarning(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(&buf)
	traceN(l, context.Background(), "SELECT 1 WHERE id = $1", 50)
	if buf.Len() != 0 {
		t.Fatalf("tanpa counter tidak boleh ada log: %s", buf.String())
	}
	if ctx := WithQueryCounter(context.Background(), 0); ctx != context.Background() {
		t.Error("threshold <= 0 harus mengembalikan ctx apa adanya")
	}
}

func TestNPlusOne_CountersArePerRequestContext(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(&buf)
	a := WithQueryCounter(context.Background(), 10)
	b := WithQueryCounter(context.Background(), 10)
	traceN(l, a, "SELECT 1 WHERE id = $1", 6)
	traceN(l, b, "SELECT 1 WHERE id = $1", 6)
	if buf.Len() != 0 {
		t.Fatalf("hitungan harus per context: %s", buf.String())
	}
}

func TestNPlusOne_LogDoesNotContainParameterValues(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(&buf)
	ctx := WithQueryCounter(context.Background(), 1)
	// fc hanya mengembalikan SQL berparameter (ParamsFilter membuang nilai); nilai rahasia tidak pernah tersedia.
	traceN(l, ctx, "SELECT * FROM users WHERE email = $1", 2)
	if strings.Contains(buf.String(), "secret") || !strings.Contains(buf.String(), "$1") {
		t.Fatalf("log = %s", buf.String())
	}
}

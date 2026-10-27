package database

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/trace/noop"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"erpora/internal/platform/config"
)

// Open harus menghormati ctx pada koneksi pertama (bukan ping otomatis GORM tanpa timeout).
func TestOpen_HonoursContextOnFirstConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Open(ctx, config.Config{
		DatabaseURL: "postgres://erp:S3cret@10.255.255.1:5432/erp?sslmode=disable",
		DBSlowQuery: 200 * time.Millisecond,
	}, noop.NewTracerProvider(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	if err == nil {
		t.Fatal("expected error")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("Open took %v; ctx deadline ignored", d)
	}
	msg := err.Error()
	if !strings.Contains(msg, "host=10.255.255.1") || strings.Contains(msg, "S3cret") {
		t.Fatalf("message = %q", msg)
	}
}

func TestConnError_RedactsAllDSNForms(t *testing.T) {
	cases := []struct {
		name, dsn, errText, wantHost string
		secret                       string
	}{
		{"url userinfo", "postgres://erp:hunter2@db:5432/erp", "dial db: password hunter2 rejected", "host=db:5432", "hunter2"},
		{"url query", "postgres://erp@db/erp?password=hunter2", "bad hunter2", "host=db", "hunter2"},
		{"keyword", "host=db.internal port=5432 user=erp password=hunter2", "failed hunter2", "host=db.internal:5432", "hunter2"},
		{"keyword quoted", "host=db user=erp password='hunter 2'", "x hunter 2 y", "host=db", "hunter 2"},
		{"short secret", "host=db password=a user=erp", "failed to connect: refused", "host=db", ""},
		{"unparseable", "garbage-hunter2", "boom hunter2", "host=?", "hunter2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := connError(c.dsn, errors.New(c.errText)).Error()
			if !strings.Contains(got, c.wantHost) {
				t.Errorf("got %q, want host %q", got, c.wantHost)
			}
			if c.secret != "" && strings.Contains(got, c.secret) {
				t.Errorf("leaks secret: %q", got)
			}
			if !strings.HasPrefix(got, "database tidak terjangkau (") {
				t.Errorf("prefix: %q", got)
			}
		})
	}
}

// Log GORM hanya memuat SQL berparameter, tidak pernah nilai parameter.
func TestSlogLogger_DoesNotLogParameterValues(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=x dbname=x"}), &gorm.Config{
		DisableAutomaticPing: true,
		DryRun:               true,
		Logger:               newSlogLogger(log, time.Nanosecond), // semua query dianggap lambat
	})
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	for i := 0; i < 500 && buf.Len() == 0; i++ { // resolusi timer kasar: ulangi sampai tercatat lambat
		db.Table("users").Where("email = ?", "victim@example.com").Find(&rows)
	}

	out := buf.String()
	if !strings.Contains(out, "users") {
		t.Fatalf("expected a slow-query log mentioning the table, got %q", out)
	}
	if strings.Contains(out, "victim@example.com") {
		t.Fatalf("parameter value leaked into log: %s", out)
	}
}

// Finding 1: password URL ber-percent-encoding tidak boleh muncul dalam bentuk mentah di pesan error.
func TestConnError_RedactsRawPercentEncodedPassword(t *testing.T) {
	dsn := "postgres://erp:p%40ss%3Aw%2Frd%25@db.local:5432/erp?sslmode=disable"
	cases := []string{
		`dial error: password p%40ss%3Aw%2Frd%25 rejected`, // bentuk mentah dari DSN
		`auth failed for p@ss:w/rd% at db.local`,           // bentuk terdekode
		`x p@ss:w%2Frd%25 y`,                               // bentuk PathEscape
	}
	for _, txt := range cases {
		got := connError(dsn, errors.New(txt)).Error()
		for _, leak := range []string{"p%40ss", "p@ss", "w%2Frd", "w/rd"} {
			if strings.Contains(got, leak) {
				t.Errorf("error %q -> %q leaks %q", txt, got, leak)
			}
		}
		if !strings.Contains(got, "host=db.local:5432") {
			t.Errorf("host hilang: %q", got)
		}
	}
}

// Finding 2: error driver dapat memuat literal input pengguna; log hanya memuat field aman.
func TestSlogLogger_ErrorLogHasOnlySafeFields(t *testing.T) {
	var buf bytes.Buffer
	l := newSlogLogger(slog.New(slog.NewTextHandler(&buf, nil)), time.Second)
	pgErr := &pgconn.PgError{
		Severity:       "ERROR",
		Code:           "22P02",
		Message:        `invalid input syntax for type uuid: "victim@example.com"`,
		Detail:         `Key (email)=(victim@example.com) already exists.`,
		TableName:      "users",
		ConstraintName: "users_email_key",
		ColumnName:     "email",
	}
	l.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT * FROM users WHERE id = $1", 0 },
		fmt.Errorf("wrapped: %w", pgErr))
	out := buf.String()
	if strings.Contains(out, "victim@example.com") {
		t.Fatalf("literal pengguna bocor ke log: %s", out)
	}
	for _, want := range []string{"22P02", "users", "users_email_key", "$1"} {
		if !strings.Contains(out, want) {
			t.Errorf("log tanpa %q: %s", want, out)
		}
	}

	buf.Reset()
	l.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 0 }, errors.New(`boom "secret-literal"`))
	if strings.Contains(buf.String(), "secret-literal") {
		t.Fatalf("error generik bocor: %s", buf.String())
	}
	if !strings.Contains(buf.String(), "database error") {
		t.Fatalf("log error tidak tercatat: %s", buf.String())
	}
}

// DSN yang tidak dapat di-parse tidak boleh membocorkan secret lewat jalur pgx.ParseConfig.
func TestOpenWith_DSNErrorRedacted(t *testing.T) {
	_, err := OpenWith(context.Background(), config.Config{DatabaseURL: "postgres://u:supersecret@[bad"},
		noop.NewTracerProvider(), slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		OpenOptions{StatementTimeout: time.Second})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "supersecret") {
		t.Fatalf("secret bocor: %q", err.Error())
	}
}

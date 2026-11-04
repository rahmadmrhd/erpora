package mail

import (
	"bytes"
	"context"
	"errors"
	"go/parser"
	"go/token"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	sdkmail "erpora/sdk/mail"
)

func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var b bytes.Buffer
	return slog.New(slog.NewJSONHandler(&b, &slog.HandlerOptions{Level: slog.LevelDebug})), &b
}

const resetMsgLink = "https://erp.example.com/reset-password#token=RAHASIA-TOKEN-123"

func resetMsg() sdkmail.Message {
	return sdkmail.Message{
		To:      []string{"budi.santoso@example.org"},
		Subject: "Atur ulang sandi",
		Text:    "Klik: " + resetMsgLink,
		HTML:    `<a href="` + resetMsgLink + `">atur ulang</a>`,
	}
}

// Produksi (bawaan): hanya penerima tersamar dan subjek; isi (tautan/token) tidak pernah ditulis.
func TestLog_DefaultRedactsRecipientAndNeverLogsBody(t *testing.T) {
	l, buf := captureLogger()
	s := NewLog(l)
	if err := s.Send(context.Background(), resetMsg()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, secret := range []string{"RAHASIA-TOKEN-123", "reset-password", "budi.santoso", "Klik:"} {
		if strings.Contains(out, secret) {
			t.Errorf("log memuat %q:\n%s", secret, out)
		}
	}
	for _, want := range []string{"Atur ulang sandi", "b***@example.org"} {
		if !strings.Contains(out, want) {
			t.Errorf("log harus memuat %q:\n%s", want, out)
		}
	}
}

// Development (WithBody eksplisit): isi penuh tampil agar tautan dapat dipakai tanpa SMTP.
func TestLog_WithBodyShowsFullContent(t *testing.T) {
	l, buf := captureLogger()
	s := NewLog(l, WithBody())
	if err := s.Send(context.Background(), resetMsg()); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "RAHASIA-TOKEN-123") || !strings.Contains(out, "budi.santoso@example.org") {
		t.Errorf("WithBody harus menulis isi dan penerima penuh:\n%s", out)
	}
}

// Driver wajib memvalidasi (kontrak sdk/mail.Sender): pesan tak sah ditolak dan tidak ditulis.
func TestLog_ValidatesMessages(t *testing.T) {
	l, buf := captureLogger()
	s := NewLog(l, WithBody())
	err := s.Send(context.Background(), sdkmail.Message{To: []string{"u@example.org"}, Subject: "a\r\nBcc: x@example.net", Text: "x"})
	if !errors.Is(err, sdkmail.ErrInvalidMessage) {
		t.Fatalf("err = %v, want ErrInvalidMessage", err)
	}
	if buf.Len() != 0 {
		t.Errorf("pesan tak sah tidak boleh dicatat: %s", buf.String())
	}
}

func TestMaskAddress(t *testing.T) {
	cases := map[string]string{
		"budi@example.org": "b***@example.org",
		"a@example.org":    "a***@example.org",
		"nodomain":         "***",
		"":                 "***",
	}
	for in, want := range cases {
		if got := maskAddress(in); got != want {
			t.Errorf("maskAddress(%q) = %q, want %q", in, got, want)
		}
	}
}

// Driver log tidak membuka soket: berkas sumbernya tidak mengimpor paket jaringan apa pun, dan tidak menyentuh
// konfigurasi SMTP. (Pengirim smtp ada di berkas lain pada paket yang sama.)
func TestLog_SourceImportsNoNetworkPackages(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "log.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		if p == "net" || strings.HasPrefix(p, "net/") || strings.Contains(p, "crypto/tls") || p == "os/exec" {
			t.Errorf("log.go mengimpor %q: driver log tidak boleh menyentuh jaringan", p)
		}
		if strings.HasPrefix(p, "erpora/internal/platform/config") {
			t.Errorf("log.go tidak boleh bergantung pada konfigurasi SMTP: %q", p)
		}
	}
}

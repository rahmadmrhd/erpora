package mail

import (
	"context"
	"log/slog"
	"strings"

	sdkmail "erpora/sdk/mail"
)

// LogOption mengatur driver log.
type LogOption func(*logSender)

// WithBody membuat driver log menulis penerima penuh dan isi (Text dan HTML) pesan. HANYA untuk development: isi
// surat reset sandi memuat tautan sekali pakai yang setara kredensial. Perakit memasangnya hanya bila ERP_ENV=development.
func WithBody() LogOption { return func(s *logSender) { s.body = true } }

type logSender struct {
	log  *slog.Logger
	body bool
}

// NewLog membuat pengirim "log": tidak membuka koneksi jaringan dan tidak mengirim apa pun; pesan hanya dicatat.
//
// Bawaan (aman untuk production): hanya penerima tersamar (huruf pertama bagian lokal + "***@domain"), subjek, dan
// ukuran isi; isi pesan, tautan, dan token tidak pernah ditulis. Dengan WithBody (development) penerima dan isi tampil
// penuh agar alur seperti reset sandi dapat dicoba tanpa server SMTP. Pesan divalidasi sdkmail.Validate lebih dulu
// (kontrak Sender): pesan tak sah ditolak dan tidak dicatat.
func NewLog(logger *slog.Logger, opts ...LogOption) sdkmail.Sender {
	if logger == nil {
		logger = slog.Default()
	}
	s := &logSender{log: logger}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Send mencatat pesan. Tidak ada I/O jaringan.
func (s *logSender) Send(ctx context.Context, m sdkmail.Message) error {
	if err := sdkmail.Validate(m); err != nil {
		return err
	}
	attrs := []any{"subject", m.Subject, "recipients", len(m.To), "text_bytes", len(m.Text), "html_bytes", len(m.HTML)}
	if s.body {
		attrs = append(attrs, "to", m.To, "text", m.Text, "html", m.HTML)
	} else {
		masked := make([]string, len(m.To))
		for i, to := range m.To {
			masked[i] = maskAddress(to)
		}
		attrs = append(attrs, "to", masked)
	}
	s.log.InfoContext(ctx, "mail: pesan dicatat oleh driver log (tidak dikirim)", attrs...)
	return nil
}

// maskAddress menyamarkan alamat: huruf pertama bagian lokal + "***@" + domain; selain itu "***".
func maskAddress(addr string) string {
	local, domain, ok := strings.Cut(addr, "@")
	if !ok || local == "" || domain == "" {
		return "***"
	}
	r := []rune(local)
	return string(r[0]) + "***@" + domain
}

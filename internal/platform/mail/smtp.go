package mail

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	netsmtp "net/smtp"
	"net/textproto"
	"strconv"
	"strings"
	"time"

	"erpora/internal/platform/config"
	sdkmail "erpora/sdk/mail"
)

// ErrSTARTTLSRequired berarti mode starttls dikonfigurasi tetapi server tidak menawarkan STARTTLS. Pengiriman dihentikan
// SEBELUM autentikasi dan sebelum amplop apa pun (tanpa downgrade ke teks polos).
var ErrSTARTTLSRequired = errors.New("mail: server SMTP tidak menawarkan STARTTLS (mode starttls mensyaratkannya)")

// Bawaan driver smtp (FR-108: sinkron dengan timeout dan retry singkat).
const (
	defaultConnectTimeout = 10 * time.Second
	defaultOpTimeout      = 30 * time.Second
	defaultMaxAttempts    = 3
	maxAttemptsCap        = 5
	backoffBase           = 300 * time.Millisecond
	backoffMax            = 2 * time.Second
	defaultHelloName      = "localhost"
)

// SMTPOption mengatur driver smtp (terutama untuk tes dan penyesuaian saat dijalankan).
type SMTPOption func(*smtpSender)

// WithRootCAs mengganti kumpulan CA tepercaya (mis. CA internal perusahaan atau sertifikat uji). Tidak ada opsi untuk
// melewati verifikasi sertifikat: verifikasi rantai dan nama host selalu menyala.
func WithRootCAs(pool *x509.CertPool) SMTPOption { return func(s *smtpSender) { s.roots = pool } }

// WithSleep mengganti penundaan antar-percobaan (tes). fn harus menghormati ctx.
func WithSleep(fn func(ctx context.Context, d time.Duration) error) SMTPOption {
	return func(s *smtpSender) { s.sleep = fn }
}

// WithTimeouts mengatur batas waktu sambung dan batas waktu total satu percobaan (sambung sampai QUIT).
func WithTimeouts(connect, operation time.Duration) SMTPOption {
	return func(s *smtpSender) {
		if connect > 0 {
			s.connectTimeout = connect
		}
		if operation > 0 {
			s.opTimeout = operation
		}
	}
}

// WithMaxAttempts mengatur jumlah percobaan maksimum (1..5; di luar itu dipotong).
func WithMaxAttempts(n int) SMTPOption {
	return func(s *smtpSender) { s.maxAttempts = min(max(n, 1), maxAttemptsCap) }
}

// WithHelloName mengatur nama pada EHLO (bawaan "localhost": tidak membocorkan nama host).
func WithHelloName(name string) SMTPOption {
	return func(s *smtpSender) {
		if name != "" {
			s.hello = name
		}
	}
}

// WithSMTPLogger mengatur logger peringatan retry (tanpa alamat penerima, isi, atau kredensial).
func WithSMTPLogger(l *slog.Logger) SMTPOption {
	return func(s *smtpSender) {
		if l != nil {
			s.log = l
		}
	}
}

type smtpSender struct {
	cfg            config.SMTPConfig // Password bertipe config.Secret: tersamar pada fmt/JSON/slog
	roots          *x509.CertPool
	sleep          func(ctx context.Context, d time.Duration) error
	connectTimeout time.Duration
	opTimeout      time.Duration
	maxAttempts    int
	hello          string
	log            *slog.Logger
	now            func() time.Time
}

// NewSMTP membuat pengirim SMTP dari konfigurasi. Galat bila konfigurasi tidak sah: host/from kosong, from bukan alamat
// polos, port di luar 1..65535, mode TLS tak dikenal, username tanpa sandi (atau sebaliknya), atau mode none bersama
// autentikasi (kredensial tidak pernah dikirim tanpa TLS). Pesan galat tidak pernah memuat sandi.
//
// Perilaku: mode starttls (bawaan) mensyaratkan STARTTLS (ErrSTARTTLSRequired bila tidak ditawarkan), mode tls memakai
// TLS implisit, mode none (hanya development; config menolaknya di production) tanpa TLS dan tanpa autentikasi.
// Autentikasi PLAIN atau LOGIN hanya setelah sambungan TLS terverifikasi, diperiksa sendiri (bukan mengandalkan
// pengecualian localhost net/smtp.PlainAuth). Sertifikat server diverifikasi terhadap Host konfigurasi. Setiap pesan
// divalidasi sdkmail.Validate sebelum jaringan disentuh. Retry hanya untuk kegagalan sementara (sambung gagal, putus,
// balasan 4xx), dibatasi, dengan jeda yang berlipat dua; 5xx, galat sertifikat/TLS, dan galat autentikasi tidak di-retry.
func NewSMTP(cfg config.SMTPConfig, opts ...SMTPOption) (sdkmail.Sender, error) {
	switch {
	case strings.TrimSpace(cfg.Host) == "" || strings.ContainsAny(cfg.Host, " \t\r\n/@:"):
		return nil, errors.New("mail: host SMTP tidak sah")
	case cfg.Port < 1 || cfg.Port > 65535:
		return nil, errors.New("mail: port SMTP harus 1..65535")
	}
	switch cfg.TLSMode {
	case config.SMTPTLSStartTLS, config.SMTPTLSImplicit, config.SMTPTLSNone:
	default:
		return nil, errors.New("mail: mode TLS SMTP harus starttls, tls, atau none")
	}
	if err := sdkmail.Validate(sdkmail.Message{To: []string{cfg.From}, Subject: "x", Text: "x"}); err != nil {
		return nil, errors.New("mail: alamat pengirim SMTP bukan alamat email polos yang sah")
	}
	if (cfg.Username == "") != (len(cfg.Password) == 0) {
		return nil, errors.New("mail: username dan sandi SMTP harus diisi bersama atau dikosongkan bersama")
	}
	if cfg.TLSMode == config.SMTPTLSNone && cfg.Username != "" {
		return nil, errors.New("mail: autentikasi SMTP tanpa TLS ditolak (mode none)")
	}
	s := &smtpSender{
		cfg: cfg, connectTimeout: defaultConnectTimeout, opTimeout: defaultOpTimeout, maxAttempts: defaultMaxAttempts,
		hello: defaultHelloName, log: slog.Default(), now: time.Now, sleep: sleepCtx,
	}
	for _, o := range opts {
		o(s)
	}
	return s, nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Send memvalidasi m, menyusun pesan MIME, lalu mengirimnya (sinkron) dengan retry singkat atas kegagalan sementara.
func (s *smtpSender) Send(ctx context.Context, m sdkmail.Message) error {
	data, err := buildMessage(s.cfg.From, m, s.now()) // juga memanggil Validate: tak ada jaringan sebelum lolos
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 1; attempt <= s.maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = s.deliver(ctx, m.To, data)
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !retryable(lastErr) || attempt == s.maxAttempts {
			break
		}
		s.log.WarnContext(ctx, "mail: pengiriman SMTP gagal sementara, mencoba lagi", "attempt", attempt, "error", lastErr)
		d := min(backoffBase<<(attempt-1), backoffMax)
		if err := s.sleep(ctx, d); err != nil {
			return err
		}
	}
	return fmt.Errorf("mail: kirim SMTP gagal: %w", lastErr)
}

// retryable: hanya kegagalan sementara. Galat sertifikat/TLS, STARTTLS tak ditawarkan, autentikasi, dan 5xx tidak.
func retryable(err error) bool {
	if errors.Is(err, ErrSTARTTLSRequired) || errors.Is(err, context.Canceled) {
		return false
	}
	var ua x509.UnknownAuthorityError
	var ci x509.CertificateInvalidError
	var he x509.HostnameError
	if errors.As(err, &ua) || errors.As(err, &ci) || errors.As(err, &he) {
		return false
	}
	var rerr tls.RecordHeaderError
	if errors.As(err, &rerr) {
		return false
	}
	var tpe *textproto.Error
	if errors.As(err, &tpe) {
		return tpe.Code >= 400 && tpe.Code < 500
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF)
}

func (s *smtpSender) tlsConfig() *tls.Config {
	return &tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12, RootCAs: s.roots}
}

// deliver melakukan satu percobaan pengiriman penuh.
func (s *smtpSender) deliver(ctx context.Context, to []string, data []byte) (err error) {
	addr := net.JoinHostPort(s.cfg.Host, strconv.Itoa(s.cfg.Port))
	d := net.Dialer{Timeout: s.connectTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	deadline := s.now().Add(s.opTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)
	// ctx batal: tutup sambungan agar pembacaan/penulisan yang menggantung segera berakhir.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	if s.cfg.TLSMode == config.SMTPTLSImplicit {
		tc := tls.Client(conn, s.tlsConfig())
		if err := tc.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return err
		}
		conn = tc
	}
	c, err := netsmtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() {
		if err != nil {
			_ = c.Close()
		}
	}()
	if err = c.Hello(s.hello); err != nil {
		return err
	}
	if s.cfg.TLSMode == config.SMTPTLSStartTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return ErrSTARTTLSRequired
		}
		if err = c.StartTLS(s.tlsConfig()); err != nil {
			return err
		}
	}
	if s.cfg.Username != "" {
		// Kredensial hanya melalui TLS yang sudah terverifikasi; diperiksa di sini karena net/smtp.PlainAuth
		// sengaja mengizinkan teks polos ke localhost.
		if _, ok := c.TLSConnectionState(); !ok {
			return errors.New("mail: autentikasi SMTP ditolak tanpa TLS")
		}
		var auth netsmtp.Auth
		if auth, err = s.pickAuth(c); err != nil {
			return err
		}
		if err = c.Auth(auth); err != nil {
			return err
		}
	}
	if err = c.Mail(s.cfg.From); err != nil {
		return err
	}
	for _, r := range to {
		if err = c.Rcpt(r); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err = w.Write(data); err != nil {
		_ = w.Close()
		return err
	}
	if err = w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// pickAuth memilih PLAIN (utama) atau LOGIN menurut yang diiklankan server.
func (s *smtpSender) pickAuth(c *netsmtp.Client) (netsmtp.Auth, error) {
	_, params := c.Extension("AUTH")
	var plain, login bool
	for _, m := range strings.Fields(strings.ToUpper(params)) {
		switch m {
		case "PLAIN":
			plain = true
		case "LOGIN":
			login = true
		}
	}
	switch {
	case plain:
		return &tlsOnlyAuth{inner: netsmtp.PlainAuth("", s.cfg.Username, string(s.cfg.Password), s.cfg.Host)}, nil
	case login:
		return &tlsOnlyAuth{inner: &loginAuth{user: s.cfg.Username, pass: string(s.cfg.Password)}}, nil
	}
	return nil, errors.New("mail: server SMTP tidak menawarkan AUTH PLAIN atau LOGIN")
}

// tlsOnlyAuth membungkus mekanisme auth dan menolak Start bila sambungan bukan TLS (lapis kedua setelah pemeriksaan
// TLSConnectionState).
type tlsOnlyAuth struct{ inner netsmtp.Auth }

func (a *tlsOnlyAuth) Start(si *netsmtp.ServerInfo) (string, []byte, error) {
	if !si.TLS {
		return "", nil, errors.New("mail: autentikasi SMTP ditolak tanpa TLS")
	}
	return a.inner.Start(si)
}

func (a *tlsOnlyAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	return a.inner.Next(fromServer, more)
}

// loginAuth mengimplementasikan AUTH LOGIN (tantangan Username:/Password:). Tidak pernah dimasukkan ke galat.
type loginAuth struct{ user, pass string }

func (a *loginAuth) Start(*netsmtp.ServerInfo) (string, []byte, error) { return "LOGIN", nil, nil }

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:":
		return []byte(a.user), nil
	case "password:":
		return []byte(a.pass), nil
	}
	return nil, errors.New("mail: tantangan AUTH LOGIN tidak dikenal")
}

// String menyamarkan pengirim pada pencetakan (konfigurasi memuat Secret yang sudah tersamar; ini lapis tambahan).
func (s *smtpSender) String() string {
	return fmt.Sprintf("smtp(%s:%d, mode=%s, from=%s, password=%s)", s.cfg.Host, s.cfg.Port, s.cfg.TLSMode, s.cfg.From, s.cfg.Password)
}

// GoString sama dengan String.
func (s *smtpSender) GoString() string { return s.String() }

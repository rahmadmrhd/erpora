package mail

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	"erpora/internal/platform/config"
	sdkmail "erpora/sdk/mail"
)

const smtpPass = "S3cr3t-sandi-smtp-uji"

type sleeps struct {
	mu sync.Mutex
	d  []time.Duration
}

func (s *sleeps) fn(_ context.Context, d time.Duration) error {
	s.mu.Lock()
	s.d = append(s.d, d)
	s.mu.Unlock()
	return nil
}

func (s *sleeps) count() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.d) }

func baseCfg(host string, port int, mode string) config.SMTPConfig {
	return config.SMTPConfig{Host: host, Port: port, From: "noreply@example.com", TLSMode: mode}
}

func authCfg(host string, port int, mode string) config.SMTPConfig {
	c := baseCfg(host, port, mode)
	c.Username, c.Password = "mailer", config.Secret(smtpPass)
	return c
}

func newSender(t *testing.T, cfg config.SMTPConfig, opts ...SMTPOption) sdkmail.Sender {
	t.Helper()
	s, err := NewSMTP(cfg, opts...)
	if err != nil {
		t.Fatalf("NewSMTP: %v", err)
	}
	return s
}

func msg() sdkmail.Message {
	return sdkmail.Message{To: []string{"user@example.org"}, Subject: "Atur ulang sandi", Text: "Halo,\nklik tautan.\n", HTML: "<p>Halo</p>"}
}

func parse(t *testing.T, data string) (*netmail.Message, string) {
	t.Helper()
	m, err := netmail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatalf("ReadMessage: %v\n%s", err, data)
	}
	b, _ := io.ReadAll(m.Body)
	return m, string(b)
}

func TestSMTP_STARTTLSDeliversWithAuthOnlyAfterTLS(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, func(s *fakeSMTP) {
		s.wantUser, s.wantPass = "mailer", smtpPass
		s.advertiseAuthPlain = true // server mengiklankan AUTH sebelum TLS: klien tetap tidak boleh memakainya
	})
	host, port := srv.addr()
	s := newSender(t, authCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool))
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	_, events, mails := srv.snapshot()
	if len(mails) != 1 {
		t.Fatalf("surat diterima = %d, want 1", len(mails))
	}
	if mails[0].From != "noreply@example.com" || len(mails[0].To) != 1 || mails[0].To[0] != "user@example.org" {
		t.Errorf("amplop = %+v", mails[0])
	}
	sawAuth := false
	for _, e := range events {
		up := strings.ToUpper(e.Line)
		if strings.HasPrefix(up, "AUTH") || strings.HasPrefix(up, "MAIL") || strings.HasPrefix(up, "RCPT") {
			sawAuth = sawAuth || strings.HasPrefix(up, "AUTH")
			if !e.TLS {
				t.Errorf("perintah %q dikirim sebelum TLS", strings.SplitN(e.Line, " ", 2)[0])
			}
		}
	}
	if !sawAuth {
		t.Error("AUTH tidak dikirim")
	}

	m, body := parse(t, mails[0].Data)
	if got := m.Header.Get("From"); got != "noreply@example.com" {
		t.Errorf("From = %q", got)
	}
	if got := m.Header.Get("To"); got != "user@example.org" {
		t.Errorf("To = %q", got)
	}
	if got := m.Header.Get("Subject"); got != "Atur ulang sandi" {
		t.Errorf("Subject = %q", got)
	}
	if m.Header.Get("Message-Id") == "" || m.Header.Get("Date") == "" || m.Header.Get("Mime-Version") != "1.0" {
		t.Errorf("header wajib kurang: %v", m.Header)
	}
	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("Content-Type = %q err=%v", m.Header.Get("Content-Type"), err)
	}
	mr := multipart.NewReader(strings.NewReader(body), params["boundary"])
	var got []string
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		// multipart.Part sudah men-decode quoted-printable dan membuang header CTE-nya.
		b, _ := io.ReadAll(p)
		got = append(got, p.Header.Get("Content-Type")+"|"+string(b))
	}
	if len(got) != 2 || !strings.HasPrefix(got[0], "text/plain; charset=utf-8|Halo,\r\nklik tautan.") || !strings.HasPrefix(got[1], "text/html; charset=utf-8|<p>Halo</p>") {
		t.Errorf("bagian = %q", got)
	}
}

func TestSMTP_ImplicitTLS(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, func(s *fakeSMTP) {
		s.implicitTLS, s.offerSTARTTLS = true, false
		s.wantUser, s.wantPass = "mailer", smtpPass
		s.authMechs = []string{"LOGIN"} // jalur LOGIN
	})
	host, port := srv.addr()
	s := newSender(t, authCfg(host, port, config.SMTPTLSImplicit), WithRootCAs(pool))
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, _, mails := srv.snapshot(); len(mails) != 1 {
		t.Fatalf("surat = %d", len(mails))
	}
}

func TestSMTP_PlainTextOnlyAndLoneLF(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, nil)
	host, port := srv.addr()
	s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool))
	m := sdkmail.Message{To: []string{"a@example.org", "b@example.org"}, Subject: "x", Text: "baris1\nbaris2\n.\nakhir"}
	if err := s.Send(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	_, _, mails := srv.snapshot()
	if len(mails) != 1 || len(mails[0].To) != 2 {
		t.Fatalf("surat = %+v", mails)
	}
	msgd, body := parse(t, mails[0].Data)
	if mt, _, _ := mime.ParseMediaType(msgd.Header.Get("Content-Type")); mt != "text/plain" {
		t.Errorf("Content-Type = %q", msgd.Header.Get("Content-Type"))
	}
	dec, _ := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if string(dec) != "baris1\r\nbaris2\r\n.\r\nakhir\r\n" {
		t.Errorf("isi = %q (titik sebaris sendiri harus selamat dari dot-stuffing)", dec)
	}
	if got := msgd.Header.Get("To"); got != "a@example.org, b@example.org" {
		t.Errorf("To = %q", got)
	}
}

func TestSMTP_NonASCIIAndLongSubjectEncoded(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, nil)
	host, port := srv.addr()
	s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool))
	subjects := []string{
		"Atur ulang sandi ERPora — sekarang",
		strings.Repeat("Pemberitahuan penting ", 40),       // ASCII panjang
		strings.Repeat("日本語の件名", 40),                       // multibyte panjang
		"=?utf-8?b?aGFjaw==?= tampak seperti encoded-word", // ASCII berisi =?
	}
	for _, subj := range subjects {
		if err := s.Send(context.Background(), sdkmail.Message{To: []string{"u@example.org"}, Subject: subj, Text: "x"}); err != nil {
			t.Fatalf("subjek %.20q: %v", subj, err)
		}
	}
	_, _, mails := srv.snapshot()
	if len(mails) != len(subjects) {
		t.Fatalf("surat = %d", len(mails))
	}
	dec := new(mime.WordDecoder)
	for i, ml := range mails {
		header, _, _ := strings.Cut(ml.Data, "\r\n\r\n")
		for _, line := range strings.Split(header, "\r\n") {
			if len(line) > 78 {
				t.Errorf("subjek %d: baris header %d byte > 78: %.40q", i, len(line), line)
			}
		}
		m, _ := parse(t, ml.Data)
		got, err := dec.DecodeHeader(m.Header.Get("Subject"))
		if err != nil || got != subjects[i] {
			t.Errorf("subjek %d: decode = %.50q err=%v, want %.50q", i, got, err, subjects[i])
		}
		if strings.Count(header, "\r\nSubject:") != 1 && !strings.HasPrefix(header, "Subject:") {
			// tepat satu header Subject
			if strings.Count(strings.ToLower(header), "subject:") != 1 {
				t.Errorf("subjek %d: header Subject ganda", i)
			}
		}
	}
}

func TestSMTP_BoundaryRandomPerMessage(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, nil)
	host, port := srv.addr()
	s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool))
	for range 2 {
		if err := s.Send(context.Background(), msg()); err != nil {
			t.Fatal(err)
		}
	}
	_, _, mails := srv.snapshot()
	var bs, ids []string
	for _, ml := range mails {
		m, _ := parse(t, ml.Data)
		_, p, _ := mime.ParseMediaType(m.Header.Get("Content-Type"))
		bs = append(bs, p["boundary"])
		ids = append(ids, m.Header.Get("Message-Id"))
	}
	if len(bs) != 2 || bs[0] == bs[1] || len(bs[0]) < 32 || ids[0] == ids[1] {
		t.Errorf("boundary/Message-ID harus acak per pesan: %v %v", bs, ids)
	}
}

func TestSMTP_RejectsWhenSTARTTLSNotOffered(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, func(s *fakeSMTP) {
		s.offerSTARTTLS = false
		s.advertiseAuthPlain = true
		s.wantUser, s.wantPass = "mailer", smtpPass
	})
	host, port := srv.addr()
	s := newSender(t, authCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool), WithSleep(new(sleeps).fn))
	err := s.Send(context.Background(), msg())
	if !errors.Is(err, ErrSTARTTLSRequired) {
		t.Fatalf("err = %v, want ErrSTARTTLSRequired", err)
	}
	for _, v := range []string{"AUTH", "MAIL", "RCPT", "DATA"} {
		if srv.sawVerb(v) {
			t.Errorf("klien mengirim %s tanpa TLS (downgrade)", v)
		}
	}
	if strings.Contains(err.Error(), smtpPass) {
		t.Errorf("galat memuat sandi: %v", err)
	}
}

func TestSMTP_CertificateVerifiedByDefault(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.wantUser, s.wantPass = "mailer", smtpPass })
	host, port := srv.addr()
	// Tanpa RootCAs uji: sertifikat self-signed harus ditolak (verifikasi bawaan menyala) dan tidak ada kredensial terkirim.
	sl := new(sleeps)
	s := newSender(t, authCfg(host, port, config.SMTPTLSStartTLS), WithSleep(sl.fn))
	err := s.Send(context.Background(), msg())
	var ua x509.UnknownAuthorityError
	if err == nil || !errors.As(err, &ua) {
		t.Fatalf("err = %v, want x509.UnknownAuthorityError", err)
	}
	if srv.sawVerb("AUTH") || srv.sawVerb("MAIL") {
		t.Error("kredensial/surat terkirim melalui TLS yang tidak terverifikasi")
	}
	if sl.count() != 0 {
		t.Errorf("galat sertifikat tidak boleh di-retry (tidur %d kali)", sl.count())
	}
	if a, _, _ := srv.snapshot(); a != 1 {
		t.Errorf("koneksi = %d, galat sertifikat tidak boleh di-retry", a)
	}
	// Dengan pool uji: berhasil.
	s = newSender(t, authCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool))
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatalf("dengan RootCAs uji: %v", err)
	}
}

func TestSMTP_NoneModeRulesAndDevDelivery(t *testing.T) {
	if _, err := NewSMTP(authCfg("127.0.0.1", 25, config.SMTPTLSNone)); err == nil {
		t.Fatal("mode none + username harus ditolak (kredensial tanpa TLS)")
	}
	cert, _ := testCert(t)
	srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.offerSTARTTLS = false })
	host, port := srv.addr()
	s := newSender(t, baseCfg(host, port, config.SMTPTLSNone))
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatalf("none dev: %v", err)
	}
	if srv.sawVerb("AUTH") || srv.sawVerb("STARTTLS") {
		t.Error("mode none tidak boleh memakai AUTH atau STARTTLS")
	}
}

func TestNewSMTP_RejectsBadConfig(t *testing.T) {
	good := baseCfg("smtp.example.com", 587, config.SMTPTLSStartTLS)
	mods := map[string]func(*config.SMTPConfig){
		"host kosong":      func(c *config.SMTPConfig) { c.Host = "" },
		"from kosong":      func(c *config.SMTPConfig) { c.From = "" },
		"from bernama":     func(c *config.SMTPConfig) { c.From = "ERPora <noreply@example.com>" },
		"from CRLF":        func(c *config.SMTPConfig) { c.From = "a@example.com\r\nBcc: b@example.com" },
		"port 0":           func(c *config.SMTPConfig) { c.Port = 0 },
		"mode tak dikenal": func(c *config.SMTPConfig) { c.TLSMode = "ssl" },
		"user tanpa sandi": func(c *config.SMTPConfig) { c.Username = "u" },
	}
	for name, mod := range mods {
		c := good
		mod(&c)
		if _, err := NewSMTP(c); err == nil {
			t.Errorf("%s: diterima, harus ditolak", name)
		}
	}
	if _, err := NewSMTP(good); err != nil {
		t.Errorf("konfigurasi sah ditolak: %v", err)
	}
}

// Header injection: Validate menolak sebelum menyentuh jaringan (tidak ada koneksi masuk ke server).
func TestSMTP_HeaderInjectionRejectedBeforeNetwork(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, nil)
	host, port := srv.addr()
	s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool))
	bad := []sdkmail.Message{
		{To: []string{"u@example.org"}, Subject: "Halo\r\nBcc: attacker@example.net", Text: "x"},
		{To: []string{"u@example.org"}, Subject: "Halo\nBcc: attacker@example.net", Text: "x"},
		{To: []string{"u@example.org"}, Subject: "Halo Bcc: attacker@example.net", Text: "x"},
		{To: []string{"u@example.org\r\nBcc: attacker@example.net"}, Subject: "x", Text: "x"},
		{To: []string{"u@example.org>\r\nRCPT TO:<attacker@example.net"}, Subject: "x", Text: "x"},
		{To: []string{"Nama <u@example.org>"}, Subject: "x", Text: "x"},
		{To: []string{"u@example.org,attacker@example.net"}, Subject: "x", Text: "x"},
		{To: []string{"u@example.org"}, Subject: "x\x00", Text: "x"},
		{To: nil, Subject: "x", Text: "x"},
	}
	for i, m := range bad {
		err := s.Send(context.Background(), m)
		if !errors.Is(err, sdkmail.ErrInvalidMessage) {
			t.Errorf("kasus %d: err = %v, want ErrInvalidMessage", i, err)
		}
	}
	if a, _, _ := srv.snapshot(); a != 0 {
		t.Errorf("server menerima %d koneksi: pesan tidak sah menyentuh jaringan", a)
	}
}

func TestSMTP_PasswordNeverInErrorsOrFormatting(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.wantUser, s.wantPass = "mailer", "sandi-lain" })
	host, port := srv.addr()
	sl := new(sleeps)
	s := newSender(t, authCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool), WithSleep(sl.fn))
	err := s.Send(context.Background(), msg())
	if err == nil {
		t.Fatal("auth salah harus gagal")
	}
	if strings.Contains(err.Error(), smtpPass) {
		t.Errorf("galat memuat sandi: %v", err)
	}
	if sl.count() != 0 {
		t.Error("kegagalan auth (5xx) tidak boleh di-retry")
	}
	for _, f := range []string{"%v", "%+v", "%#v", "%s"} {
		if out := fmt.Sprintf(f, s); strings.Contains(out, smtpPass) {
			t.Errorf("format %s membocorkan sandi: %s", f, out)
		}
	}
}

func TestSMTP_RetryTransientBounded(t *testing.T) {
	cert, pool := testCert(t)
	t.Run("451 lalu sukses: satu surat", func(t *testing.T) {
		srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.dataReplies = []string{"451 coba lagi"} })
		host, port := srv.addr()
		sl := new(sleeps)
		s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool), WithSleep(sl.fn))
		if err := s.Send(context.Background(), msg()); err != nil {
			t.Fatal(err)
		}
		if _, _, mails := srv.snapshot(); len(mails) != 1 {
			t.Errorf("surat = %d, want 1", len(mails))
		}
		if sl.count() != 1 {
			t.Errorf("tidur = %d, want 1 (jeda sebelum percobaan kedua)", sl.count())
		}
	})
	t.Run("transien terus-menerus: dibatasi", func(t *testing.T) {
		srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.dataReplies = []string{"451 a", "451 b", "451 c", "451 d", "451 e"} })
		host, port := srv.addr()
		sl := new(sleeps)
		s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool), WithSleep(sl.fn), WithMaxAttempts(3))
		if err := s.Send(context.Background(), msg()); err == nil {
			t.Fatal("harus gagal setelah percobaan habis")
		}
		if a, _, _ := srv.snapshot(); a != 3 {
			t.Errorf("koneksi = %d, want tepat 3 percobaan", a)
		}
	})
	t.Run("5xx permanen tidak di-retry", func(t *testing.T) {
		srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.rcptReply = "550 no such user" })
		host, port := srv.addr()
		sl := new(sleeps)
		s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool), WithSleep(sl.fn))
		if err := s.Send(context.Background(), msg()); err == nil {
			t.Fatal("harus gagal")
		}
		if a, _, _ := srv.snapshot(); a != 1 || sl.count() != 0 {
			t.Errorf("koneksi = %d tidur = %d, 5xx tidak boleh di-retry", a, sl.count())
		}
	})
	t.Run("gagal sambung di-retry lalu menyerah", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close() // port tertutup: dial ditolak
		sl := new(sleeps)
		s := newSender(t, baseCfg("127.0.0.1", port, config.SMTPTLSStartTLS), WithSleep(sl.fn), WithMaxAttempts(3))
		if err := s.Send(context.Background(), msg()); err == nil {
			t.Fatal("harus gagal")
		}
		if sl.count() != 2 {
			t.Errorf("tidur = %d, want 2 (di antara 3 percobaan)", sl.count())
		}
	})
	t.Run("ctx batal menghentikan retry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s := newSender(t, baseCfg("127.0.0.1", 1, config.SMTPTLSStartTLS))
		if err := s.Send(ctx, msg()); !errors.Is(err, context.Canceled) {
			t.Errorf("err = %v, want context.Canceled", err)
		}
	})
}

func TestSMTP_OperationTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() { // server yang menerima lalu diam (tanpa banner)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			defer c.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	s := newSender(t, baseCfg("127.0.0.1", port, config.SMTPTLSStartTLS), WithTimeouts(time.Second, 200*time.Millisecond), WithMaxAttempts(1))
	start := time.Now()
	if err := s.Send(context.Background(), msg()); err == nil {
		t.Fatal("server diam harus menghasilkan galat")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("timeout operasi tidak ditegakkan: %v", d)
	}
}

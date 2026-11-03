package mail

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"erpora/internal/platform/config"
)

const leakAddr = "budi@x.co"

func bufLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// B-1: teks balasan server (sering menggemakan alamat penerima) tidak boleh masuk galat maupun log.
func TestSanitizeErrKeepsOnlySMTPCode(t *testing.T) {
	for _, code := range []int{550, 450} {
		in := fmt.Errorf("rcpt: %w", &textproto.Error{Code: code, Msg: "5.1.1 <" + leakAddr + ">: User unknown"})
		out := sanitizeErr(in)
		var re *ReplyError
		if !errors.As(out, &re) || re.Code != code {
			t.Fatalf("code %d: %T %v", code, out, out)
		}
		if strings.Contains(out.Error(), leakAddr) || !strings.Contains(out.Error(), fmt.Sprint(code)) {
			t.Errorf("galat bersih = %q", out)
		}
		if retryable(out) != (code < 500) {
			t.Errorf("retryable(%d) = %v", code, retryable(out))
		}
	}
	if sanitizeErr(nil) != nil {
		t.Error("nil harus tetap nil")
	}
	plain := errors.New("galat jaringan")
	if sanitizeErr(plain) != plain {
		t.Error("galat non-SMTP tidak boleh diubah")
	}
}

func TestSMTP_ServerReplyTextNeverLoggedOrReturned(t *testing.T) {
	cert, pool := testCert(t)
	t.Run("550 permanen", func(t *testing.T) {
		srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.rcptReply = "550 5.1.1 <" + leakAddr + ">: User unknown" })
		host, port := srv.addr()
		var logs bytes.Buffer
		s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool), WithSleep(new(sleeps).fn), WithSMTPLogger(bufLogger(&logs)))
		err := s.Send(context.Background(), msg())
		if err == nil {
			t.Fatal("harus gagal")
		}
		if strings.Contains(err.Error(), leakAddr) || strings.Contains(logs.String(), leakAddr) {
			t.Fatalf("alamat bocor: err=%q logs=%q", err, logs.String())
		}
		var re *ReplyError
		if !errors.As(err, &re) || re.Code != 550 {
			t.Errorf("kode SMTP harus tersedia: %v", err)
		}
	})
	t.Run("450 greylisting di-retry dan dicatat tanpa alamat", func(t *testing.T) {
		srv := newFakeSMTP(t, cert, func(s *fakeSMTP) {
			s.dataReplies = []string{"450 4.2.0 <" + leakAddr + "> greylisted", "450 4.2.0 <" + leakAddr + "> greylisted", "450 4.2.0 <" + leakAddr + "> greylisted"}
		})
		host, port := srv.addr()
		var logs bytes.Buffer
		s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool), WithSleep(new(sleeps).fn), WithSMTPLogger(bufLogger(&logs)), WithMaxAttempts(3))
		err := s.Send(context.Background(), msg())
		if err == nil {
			t.Fatal("harus gagal setelah percobaan habis")
		}
		if !strings.Contains(logs.String(), "450") {
			t.Errorf("log retry harus memuat kode SMTP: %q", logs.String())
		}
		if strings.Contains(err.Error(), leakAddr) || strings.Contains(logs.String(), leakAddr) {
			t.Fatalf("alamat bocor: err=%q logs=%q", err, logs.String())
		}
	})
}

// B-2: server sudah 250 untuk DATA lalu memutus sebelum QUIT dijawab: surat DITERIMA, tepat satu pengiriman, sukses.
func TestSMTP_QuitFailureAfterAcceptedDataIsSuccessNotRetry(t *testing.T) {
	cert, pool := testCert(t)
	srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.dropAfterData = true })
	host, port := srv.addr()
	sl := new(sleeps)
	var logs bytes.Buffer
	s := newSender(t, baseCfg(host, port, config.SMTPTLSStartTLS), WithRootCAs(pool), WithSleep(sl.fn), WithSMTPLogger(bufLogger(&logs)))
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatalf("Send = %v, want sukses (surat sudah diterima server)", err)
	}
	accepted, _, mails := srv.snapshot()
	if accepted != 1 || len(mails) != 1 || sl.count() != 0 {
		t.Fatalf("koneksi=%d surat=%d tidur=%d, want 1/1/0 (tanpa kirim ganda)", accepted, len(mails), sl.count())
	}
	if strings.Contains(logs.String(), "user@example.org") {
		t.Errorf("log memuat penerima: %q", logs.String())
	}
}

// B-3: TLS implisit; server menyelesaikan handshake lalu diam. Pembatalan ctx harus menutup soket mentah dan
// membebaskan Send segera (goroutine pembatal tidak lagi berbagi variabel conn yang ditimpa).
func TestSMTP_ImplicitTLSCancelUnblocksStalledServer(t *testing.T) {
	cert, pool := testCert(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		tc := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
		if tc.Handshake() != nil {
			return
		}
		time.Sleep(5 * time.Second) // tanpa banner 220
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	s := newSender(t, baseCfg("127.0.0.1", port, config.SMTPTLSImplicit), WithRootCAs(pool), WithMaxAttempts(1), WithTimeouts(time.Second, 30*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := s.Send(ctx, msg()); err == nil {
		t.Fatal("harus galat")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("pembatalan tidak membebaskan Send: %v", d)
	}
}

// B-6: literal IPv6 diterima (konsisten dengan config.validSMTPHost: "nama host atau IP") dan benar-benar dapat dikirimi.
func TestNewSMTP_HostForms(t *testing.T) {
	good := baseCfg("smtp.example.com", 587, config.SMTPTLSStartTLS)
	for host, ok := range map[string]bool{
		"smtp.example.com": true, "127.0.0.1": true, "::1": true, "2001:db8::25": true,
		"[::1]": false, "fe80::1%eth0": false, "::1:99999": false, "smtp.example.com:587": false,
		"a@b": false, "": false, "a b": false, "http://x": false,
	} {
		c := good
		c.Host = host
		_, err := NewSMTP(c)
		if (err == nil) != ok {
			t.Errorf("host %q: err=%v, want ok=%v", host, err, ok)
		}
	}
}

func TestSMTP_DeliversToIPv6Literal(t *testing.T) {
	cert, _ := testCert(t)
	srv := newFakeSMTP(t, cert, func(s *fakeSMTP) { s.listen = "[::1]:0"; s.offerSTARTTLS = false })
	port := srv.ln.Addr().(*net.TCPAddr).Port
	s := newSender(t, baseCfg("::1", port, config.SMTPTLSNone), WithSleep(new(sleeps).fn))
	if err := s.Send(context.Background(), msg()); err != nil {
		t.Fatalf("Send ke ::1: %v", err)
	}
	if _, _, mails := srv.snapshot(); len(mails) != 1 {
		t.Fatalf("surat = %d", len(mails))
	}
}

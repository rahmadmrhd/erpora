package mail

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"
)

// testCert membuat sertifikat self-signed (CA sekaligus daun) untuk 127.0.0.1 dan localhost.
func testCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "fake-smtp"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// smtpEvent adalah satu perintah klien yang diterima server tiruan.
type smtpEvent struct {
	Line string // baris perintah apa adanya (AUTH PLAIN memuat kredensial base64: hanya untuk tes)
	TLS  bool   // sambungan sudah TLS saat perintah diterima
}

type receivedMail struct {
	From string
	To   []string
	Data string // isi DATA setelah dot-unstuffing, CRLF
}

// fakeSMTP adalah server SMTP tiruan dalam proses di atas net.Listener nyata (127.0.0.1).
type fakeSMTP struct {
	ln   net.Listener
	cert tls.Certificate

	// Perilaku (atur sebelum klien terhubung).
	implicitTLS        bool     // TLS sejak sambungan (port 465)
	offerSTARTTLS      bool     // iklankan dan layani STARTTLS
	advertiseAuthPlain bool     // iklankan AUTH juga sebelum TLS (untuk menguji klien tidak mengirim kredensial)
	authMechs          []string // mekanisme yang diiklankan (default PLAIN LOGIN)
	wantUser, wantPass string   // kredensial yang diterima; kosong = auth tidak diperlukan
	rcptReply          string   // balasan RCPT (default 250 OK); mis. "550 no such user"
	dataReplies        []string // balasan akhir DATA per percobaan berurutan (kosong = 250 OK); mis. "451 try again"

	mu       sync.Mutex
	accepted int
	events   []smtpEvent
	mails    []receivedMail
	dataN    int
	wg       sync.WaitGroup
}

func newFakeSMTP(t *testing.T, cert tls.Certificate, mod func(*fakeSMTP)) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTP{ln: ln, cert: cert, offerSTARTTLS: true, authMechs: []string{"PLAIN", "LOGIN"}}
	if mod != nil {
		mod(s)
	}
	s.wg.Add(1)
	go s.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		s.wg.Wait()
	})
	return s
}

func (s *fakeSMTP) addr() (host string, port int) {
	a := s.ln.Addr().(*net.TCPAddr)
	return "127.0.0.1", a.Port
}

func (s *fakeSMTP) serve() {
	defer s.wg.Done()
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.accepted++
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handle(c)
		}()
	}
}

func (s *fakeSMTP) record(line string, isTLS bool) {
	s.mu.Lock()
	s.events = append(s.events, smtpEvent{Line: line, TLS: isTLS})
	s.mu.Unlock()
}

func (s *fakeSMTP) handle(raw net.Conn) {
	conn := raw
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	isTLS := false
	if s.implicitTLS {
		tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12})
		if err := tc.Handshake(); err != nil {
			return
		}
		conn, isTLS = tc, true
	}
	r := textproto.NewReader(bufio.NewReader(conn))
	w := func(format string, a ...any) { _, _ = fmt.Fprintf(conn, format+"\r\n", a...) }
	w("220 fake ESMTP")
	var from string
	var rcpt []string
	authed := s.wantUser == ""
	for {
		line, err := r.ReadLine()
		if err != nil {
			return
		}
		verb := strings.ToUpper(line)
		s.record(line, isTLS)
		switch {
		case strings.HasPrefix(verb, "EHLO"), strings.HasPrefix(verb, "HELO"):
			lines := []string{"fake"}
			lines = append(lines, "SIZE 1048576")
			if s.offerSTARTTLS && !isTLS {
				lines = append(lines, "STARTTLS")
			}
			if (isTLS || s.advertiseAuthPlain) && s.wantUser != "" {
				lines = append(lines, "AUTH "+strings.Join(s.authMechs, " "))
			}
			for i, l := range lines {
				sep := "-"
				if i == len(lines)-1 {
					sep = " "
				}
				w("250%s%s", sep, l)
			}
		case verb == "STARTTLS":
			if !s.offerSTARTTLS || isTLS {
				w("502 not offered")
				continue
			}
			w("220 ready")
			tc := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12})
			if err := tc.Handshake(); err != nil {
				return
			}
			conn, isTLS = tc, true
			r = textproto.NewReader(bufio.NewReader(conn))
			w = func(format string, a ...any) { _, _ = fmt.Fprintf(conn, format+"\r\n", a...) }
		case strings.HasPrefix(verb, "AUTH PLAIN"):
			arg := strings.TrimSpace(line[len("AUTH PLAIN"):])
			if arg == "" {
				w("334 ")
				arg, err = r.ReadLine()
				if err != nil {
					return
				}
			}
			dec, _ := base64.StdEncoding.DecodeString(arg)
			parts := strings.Split(string(dec), "\x00")
			if len(parts) == 3 && parts[1] == s.wantUser && parts[2] == s.wantPass {
				authed = true
				w("235 ok")
			} else {
				w("535 5.7.8 authentication failed")
			}
		case strings.HasPrefix(verb, "AUTH LOGIN"):
			w("334 %s", base64.StdEncoding.EncodeToString([]byte("Username:")))
			u, _ := r.ReadLine()
			w("334 %s", base64.StdEncoding.EncodeToString([]byte("Password:")))
			p, _ := r.ReadLine()
			du, _ := base64.StdEncoding.DecodeString(u)
			dp, _ := base64.StdEncoding.DecodeString(p)
			if string(du) == s.wantUser && string(dp) == s.wantPass {
				authed = true
				w("235 ok")
			} else {
				w("535 5.7.8 authentication failed")
			}
		case strings.HasPrefix(verb, "MAIL FROM:"):
			if !authed {
				w("530 authentication required")
				continue
			}
			from = strings.Trim(strings.TrimSpace(line[len("MAIL FROM:"):]), "<>")
			rcpt = nil
			w("250 ok")
		case strings.HasPrefix(verb, "RCPT TO:"):
			if s.rcptReply != "" {
				w("%s", s.rcptReply)
				continue
			}
			rcpt = append(rcpt, strings.Trim(strings.TrimSpace(line[len("RCPT TO:"):]), "<>"))
			w("250 ok")
		case verb == "DATA":
			w("354 go")
			var sb strings.Builder
			for {
				l, err := r.ReadLine()
				if err != nil {
					return
				}
				if l == "." {
					break
				}
				l = strings.TrimPrefix(l, ".")
				sb.WriteString(l + "\r\n")
			}
			s.mu.Lock()
			n := s.dataN
			s.dataN++
			s.mu.Unlock()
			if n < len(s.dataReplies) && s.dataReplies[n] != "" {
				w("%s", s.dataReplies[n])
				continue
			}
			s.mu.Lock()
			s.mails = append(s.mails, receivedMail{From: from, To: rcpt, Data: sb.String()})
			s.mu.Unlock()
			w("250 queued")
		case verb == "RSET", verb == "NOOP":
			w("250 ok")
		case verb == "QUIT":
			w("221 bye")
			return
		default:
			w("500 unknown")
		}
	}
}

func (s *fakeSMTP) snapshot() (accepted int, events []smtpEvent, mails []receivedMail) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.accepted, append([]smtpEvent(nil), s.events...), append([]receivedMail(nil), s.mails...)
}

// sawVerb melaporkan apakah klien pernah mengirim perintah berawalan prefix (huruf besar).
func (s *fakeSMTP) sawVerb(prefix string) bool {
	_, ev, _ := s.snapshot()
	for _, e := range ev {
		if strings.HasPrefix(strings.ToUpper(e.Line), prefix) {
			return true
		}
	}
	return false
}

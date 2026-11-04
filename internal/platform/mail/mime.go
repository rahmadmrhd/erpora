package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"mime/quotedprintable"
	"strings"
	"time"
	"unicode/utf8"

	sdkmail "erpora/sdk/mail"
)

// maxHeaderLine adalah batas panjang baris header yang disarankan RFC 5322 (78 termasuk nama header); baris yang lebih
// panjang dilipat (CRLF + spasi) di antara token.
const maxHeaderLine = 76

// buildMessage menyusun pesan RFC 5322/MIME dari m yang SUDAH lolos sdkmail.Validate: alamat polos ASCII, subjek tanpa
// karakter kontrol. Subjek non-ASCII (atau panjang, atau berisi "=?") di-encode RFC 2047 (base64, UTF-8, dilipat di
// antara encoded-word); isi di-encode quoted-printable dengan CRLF; boundary multipart acak (crypto/rand). Tidak ada
// nilai pemanggil yang disalin ke header tanpa melalui validasi ketat atau encoding.
func buildMessage(from string, m sdkmail.Message, now time.Time) ([]byte, error) {
	if err := sdkmail.Validate(m); err != nil {
		return nil, err
	}
	if err := sdkmail.Validate(sdkmail.Message{To: []string{from}, Subject: "x", Text: "x"}); err != nil {
		return nil, fmt.Errorf("mail: alamat pengirim tidak sah: %w", err)
	}
	var buf bytes.Buffer
	hdr := func(name, value string) {
		// Pertahanan berlapis: nilai header tidak pernah memuat CR/LF mentah selain lipatan yang kita buat sendiri.
		buf.WriteString(name + ": " + value + "\r\n")
	}
	hdr("From", from)
	hdr("To", foldList(m.To))
	hdr("Subject", encodeSubject(m.Subject))
	hdr("Date", now.UTC().Format(time.RFC1123Z))
	id, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	_, domain, _ := strings.Cut(from, "@")
	hdr("Message-ID", "<"+id+"@"+domain+">")
	hdr("MIME-Version", "1.0")

	switch {
	case m.Text != "" && m.HTML != "":
		boundary, err := randomHex(24)
		if err != nil {
			return nil, err
		}
		hdr("Content-Type", `multipart/alternative; boundary="`+boundary+`"`)
		buf.WriteString("\r\n")
		for _, p := range []struct{ ct, body string }{{"text/plain", m.Text}, {"text/html", m.HTML}} {
			buf.WriteString("--" + boundary + "\r\n")
			buf.WriteString("Content-Type: " + p.ct + "; charset=utf-8\r\n")
			buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
			if err := writeQP(&buf, p.body); err != nil {
				return nil, err
			}
			buf.WriteString("\r\n")
		}
		buf.WriteString("--" + boundary + "--\r\n")
	default:
		ct, body := "text/plain", m.Text
		if m.Text == "" {
			ct, body = "text/html", m.HTML
		}
		hdr("Content-Type", ct+"; charset=utf-8")
		hdr("Content-Transfer-Encoding", "quoted-printable")
		buf.WriteString("\r\n")
		if err := writeQP(&buf, body); err != nil {
			return nil, err
		}
		buf.WriteString("\r\n")
	}
	return buf.Bytes(), nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("mail: acak: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// writeQP menulis body quoted-printable dengan akhir baris CRLF (LF dan CR sendirian dinormalkan lebih dulu).
func writeQP(w io.Writer, body string) error {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	body = strings.ReplaceAll(body, "\n", "\r\n")
	qp := quotedprintable.NewWriter(w)
	if _, err := qp.Write([]byte(body)); err != nil {
		return err
	}
	return qp.Close()
}

// foldList menggabungkan alamat dengan ", " dan melipat baris yang melebihi maxHeaderLine.
func foldList(addrs []string) string {
	var sb strings.Builder
	line := len("To: ")
	for i, a := range addrs {
		piece := a
		if i < len(addrs)-1 {
			piece += ","
		}
		if i > 0 {
			if line+1+len(piece) > maxHeaderLine {
				sb.WriteString("\r\n ")
				line = 1
			} else {
				sb.WriteString(" ")
				line++
			}
		}
		sb.WriteString(piece)
		line += len(piece)
	}
	return sb.String()
}

// encodeSubject mengembalikan nilai header Subject yang aman: ASCII pendek tanpa "=?" apa adanya; selainnya
// encoded-word RFC 2047 (=?UTF-8?B?...?=), tiap kata <= 75 karakter, dipisah CRLF + spasi, dipotong pada batas rune.
func encodeSubject(s string) string {
	plain := len(s) <= 60 && !strings.Contains(s, "=?")
	for i := 0; plain && i < len(s); i++ {
		if s[i] >= 0x80 {
			plain = false
		}
	}
	if plain {
		return s
	}
	const chunk = 39 // 39 byte -> 52 karakter base64; + 12 untuk =?UTF-8?B?...?= = 64; dengan "Subject: " 73 (< 78)
	var words []string
	for len(s) > 0 {
		n := min(chunk, len(s))
		for n > 0 && n < len(s) && !utf8.RuneStart(s[n]) {
			n--
		}
		if n == 0 { // tidak terjadi untuk UTF-8 valid; gagal-aman
			n = len(s)
		}
		words = append(words, "=?UTF-8?B?"+base64.StdEncoding.EncodeToString([]byte(s[:n]))+"?=")
		s = s[n:]
	}
	return strings.Join(words, "\r\n ")
}

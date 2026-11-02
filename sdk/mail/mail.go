// Package mail memuat seam pengiriman email minimal untuk modul: Message, Sender, dan Send yang gagal
// tertutup bila tidak ada Sender di context. Driver (smtp, log) milik platform. Paket ini daun (D9).
package mail

import (
	"context"
	"errors"
	"fmt"
	netmail "net/mail"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Batas ukuran wajar agar Send tidak menjadi vektor penyalahgunaan (banjir/ledakan memori).
const (
	MaxRecipients = 50
	MaxSubjectLen = 998 // batas panjang baris header RFC 5322
	MaxBodyBytes  = 1 << 20
)

var (
	// ErrNoSender: tidak ada Sender di context; email tidak pernah dikirim diam-diam (gagal tertutup).
	ErrNoSender = errors.New("mail: tidak ada sender di context")
	// ErrInvalidMessage: Message ditolak validasi (penerima tidak valid, injeksi header, terlalu besar).
	ErrInvalidMessage = errors.New("mail: pesan tidak valid")
)

// Message adalah satu email. Text wajib sebagai fallback; HTML opsional.
type Message struct {
	To      []string
	Subject string
	Text    string
	HTML    string
}

// Sender mengirim Message; diimplementasi platform.
//
// Kontrak untuk implementer (driver): Send di sini TIDAK menjamin pesan sudah divalidasi, karena
// pemanggil dapat memegang Sender langsung dan melewati mail.Send. Driver wajib memanggil Validate
// pada setiap Message sebelum menulisnya ke transport (mencegah injeksi header SMTP). Validate ketat
// (alamat ASCII polos, tanpa domain literal/IDN/encoded-word, tanpa karakter kontrol/format/pemisah);
// driver tidak boleh melonggarkannya, dan harus meng-encode Subject non-ASCII sendiri.
type Sender interface {
	Send(ctx context.Context, m Message) error
}

type senderKey struct{}

// WithSender memasang s di ctx; dipanggil platform saat merakit modul.
func WithSender(ctx context.Context, s Sender) context.Context {
	return context.WithValue(ctx, senderKey{}, s)
}

// From mengembalikan Sender di ctx; ok false bila tidak ada, nil, atau nil bertipe (mis. (*T)(nil)
// yang dibungkus antarmuka) sehingga pemanggil tidak panik saat memakainya.
func From(ctx context.Context) (Sender, bool) {
	s, ok := ctx.Value(senderKey{}).(Sender)
	if !ok || isNil(s) {
		return nil, false
	}
	return s, true
}

// Send memvalidasi m (Validate) lalu mengirimnya lewat Sender di ctx. Tanpa Sender mengembalikan
// ErrNoSender.
func Send(ctx context.Context, m Message) error {
	s, ok := From(ctx)
	if !ok {
		return ErrNoSender
	}
	if err := Validate(m); err != nil {
		return err
	}
	return s.Send(ctx, m)
}

// Validate memeriksa m dan mengembalikan error yang membungkus ErrInvalidMessage bila tidak sah.
// Dipakai Send, dan wajib dipakai driver Sender (lihat kontrak Sender).
//
//   - To: tidak kosong, paling banyak MaxRecipients; tiap entri harus alamat polos ASCII yang ketat
//     (lihat validateAddress): tanpa nama tampilan/sudut/koma/ruang/kutip, domain berupa nama host
//     dengan titik (bukan literal [..], bukan IP, bukan IDN), tanpa encoded-word, paling panjang
//     254 byte (lokal 64).
//   - Subject: wajib, tanpa karakter kontrol (CR, LF, NUL, tab, DEL, C1), pemisah baris/paragraf
//     (U+2028/2029), maupun karakter format (zero-width, bidi override, BOM), paling panjang MaxSubjectLen.
//     Subjek non-ASCII biasa sah; driver yang meng-encode header (RFC 2047).
//   - Subject, Text, HTML, dan alamat harus UTF-8 valid.
//   - Text atau HTML wajib ada; masing-masing paling besar MaxBodyBytes.
func Validate(m Message) error {
	if len(m.To) == 0 {
		return fmt.Errorf("%w: penerima kosong", ErrInvalidMessage)
	}
	if len(m.To) > MaxRecipients {
		return fmt.Errorf("%w: terlalu banyak penerima", ErrInvalidMessage)
	}
	for _, to := range m.To {
		if err := validateAddress(to); err != nil {
			return err
		}
	}
	// Byte UTF-8 tidak valid ditolak di semua bidang teks: iterasi rune mengubahnya jadi U+FFFD (kategori
	// So) yang lolos pemeriksaan karakter kontrol, padahal driver/transport dapat menafsirkannya lain.
	if !utf8.ValidString(m.Subject) || !utf8.ValidString(m.Text) || !utf8.ValidString(m.HTML) {
		return fmt.Errorf("%w: UTF-8 tidak valid", ErrInvalidMessage)
	}
	if strings.TrimSpace(m.Subject) == "" {
		return fmt.Errorf("%w: subjek kosong", ErrInvalidMessage)
	}
	if hasControl(m.Subject) {
		return fmt.Errorf("%w: karakter kontrol pada subjek", ErrInvalidMessage)
	}
	if len(m.Subject) > MaxSubjectLen {
		return fmt.Errorf("%w: subjek terlalu panjang", ErrInvalidMessage)
	}
	if m.Text == "" && m.HTML == "" {
		return fmt.Errorf("%w: isi kosong", ErrInvalidMessage)
	}
	if len(m.Text) > MaxBodyBytes || len(m.HTML) > MaxBodyBytes {
		return fmt.Errorf("%w: isi terlalu besar", ErrInvalidMessage)
	}
	return nil
}

// Batas alamat (RFC 5321 4.5.3.1): total 254 byte (path 256 dikurangi tanda sudut), bagian lokal 64,
// label domain 63.
const (
	maxAddressLen = 254
	maxLocalLen   = 64
	maxLabelLen   = 63
)

// validateAddress menerapkan validasi ketat (keputusan pemilik produk, M-3): hanya alamat polos ASCII
// `lokal@domain.tld`. Ditolak: nama tampilan/sudut/komentar/kutip, domain literal ([...]), domain
// tanpa titik atau berupa IP, non-ASCII/IDN (sampai ada dukungan IDN; pakai bentuk punycode xn--),
// bentuk encoded-word (=?...?=), karakter kontrol/format/pemisah, dan kelebihan panjang.
func validateAddress(to string) error {
	bad := func(why string) error {
		return fmt.Errorf("%w: alamat penerima tidak sah (%s)", ErrInvalidMessage, why)
	}
	if to == "" {
		return fmt.Errorf("%w: alamat penerima kosong", ErrInvalidMessage)
	}
	if len(to) > maxAddressLen {
		return bad("terlalu panjang")
	}
	if !utf8.ValidString(to) {
		return bad("UTF-8 tidak valid")
	}
	if hasControl(to) {
		return bad("karakter kontrol")
	}
	for i := 0; i < len(to); i++ {
		if to[i] >= 0x80 {
			return bad("non-ASCII")
		}
	}
	if strings.Contains(to, "=?") {
		return bad("encoded-word")
	}
	addr, err := netmail.ParseAddress(to)
	// Hanya alamat polos: hasil parse harus identik dengan masukan (menolak nama tampilan, sudut,
	// ruang di tepi, dan bentuk lain yang dinormalkan parser).
	if err != nil || addr.Name != "" || addr.Address != to {
		return bad("bukan alamat polos")
	}
	local, domain, ok := strings.Cut(to, "@")
	if !ok || strings.Contains(domain, "@") {
		return bad("harus tepat satu @")
	}
	if !validLocal(local) {
		return bad("bagian lokal")
	}
	if !validDomain(domain) {
		return bad("domain")
	}
	return nil
}

// validLocal: dot-atom RFC 5322 (tanpa bentuk dikutip), paling panjang 64 byte.
func validLocal(l string) bool {
	if l == "" || len(l) > maxLocalLen || l[0] == '.' || l[len(l)-1] == '.' || strings.Contains(l, "..") {
		return false
	}
	for i := 0; i < len(l); i++ {
		c := l[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.':
		case strings.IndexByte("!#$%&'*+-/=?^_`{|}~", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// validDomain: nama host ASCII dengan minimal dua label (ada titik), label LDH 1..63 byte tanpa
// tanda hubung di tepi, TLD tidak semuanya angka (menolak alamat IP).
func validDomain(d string) bool {
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return false
	}
	for _, lb := range labels {
		if lb == "" || len(lb) > maxLabelLen || lb[0] == '-' || lb[len(lb)-1] == '-' {
			return false
		}
		for i := 0; i < len(lb); i++ {
			c := lb[i]
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
				return false
			}
		}
	}
	tld := labels[len(labels)-1]
	return strings.Trim(tld, "0123456789") != ""
}

// hasControl melaporkan karakter kontrol (Cc), pemisah baris/paragraf (Zl, Zp), dan karakter format
// (Cf: zero-width, bidi override, BOM, soft hyphen) yang dapat menyelundupkan header atau menipu tampilan.
func hasControl(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf)
	}) >= 0
}

// isNil melaporkan nil biasa maupun nil bertipe pada tipe yang dapat nil.
func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Func, reflect.Interface, reflect.Chan, reflect.Slice, reflect.UnsafePointer:
		return rv.IsNil()
	}
	return false
}

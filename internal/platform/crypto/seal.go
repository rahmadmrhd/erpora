// Package crypto menyegel rahasia saat diam (kunci privat JWT, secret TOTP, secret settings) dengan
// AES-256-GCM berkunci ERP_SECRET_KEY.
//
// Format hasil Seal: versi (1 byte) || id kunci (1 byte) || nonce (12 byte, acak) || ciphertext+tag GCM.
// Seluruh header (versi + id kunci) ikut sebagai awal AAD, sehingga header tidak dapat diubah tanpa terdeteksi
// (AAD efektif = header || aad pemanggil). AAD pemanggil WAJIB tidak kosong: ia mengikat ciphertext ke
// konteksnya (mis. tabel, kolom, dan id baris) sehingga blob tidak bisa dipindah ke tempat lain.
// Id kunci memungkinkan rotasi: Seal memakai kunci aktif, Open memilih kunci menurut id (aktif atau lama).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
)

const (
	// KeySize adalah panjang material kunci wajib (AES-256).
	KeySize = 32

	formatV1   byte = 1 // versi format: header 2 byte + nonce 12 byte
	headerSize int  = 2 // versi + id kunci
	nonceSize  int  = 12
)

var (
	// ErrOpen berarti blob gagal dibuka: dirusak (termasuk header), AAD salah, kunci salah, atau terlalu pendek.
	// Sebab sengaja tidak dibedakan.
	ErrOpen = errors.New("crypto: gagal membuka data tersegel")
	// ErrUnknownVersion berarti byte versi format tidak dikenal.
	ErrUnknownVersion = errors.New("crypto: versi format tersegel tidak dikenal")
	// ErrUnknownKeyID berarti id kunci pada header tidak dimiliki Sealer ini (bukan kunci aktif maupun lama).
	ErrUnknownKeyID = errors.New("crypto: id kunci tersegel tidak dikenal")
	// ErrEmptyAAD berarti aad kosong atau nil; konteks pengikat wajib diberikan.
	ErrEmptyAAD = errors.New("crypto: aad wajib tidak kosong")
)

// KeyIDOf menurunkan id kunci (1 byte) dari material: byte pertama SHA-256(material). Deterministik, sehingga id pada
// header segel tetap cocok ketika kunci pindah dari ERP_SECRET_KEY ke ERP_SECRET_KEY_PREVIOUS saat rotasi tanpa
// konfigurasi id terpisah. Hash searah: id tidak membocorkan material.
func KeyIDOf(material []byte) byte {
	h := sha256.Sum256(material)
	return h[0]
}

// NewSealerFromMaterial seperti NewSealer, dengan id kunci diturunkan lewat KeyIDOf. Entri previous yang kosong
// (nil atau panjang 0; kasus normal tanpa rotasi) dilewati; entri tidak kosong dengan panjang salah ditolak. Bila dua
// kunci kebetulan berid sama (peluang 1/256) atau kunci lama sama dengan yang aktif, kembalikan galat.
func NewSealerFromMaterial(active []byte, previous ...[]byte) (*Sealer, error) {
	prev := make([]Key, 0, len(previous))
	for _, m := range previous {
		if len(m) == 0 {
			continue
		}
		prev = append(prev, Key{ID: KeyIDOf(m), Material: m})
	}
	return NewSealer(Key{ID: KeyIDOf(active), Material: active}, prev...)
}

// Key adalah satu kunci penyegel bernomor. Material harus tepat KeySize byte. Pencetakan (fmt, slog) menyamarkan
// Material dan hanya menampilkan ID; field tetap dapat dibaca langsung oleh kode.
type Key struct {
	ID       byte
	Material []byte
}

func (k Key) redacted() string { return fmt.Sprintf("crypto.Key{ID:%d Material:****}", k.ID) }

// String dan GoString menyamarkan Material.
func (k Key) String() string   { return k.redacted() }
func (k Key) GoString() string { return k.redacted() }

// Format menyamarkan Material untuk semua verba (%v %+v %#v %s %x %d ...).
func (k Key) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(k.redacted())) }

// LogValue menyamarkan Material pada slog (handler teks maupun JSON).
func (k Key) LogValue() slog.Value { return slog.StringValue(k.redacted()) }

// Sealer menyegel dan membuka data. Aman dipakai bersamaan oleh banyak goroutine.
type Sealer struct {
	activeID byte
	aeads    map[byte]cipher.AEAD
}

// NewSealer membuat Sealer dengan kunci aktif (dipakai Seal dan Open) dan kunci lama opsional (hanya Open, untuk
// rotasi). Id kunci harus unik. Material dipakai untuk membangun cipher (aes.NewCipher membuat jadwal kunci internal,
// sehingga slice pemanggil tidak dipegang dan boleh ditimpa); nilainya tidak pernah muncul di galat.
func NewSealer(active Key, previous ...Key) (*Sealer, error) {
	s := &Sealer{activeID: active.ID, aeads: make(map[byte]cipher.AEAD, 1+len(previous))}
	for _, k := range append([]Key{active}, previous...) {
		if _, dup := s.aeads[k.ID]; dup {
			return nil, fmt.Errorf("crypto: id kunci %d dipakai lebih dari sekali", k.ID)
		}
		aead, err := newAEAD(k)
		if err != nil {
			return nil, err
		}
		s.aeads[k.ID] = aead
	}
	return s, nil
}

func newAEAD(k Key) (cipher.AEAD, error) {
	if len(k.Material) != KeySize {
		return nil, fmt.Errorf("crypto: kunci id %d harus %d byte (diterima %d byte)", k.ID, KeySize, len(k.Material))
	}
	block, err := aes.NewCipher(k.Material) // aes.NewCipher menyalin kunci ke jadwal kunci internal
	if err != nil {
		return nil, errors.New("crypto: inisialisasi cipher gagal")
	}
	aead, err := cipher.NewGCMWithNonceSize(block, nonceSize)
	if err != nil {
		return nil, errors.New("crypto: inisialisasi GCM gagal")
	}
	return aead, nil
}

// effectiveAAD = header || aad pemanggil.
func effectiveAAD(header, aad []byte) []byte {
	out := make([]byte, 0, len(header)+len(aad))
	return append(append(out, header...), aad...)
}

// Seal mengenkripsi plaintext dengan kunci aktif dan nonce acak baru. aad wajib tidak kosong (ErrEmptyAAD).
func (s *Sealer) Seal(plaintext, aad []byte) ([]byte, error) {
	if len(aad) == 0 {
		return nil, ErrEmptyAAD
	}
	aead := s.aeads[s.activeID]
	out := make([]byte, headerSize+nonceSize, headerSize+nonceSize+len(plaintext)+aead.Overhead())
	out[0], out[1] = formatV1, s.activeID
	nonce := out[headerSize : headerSize+nonceSize]
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("crypto: nonce acak: %w", err)
	}
	return aead.Seal(out, nonce, plaintext, effectiveAAD(out[:headerSize], aad)), nil
}

// Open membuka hasil Seal. aad harus sama persis dengan saat Seal dan tidak kosong (ErrEmptyAAD). Gagal dengan
// ErrUnknownVersion (versi tak dikenal), ErrUnknownKeyID (id kunci tak dimiliki), atau ErrOpen untuk kegagalan lain.
func (s *Sealer) Open(sealed, aad []byte) ([]byte, error) {
	if len(aad) == 0 {
		return nil, ErrEmptyAAD
	}
	if len(sealed) == 0 {
		return nil, ErrOpen
	}
	if sealed[0] != formatV1 {
		return nil, ErrUnknownVersion
	}
	if len(sealed) < headerSize {
		return nil, ErrOpen
	}
	aead, ok := s.aeads[sealed[1]]
	if !ok {
		return nil, ErrUnknownKeyID
	}
	if len(sealed) < headerSize+nonceSize+aead.Overhead() {
		return nil, ErrOpen
	}
	nonce := sealed[headerSize : headerSize+nonceSize]
	plain, err := aead.Open(nil, nonce, sealed[headerSize+nonceSize:], effectiveAAD(sealed[:headerSize], aad))
	if err != nil {
		return nil, ErrOpen
	}
	return plain, nil
}

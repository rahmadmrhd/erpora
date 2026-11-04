// Package password menyediakan hashing sandi (argon2id, format PHC), kebijakan sandi, dan hash tiruan untuk menyeragamkan
// waktu respons login (PRD-001 FR-101, security-guidelines §3.4-§3.5, ADR-0013).
//
// Sandi tidak pernah dicatat ke log atau disertakan dalam pesan galat. Paket ini tidak mengimpor paket platform lain
// sehingga dapat dipakai config (validasi parameter), store, dan service identity tanpa siklus.
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/text/unicode/norm"
)

// Batas dan nilai tetap.
const (
	// MaxPasswordBytes adalah panjang maksimum sandi dalam byte (membatasi biaya hash dan memori yang dikonsumsi
	// masukan tak tepercaya).
	MaxPasswordBytes = 1024

	// Batas atas parameter yang diterima, baik dari konfigurasi maupun dari hash tersimpan (PHC yang dimanipulasi tidak
	// boleh memaksa server memakai memori/CPU tak terbatas). Konstanta tanpa tipe agar dapat dipakai pada tipe apa pun.
	MaxMemoryKiB   = 256 * 1024 // 256 MiB per hash
	MaxTime        = 10
	MaxParallelism = 16

	saltLen = 16
	keyLen  = 32

	// Batas panjang salt dan hash pada PHC yang diterima (format sah, bukan salah satu panjang tetap kita).
	minSaltLen, maxSaltLen = 8, 64
	minKeyLen, maxKeyLen   = 16, 64

	// Batas bawah konfigurasi: rekomendasi OWASP Password Storage Cheat Sheet untuk argon2id,
	// m=19 MiB t=2 p=1 (minimum) atau m=46 MiB t=1 p=1.
	minConfigMemoryKiB     = 19 * 1024
	minConfigMemoryKiBTime = 46 * 1024 // memori minimum bila t=1
)

// Galat paket. Tidak ada yang memuat sandi atau isi hash.
var (
	ErrInvalidHash     = errors.New("password: hash tersimpan cacat atau parameternya di luar batas")
	ErrInvalidParams   = errors.New("password: parameter argon2 tidak valid")
	ErrPasswordTooLong = errors.New("password: sandi melebihi batas panjang")
)

// Params adalah parameter argon2id.
type Params struct {
	MemoryKiB   uint32 // memori dalam KiB (m)
	Time        uint32 // jumlah iterasi (t)
	Parallelism uint8  // derajat paralelisme (p)
}

// DefaultParams mengikuti OWASP Password Storage Cheat Sheet (argon2id, konfigurasi minimum yang direkomendasikan):
// m=19456 KiB (19 MiB), t=2, p=1. Dapat diganti lewat ERP_ARGON2_MEMORY_KIB, ERP_ARGON2_TIME, ERP_ARGON2_PARALLELISM.
func DefaultParams() Params { return Params{MemoryKiB: 19456, Time: 2, Parallelism: 1} }

// ParamError menyebut bidang parameter yang tidak valid: "memory", "time", "parallelism", atau "combination".
type ParamError struct{ Field, Reason string }

func (e *ParamError) Error() string { return "password: parameter " + e.Field + ": " + e.Reason }

// validate memeriksa batas yang dibutuhkan agar argon2 dapat dijalankan dengan aman (dipakai Hash dan Verify).
func (p Params) validate() error {
	switch {
	case p.Parallelism < 1 || p.Parallelism > MaxParallelism:
		return &ParamError{"parallelism", fmt.Sprintf("harus 1..%d", MaxParallelism)}
	case p.Time < 1 || p.Time > MaxTime:
		return &ParamError{"time", fmt.Sprintf("harus 1..%d", MaxTime)}
	case p.MemoryKiB > MaxMemoryKiB:
		return &ParamError{"memory", fmt.Sprintf("maksimum %d KiB", MaxMemoryKiB)}
	case p.MemoryKiB < 8*uint32(p.Parallelism):
		return &ParamError{"memory", "minimum 8 KiB per jalur paralelisme"}
	}
	return nil
}

// ValidateConfig memeriksa parameter yang dikonfigurasi operator: batas atas yang sama dengan validate ditambah batas
// bawah OWASP (m >= 19 MiB; t=1 hanya bila m >= 46 MiB) agar konfigurasi tidak dapat melemahkan hash sandi.
func (p Params) ValidateConfig() error {
	if err := p.validate(); err != nil {
		return err
	}
	if p.MemoryKiB < minConfigMemoryKiB {
		return &ParamError{"memory", fmt.Sprintf("minimum %d KiB (rekomendasi OWASP)", minConfigMemoryKiB)}
	}
	if p.Time == 1 && p.MemoryKiB < minConfigMemoryKiBTime {
		return &ParamError{"combination", fmt.Sprintf("time=1 hanya diterima bila memory >= %d KiB (rekomendasi OWASP); selain itu time minimum 2", minConfigMemoryKiBTime)}
	}
	return nil
}

// Hasher menghitung dan memverifikasi hash argon2id dengan Params tertentu.
type Hasher struct {
	Params Params

	dummyOnce sync.Once
	dummyPHC  string
}

// normalize menormalkan sandi ke NFKC (keputusan pemilik produk 3 Nov 2026; ASVS 5.0 menganjurkan normalisasi Unicode
// agar sandi yang sama dari IME atau perangkat berbeda, mis. "é" komposisi vs "e"+U+0301, tetap cocok). Pemanggil
// WAJIB sudah memeriksa batas MaxPasswordBytes pada masukan mentah: normalisasi dapat memperbesar hasil (terburuk
// U+FDFA, 3 byte menjadi 33 byte), tetapi terbatas karena masukannya <= 1024 byte. NUL dan karakter kontrol tidak diubah.
func normalize(plain string) string { return norm.NFKC.String(plain) }

// Hash menghasilkan hash PHC `$argon2id$v=19$m=..,t=..,p=..$salt$hash` dengan salt acak 16 byte (crypto/rand).
func (h *Hasher) Hash(plain string) (string, error) {
	if len(plain) > MaxPasswordBytes {
		return "", ErrPasswordTooLong
	}
	if err := h.Params.validate(); err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidParams, err)
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("password: baca crypto/rand: %w", err)
	}
	key := argon2.IDKey([]byte(normalize(plain)), salt, h.Params.Time, h.Params.MemoryKiB, h.Params.Parallelism, keyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.Params.MemoryKiB, h.Params.Time, h.Params.Parallelism, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify memeriksa plain terhadap hash PHC tersimpan dengan perbandingan konstan-waktu. needsRehash true hanya bila
// sandi benar dan parameter (atau panjang salt/hash) hash tersimpan berbeda dari Params h, sehingga pemanggil dapat
// menyimpan hash baru. PHC cacat, bukan argon2id/v=19, atau berparameter di luar batas dikembalikan sebagai
// ErrInvalidHash TANPA menjalankan argon2 (cegah DoS lewat hash yang dimanipulasi). Sandi lebih panjang dari
// MaxPasswordBytes ditolak (ok=false, tanpa galat): panjang maksimum adalah kebijakan publik.
func (h *Hasher) Verify(plain, phc string) (ok, needsRehash bool, err error) {
	p, salt, want, err := parsePHC(phc)
	if err != nil {
		return false, false, err
	}
	if len(plain) > MaxPasswordBytes {
		return false, false, nil
	}
	got := argon2.IDKey([]byte(normalize(plain)), salt, p.Time, p.MemoryKiB, p.Parallelism, uint32(len(want))) //nolint:gosec // panjang hash 16..64 sudah divalidasi parsePHC
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, false, nil
	}
	rehash := p != h.Params || len(salt) != saltLen || len(want) != keyLen
	return true, rehash, nil
}

// parsePHC menguraikan hash PHC argon2id secara ketat. Tidak panik; pesan galat tidak memuat isi masukan.
func parsePHC(phc string) (p Params, salt, key []byte, err error) {
	parts := strings.Split(phc, "$")
	// "", "argon2id", "v=19", "m=..,t=..,p=..", salt, hash
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return Params{}, nil, nil, ErrInvalidHash
	}
	kv := strings.Split(parts[3], ",")
	if len(kv) != 3 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	var vals [3]uint64
	for i, name := range []string{"m=", "t=", "p="} {
		s, found := strings.CutPrefix(kv[i], name)
		if !found || s == "" {
			return Params{}, nil, nil, ErrInvalidHash
		}
		for _, c := range []byte(s) { // hanya digit desimal
			if c < '0' || c > '9' {
				return Params{}, nil, nil, ErrInvalidHash
			}
		}
		v, perr := strconv.ParseUint(s, 10, 32)
		if perr != nil {
			return Params{}, nil, nil, ErrInvalidHash
		}
		vals[i] = v
	}
	// Periksa batas SEBELUM konversi ke uint8 agar p=256 tidak melingkar menjadi 0.
	if vals[0] > MaxMemoryKiB || vals[1] > MaxTime || vals[2] > MaxParallelism {
		return Params{}, nil, nil, ErrInvalidHash
	}
	p = Params{MemoryKiB: uint32(vals[0]), Time: uint32(vals[1]), Parallelism: uint8(vals[2])} //nolint:gosec // batas atas sudah diperiksa tepat di atas
	if p.validate() != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	enc := base64.RawStdEncoding.Strict()
	if salt, err = enc.DecodeString(parts[4]); err != nil || len(salt) < minSaltLen || len(salt) > maxSaltLen {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if key, err = enc.DecodeString(parts[5]); err != nil || len(key) < minKeyLen || len(key) > maxKeyLen {
		return Params{}, nil, nil, ErrInvalidHash
	}
	return p, salt, key, nil
}

// --- Hash tiruan (keseragaman waktu respons login) ---

var (
	defaultDummyOnce sync.Once
	defaultDummy     *Hasher
)

// Dummy mengembalikan Hasher berparameter DefaultParams (satu instans per proses, hash tiruannya sudah dihitung).
// Dipakai untuk email yang tidak dikenal: panggil `d.Verify(plain, d.DummyHash())` dan abaikan hasilnya agar biaya dan
// waktu setara dengan sandi salah pada akun nyata. Bila deployment memakai parameter non-default (ERP_ARGON2_*), pakai
// NewDummy(params) dengan parameter yang sama dengan Hasher sungguhan supaya biayanya identik.
func Dummy() *Hasher {
	defaultDummyOnce.Do(func() { defaultDummy = NewDummy(DefaultParams()) })
	return defaultDummy
}

// NewDummy membuat Hasher untuk hash tiruan dengan parameter p; hash tiruannya dihitung segera (bukan saat permintaan
// pertama) supaya permintaan pertama tidak lebih lambat dari yang lain.
func NewDummy(p Params) *Hasher {
	h := &Hasher{Params: p}
	_ = h.DummyHash()
	return h
}

// DummyHash mengembalikan hash PHC dari rahasia acak yang tidak pernah diungkap, dihitung sekali dengan h.Params
// (stabil selama proses hidup). Tidak ada sandi yang dapat cocok dengannya secara praktis. Bila Params tidak valid,
// dipakai DefaultParams agar tidak mengembalikan string kosong (yang akan ditolak cepat dan membocorkan waktu).
func (h *Hasher) DummyHash() string {
	h.dummyOnce.Do(func() {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			panic("password: crypto/rand tidak tersedia: " + err.Error()) // tidak ada degradasi aman untuk CSPRNG
		}
		src := h
		if h.Params.validate() != nil {
			src = &Hasher{Params: DefaultParams()}
		}
		phc, err := src.Hash(hex.EncodeToString(secret))
		if err != nil {
			panic("password: gagal membuat hash tiruan: " + err.Error())
		}
		h.dummyPHC = phc
	})
	return h.dummyPHC
}

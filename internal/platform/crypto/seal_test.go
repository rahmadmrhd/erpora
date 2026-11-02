package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

var testAAD = []byte("tabel:id/kolom")

func testKey(id, b byte) Key { return Key{ID: id, Material: bytes.Repeat([]byte{b}, KeySize)} }

func newTestSealer(t *testing.T) *Sealer {
	t.Helper()
	s, err := NewSealer(testKey(1, 7))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestNewSealerRejectsWrongKeyLength(t *testing.T) {
	for _, n := range []int{0, 1, 16, 24, 31, 33, 64} {
		if _, err := NewSealer(Key{ID: 1, Material: make([]byte, n)}); err == nil {
			t.Errorf("NewSealer(%d byte) harus gagal", n)
		}
		if _, err := NewSealer(testKey(1, 1), Key{ID: 2, Material: make([]byte, n)}); err == nil {
			t.Errorf("NewSealer(kunci lama %d byte) harus gagal", n)
		}
	}
	if _, err := NewSealer(Key{ID: 1}); err == nil {
		t.Error("NewSealer(material nil) harus gagal")
	}
	if _, err := NewSealer(testKey(1, 1)); err != nil {
		t.Errorf("kunci 32 byte harus diterima: %v", err)
	}
}

func TestNewSealerRejectsDuplicateKeyID(t *testing.T) {
	if _, err := NewSealer(testKey(1, 1), testKey(1, 2)); err == nil {
		t.Error("id kunci ganda (aktif dan lama) harus ditolak")
	}
	if _, err := NewSealer(testKey(1, 1), testKey(2, 2), testKey(2, 3)); err == nil {
		t.Error("id kunci ganda di antara kunci lama harus ditolak")
	}
}

func TestNewSealerErrorDoesNotLeakKey(t *testing.T) {
	key := []byte("rahasia-yang-tidak-boleh-bocor")
	_, err := NewSealer(Key{ID: 1, Material: key})
	if err == nil {
		t.Fatal("kunci pendek harus ditolak")
	}
	if strings.Contains(err.Error(), string(key)) {
		t.Errorf("pesan galat memuat kunci: %v", err)
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	s := newTestSealer(t)
	cases := []struct {
		name       string
		plain, aad []byte
	}{
		{"biasa", []byte("rahasia totp"), []byte("user_mfa_factors:42")},
		{"kosong", []byte{}, []byte("x")},
		{"biner", []byte{0x00, 0xff, 0x10, 0x80}, []byte{0x00}},
		{"besar", bytes.Repeat([]byte("A"), 1<<16), []byte("big")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sealed, err := s.Seal(c.plain, c.aad)
			if err != nil {
				t.Fatal(err)
			}
			if len(c.plain) > 0 && bytes.Contains(sealed, c.plain) {
				t.Error("ciphertext memuat plaintext mentah")
			}
			got, err := s.Open(sealed, c.aad)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, c.plain) {
				t.Errorf("Open = %x, want %x", got, c.plain)
			}
		})
	}
}

func TestSealOpenRejectEmptyAAD(t *testing.T) {
	s := newTestSealer(t)
	for _, aad := range [][]byte{nil, {}} {
		if _, err := s.Seal([]byte("x"), aad); !errors.Is(err, ErrEmptyAAD) {
			t.Errorf("Seal(aad=%#v): err = %v, want ErrEmptyAAD", aad, err)
		}
	}
	sealed, err := s.Seal([]byte("x"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	for _, aad := range [][]byte{nil, {}} {
		if _, err := s.Open(sealed, aad); !errors.Is(err, ErrEmptyAAD) {
			t.Errorf("Open(aad=%#v): err = %v, want ErrEmptyAAD", aad, err)
		}
	}
}

func TestSealFormatHasVersionKeyIDAndNonce(t *testing.T) {
	s, err := NewSealer(testKey(9, 7))
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.Seal([]byte("abc"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	if sealed[0] != formatV1 {
		t.Errorf("byte versi = %#x, want %#x", sealed[0], formatV1)
	}
	if sealed[1] != 9 {
		t.Errorf("byte id kunci = %d, want 9", sealed[1])
	}
	// versi (1) + id kunci (1) + nonce (12) + plaintext (3) + tag (16)
	if want := 1 + 1 + 12 + 3 + 16; len(sealed) != want {
		t.Errorf("len = %d, want %d", len(sealed), want)
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	s := newTestSealer(t)
	sealed, err := s.Seal([]byte("rahasia"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	// Ubah satu bit pada setiap posisi setelah header (nonce, ciphertext, tag).
	for i := headerSize; i < len(sealed); i++ {
		mut := append([]byte(nil), sealed...)
		mut[i] ^= 0x01
		if _, err := s.Open(mut, testAAD); !errors.Is(err, ErrOpen) {
			t.Fatalf("posisi %d: err = %v, want ErrOpen", i, err)
		}
	}
	// Terpotong atau kosong.
	for _, n := range []int{0, 1, 2, 5, 14, len(sealed) - 1} {
		if _, err := s.Open(sealed[:n], testAAD); err == nil {
			t.Errorf("Open(terpotong %d) harus gagal", n)
		}
	}
}

func TestOpenRejectsWrongAAD(t *testing.T) {
	s := newTestSealer(t)
	sealed, err := s.Seal([]byte("rahasia"), []byte("tabel-a:1"))
	if err != nil {
		t.Fatal(err)
	}
	for _, aad := range [][]byte{[]byte("tabel-a:2"), []byte("tabel-b:1"), []byte("tabel-a:1 ")} {
		if _, err := s.Open(sealed, aad); !errors.Is(err, ErrOpen) {
			t.Errorf("aad %q: err = %v, want ErrOpen", aad, err)
		}
	}
}

func TestOpenRejectsWrongKey(t *testing.T) {
	a, _ := NewSealer(testKey(1, 1))
	b, _ := NewSealer(testKey(1, 2)) // id sama, material beda
	sealed, err := a.Seal([]byte("rahasia"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Open(sealed, testAAD); !errors.Is(err, ErrOpen) {
		t.Errorf("err = %v, want ErrOpen", err)
	}
}

func TestSealUsesFreshNonce(t *testing.T) {
	s := newTestSealer(t)
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		sealed, err := s.Seal([]byte("sama"), testAAD)
		if err != nil {
			t.Fatal(err)
		}
		nonce := string(sealed[headerSize : headerSize+nonceSize])
		if seen[nonce] {
			t.Fatalf("nonce berulang pada iterasi %d", i)
		}
		seen[nonce] = true
	}
	a, _ := s.Seal([]byte("sama"), testAAD)
	b, _ := s.Seal([]byte("sama"), testAAD)
	if bytes.Equal(a, b) {
		t.Error("dua Seal atas masukan sama harus berbeda")
	}
}

func TestOpenRejectsUnknownVersionByte(t *testing.T) {
	s := newTestSealer(t)
	sealed, err := s.Seal([]byte("rahasia"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []byte{0x00, 0x02, 0x7f, 0xff} {
		mut := append([]byte(nil), sealed...)
		mut[0] = v
		if _, err := s.Open(mut, testAAD); !errors.Is(err, ErrUnknownVersion) {
			t.Errorf("versi %#x: err = %v, want ErrUnknownVersion", v, err)
		}
	}
}

func TestOpenRejectsUnknownKeyID(t *testing.T) {
	s := newTestSealer(t)
	sealed, err := s.Seal([]byte("rahasia"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	mut := append([]byte(nil), sealed...)
	mut[1] = 2
	if _, err := s.Open(mut, testAAD); !errors.Is(err, ErrUnknownKeyID) {
		t.Errorf("err = %v, want ErrUnknownKeyID", err)
	}
}

// Header (versi + id kunci) ikut AAD: mengganti id kunci ke kunci lain yang berisi MATERIAL SAMA tetap harus gagal.
func TestOpenRejectsHeaderTamperEvenWithSameKeyMaterial(t *testing.T) {
	s, err := NewSealer(testKey(1, 5), testKey(2, 5)) // dua id, material identik
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.Seal([]byte("rahasia"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	mut := append([]byte(nil), sealed...)
	mut[1] = 2
	if _, err := s.Open(mut, testAAD); !errors.Is(err, ErrOpen) {
		t.Errorf("header diubah: err = %v, want ErrOpen", err)
	}
	if got, err := s.Open(sealed, testAAD); err != nil || string(got) != "rahasia" {
		t.Errorf("asli harus tetap terbuka: %v", err)
	}
}

func TestPreviousKeyOnlyOpens(t *testing.T) {
	old, _ := NewSealer(testKey(1, 1))
	sealedOld, err := old.Seal([]byte("lama"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := NewSealer(testKey(2, 2), testKey(1, 1))
	if err != nil {
		t.Fatal(err)
	}
	got, err := rotated.Open(sealedOld, testAAD)
	if err != nil || string(got) != "lama" {
		t.Fatalf("kunci lama harus membuka data lama: %v", err)
	}
	sealedNew, err := rotated.Seal([]byte("baru"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	if sealedNew[1] != 2 {
		t.Errorf("Seal harus memakai kunci aktif (id 2), dapat id %d", sealedNew[1])
	}
	if _, err := old.Open(sealedNew, testAAD); !errors.Is(err, ErrUnknownKeyID) {
		t.Errorf("sealer lama tak punya id 2: err = %v, want ErrUnknownKeyID", err)
	}
}

func TestSealerDoesNotRetainCallerKeySlice(t *testing.T) {
	k := testKey(1, 9)
	s, err := NewSealer(k)
	if err != nil {
		t.Fatal(err)
	}
	sealed, err := s.Seal([]byte("x"), testAAD)
	if err != nil {
		t.Fatal(err)
	}
	for i := range k.Material { // pemanggil menghapus kuncinya
		k.Material[i] = 0
	}
	if _, err := s.Open(sealed, testAAD); err != nil {
		t.Errorf("Sealer harus tetap berfungsi setelah slice kunci pemanggil ditimpa: %v", err)
	}
}

func TestKeyIDOfIsStableFingerprintOfMaterial(t *testing.T) {
	a := bytes.Repeat([]byte{1}, KeySize)
	b := bytes.Repeat([]byte{2}, KeySize)
	if KeyIDOf(a) != KeyIDOf(append([]byte(nil), a...)) {
		t.Error("id harus deterministik atas material yang sama")
	}
	// Id tidak boleh membocorkan material: hanya 1 byte dari hash; material berbeda hampir selalu id berbeda.
	diff := 0
	for i := 0; i < 64; i++ {
		x := bytes.Repeat([]byte{byte(i)}, KeySize)
		if KeyIDOf(x) != KeyIDOf(b) {
			diff++
		}
	}
	if diff < 50 {
		t.Errorf("id terlalu sering bertabrakan: %d/64 berbeda", diff)
	}
}

// Rotasi tanpa konfigurasi id: kunci aktif baru + kunci lama membuka data lama, dan data baru tetap terbuka setelah
// kunci lama dilepas (id diturunkan dari material, jadi tidak berubah ketika konfigurasi berubah).
func TestNewSealerFromMaterialSupportsRotation(t *testing.T) {
	oldMat := bytes.Repeat([]byte{0x11}, KeySize)
	newMat := bytes.Repeat([]byte{0x22}, KeySize)

	old, err := NewSealerFromMaterial(oldMat)
	if err != nil {
		t.Fatal(err)
	}
	sealedOld, err := old.Seal([]byte("lama"), testAAD)
	if err != nil {
		t.Fatal(err)
	}

	rotated, err := NewSealerFromMaterial(newMat, oldMat)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := rotated.Open(sealedOld, testAAD); err != nil || string(got) != "lama" {
		t.Fatalf("kunci lama harus membuka data lama: %v", err)
	}
	sealedNew, err := rotated.Seal([]byte("baru"), testAAD)
	if err != nil {
		t.Fatal(err)
	}

	finished, err := NewSealerFromMaterial(newMat) // kunci lama dilepas setelah reseal
	if err != nil {
		t.Fatal(err)
	}
	if got, err := finished.Open(sealedNew, testAAD); err != nil || string(got) != "baru" {
		t.Fatalf("data baru harus terbuka tanpa kunci lama: %v", err)
	}
	if _, err := finished.Open(sealedOld, testAAD); err == nil {
		t.Error("data lama tidak boleh terbuka tanpa kunci lama")
	}
}

func TestNewSealerFromMaterialRejectsSameKeyAndBadLength(t *testing.T) {
	m := bytes.Repeat([]byte{0x33}, KeySize)
	if _, err := NewSealerFromMaterial(m, m); err == nil {
		t.Error("kunci lama sama dengan kunci aktif harus ditolak")
	}
	if _, err := NewSealerFromMaterial(m[:16]); err == nil {
		t.Error("panjang salah harus ditolak")
	}
}

// Tanpa rotasi, config meneruskan SecretKeyPrevious kosong: entri kosong/nil dilewati, bukan ditolak.
func TestNewSealerFromMaterialSkipsEmptyPrevious(t *testing.T) {
	m := bytes.Repeat([]byte{0x44}, KeySize)
	for name, prev := range map[string][][]byte{
		"nil":          {nil},
		"slice kosong": {{}},
		"campuran":     {nil, {}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			s, err := NewSealerFromMaterial(m, prev...)
			if err != nil {
				t.Fatalf("previous kosong harus dilewati: %v", err)
			}
			sealed, err := s.Seal([]byte("x"), testAAD)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.Open(sealed, testAAD); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Tidak kosong dengan panjang salah tetap ditolak, juga bila diselingi entri kosong.
	if _, err := NewSealerFromMaterial(m, nil, m[:5]); err == nil {
		t.Error("previous tidak kosong dengan panjang salah harus ditolak")
	}
	// Kunci aktif kosong tetap ditolak.
	if _, err := NewSealerFromMaterial(nil); err == nil {
		t.Error("kunci aktif kosong harus ditolak")
	}
}

// Key tidak boleh mencetak Material lewat fmt (%v %+v %#v %s %x %d) maupun slog.
func TestKeyNeverPrintsMaterial(t *testing.T) {
	mat := bytes.Repeat([]byte{0xAB}, KeySize)
	k := Key{ID: 7, Material: mat}
	outs := map[string]string{
		"%v": fmt.Sprintf("%v", k), "%+v": fmt.Sprintf("%+v", k), "%#v": fmt.Sprintf("%#v", k),
		"%s": fmt.Sprintf("%s", k), "%x": fmt.Sprintf("%x", k), "%d": fmt.Sprintf("%d", k),
		"ptr %v": fmt.Sprintf("%v", &k), "slice %v": fmt.Sprintf("%v", []Key{k}), "String": k.String(), "GoString": k.GoString(),
	}
	var tb, jb bytes.Buffer
	slog.New(slog.NewTextHandler(&tb, nil)).Info("k", "key", k)
	slog.New(slog.NewJSONHandler(&jb, nil)).Info("k", "key", k)
	outs["slog text"], outs["slog json"] = tb.String(), jb.String()
	leaks := []string{string(mat), fmt.Sprintf("%x", mat), fmt.Sprintf("%v", mat), "171 171", "abab"}
	for name, out := range outs {
		for _, leak := range leaks {
			if strings.Contains(out, leak) {
				t.Errorf("%s membocorkan material (%q): %s", name, leak, out)
			}
		}
		if !strings.Contains(out, "7") {
			t.Errorf("%s harus tetap memuat id kunci: %s", name, out)
		}
	}
	// Material tetap dapat dipakai lewat field (redaksi hanya pada pencetakan).
	if !bytes.Equal(k.Material, mat) {
		t.Error("Material tidak boleh berubah")
	}
}

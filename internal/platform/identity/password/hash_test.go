package password

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// testParams adalah parameter kecil agar tes cepat; BUKAN untuk produksi.
var testParams = Params{MemoryKiB: 1024, Time: 1, Parallelism: 1}

var phcFormat = regexp.MustCompile(`^\$argon2id\$v=19\$m=\d+,t=\d+,p=\d+\$[A-Za-z0-9+/]+\$[A-Za-z0-9+/]+$`)

func TestHashPHCFormat(t *testing.T) {
	h := &Hasher{Params: testParams}
	phc, err := h.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !phcFormat.MatchString(phc) {
		t.Fatalf("format PHC salah: %q", phc)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=1024,t=1,p=1$") {
		t.Fatalf("parameter tidak tercermin: %q", phc)
	}
	if strings.Contains(phc, "correct") {
		t.Fatal("hash memuat sandi")
	}
}

func TestHashSaltIsRandomPerHash(t *testing.T) {
	h := &Hasher{Params: testParams}
	a, err := h.Hash("same password value")
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.Hash("same password value")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("dua hash dari sandi sama identik: salt tidak acak")
	}
}

func TestVerifyCorrectAndWrong(t *testing.T) {
	h := &Hasher{Params: testParams}
	phc, err := h.Hash("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	ok, rehash, err := h.Verify("correct horse battery", phc)
	if err != nil || !ok || rehash {
		t.Fatalf("benar: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	for _, wrong := range []string{"correct horse batterY", "", "correct horse battery ", "x"} {
		ok, _, err := h.Verify(wrong, phc)
		if err != nil || ok {
			t.Fatalf("salah %q: ok=%v err=%v", wrong, ok, err)
		}
	}
}

func TestVerifyNeedsRehashWhenParamsRaised(t *testing.T) {
	old := &Hasher{Params: testParams}
	phc, err := old.Hash("rehash me please")
	if err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]Params{
		"memori":     {MemoryKiB: 2048, Time: 1, Parallelism: 1},
		"waktu":      {MemoryKiB: 1024, Time: 2, Parallelism: 1},
		"paralelism": {MemoryKiB: 1024, Time: 1, Parallelism: 2},
	} {
		ok, rehash, err := (&Hasher{Params: p}).Verify("rehash me please", phc)
		if err != nil || !ok || !rehash {
			t.Errorf("%s: ok=%v rehash=%v err=%v", name, ok, rehash, err)
		}
	}
	// Sandi salah tidak pernah meminta rehash.
	ok, rehash, err := (&Hasher{Params: Params{MemoryKiB: 2048, Time: 1, Parallelism: 1}}).Verify("salah", phc)
	if err != nil || ok || rehash {
		t.Errorf("salah: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
}

func TestVerifyRejectsMalformedPHCWithoutPanic(t *testing.T) {
	h := &Hasher{Params: testParams}
	good, err := h.Hash("whatever password")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(good, "$") // "", argon2id, v=19, params, salt, hash
	join := func(p ...string) string { return strings.Join(p, "$") }
	cases := map[string]string{
		"kosong":            "",
		"sampah":            "bukan-phc",
		"hanya pemisah":     "$$$$$",
		"argon2i":           join("", "argon2i", parts[2], parts[3], parts[4], parts[5]),
		"argon2d":           join("", "argon2d", parts[2], parts[3], parts[4], parts[5]),
		"bcrypt":            "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz01234",
		"versi lama":        join("", "argon2id", "v=16", parts[3], parts[4], parts[5]),
		"versi hilang":      join("", "argon2id", parts[3], parts[4], parts[5]),
		"segmen kurang":     join("", "argon2id", parts[2], parts[3], parts[4]),
		"segmen lebih":      good + "$extra",
		"param hilang":      join("", "argon2id", parts[2], "m=1024,t=1", parts[4], parts[5]),
		"param urutan":      join("", "argon2id", parts[2], "t=1,m=1024,p=1", parts[4], parts[5]),
		"param bukan angka": join("", "argon2id", parts[2], "m=abc,t=1,p=1", parts[4], parts[5]),
		"param negatif":     join("", "argon2id", parts[2], "m=-1,t=1,p=1", parts[4], parts[5]),
		"param nol":         join("", "argon2id", parts[2], "m=1024,t=0,p=1", parts[4], parts[5]),
		"param p nol":       join("", "argon2id", parts[2], "m=1024,t=1,p=0", parts[4], parts[5]),
		"memori < 8*p":      join("", "argon2id", parts[2], "m=8,t=1,p=2", parts[4], parts[5]),
		"salt bukan base64": join("", "argon2id", parts[2], parts[3], "***", parts[5]),
		"salt kosong":       join("", "argon2id", parts[2], parts[3], "", parts[5]),
		"hash kosong":       join("", "argon2id", parts[2], parts[3], parts[4], ""),
		"salt terlalu pendek": join("", "argon2id", parts[2], parts[3],
			"AAAA", parts[5]),
		"hash terlalu pendek": join("", "argon2id", parts[2], parts[3], parts[4], "AAAA"),
		"salt terlalu panjang": join("", "argon2id", parts[2], parts[3],
			strings.Repeat("A", 200), parts[5]),
		"hash terlalu panjang": join("", "argon2id", parts[2], parts[3], parts[4], strings.Repeat("A", 400)),
		"param overflow":       join("", "argon2id", parts[2], "m=99999999999999999999,t=1,p=1", parts[4], parts[5]),
		"NUL":                  good + "\x00",
	}
	for name, phc := range cases {
		t.Run(name, func(t *testing.T) {
			ok, rehash, err := h.Verify("whatever password", phc)
			if ok || rehash {
				t.Fatalf("ok=%v rehash=%v: PHC cacat harus ditolak", ok, rehash)
			}
			if !errors.Is(err, ErrInvalidHash) {
				t.Fatalf("err = %v, want ErrInvalidHash", err)
			}
		})
	}
}

// DoS lewat PHC yang dimanipulasi: parameter di atas batas wajar ditolak SEBELUM argon2 dijalankan.
func TestVerifyRejectsExtremeParamsFast(t *testing.T) {
	h := &Hasher{Params: testParams}
	good, err := h.Hash("whatever password")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(good, "$")
	for _, params := range []string{
		fmt.Sprintf("m=%d,t=1,p=1", MaxMemoryKiB+1),
		"m=4294967295,t=1,p=1",
		fmt.Sprintf("m=1024,t=%d,p=1", MaxTime+1),
		"m=1024,t=4294967295,p=1",
		fmt.Sprintf("m=1024,t=1,p=%d", MaxParallelism+1),
		"m=1024,t=1,p=255",
		"m=1024,t=1,p=256",
	} {
		phc := strings.Join([]string{"", "argon2id", parts[2], params, parts[4], parts[5]}, "$")
		start := time.Now()
		ok, _, err := h.Verify("whatever password", phc)
		if ok || !errors.Is(err, ErrInvalidHash) {
			t.Errorf("%s: ok=%v err=%v, want ErrInvalidHash", params, ok, err)
		}
		if d := time.Since(start); d > 200*time.Millisecond {
			t.Errorf("%s: penolakan lambat (%v): argon2 kemungkinan dijalankan", params, d)
		}
	}
}

func TestPasswordEdgeCases(t *testing.T) {
	h := &Hasher{Params: testParams}
	cases := map[string]string{
		"kosong":    "",
		"unicode":   "päßwörd-密码-🔐-кириллица",
		"NUL":       "abc\x00def\x00ghi",
		"hanya NUL": "\x00\x00\x00",
		"spasi":     "   spasi   ",
		"1024 byte": strings.Repeat("a", MaxPasswordBytes),
	}
	for name, pw := range cases {
		t.Run(name, func(t *testing.T) {
			phc, err := h.Hash(pw)
			if err != nil {
				t.Fatalf("Hash: %v", err)
			}
			ok, _, err := h.Verify(pw, phc)
			if err != nil || !ok {
				t.Fatalf("Verify benar: ok=%v err=%v", ok, err)
			}
			ok, _, err = h.Verify(pw+"x", phc)
			if err != nil || ok {
				t.Fatalf("Verify salah: ok=%v err=%v", ok, err)
			}
		})
	}
	// NUL tidak boleh memotong sandi (mis. perilaku C-string).
	phc, err := h.Hash("abc\x00def")
	if err != nil {
		t.Fatal(err)
	}
	if ok, _, _ := h.Verify("abc", phc); ok {
		t.Fatal("sandi dipotong pada NUL")
	}
}

func TestPasswordTooLongRejected(t *testing.T) {
	h := &Hasher{Params: testParams}
	long := strings.Repeat("a", MaxPasswordBytes+1)
	if _, err := h.Hash(long); !errors.Is(err, ErrPasswordTooLong) {
		t.Fatalf("Hash err = %v, want ErrPasswordTooLong", err)
	}
	phc, _ := h.Hash("a valid password")
	ok, _, err := h.Verify(long, phc)
	if ok || err != nil {
		t.Fatalf("Verify sandi terlalu panjang: ok=%v err=%v (harus ditolak biasa, bukan galat)", ok, err)
	}
}

func TestHashRejectsInvalidParams(t *testing.T) {
	for name, p := range map[string]Params{
		"nol":                  {},
		"memori < 8*p":         {MemoryKiB: 8, Time: 1, Parallelism: 2},
		"memori di atas":       {MemoryKiB: MaxMemoryKiB + 1, Time: 1, Parallelism: 1},
		"waktu di atas":        {MemoryKiB: 1024, Time: MaxTime + 1, Parallelism: 1},
		"paralelisme di atas":  {MemoryKiB: 1024, Time: 1, Parallelism: MaxParallelism + 1},
		"paralelisme nol":      {MemoryKiB: 1024, Time: 1, Parallelism: 0},
		"waktu nol":            {MemoryKiB: 1024, Time: 0, Parallelism: 1},
		"memori nol":           {MemoryKiB: 0, Time: 1, Parallelism: 1},
		"memori sangat rendah": {MemoryKiB: 4, Time: 1, Parallelism: 1},
	} {
		if _, err := (&Hasher{Params: p}).Hash("a valid password"); !errors.Is(err, ErrInvalidParams) {
			t.Errorf("%s: err = %v, want ErrInvalidParams", name, err)
		}
	}
}

func TestErrorsNeverContainPassword(t *testing.T) {
	const secret = "SuperSecretPassw0rd-jangan-bocor"
	h := &Hasher{Params: testParams}
	_, _, err := h.Verify(secret, "bukan-phc")
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("err = %v", err)
	}
	_, err = (&Hasher{}).Hash(secret)
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("err = %v", err)
	}
	_, err = h.Hash(secret + strings.Repeat("x", MaxPasswordBytes))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultParamsMeetOWASPAndConfigBounds(t *testing.T) {
	p := DefaultParams()
	if err := p.ValidateConfig(); err != nil {
		t.Fatalf("default tidak lolos validasi konfigurasi: %v", err)
	}
	if p.MemoryKiB < 19*1024 || p.Time < 2 || p.Parallelism < 1 {
		t.Fatalf("default di bawah minimum OWASP (m>=19 MiB, t>=2, p>=1): %+v", p)
	}
}

func TestValidateConfigBounds(t *testing.T) {
	ok := []Params{
		DefaultParams(),
		{MemoryKiB: 47104, Time: 1, Parallelism: 1}, // OWASP: m=46 MiB, t=1
		{MemoryKiB: MaxMemoryKiB, Time: MaxTime, Parallelism: MaxParallelism},
	}
	for _, p := range ok {
		if err := p.ValidateConfig(); err != nil {
			t.Errorf("%+v ditolak: %v", p, err)
		}
	}
	bad := map[string]Params{
		"memori di bawah OWASP":     {MemoryKiB: 19455, Time: 2, Parallelism: 1},
		"memori di atas batas":      {MemoryKiB: MaxMemoryKiB + 1, Time: 2, Parallelism: 1},
		"t=1 butuh memori besar":    {MemoryKiB: 19456, Time: 1, Parallelism: 1},
		"waktu nol":                 {MemoryKiB: 19456, Time: 0, Parallelism: 1},
		"waktu di atas batas":       {MemoryKiB: 19456, Time: MaxTime + 1, Parallelism: 1},
		"paralelisme nol":           {MemoryKiB: 19456, Time: 2, Parallelism: 0},
		"paralelisme di atas batas": {MemoryKiB: 19456, Time: 2, Parallelism: MaxParallelism + 1},
	}
	for name, p := range bad {
		err := p.ValidateConfig()
		var pe *ParamError
		if !errors.As(err, &pe) {
			t.Errorf("%s: err = %v, want *ParamError", name, err)
		}
	}
}

// --- Dummy dan keseragaman waktu (Review Focus 1) ---

func TestDummyHashIsValidPHCAndNeverMatches(t *testing.T) {
	d := NewDummy(testParams)
	phc := d.DummyHash()
	if !phcFormat.MatchString(phc) {
		t.Fatalf("format PHC salah: %q", phc)
	}
	if d.DummyHash() != phc {
		t.Fatal("hash tiruan harus stabil per proses")
	}
	for _, guess := range []string{"", "password", "admin", "dummy"} {
		ok, _, err := d.Verify(guess, phc)
		if err != nil || ok {
			t.Fatalf("hash tiruan cocok dengan %q: ok=%v err=%v", guess, ok, err)
		}
	}
	if Dummy() == nil || Dummy().Params != DefaultParams() {
		t.Fatal("Dummy() harus memakai DefaultParams")
	}
}

func median(ds []time.Duration) time.Duration {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}

// TestVerifyUnknownUserTimingMatchesKnownUser: login untuk email tak dikenal (Verify atas hash tiruan) harus memakan
// waktu yang sama dengan sandi salah pada akun nyata. Median atas 200 iterasi berselang-seling setelah pemanasan.
func TestVerifyUnknownUserTimingMatchesKnownUser(t *testing.T) {
	// Parameter kecil tetapi cukup besar agar biaya argon2 mendominasi noise penjadwalan.
	params := Params{MemoryKiB: 8192, Time: 2, Parallelism: 1}
	real := &Hasher{Params: params}
	realPHC, err := real.Hash("the real user password")
	if err != nil {
		t.Fatal(err)
	}
	dummy := NewDummy(params)
	dummyPHC := dummy.DummyHash()

	const iterations = 200
	for range 10 { // pemanasan: alokasi memori dan cache
		_, _, _ = real.Verify("wrong password attempt", realPHC)
		_, _, _ = dummy.Verify("wrong password attempt", dummyPHC)
	}
	known := make([]time.Duration, 0, iterations)
	unknown := make([]time.Duration, 0, iterations)
	timeVerify := func(h *Hasher, phc string) time.Duration {
		start := time.Now()
		_, _, _ = h.Verify("wrong password attempt", phc)
		return time.Since(start)
	}
	for i := range iterations {
		// Urutan bergantian: biaya alokasi/GC dari pemanggilan sebelumnya tidak boleh selalu membebani satu sisi.
		if i%2 == 0 {
			known = append(known, timeVerify(real, realPHC))
			unknown = append(unknown, timeVerify(dummy, dummyPHC))
		} else {
			unknown = append(unknown, timeVerify(dummy, dummyPHC))
			known = append(known, timeVerify(real, realPHC))
		}
	}
	mk, mu := median(known), median(unknown)
	hi, lo := mk, mu
	if lo > hi {
		hi, lo = lo, hi
	}
	ratio := float64(hi) / float64(lo)
	t.Logf("median known=%v unknown=%v rasio=%.3f", mk, mu, ratio)
	if ratio > 1.2 {
		t.Fatalf("rasio median waktu %.3f > 1,2: email tak dikenal dapat dibedakan lewat waktu", ratio)
	}
}

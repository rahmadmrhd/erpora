package mfa

import (
	"bytes"
	"context"
	"encoding/base32"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"erpora/internal/platform/crypto"
	"erpora/internal/platform/identity/store"
	"erpora/sdk/identity"
)

const (
	userA = "0190a1b2-0000-7000-8000-00000000000a"
	userB = "0190a1b2-0000-7000-8000-00000000000b"
	admin = "0190a1b2-0000-7000-8000-0000000000ad"
)

var _ Store = (*store.Store)(nil) // store sungguhan memenuhi antarmuka sempit paket ini

type env struct {
	svc   *Service
	st    *fakeStore
	lock  *fakeLockout
	audit *fakeAudit
	clk   *clock
	seal  *crypto.Sealer
	logs  *bytes.Buffer
	ctx   context.Context
}

func newEnv(t *testing.T) *env {
	t.Helper()
	seal, err := crypto.NewSealerFromMaterial(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	e := &env{
		st: newFakeStore(), lock: &fakeLockout{}, audit: &fakeAudit{}, seal: seal, logs: &bytes.Buffer{},
		clk: &clock{t: time.Unix(1234567890, 0).UTC()},
	}
	e.st.addUser(testTenant, userA, "ana@example.com")
	e.st.addUser(testTenant, userB, "budi@example.com")
	e.st.addUser(testTenant, admin, "root@example.com", "administrator")
	e.svc, err = New(Deps{
		Store: e.st, Sealer: seal, Lockout: e.lock, Auditor: e.audit, Issuer: "ERPora", Now: e.clk.Now,
		Logger: slog.New(slog.NewTextHandler(e.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatal(err)
	}
	e.ctx = WithClientIP(contextWithTenant(testTenant), "203.0.113.7")
	return e
}

func secretBytes(t *testing.T, info EnrollmentInfo) []byte {
	t.Helper()
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(info.Secret)
	if err != nil {
		t.Fatalf("secret bukan base32 tanpa padding: %v", err)
	}
	return b
}

// enroll mendaftarkan dan mengonfirmasi TOTP; mengembalikan secret dan recovery code. Setelah itu jam digeser dua
// langkah penuh agar kode pada langkah konfirmasi tidak menghalangi uji berikutnya.
func (e *env) enroll(t *testing.T, user string) ([]byte, []string) {
	t.Helper()
	info, err := e.svc.BeginEnrollment(e.ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	sec := secretBytes(t, info)
	codes, err := e.svc.ConfirmEnrollment(e.ctx, user, TOTP{}.Code(sec, e.clk.Now()))
	if err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(2 * Period * time.Second)
	return sec, codes
}

func TestBeginEnrollmentURIAndSealing(t *testing.T) {
	e := newEnv(t)
	info, err := e.svc.BeginEnrollment(e.ctx, userA)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(info.Secret, "=") {
		t.Errorf("secret base32 harus tanpa padding: %q", info.Secret)
	}
	sec := secretBytes(t, info)
	if len(sec) != 20 {
		t.Errorf("panjang secret = %d byte, want 20", len(sec))
	}
	u, err := url.Parse(info.OTPAuthURI)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "otpauth" || u.Host != "totp" {
		t.Errorf("URI = %s", info.OTPAuthURI)
	}
	if label, _ := url.PathUnescape(strings.TrimPrefix(u.EscapedPath(), "/")); label != "ERPora:ana@example.com" {
		t.Errorf("label = %q", label)
	}
	q := u.Query()
	for k, want := range map[string]string{"secret": info.Secret, "issuer": "ERPora", "algorithm": "SHA1", "digits": "6", "period": "30"} {
		if q.Get(k) != want {
			t.Errorf("param %s = %q, want %q", k, q.Get(k), want)
		}
	}
	// tersimpan tersegel: kolom tidak memuat secret mentah maupun base32
	f, ok := e.st.factor(userA)
	if !ok {
		t.Fatal("faktor pending tidak tersimpan")
	}
	if bytes.Contains(f.SecretSealed, sec) || bytes.Contains(f.SecretSealed, []byte(info.Secret)) {
		t.Error("secret tersimpan tanpa tersegel")
	}
	got, err := e.seal.Open(f.SecretSealed, secretAAD(userA))
	if err != nil || !bytes.Equal(got, sec) {
		t.Fatalf("Open dengan AAD benar: %v", err)
	}
	if _, err := e.seal.Open(f.SecretSealed, secretAAD(userB)); err == nil {
		t.Error("AAD pengguna lain berhasil membuka")
	}
	if _, err := e.seal.Open(f.SecretSealed, []byte("signing_keys|private|"+userA)); err == nil {
		t.Error("AAD tujuan lain berhasil membuka")
	}
	// tidak aktif sebelum konfirmasi
	if f.ConfirmedAt != nil {
		t.Error("faktor aktif sebelum ConfirmEnrollment")
	}
	if req, err := e.svc.Required(e.ctx, userA); err != nil || req {
		t.Errorf("Required sebelum konfirmasi = %v, %v, want false", req, err)
	}
	if err := e.svc.Verify(e.ctx, userA, TOTP{}.Code(sec, e.clk.Now())); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("Verify pada pendaftaran belum aktif = %v, want ErrInvalidCode", err)
	}
	if _, err := e.svc.RegenerateRecoveryCodes(e.ctx, userA); !errors.Is(err, ErrNotEnrolled) {
		t.Errorf("Regenerate tanpa faktor aktif = %v, want ErrNotEnrolled", err)
	}
}

func TestBeginEnrollmentReplacesPendingButNotConfirmed(t *testing.T) {
	e := newEnv(t)
	i1, _ := e.svc.BeginEnrollment(e.ctx, userA)
	i2, err := e.svc.BeginEnrollment(e.ctx, userA)
	if err != nil || i1.Secret == i2.Secret {
		t.Fatalf("pendaftaran ulang pending harus mengganti secret: %v", err)
	}
	// kode dari secret lama tidak dapat mengonfirmasi
	if _, err := e.svc.ConfirmEnrollment(e.ctx, userA, TOTP{}.Code(secretBytes(t, i1), e.clk.Now())); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("konfirmasi dengan secret lama = %v", err)
	}
	if _, err := e.svc.ConfirmEnrollment(e.ctx, userA, TOTP{}.Code(secretBytes(t, i2), e.clk.Now())); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.BeginEnrollment(e.ctx, userA); !errors.Is(err, ErrAlreadyEnrolled) {
		t.Errorf("Begin pada faktor aktif = %v, want ErrAlreadyEnrolled", err)
	}
}

func TestConfirmEnrollment(t *testing.T) {
	e := newEnv(t)
	info, _ := e.svc.BeginEnrollment(e.ctx, userA)
	sec := secretBytes(t, info)
	good := TOTP{}.Code(sec, e.clk.Now())

	t.Run("kode salah gagal dan tidak mengaktifkan", func(t *testing.T) {
		bad := fmt.Sprintf("%06d", (atoi(good)+1)%1000000)
		if _, err := e.svc.ConfirmEnrollment(e.ctx, userA, bad); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("err = %v", err)
		}
		if f, _ := e.st.factor(userA); f.ConfirmedAt != nil {
			t.Error("faktor aktif setelah konfirmasi gagal")
		}
		if n := e.st.unusedCodes(userA); n != 0 {
			t.Errorf("recovery code dibuat pada konfirmasi gagal: %d", n)
		}
		if fails, _ := e.lock.counts(); fails != 1 {
			t.Errorf("kegagalan konfirmasi harus dicatat ke lockout: %d", fails)
		}
	})

	t.Run("kode benar mengaktifkan dan menghasilkan 10 recovery code", func(t *testing.T) {
		codes, err := e.svc.ConfirmEnrollment(e.ctx, userA, good)
		if err != nil {
			t.Fatal(err)
		}
		if len(codes) != 10 {
			t.Fatalf("jumlah recovery code = %d", len(codes))
		}
		f, _ := e.st.factor(userA)
		if f.ConfirmedAt == nil {
			t.Fatal("faktor belum aktif")
		}
		if n := e.st.unusedCodes(userA); n != 10 {
			t.Errorf("hash tersimpan = %d", n)
		}
		// hash tersimpan, bukan plaintext
		for _, c := range codes {
			canon, _ := normalizeRecoveryCode(c)
			if _, ok := e.st.codes[userA][string(hashRecoveryCode(userA, canon))]; !ok {
				t.Errorf("hash kode %q tidak tersimpan", c)
			}
			for h := range e.st.codes[userA] {
				if strings.Contains(h, canon) {
					t.Error("plaintext recovery code tersimpan")
				}
			}
		}
		if req, _ := e.svc.Required(e.ctx, userA); !req {
			t.Error("Required harus true setelah faktor aktif")
		}
	})

	t.Run("kode konfirmasi tidak dapat dipakai ulang untuk Verify", func(t *testing.T) {
		if err := e.svc.Verify(e.ctx, userA, good); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("replay kode konfirmasi = %v", err)
		}
	})

	t.Run("konfirmasi kedua ditolak dan tidak mengganti recovery code", func(t *testing.T) {
		before := e.st.hashes[userA]
		if _, err := e.svc.ConfirmEnrollment(e.ctx, userA, good); !errors.Is(err, ErrAlreadyEnrolled) {
			t.Errorf("err = %v, want ErrAlreadyEnrolled", err)
		}
		if len(e.st.hashes[userA]) != len(before) || !bytes.Equal(e.st.hashes[userA][0], before[0]) {
			t.Error("recovery code berubah")
		}
	})
}

func TestConfirmWithoutBegin(t *testing.T) {
	e := newEnv(t)
	if _, err := e.svc.ConfirmEnrollment(e.ctx, userA, "123456"); !errors.Is(err, ErrNotEnrolled) {
		t.Errorf("err = %v, want ErrNotEnrolled", err)
	}
}

func TestConfirmReplaceFailureIsReported(t *testing.T) {
	e := newEnv(t)
	info, _ := e.svc.BeginEnrollment(e.ctx, userA)
	e.st.failReplace = true
	codes, err := e.svc.ConfirmEnrollment(e.ctx, userA, TOTP{}.Code(secretBytes(t, info), e.clk.Now()))
	if err == nil || codes != nil {
		t.Fatalf("galat penyimpanan recovery code harus dilaporkan: %v, %v", codes, err)
	}
	// faktor sudah aktif; pemulihan lewat RegenerateRecoveryCodes
	e.st.failReplace = false
	got, err := e.svc.RegenerateRecoveryCodes(e.ctx, userA)
	if err != nil || len(got) != 10 {
		t.Fatalf("Regenerate = %d, %v", len(got), err)
	}
}

func TestVerifyTOTPFlow(t *testing.T) {
	e := newEnv(t)
	sec, _ := e.enroll(t, userA)
	code := func() string { return TOTP{}.Code(sec, e.clk.Now()) }

	if err := e.svc.Verify(e.ctx, userA, code()); err != nil {
		t.Fatalf("kode benar: %v", err)
	}
	if fails, succs := e.lock.counts(); fails != 0 || succs != 1 {
		t.Errorf("lockout: fails=%d succs=%d", fails, succs)
	}
	if got := e.lock.successes[0]; got.userID != userA || got.ip != "203.0.113.7" {
		t.Errorf("RegisterSuccess = %+v", got)
	}
	// replay pada langkah yang sama ditolak
	if err := e.svc.Verify(e.ctx, userA, code()); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("replay = %v", err)
	}
	// kode langkah sebelumnya (dalam jendela) ditolak
	prev := TOTP{}.Code(sec, e.clk.Now().Add(-Period*time.Second))
	if err := e.svc.Verify(e.ctx, userA, prev); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("kode langkah sebelumnya = %v", err)
	}
	if fails, _ := e.lock.counts(); fails != 2 {
		t.Errorf("kegagalan tercatat = %d, want 2", fails)
	}
}

func TestVerifyClockDrift(t *testing.T) {
	e := newEnv(t)
	sec, _ := e.enroll(t, userA)
	// perangkat pengguna lebih lambat satu langkah: diterima
	if err := e.svc.Verify(e.ctx, userA, TOTP{}.Code(sec, e.clk.Now().Add(-Period*time.Second))); err != nil {
		t.Errorf("drift -1: %v", err)
	}
	e.clk.Advance(10 * Period * time.Second)
	if err := e.svc.Verify(e.ctx, userA, TOTP{}.Code(sec, e.clk.Now().Add(Period*time.Second))); err != nil {
		t.Errorf("drift +1: %v", err)
	}
	e.clk.Advance(10 * Period * time.Second)
	if err := e.svc.Verify(e.ctx, userA, TOTP{}.Code(sec, e.clk.Now().Add(2*Period*time.Second))); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("drift +2 = %v", err)
	}
	if err := e.svc.Verify(e.ctx, userA, TOTP{}.Code(sec, e.clk.Now().Add(-2*Period*time.Second))); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("drift -2 = %v", err)
	}
}

// Review Focus 5: banyak verifikasi serentak dengan kode sama, tepat satu sukses.
func TestVerifyConcurrentSameCodeExactlyOneWins(t *testing.T) {
	e := newEnv(t)
	sec, _ := e.enroll(t, userA)
	code := TOTP{}.Code(sec, e.clk.Now())
	var wins, invalid atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			switch err := e.svc.Verify(e.ctx, userA, code); {
			case err == nil:
				wins.Add(1)
			case errors.Is(err, ErrInvalidCode):
				invalid.Add(1)
			default:
				t.Errorf("galat tak terduga: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if wins.Load() != 1 || invalid.Load() != 23 {
		t.Fatalf("wins=%d invalid=%d, want 1 dan 23", wins.Load(), invalid.Load())
	}
}

func TestUniformErrorForAllFailureCauses(t *testing.T) {
	e := newEnv(t)
	sec, codes := e.enroll(t, userA)
	good := TOTP{}.Code(sec, e.clk.Now())
	if err := e.svc.Verify(e.ctx, userA, good); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.UseRecoveryCode(e.ctx, userA, codes[0]); err != nil { // jadikan codes[0] terpakai
		t.Fatal(err)
	}
	causes := map[string]func() error{
		"salah":             func() error { return e.svc.Verify(e.ctx, userA, fmt.Sprintf("%06d", (atoi(good)+7)%1000000)) },
		"replay":            func() error { return e.svc.Verify(e.ctx, userA, good) },
		"cacat":             func() error { return e.svc.Verify(e.ctx, userA, "abc") },
		"kosong":            func() error { return e.svc.Verify(e.ctx, userA, "") },
		"tanpa faktor":      func() error { return e.svc.Verify(e.ctx, userB, good) },
		"recovery salah":    func() error { return e.svc.UseRecoveryCode(e.ctx, userA, "AAAA-AAAA-AAAA-AAAA") },
		"recovery cacat":    func() error { return e.svc.UseRecoveryCode(e.ctx, userA, "x") },
		"recovery terpakai": func() error { return e.svc.UseRecoveryCode(e.ctx, userA, codes[0]) },
	}
	var want string
	for name, run := range causes {
		err := run()
		if !errors.Is(err, ErrInvalidCode) {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if want == "" {
			want = err.Error()
		}
		if err.Error() != want {
			t.Errorf("%s: pesan galat berbeda: %q vs %q", name, err.Error(), want)
		}
		if strings.Contains(err.Error(), good) || strings.Contains(err.Error(), string(sec)) {
			t.Errorf("%s: galat memuat kode/secret", name)
		}
	}
}

func TestRecoveryCodes(t *testing.T) {
	e := newEnv(t)
	_, codes := e.enroll(t, userA)

	t.Run("normalisasi huruf besar/kecil dan tanda hubung", func(t *testing.T) {
		variant := strings.ToLower(strings.ReplaceAll(codes[0], "-", ""))
		if err := e.svc.UseRecoveryCode(e.ctx, userA, variant); err != nil {
			t.Fatalf("varian %q: %v", variant, err)
		}
		if err := e.svc.UseRecoveryCode(e.ctx, userA, codes[1]); err != nil {
			t.Fatalf("kanonik: %v", err)
		}
		if n := e.st.unusedCodes(userA); n != 8 {
			t.Errorf("tersisa %d, want 8", n)
		}
		if fails, succs := e.lock.counts(); fails != 0 || succs != 2 {
			t.Errorf("lockout: fails=%d succs=%d", fails, succs)
		}
	})
	t.Run("sekali pakai", func(t *testing.T) {
		if err := e.svc.UseRecoveryCode(e.ctx, userA, codes[0]); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("pakai ulang = %v", err)
		}
	})
	t.Run("kode pengguna lain tidak berlaku", func(t *testing.T) {
		if err := e.svc.UseRecoveryCode(e.ctx, userB, codes[2]); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("kode milik A dipakai B = %v", err)
		}
		if n := e.st.unusedCodes(userA); n != 8 {
			t.Errorf("kode A terpakai oleh B: %d", n)
		}
	})
	t.Run("pemakaian serentak tepat satu sukses", func(t *testing.T) {
		var wins atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if err := e.svc.UseRecoveryCode(e.ctx, userA, codes[5]); err == nil {
					wins.Add(1)
				} else if !errors.Is(err, ErrInvalidCode) {
					t.Errorf("galat tak terduga: %v", err)
				}
			}()
		}
		close(start)
		wg.Wait()
		if wins.Load() != 1 {
			t.Fatalf("wins = %d, want 1", wins.Load())
		}
	})
	t.Run("regenerasi mencabut kode lama", func(t *testing.T) {
		fresh, err := e.svc.RegenerateRecoveryCodes(e.ctx, userA)
		if err != nil || len(fresh) != 10 {
			t.Fatalf("Regenerate = %d, %v", len(fresh), err)
		}
		if err := e.svc.UseRecoveryCode(e.ctx, userA, codes[3]); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("kode lama masih berlaku: %v", err)
		}
		if err := e.svc.UseRecoveryCode(e.ctx, userA, fresh[0]); err != nil {
			t.Errorf("kode baru: %v", err)
		}
	})
}

func TestLockoutIntegration(t *testing.T) {
	t.Run("kegagalan memakai (userID, IP) dan keadaan jeda menjadi LockedError", func(t *testing.T) {
		e := newEnv(t)
		e.enroll(t, userA)
		e.lock.state = LockState{RetryAfter: 30 * time.Second}
		err := e.svc.Verify(e.ctx, userA, "000000")
		var le *LockedError
		if !errors.Is(err, ErrLocked) || !errors.As(err, &le) || le.RetryAfter != 30*time.Second {
			t.Fatalf("err = %v", err)
		}
		if got := e.lock.failures[0]; got.userID != userA || got.ip != "203.0.113.7" {
			t.Errorf("RegisterFailure = %+v", got)
		}
		e.lock.state = LockState{AccountLocked: true}
		if err := e.svc.UseRecoveryCode(e.ctx, userA, "AAAA-AAAA-AAAA-AAAA"); !errors.Is(err, ErrLocked) {
			t.Errorf("akun terkunci = %v", err)
		}
		e.lock.state = LockState{PairBlocked: true, RetryAfter: time.Hour}
		if err := e.svc.Verify(e.ctx, userA, "000000"); !errors.Is(err, ErrLocked) {
			t.Errorf("pasangan diblokir = %v", err)
		}
	})
	t.Run("galat Lockout gagal tertutup", func(t *testing.T) {
		e := newEnv(t)
		e.enroll(t, userA)
		e.lock.failErr = errors.New("db mati")
		if err := e.svc.Verify(e.ctx, userA, "000000"); err == nil {
			t.Error("harus galat")
		}
	})
	t.Run("galat RegisterSuccess tidak menggagalkan verifikasi sah", func(t *testing.T) {
		e := newEnv(t)
		sec, _ := e.enroll(t, userA)
		e.lock.succErr = errors.New("db mati")
		if err := e.svc.Verify(e.ctx, userA, TOTP{}.Code(sec, e.clk.Now())); err != nil {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("pra-pemeriksaan memblokir kode benar tanpa mengonsumsi step", func(t *testing.T) {
		e := newEnv(t)
		sec, codes := e.enroll(t, userA)
		svc, err := New(Deps{Store: e.st, Sealer: e.seal, Lockout: checkingLockout{e.lock}, Issuer: "ERPora", Now: e.clk.Now})
		if err != nil {
			t.Fatal(err)
		}
		e.lock.blocked = LockState{PairBlocked: true, RetryAfter: time.Hour}
		good := TOTP{}.Code(sec, e.clk.Now())
		before, _ := e.st.factor(userA)
		if err := svc.Verify(e.ctx, userA, good); !errors.Is(err, ErrLocked) {
			t.Fatalf("kode benar saat diblokir = %v, want ErrLocked", err)
		}
		if err := svc.UseRecoveryCode(e.ctx, userA, codes[0]); !errors.Is(err, ErrLocked) {
			t.Fatalf("recovery saat diblokir = %v, want ErrLocked", err)
		}
		after, _ := e.st.factor(userA)
		if after.LastUsedStep != before.LastUsedStep || e.st.unusedCodes(userA) != 10 {
			t.Error("pra-pemeriksaan tidak boleh mengonsumsi step atau recovery code")
		}
		if fails, _ := e.lock.counts(); fails != 0 {
			t.Error("percobaan yang diblokir tidak boleh menambah hitungan kegagalan")
		}
		e.lock.blocked = LockState{}
		if err := svc.Verify(e.ctx, userA, good); err != nil {
			t.Errorf("setelah blokir lepas: %v", err)
		}
		e.lock.checkErr = errors.New("db mati")
		if err := svc.Verify(e.ctx, userA, good); err == nil || errors.Is(err, ErrInvalidCode) {
			t.Errorf("galat pra-pemeriksaan harus gagal tertutup: %v", err)
		}
	})
}

func TestReset(t *testing.T) {
	t.Run("menghapus faktor dan recovery code dan tercatat", func(t *testing.T) {
		e := newEnv(t)
		e.enroll(t, userA)
		if err := e.svc.Reset(e.ctx, "cli:core:reset-mfa", userA); err != nil {
			t.Fatal(err)
		}
		if _, ok := e.st.factor(userA); ok {
			t.Error("faktor masih ada")
		}
		if n := e.st.unusedCodes(userA); n != 0 {
			t.Errorf("recovery code tersisa: %d", n)
		}
		if req, _ := e.svc.Required(e.ctx, userA); req {
			t.Error("Required harus false setelah reset (non-admin)")
		}
		var found bool
		for _, ev := range e.audit.events {
			if ev.Action == "mfa.reset" && ev.Actor == "cli:core:reset-mfa" && ev.UserID == userA {
				found = true
			}
		}
		if !found {
			t.Errorf("audit reset tidak tercatat: %+v", e.audit.events)
		}
		if _, err := e.svc.BeginEnrollment(e.ctx, userA); err != nil {
			t.Errorf("pendaftaran ulang: %v", err)
		}
	})
	t.Run("actor kosong ditolak", func(t *testing.T) {
		e := newEnv(t)
		if err := e.svc.Reset(e.ctx, "", userA); err == nil {
			t.Error("actor kosong diterima")
		}
	})
	t.Run("pengguna biasa tidak dapat mereset akun lain", func(t *testing.T) {
		e := newEnv(t)
		e.enroll(t, userA)
		ctx := identity.WithPrincipal(e.ctx, identity.Principal{UserID: userB})
		if err := e.svc.Reset(ctx, userB, userA); !errors.Is(err, ErrForbidden) {
			t.Errorf("err = %v, want ErrForbidden", err)
		}
		if _, ok := e.st.factor(userA); !ok {
			t.Error("faktor A terhapus oleh pengguna biasa")
		}
	})
	t.Run("pengguna tidak dapat mereset dirinya sendiri", func(t *testing.T) {
		e := newEnv(t)
		e.enroll(t, admin)
		ctx := identity.WithPrincipal(e.ctx, identity.Principal{UserID: admin})
		if err := e.svc.Reset(ctx, admin, admin); !errors.Is(err, ErrForbidden) {
			t.Errorf("self-reset = %v, want ErrForbidden", err)
		}
		if _, ok := e.st.factor(admin); !ok {
			t.Error("faktor admin terhapus")
		}
	})
	t.Run("actor harus sama dengan Principal", func(t *testing.T) {
		e := newEnv(t)
		e.enroll(t, userA)
		ctx := identity.WithPrincipal(e.ctx, identity.Principal{UserID: userB})
		if err := e.svc.Reset(ctx, admin, userA); !errors.Is(err, ErrForbidden) {
			t.Errorf("actor palsu = %v, want ErrForbidden", err)
		}
	})
	t.Run("administrator dapat mereset akun lain", func(t *testing.T) {
		e := newEnv(t)
		e.enroll(t, userA)
		ctx := identity.WithPrincipal(e.ctx, identity.Principal{UserID: admin})
		if err := e.svc.Reset(ctx, admin, userA); err != nil {
			t.Fatal(err)
		}
		if _, ok := e.st.factor(userA); ok {
			t.Error("faktor masih ada")
		}
	})
	t.Run("pengguna di luar tenant: tidak ditemukan", func(t *testing.T) {
		e := newEnv(t)
		ctx := WithClientIP(contextWithTenant(otherTenant), "203.0.113.7")
		if err := e.svc.Reset(ctx, "cli:core:reset-mfa", userA); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})
}

func TestRequired(t *testing.T) {
	e := newEnv(t)
	if req, err := e.svc.Required(e.ctx, admin); err != nil || !req {
		t.Errorf("administrator tanpa faktor = %v, %v, want true", req, err)
	}
	if req, err := e.svc.Required(e.ctx, userA); err != nil || req {
		t.Errorf("pengguna biasa tanpa faktor = %v, %v, want false", req, err)
	}
	e.enroll(t, userA)
	if req, err := e.svc.Required(e.ctx, userA); err != nil || !req {
		t.Errorf("pengguna biasa dengan faktor aktif = %v, %v, want true (ADR-0013 section 4)", req, err)
	}
}

// IDOR: operasi hanya atas pengguna dari Principal; tenant dari context.
func TestSelfOnlyAndTenantIsolation(t *testing.T) {
	e := newEnv(t)
	sec, codes := e.enroll(t, userA)
	asB := identity.WithPrincipal(e.ctx, identity.Principal{UserID: userB})
	code := TOTP{}.Code(sec, e.clk.Now())
	before, _ := e.st.factor(userA)

	if err := e.svc.Verify(asB, userA, code); !errors.Is(err, ErrForbidden) {
		t.Errorf("Verify A oleh B = %v", err)
	}
	if err := e.svc.UseRecoveryCode(asB, userA, codes[0]); !errors.Is(err, ErrForbidden) {
		t.Errorf("recovery A oleh B = %v", err)
	}
	if _, err := e.svc.BeginEnrollment(asB, userA); !errors.Is(err, ErrForbidden) {
		t.Errorf("Begin A oleh B = %v", err)
	}
	if _, err := e.svc.ConfirmEnrollment(asB, userA, code); !errors.Is(err, ErrForbidden) {
		t.Errorf("Confirm A oleh B = %v", err)
	}
	if _, err := e.svc.RegenerateRecoveryCodes(asB, userA); !errors.Is(err, ErrForbidden) {
		t.Errorf("Regenerate A oleh B = %v", err)
	}
	if fails, succs := e.lock.counts(); fails != 0 || succs != 0 {
		t.Errorf("pelanggaran IDOR tidak boleh menyentuh lockout korban: %d/%d", fails, succs)
	}
	if after, _ := e.st.factor(userA); after.LastUsedStep != before.LastUsedStep || !bytes.Equal(after.SecretSealed, before.SecretSealed) {
		t.Error("state A berubah")
	}
	if n := e.st.unusedCodes(userA); n != 10 {
		t.Errorf("kode A terpakai: %d", n)
	}
	asA := identity.WithPrincipal(e.ctx, identity.Principal{UserID: userA})
	if err := e.svc.Verify(asA, userA, code); err != nil {
		t.Errorf("Verify oleh pemilik: %v", err)
	}
	// tenant lain: userA bukan anggota
	other := WithClientIP(contextWithTenant(otherTenant), "203.0.113.7")
	e.clk.Advance(2 * Period * time.Second)
	if err := e.svc.Verify(other, userA, TOTP{}.Code(sec, e.clk.Now())); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("Verify lintas tenant = %v, want ErrInvalidCode", err)
	}
	if err := e.svc.UseRecoveryCode(other, userA, codes[0]); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("recovery lintas tenant = %v, want ErrInvalidCode", err)
	}
	if _, err := e.svc.BeginEnrollment(other, userA); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Begin lintas tenant = %v, want ErrNotFound", err)
	}
}

func TestNoTenantInContext(t *testing.T) {
	e := newEnv(t)
	if err := e.svc.Verify(context.Background(), userA, "123456"); !errors.Is(err, ErrNoTenant) {
		t.Errorf("err = %v", err)
	}
	if _, err := e.svc.Required(context.Background(), userA); !errors.Is(err, ErrNoTenant) {
		t.Errorf("err = %v", err)
	}
}

func TestSecretsNeverLoggedOrInErrors(t *testing.T) {
	e := newEnv(t)
	info, _ := e.svc.BeginEnrollment(e.ctx, userA)
	sec := secretBytes(t, info)
	good := TOTP{}.Code(sec, e.clk.Now())
	codes, _ := e.svc.ConfirmEnrollment(e.ctx, userA, good)
	e.clk.Advance(2 * Period * time.Second)
	_ = e.svc.Verify(e.ctx, userA, "999999")
	_ = e.svc.UseRecoveryCode(e.ctx, userA, "ZZZZ-ZZZZ-ZZZZ-ZZZZ")
	_ = e.svc.UseRecoveryCode(e.ctx, userA, codes[0])
	_, _ = e.svc.RegenerateRecoveryCodes(e.ctx, userA)
	_ = e.svc.Reset(e.ctx, "cli:core:reset-mfa", userA)
	log := e.logs.String()
	if log == "" {
		t.Fatal("uji tidak bermakna: tidak ada log sama sekali")
	}
	for _, secret := range append([]string{info.Secret, "999999", "ZZZZ-ZZZZ-ZZZZ-ZZZZ"}, codes...) {
		if strings.Contains(log, secret) {
			t.Errorf("log memuat rahasia %q:\n%s", secret, log)
		}
	}
	if rendered := fmt.Sprintf("%v %+v %#v", info, info, info); strings.Contains(rendered, info.Secret) {
		t.Error("EnrollmentInfo tidak menyamarkan secret pada format")
	}
}

func TestNewValidatesDeps(t *testing.T) {
	e := newEnv(t)
	base := Deps{Store: e.st, Sealer: e.seal, Lockout: e.lock, Issuer: "ERPora"}
	if _, err := New(base); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Deps){
		"tanpa store":         func(d *Deps) { d.Store = nil },
		"tanpa sealer":        func(d *Deps) { d.Sealer = nil },
		"tanpa lockout":       func(d *Deps) { d.Lockout = nil },
		"tanpa issuer":        func(d *Deps) { d.Issuer = "" },
		"issuer bertitik dua": func(d *Deps) { d.Issuer = "A:B" },
	} {
		d := base
		mut(&d)
		if _, err := New(d); err == nil {
			t.Errorf("%s: New harus gagal", name)
		}
	}
}

// Balapan Begin vs Confirm: pendaftaran pending diganti (secret B) di antara pemuatan faktor (secret A, kode A
// terverifikasi) dan penetapan konfirmasi. Konfirmasi terikat CAS pada secret yang diverifikasi, jadi faktor tidak
// aktif dengan secret B yang tak pernah dibuktikan pemiliknya.
func TestConfirmEnrollmentBoundToVerifiedSecret(t *testing.T) {
	e := newEnv(t)
	infoA, _ := e.svc.BeginEnrollment(e.ctx, userA)
	secA := secretBytes(t, infoA)
	codeA := TOTP{}.Code(secA, e.clk.Now())
	var infoB EnrollmentInfo
	e.st.beforeConfirm = func() {
		e.st.beforeConfirm = nil
		var err error
		if infoB, err = e.svc.BeginEnrollment(e.ctx, userA); err != nil {
			t.Error(err)
		}
	}
	codes, err := e.svc.ConfirmEnrollment(e.ctx, userA, codeA)
	if !errors.Is(err, ErrInvalidCode) || codes != nil {
		t.Fatalf("konfirmasi dengan secret yang sudah diganti harus ErrInvalidCode, dapat %v, %v", codes, err)
	}
	f, ok := e.st.factor(userA)
	if !ok || f.ConfirmedAt != nil {
		t.Fatalf("faktor tidak boleh aktif: %+v", f)
	}
	if n := e.st.unusedCodes(userA); n != 0 {
		t.Errorf("recovery code tak boleh terbit: %d", n)
	}
	// pendaftaran terbaru (secret B) tetap dapat dikonfirmasi dengan kode B
	if _, err := e.svc.ConfirmEnrollment(e.ctx, userA, TOTP{}.Code(secretBytes(t, infoB), e.clk.Now())); err != nil {
		t.Fatalf("konfirmasi secret terbaru: %v", err)
	}
}

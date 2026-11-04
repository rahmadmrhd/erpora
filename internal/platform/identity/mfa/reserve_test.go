package mfa

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// reservingLockout memenuhi LockReserver di atas fakeLockout dan mencatat penyelesaian setiap reservasi.
type reservingLockout struct {
	*fakeLockout
	mu2       sync.Mutex
	reserved  int
	failed    int
	released  int
	blocked   LockState // dikembalikan Reserve bila restricted
	reserveEr error
}

func (r *reservingLockout) Reserve(_ context.Context, _, _ string) (any, LockState, error) {
	r.mu2.Lock()
	defer r.mu2.Unlock()
	if r.reserveEr != nil {
		return nil, LockState{}, r.reserveEr
	}
	if r.blocked.restricted() {
		return nil, r.blocked, nil
	}
	r.reserved++
	return r.reserved, LockState{}, nil
}

func (r *reservingLockout) Fail(context.Context, any) (LockState, error) {
	r.mu2.Lock()
	defer r.mu2.Unlock()
	r.failed++
	return r.state, r.failErr
}

func (r *reservingLockout) Release(context.Context, any) error {
	r.mu2.Lock()
	defer r.mu2.Unlock()
	r.released++
	return nil
}

func (r *reservingLockout) counts2() (reserved, failed, released int) {
	r.mu2.Lock()
	defer r.mu2.Unlock()
	return r.reserved, r.failed, r.released
}

func newReservingEnv(t *testing.T) (*env, *reservingLockout) {
	t.Helper()
	e := newEnv(t)
	rl := &reservingLockout{fakeLockout: e.lock}
	svc, err := New(Deps{Store: e.st, Sealer: e.seal, Lockout: rl, Auditor: e.audit, Issuer: "ERPora", Now: e.clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	return e, rl
}

// Kegagalan kode: reservasi dipertahankan (Fail), tidak dicatat dua kali lewat RegisterFailure.
func TestReservationWrongCodeKeptAsFailureOnce(t *testing.T) {
	e, rl := newReservingEnv(t)
	e.enroll(t, userA)
	for name, run := range map[string]func() error{
		"totp":     func() error { return e.svc.Verify(e.ctx, userA, "000000") },
		"recovery": func() error { return e.svc.UseRecoveryCode(e.ctx, userA, "AAAA-AAAA-AAAA-AAAA") },
	} {
		_, f0, r0 := rl.counts2()
		if err := run(); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("%s: %v", name, err)
		}
		_, f1, r1 := rl.counts2()
		if f1-f0 != 1 || r1-r0 != 0 {
			t.Errorf("%s: Fail=%d Release=%d, want 1 dan 0", name, f1-f0, r1-r0)
		}
	}
	if fails, _ := e.lock.counts(); fails != 0 {
		t.Errorf("RegisterFailure dipanggil %d kali; reservasi sudah mencatat kegagalan", fails)
	}
}

// Kode benar dan keluar bukan-kegagalan: reservasi dilepas.
func TestReservationReleasedOnSuccessAndNonFailureExits(t *testing.T) {
	e, rl := newReservingEnv(t)
	sec, _ := e.enroll(t, userA)
	_, f0, r0 := rl.counts2()
	if err := e.svc.Verify(e.ctx, userA, TOTP{}.Code(sec, e.clk.Now())); err != nil {
		t.Fatal(err)
	}
	_, f1, r1 := rl.counts2()
	if f1 != f0 || r1-r0 != 1 {
		t.Errorf("sukses: Fail=%d Release=%d, want 0 dan 1", f1-f0, r1-r0)
	}
	if _, succs := e.lock.counts(); succs != 1 {
		t.Errorf("RegisterSuccess = %d", succs)
	}
	// ConfirmEnrollment tanpa pendaftaran pending: galat bukan kegagalan.
	_, _, r2 := rl.counts2()
	if _, err := e.svc.ConfirmEnrollment(e.ctx, userB, "123456"); !errors.Is(err, ErrNotEnrolled) {
		t.Fatalf("err = %v", err)
	}
	if _, _, r3 := rl.counts2(); r3-r2 != 1 {
		t.Errorf("ErrNotEnrolled harus melepas reservasi: %d", r3-r2)
	}
	// ConfirmEnrollment sukses melepas reservasi (tanpa RegisterSuccess: itu dicatat pemanggil).
	info, err := e.svc.BeginEnrollment(e.ctx, userB)
	if err != nil {
		t.Fatal(err)
	}
	_, _, r4 := rl.counts2()
	succBefore := func() int { _, s := e.lock.counts(); return s }()
	if _, err := e.svc.ConfirmEnrollment(e.ctx, userB, TOTP{}.Code(secretBytes(t, info), e.clk.Now())); err != nil {
		t.Fatal(err)
	}
	if _, _, r5 := rl.counts2(); r5-r4 != 1 {
		t.Errorf("konfirmasi sukses harus melepas reservasi: %d", r5-r4)
	}
	if _, s := e.lock.counts(); s != succBefore {
		t.Error("ConfirmEnrollment tidak boleh memanggil RegisterSuccess")
	}
}

func TestReservationBlockedRejectsWithoutVerifyingOrReserving(t *testing.T) {
	e, rl := newReservingEnv(t)
	sec, codes := e.enroll(t, userA)
	rl.blocked = LockState{PairBlocked: true, RetryAfter: 30 * time.Second}
	good := TOTP{}.Code(sec, e.clk.Now())
	before, _ := e.st.factor(userA)
	var le *LockedError
	if err := e.svc.Verify(e.ctx, userA, good); !errors.As(err, &le) || le.RetryAfter != 30*time.Second {
		t.Fatalf("Verify = %v", err)
	}
	if err := e.svc.UseRecoveryCode(e.ctx, userA, codes[0]); !errors.Is(err, ErrLocked) {
		t.Fatalf("UseRecoveryCode = %v", err)
	}
	after, _ := e.st.factor(userA)
	if after.LastUsedStep != before.LastUsedStep || e.st.unusedCodes(userA) != 10 {
		t.Error("percobaan yang ditolak tidak boleh mengonsumsi step atau recovery code")
	}
	rl.blocked = LockState{AccountLocked: true}
	if err := e.svc.Verify(e.ctx, userA, good); !errors.Is(err, ErrLocked) {
		t.Fatalf("akun terkunci = %v", err)
	}
	rl.blocked = LockState{}
	rl.reserveEr = errors.New("db mati")
	if err := e.svc.Verify(e.ctx, userA, good); err == nil || errors.Is(err, ErrInvalidCode) {
		t.Errorf("galat Reserve harus gagal tertutup: %v", err)
	}
}

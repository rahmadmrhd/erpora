package identity

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	sdkidentity "erpora/sdk/identity"
)

const (
	cpOld    = "sandi-lama-panjang"
	cpStrong = "kata-sandi-baru-yang-panjang-2026"
)

func cpSetup() (*fakeEnv, sdkidentity.Service) {
	e := newFakeEnv()
	e.store.addUser(userA, "a@example.test", "h:"+cpOld)
	return e, e.service()
}

// ChangePassword memakai reservasi atomik (seperti login): N permintaan serentak dengan sandi lama salah pada pasangan
// yang hanya menjatahkan satu tebakan hanya meloloskan SATU ke verifikasi.
func TestChangePassword_ConcurrentGuessesReachVerifyAtMostOnce(t *testing.T) {
	e, svc := cpSetup()
	e.lock.limit = 1
	const burst = 24
	var wg sync.WaitGroup
	var wrong, limited atomic.Int32
	start := make(chan struct{})
	for range burst {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := svc.ChangePassword(principalCtx(userA, sessA, "pwd"), sdkidentity.ChangeInput{CurrentPassword: "salah-salah-salah", NewPassword: cpStrong})
			switch {
			case errors.Is(err, sdkidentity.ErrWrongCurrentPassword):
				wrong.Add(1)
			default:
				if _, ok := sdkidentity.AsRateLimited(err); ok {
					limited.Add(1)
				} else {
					t.Errorf("err = %v", err)
				}
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := e.gate.verifies.Load(); got != 1 {
		t.Fatalf("verifikasi = %d, want tepat 1", got)
	}
	if wrong.Load() != 1 || limited.Load() != burst-1 {
		t.Fatalf("salah=%d dibatasi=%d, want 1 dan %d", wrong.Load(), limited.Load(), burst-1)
	}
	if e.lock.failed != 1 || e.lock.released != 0 {
		t.Fatalf("Fail=%d Release=%d, want 1 dan 0 (kegagalan dipertahankan)", e.lock.failed, e.lock.released)
	}
}

// Reservasi dilepas pada sukses dan semua keluar non-kegagalan; kebijakan sandi baru dicek sebelum lockout (tanpa reservasi).
func TestChangePassword_ReservationReleasedOnNonFailureExits(t *testing.T) {
	check := func(name string, e *fakeEnv, wantReserved, wantReleased, wantFailed int) {
		t.Helper()
		if e.lock.reserved != wantReserved || e.lock.released != wantReleased || e.lock.failed != wantFailed || e.lock.active != wantFailed {
			t.Errorf("%s: reserved=%d released=%d failed=%d active=%d, want %d/%d/%d", name, e.lock.reserved, e.lock.released, e.lock.failed, e.lock.active, wantReserved, wantReleased, wantFailed)
		}
	}
	t.Run("sukses", func(t *testing.T) {
		e, svc := cpSetup()
		if err := svc.ChangePassword(principalCtx(userA, sessA, "pwd"), sdkidentity.ChangeInput{CurrentPassword: cpOld, NewPassword: cpStrong}); err != nil {
			t.Fatal(err)
		}
		check("sukses", e, 1, 1, 0)
	})
	t.Run("galat hash sandi baru", func(t *testing.T) {
		e, svc := cpSetup()
		e.gate.hashErr = errBoom
		if err := svc.ChangePassword(principalCtx(userA, sessA, "pwd"), sdkidentity.ChangeInput{CurrentPassword: cpOld, NewPassword: cpStrong}); err == nil {
			t.Fatal("harus galat")
		}
		check("galat hash", e, 1, 1, 0)
	})
	t.Run("pengguna tidak termuat", func(t *testing.T) {
		e, svc := cpSetup()
		delete(e.store.users, userA)
		if err := svc.ChangePassword(principalCtx(userA, sessA, "pwd"), sdkidentity.ChangeInput{CurrentPassword: cpOld, NewPassword: cpStrong}); err == nil {
			t.Fatal("harus galat")
		}
		check("GetUser gagal", e, 1, 1, 0)
	})
	t.Run("sandi lama salah: kegagalan dipertahankan", func(t *testing.T) {
		e, svc := cpSetup()
		if err := svc.ChangePassword(principalCtx(userA, sessA, "pwd"), sdkidentity.ChangeInput{CurrentPassword: "salah-salah-salah", NewPassword: cpStrong}); !errors.Is(err, sdkidentity.ErrWrongCurrentPassword) {
			t.Fatalf("err = %v", err)
		}
		check("salah", e, 1, 0, 1)
	})
	t.Run("kebijakan sandi baru: tanpa reservasi", func(t *testing.T) {
		e, svc := cpSetup()
		if err := svc.ChangePassword(principalCtx(userA, sessA, "pwd"), sdkidentity.ChangeInput{CurrentPassword: cpOld, NewPassword: "pendek"}); err == nil {
			t.Fatal("harus galat")
		}
		check("kebijakan", e, 0, 0, 0)
		if e.gate.verifies.Load() != 0 {
			t.Error("verifikasi tidak boleh berjalan")
		}
	})
	t.Run("diblokir: tanpa reservasi dan tanpa verifikasi", func(t *testing.T) {
		e, svc := cpSetup()
		e.lock.limit = 1
		e.lock.active = 1
		if _, ok := sdkidentity.AsRateLimited(svc.ChangePassword(principalCtx(userA, sessA, "pwd"), sdkidentity.ChangeInput{CurrentPassword: cpOld, NewPassword: cpStrong})); !ok {
			t.Fatal("harus RateLimited")
		}
		if e.lock.reserved != 0 || e.gate.verifies.Load() != 0 {
			t.Errorf("reserved=%d verifies=%d", e.lock.reserved, e.gate.verifies.Load())
		}
	})
}

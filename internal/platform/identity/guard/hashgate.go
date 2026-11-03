package guard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"erpora/internal/platform/identity/password"
)

// DefaultHashGateWait adalah lama maksimum sebuah permintaan menunggu slot hash sebelum dijawab 429.
const DefaultHashGateWait = 2 * time.Second

// ErrHashBusy berarti gerbang hash penuh melebihi batas tunggu (dipetakan ke 429 rate_limited + Retry-After lewat
// RateLimitedError; tidak membedakan email ada atau tidak karena jalur akun-tak-dikenal ikut melewati gerbang).
var ErrHashBusy = errors.New("guard: pembatas hash penuh")

// RateLimitedError adalah galat yang harus dijawab 429 rate_limited dengan header Retry-After. Cause menyebut sumber
// untuk log (tidak pernah dikirim ke klien).
type RateLimitedError struct {
	RetryAfter time.Duration
	Cause      error
}

func (e *RateLimitedError) Error() string {
	if e.Cause != nil {
		return "guard: dibatasi laju: " + e.Cause.Error()
	}
	return "guard: dibatasi laju"
}

func (e *RateLimitedError) Unwrap() error { return e.Cause }

// AsRateLimited memberi tahu apakah err (atau galat yang dibungkusnya) adalah *RateLimitedError dan berapa lama klien
// harus menunggu. Pemanggil menjawab httpx.TooManyRequests(w, r, retryAfter).
func AsRateLimited(err error) (retryAfter time.Duration, ok bool) {
	var rl *RateLimitedError
	if errors.As(err, &rl) {
		return rl.RetryAfter, true
	}
	return 0, false
}

// HashGate adalah semaphore berkapasitas terbatas yang membungkus setiap Hash/Verify/verifikasi tiruan argon2id agar
// login serentak tidak menghabiskan RAM (default 19 MiB per Verify) atau CPU. Kapasitas dari ERP_ARGON2_MAX_CONCURRENT
// (config.Config.Argon2MaxConcurrent). Aman untuk dipakai serentak. Dibuat dengan NewHashGate.
type HashGate struct {
	sem  chan struct{}
	wait time.Duration
}

// NewHashGate membuat gerbang berkapasitas capacity (>= 1) yang membuat permintaan menunggu paling lama maxWait (> 0)
// untuk slot sebelum ErrHashBusy.
func NewHashGate(capacity int, maxWait time.Duration) (*HashGate, error) {
	if capacity < 1 {
		return nil, errors.New("guard: kapasitas HashGate harus >= 1")
	}
	if maxWait <= 0 {
		return nil, errors.New("guard: batas tunggu HashGate harus > 0")
	}
	return &HashGate{sem: make(chan struct{}, capacity), wait: maxWait}, nil
}

// Capacity adalah jumlah slot.
func (g *HashGate) Capacity() int { return cap(g.sem) }

// Do menjalankan fn setelah mendapat slot. Gerbang penuh lebih lama dari batas tunggu: *RateLimitedError (Cause
// ErrHashBusy, RetryAfter 1 detik); ctx batal: galat ctx (bukan 429). Slot selalu dilepas, juga saat fn panik. Galat fn
// dikembalikan apa adanya.
func (g *HashGate) Do(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case g.sem <- struct{}{}:
	default:
		timer := time.NewTimer(g.wait)
		defer timer.Stop()
		select {
		case g.sem <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return &RateLimitedError{RetryAfter: time.Second, Cause: ErrHashBusy}
		}
	}
	defer func() { <-g.sem }()
	return fn()
}

// HashRunner adalah bagian HashGate yang dibutuhkan pemanggil (Authenticator) agar tes dapat mengamati atau
// memaksa penuhnya gerbang.
type HashRunner interface {
	Do(ctx context.Context, fn func() error) error
}

// runVerify menjalankan h.Verify melalui r. Galat gerbang dikembalikan sebagai err (periksa AsRateLimited); galat
// Hasher (mis. password.ErrInvalidHash) juga lewat err.
func runVerify(ctx context.Context, r HashRunner, h *password.Hasher, plain, phc string) (ok, needsRehash bool, err error) {
	err = r.Do(ctx, func() error {
		var verr error
		ok, needsRehash, verr = h.Verify(plain, phc)
		return verr
	})
	if err != nil {
		return false, false, err
	}
	return ok, needsRehash, nil
}

// Verify menjalankan h.Verify melalui gerbang (lihat runVerify untuk semantik galat).
func (g *HashGate) Verify(ctx context.Context, h *password.Hasher, plain, phc string) (ok, needsRehash bool, err error) {
	return runVerify(ctx, g, h, plain, phc)
}

// Hash menjalankan h.Hash melalui gerbang.
func (g *HashGate) Hash(ctx context.Context, h *password.Hasher, plain string) (phc string, err error) {
	err = g.Do(ctx, func() error {
		var herr error
		phc, herr = h.Hash(plain)
		return herr
	})
	if err != nil {
		return "", fmt.Errorf("guard: hash melalui gerbang: %w", err)
	}
	return phc, nil
}

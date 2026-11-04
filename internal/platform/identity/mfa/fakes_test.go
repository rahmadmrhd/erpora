package mfa

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

var (
	testTenant  = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	otherTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

func contextWithTenant(t tenancy.TenantID) context.Context {
	return tenancy.WithTenant(context.Background(), t)
}

// fakeStore meniru semantik atomik store.Store (bersyarat di bawah satu mutex) untuk uji unit.
type fakeStore struct {
	mu      sync.Mutex
	users   map[string]store.User                // id -> user
	members map[tenancy.TenantID]map[string]bool // tenant -> user id
	roles   map[string]map[string]bool           // user id -> role key
	factors map[string]*store.MFAFactor          // user id -> faktor
	codes   map[string]map[string]*time.Time     // user id -> hash -> used_at
	hashes  map[string][][]byte                  // user id -> set hash terakhir (inspeksi)

	failReplace bool
	// beforeConfirm (opsional) dipanggil di awal ConfirmMFAFactor sebelum mengunci.
	beforeConfirm func()
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:   map[string]store.User{},
		members: map[tenancy.TenantID]map[string]bool{},
		roles:   map[string]map[string]bool{},
		factors: map[string]*store.MFAFactor{},
		codes:   map[string]map[string]*time.Time{},
		hashes:  map[string][][]byte{},
	}
}

func (f *fakeStore) addUser(t tenancy.TenantID, id, email string, roles ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users[id] = store.User{ID: id, Email: email}
	if f.members[t] == nil {
		f.members[t] = map[string]bool{}
	}
	f.members[t][id] = true
	f.roles[id] = map[string]bool{}
	for _, r := range roles {
		f.roles[id][r] = true
	}
}

func (f *fakeStore) member(t tenancy.TenantID, id string) bool { return f.members[t][id] }

func (f *fakeStore) GetUser(_ context.Context, t tenancy.TenantID, id string) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.member(t, id) {
		return store.User{}, store.ErrNotFound
	}
	return f.users[id], nil
}

func (f *fakeStore) GetMFAFactor(_ context.Context, t tenancy.TenantID, id string) (store.MFAFactor, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fa, ok := f.factors[id]
	if !ok || !f.member(t, id) {
		return store.MFAFactor{}, store.ErrNotFound
	}
	c := *fa
	c.SecretSealed = bytes.Clone(fa.SecretSealed)
	return c, nil
}

func (f *fakeStore) UpsertPendingMFAFactor(_ context.Context, t tenancy.TenantID, id string, sealed []byte, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.member(t, id) {
		return false, store.ErrNotFound
	}
	if fa, ok := f.factors[id]; ok && fa.ConfirmedAt != nil {
		return false, nil
	}
	f.factors[id] = &store.MFAFactor{ID: uuid.NewString(), UserID: id, Type: "totp", SecretSealed: bytes.Clone(sealed), CreatedAt: at}
	return true, nil
}

func (f *fakeStore) ConfirmMFAFactor(_ context.Context, t tenancy.TenantID, id string, expectedSealed []byte, step int64, at time.Time) (bool, error) {
	if f.beforeConfirm != nil { // uji balapan: sisipkan perubahan pending di antara muat faktor dan konfirmasi
		f.beforeConfirm()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	fa, ok := f.factors[id]
	if !ok || !f.member(t, id) || fa.ConfirmedAt != nil || !bytes.Equal(fa.SecretSealed, expectedSealed) {
		return false, nil
	}
	fa.ConfirmedAt = &at
	fa.LastUsedStep = step
	return true, nil
}

func (f *fakeStore) AdvanceMFAStep(_ context.Context, t tenancy.TenantID, id string, step int64) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fa, ok := f.factors[id]
	if !ok || !f.member(t, id) || fa.ConfirmedAt == nil || fa.LastUsedStep >= step {
		return false, nil
	}
	fa.LastUsedStep = step
	return true, nil
}

func (f *fakeStore) ResetMFA(_ context.Context, t tenancy.TenantID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.member(t, id) {
		return store.ErrNotFound
	}
	delete(f.factors, id)
	delete(f.codes, id)
	delete(f.hashes, id)
	return nil
}

func (f *fakeStore) ReplaceRecoveryCodes(_ context.Context, t tenancy.TenantID, id string, hashes [][]byte, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.member(t, id) {
		return store.ErrNotFound
	}
	if f.failReplace {
		return context.DeadlineExceeded
	}
	m := map[string]*time.Time{}
	for _, h := range hashes {
		if len(h) != 32 {
			return store.ErrInvalidArgument
		}
		m[string(h)] = nil
	}
	f.codes[id] = m
	f.hashes[id] = hashes
	return nil
}

func (f *fakeStore) UseRecoveryCode(_ context.Context, t tenancy.TenantID, id string, hash []byte, at time.Time) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.member(t, id) {
		return false, nil
	}
	used, ok := f.codes[id][string(hash)]
	if !ok || used != nil {
		return false, nil
	}
	f.codes[id][string(hash)] = &at
	return true, nil
}

func (f *fakeStore) HasRole(_ context.Context, t tenancy.TenantID, id, role string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.member(t, id) && f.roles[id][role], nil
}

func (f *fakeStore) factor(id string) (store.MFAFactor, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fa, ok := f.factors[id]
	if !ok {
		return store.MFAFactor{}, false
	}
	return *fa, true
}

func (f *fakeStore) unusedCodes(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, u := range f.codes[id] {
		if u == nil {
			n++
		}
	}
	return n
}

// fakeLockout merekam panggilan dan mengembalikan state yang dapat diatur.
type fakeLockout struct {
	mu        sync.Mutex
	failures  []lockCall
	successes []lockCall
	state     LockState
	failErr   error
	succErr   error
	blocked   LockState // dikembalikan Check pada checkingLockout
	checkErr  error
}

type lockCall struct{ userID, ip string }

func (l *fakeLockout) RegisterFailure(_ context.Context, userID, ip string) (LockState, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures = append(l.failures, lockCall{userID, ip})
	return l.state, l.failErr
}

func (l *fakeLockout) RegisterSuccess(_ context.Context, userID, ip string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.successes = append(l.successes, lockCall{userID, ip})
	return l.succErr
}

func (l *fakeLockout) counts() (fails, succs int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.failures), len(l.successes)
}

// checkingLockout menambah LockChecker (pra-pemeriksaan) di atas fakeLockout.
type checkingLockout struct{ *fakeLockout }

func (c checkingLockout) Check(_ context.Context, _, _ string) (LockState, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.blocked, c.checkErr
}

type fakeAudit struct {
	mu     sync.Mutex
	events []AuditEvent
}

func (a *fakeAudit) Record(_ context.Context, e AuditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

// clock adalah jam uji yang dapat digeser.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

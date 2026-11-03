package guard

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

var testTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))

func tctx() context.Context { return tenancy.WithTenant(context.Background(), testTenant) }

// clock adalah jam palsu yang dapat dimajukan (jam yang dapat diinjeksi, tes deterministik).
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *clock { return &clock{t: time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type attemptRow struct {
	user, ip, kind string
	success        bool
	at             time.Time
	id             int64
}

// fakeStore memenuhi AttemptStore dan LoginAccounts dengan semantik yang sama seperti store sungguhan (kegagalan
// berturut-turut per pasangan sejak sukses terakhir, hitungan lintas IP per jendela, LockUser bersyarat active) dan
// mencatat urutan pemanggilan agar jalur login dapat dibandingkan satu per satu.
type fakeStore struct {
	mu       sync.Mutex
	calls    []string
	attempts []attemptRow
	users    map[string]*store.User // id -> user
	byEmail  map[string]string      // email ternormalisasi -> id
	delay    time.Duration          // latensi DB tiruan per panggilan
	failOn   map[string]error       // nama metode -> galat yang dikembalikan
	nextID   int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{users: map[string]*store.User{}, byEmail: map[string]string{}, failOn: map[string]error{}}
}

func (f *fakeStore) addUser(email, hash, status string) store.User {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := &store.User{ID: uuid.NewString(), Email: store.NormalizeEmail(email), PasswordHash: hash, Status: status, MembershipStatus: store.MembershipActive}
	f.users[u.ID] = u
	f.byEmail[u.Email] = u.ID
	return *u
}

func (f *fakeStore) enter(name string) error {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	err := f.failOn[name]
	d := f.delay
	f.mu.Unlock()
	if d > 0 {
		time.Sleep(d)
	}
	return err
}

func (f *fakeStore) resetCalls() {
	f.mu.Lock()
	f.calls = nil
	f.mu.Unlock()
}

func (f *fakeStore) trace() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func (f *fakeStore) status(id string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.users[id].Status
}

func (f *fakeStore) rows(userID string) []attemptRow {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []attemptRow
	for _, a := range f.attempts {
		if a.user == userID {
			out = append(out, a)
		}
	}
	return out
}

func (f *fakeStore) FindUserByEmail(_ context.Context, _ tenancy.TenantID, email string) (store.User, error) {
	if err := f.enter("FindUserByEmail"); err != nil {
		return store.User{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	id, ok := f.byEmail[store.NormalizeEmail(email)]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return *f.users[id], nil
}

func (f *fakeStore) GetUser(_ context.Context, _ tenancy.TenantID, id string) (store.User, error) {
	if err := f.enter("GetUser"); err != nil {
		return store.User{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return *u, nil
}

func (f *fakeStore) RecordLoginAttempt(_ context.Context, _ tenancy.TenantID, userID, ip, kind string, success bool, at time.Time) error {
	if err := f.enter("RecordLoginAttempt"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.users[userID]; !ok {
		return store.ErrNotFound
	}
	f.attempts = append(f.attempts, attemptRow{user: userID, ip: ip, kind: kind, success: success, at: at})
	return nil
}

// pairStateLocked menghitung keadaan pasangan seperti kueri store sungguhan; pemanggil memegang f.mu.
func (f *fakeStore) pairStateLocked(userID, ip string, since time.Time) store.PairState {
	var lastSuccess time.Time
	for _, a := range f.attempts {
		if a.user == userID && a.ip == ip && a.success && a.at.After(lastSuccess) {
			lastSuccess = a.at
		}
	}
	var st store.PairState
	for _, a := range f.attempts {
		if a.user == userID && a.ip == ip && !a.success && a.at.After(lastSuccess) && !a.at.Before(since) {
			st.ConsecutiveFailures++
			if st.LastFailureAt == nil || a.at.After(*st.LastFailureAt) {
				at := a.at
				st.LastFailureAt = &at
			}
		}
	}
	return st
}

// ReserveLoginAttempt meniru store sungguhan: membaca keadaan, memutuskan, dan menyisipkan dalam SATU bagian kritis
// (setara kunci advisory per pasangan).
func (f *fakeStore) ReserveLoginAttempt(_ context.Context, _ tenancy.TenantID, userID, ip, kind string, at, since time.Time, admit func(store.PairState) bool) (store.PairState, bool, int64, error) {
	if err := f.enter("ReserveLoginAttempt"); err != nil {
		return store.PairState{}, false, 0, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	before := f.pairStateLocked(userID, ip, since)
	if !admit(before) {
		return before, false, 0, nil
	}
	if _, ok := f.users[userID]; !ok {
		return before, false, 0, store.ErrNotFound
	}
	f.nextID++
	f.attempts = append(f.attempts, attemptRow{user: userID, ip: ip, kind: kind, at: at, id: f.nextID})
	return before, true, f.nextID, nil
}

func (f *fakeStore) ReleaseLoginAttempt(_ context.Context, _ tenancy.TenantID, userID string, id int64) error {
	if err := f.enter("ReleaseLoginAttempt"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	kept := f.attempts[:0]
	for _, a := range f.attempts {
		if a.id != id || a.user != userID || a.success {
			kept = append(kept, a)
		}
	}
	f.attempts = kept
	return nil
}

func (f *fakeStore) PairFailureState(ctx context.Context, t tenancy.TenantID, userID, ip string) (store.PairState, error) {
	return f.PairFailureStateSince(ctx, t, userID, ip, time.Time{})
}

func (f *fakeStore) PairFailureStateSince(_ context.Context, _ tenancy.TenantID, userID, ip string, since time.Time) (store.PairState, error) {
	if err := f.enter("PairFailureState"); err != nil {
		return store.PairState{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var lastSuccess time.Time
	for _, a := range f.attempts {
		if a.user == userID && a.ip == ip && a.success && a.at.After(lastSuccess) {
			lastSuccess = a.at
		}
	}
	var st store.PairState
	for _, a := range f.attempts {
		if a.user == userID && a.ip == ip && !a.success && a.at.After(lastSuccess) && !a.at.Before(since) {
			st.ConsecutiveFailures++
			if st.LastFailureAt == nil || a.at.After(*st.LastFailureAt) {
				at := a.at
				st.LastFailureAt = &at
			}
		}
	}
	return st, nil
}

func (f *fakeStore) CrossIPFailures(_ context.Context, _ tenancy.TenantID, userID string, since time.Time) (store.CrossIPState, error) {
	if err := f.enter("CrossIPFailures"); err != nil {
		return store.CrossIPState{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ips := map[string]bool{}
	var st store.CrossIPState
	for _, a := range f.attempts {
		if a.user == userID && !a.success && !a.at.Before(since) {
			st.Failures++
			ips[a.ip] = true
		}
	}
	st.DistinctIPs = len(ips)
	return st, nil
}

func (f *fakeStore) LockUser(_ context.Context, _ tenancy.TenantID, userID string, _ time.Time) (bool, error) {
	if err := f.enter("LockUser"); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[userID]
	if !ok || u.Status != store.UserActive {
		return false, nil
	}
	u.Status = store.UserLocked
	return true, nil
}

func (f *fakeStore) UnlockUser(_ context.Context, _ tenancy.TenantID, userID string) (bool, error) {
	if err := f.enter("UnlockUser"); err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	u, ok := f.users[userID]
	if !ok {
		return false, nil
	}
	// Seperti store: SELALU menghapus catatan percobaan pengguna, juga bila akun tidak locked.
	kept := f.attempts[:0]
	for _, a := range f.attempts {
		if a.user != userID {
			kept = append(kept, a)
		}
	}
	f.attempts = kept
	if u.Status != store.UserLocked {
		return false, nil
	}
	u.Status = store.UserActive
	return true, nil
}

var errBoom = errors.New("boom")

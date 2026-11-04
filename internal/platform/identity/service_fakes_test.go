package identity

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/guard"
	"erpora/internal/platform/identity/password"
	"erpora/internal/platform/identity/session"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/identity/token"
	"erpora/internal/platform/tenancy"
	sdkidentity "erpora/sdk/identity"
)

var (
	svcTenant = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	userA     = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a01"
	userB     = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a02"
	sessA     = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b01"
	sessB     = "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4b02"
)

func quietLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// fakeEnv mengumpulkan semua fake agar satu tes dapat memeriksa interaksi lintas komponen.
type fakeEnv struct {
	login    *fakeLogin
	gate     *fakeGate
	lock     *fakeLock
	sessions *fakeSessions
	issuer   *fakeIssuer
	keys     *fakeKeys
	mfa      *fakeMFA
	status   *fakeStatus
	reset    *fakeReset
	settings *fakeSettings
	store    *fakeStore
	audit    *fakeAuditor
}

func newFakeEnv() *fakeEnv {
	return &fakeEnv{
		login: &fakeLogin{}, gate: &fakeGate{}, lock: &fakeLock{}, sessions: &fakeSessions{}, issuer: &fakeIssuer{},
		keys: &fakeKeys{}, mfa: &fakeMFA{}, status: &fakeStatus{}, reset: &fakeReset{}, settings: &fakeSettings{},
		store: newFakeStore(), audit: &fakeAuditor{},
	}
}

func (e *fakeEnv) deps() Deps {
	return Deps{
		Login: e.login, Gate: e.gate, Hasher: &password.Hasher{Params: password.DefaultParams()}, Lockout: e.lock,
		Sessions: e.sessions, Issuer: e.issuer, Keys: e.keys, MFA: e.mfa, MFAState: e.status, Reset: e.reset,
		Settings: e.settings, Store: e.store, Auditor: e.audit, Log: quietLog(),
		Now: func() time.Time { return time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC) },
	}
}

func (e *fakeEnv) service() sdkidentity.Service {
	s, err := NewService(e.deps())
	if err != nil {
		panic(err)
	}
	return s
}

func (e *fakeEnv) admin() sdkidentity.Admin {
	a, err := NewAdmin(e.deps())
	if err != nil {
		panic(err)
	}
	return a
}

// principalCtx membangun context handler: tenant + Principal (pengguna, sesi, amr).
func principalCtx(user, sid string, amr ...string) context.Context {
	ctx := tenancy.WithTenant(context.Background(), svcTenant)
	return sdkidentity.WithPrincipal(ctx, sdkidentity.Principal{UserID: user, SessionID: sid, Kind: sdkidentity.KindHuman, AMR: amr})
}

func tenantCtx() context.Context { return tenancy.WithTenant(context.Background(), svcTenant) }

type fakeLogin struct {
	out guard.Outcome
	err error
	in  guard.LoginInput
}

func (f *fakeLogin) Verify(_ context.Context, in guard.LoginInput) (guard.Outcome, error) {
	f.in = in
	return f.out, f.err
}

// fakeGate: hash = "h:"+plain; verifikasi cocok bila phc == "h:"+plain.
type fakeGate struct {
	hashErr  error
	hashed   []string
	verifies atomic.Int32 // jumlah panggilan Verify (verifikasi argon2)
}

func (g *fakeGate) Verify(_ context.Context, _ *password.Hasher, plain, phc string) (bool, bool, error) {
	g.verifies.Add(1)
	if phc == "" {
		return false, false, password.ErrInvalidHash
	}
	return phc == "h:"+plain, false, nil
}

func (g *fakeGate) Hash(_ context.Context, _ *password.Hasher, plain string) (string, error) {
	if g.hashErr != nil {
		return "", g.hashErr
	}
	g.hashed = append(g.hashed, plain)
	return "h:" + plain, nil
}

// fakeLock meniru Lockout: Reserve atomik (satu mutex) dengan batas tebakan opsional (limit > 0: reservasi aktif
// sebanyak limit atau lebih ditolak sebagai PairBlocked), seperti jadwal jeda guard yang sungguhan.
type fakeLock struct {
	mu           sync.Mutex
	checkState   guard.State // dikembalikan Reserve bila restricted (tanpa reservasi)
	failState    guard.State
	failures     []string // "user|ip" (satu per Fail)
	successes    []string
	unlocked     []string
	failErr      error
	checkedPairs []string // "user|ip" per Reserve
	limit        int
	reserved     int // reservasi yang dibuat
	released     int
	failed       int
	active       int // reservasi yang belum dilepas (gagal tetap aktif)
	lastPair     string
}

func (l *fakeLock) Reserve(_ context.Context, _, userID, ip string) (guard.Reservation, guard.State, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.checkedPairs = append(l.checkedPairs, userID+"|"+ip)
	if l.checkState.AccountLocked || l.checkState.PairBlocked || l.checkState.RetryAfter > 0 {
		return guard.Reservation{}, l.checkState, nil
	}
	if l.limit > 0 && l.active >= l.limit {
		return guard.Reservation{}, guard.State{PairBlocked: true, RetryAfter: 30 * time.Second}, nil
	}
	l.reserved++
	l.active++
	l.lastPair = userID + "|" + ip
	return guard.Reservation{}, guard.State{}, nil
}

func (l *fakeLock) Fail(context.Context, guard.Reservation) (guard.State, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failed++
	l.failures = append(l.failures, l.lastPair)
	return l.failState, l.failErr
}

func (l *fakeLock) Release(context.Context, guard.Reservation) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.released++
	l.active--
	return nil
}

func (l *fakeLock) RegisterSuccess(_ context.Context, userID, ip string) error {
	l.successes = append(l.successes, userID+"|"+ip)
	return nil
}

func (l *fakeLock) Unlock(_ context.Context, userID string) error {
	l.unlocked = append(l.unlocked, userID)
	return nil
}

type fakeSessions struct {
	mu          sync.Mutex
	started     []session.StartInput
	startErr    error
	rotateSess  session.Session
	rotateErr   error
	rotated     []string // "raw|origin"
	revoked     []string // "sid|reason"
	revokeErr   error
	revokedAll  []string
	marked      []string
	nextRefresh string
	upgraded    []string // "user|sid|amr"
	upgradeErr  error
}

func (f *fakeSessions) Start(_ context.Context, in session.StartInput) (session.Session, session.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = append(f.started, in)
	if f.startErr != nil {
		return session.Session{}, session.RefreshToken{}, f.startErr
	}
	rt := f.nextRefresh
	if rt == "" {
		rt = "RAW-REFRESH-TOKEN-SECRET"
	}
	return session.Session{ID: sessA, UserID: in.UserID, AMR: in.AMR}, session.RefreshToken{Value: rt, ExpiresAt: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)}, nil
}

func (f *fakeSessions) Rotate(_ context.Context, raw, origin string) (session.Session, session.RefreshToken, error) {
	f.rotated = append(f.rotated, raw+"|"+origin)
	if f.rotateErr != nil {
		return session.Session{}, session.RefreshToken{}, f.rotateErr
	}
	return f.rotateSess, session.RefreshToken{Value: "NEW-REFRESH", ExpiresAt: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)}, nil
}

func (f *fakeSessions) Upgrade(_ context.Context, userID, sid, addAMR string) (session.Session, session.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upgraded = append(f.upgraded, userID+"|"+sid+"|"+addAMR)
	if f.upgradeErr != nil {
		return session.Session{}, session.RefreshToken{}, f.upgradeErr
	}
	return session.Session{ID: sid, UserID: userID}, session.RefreshToken{Value: "UPGRADED-REFRESH", ExpiresAt: time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)}, nil
}

func (f *fakeSessions) Revoke(_ context.Context, sid, reason string) error {
	f.revoked = append(f.revoked, sid+"|"+reason)
	return f.revokeErr
}

func (f *fakeSessions) RevokeAllForUser(_ context.Context, userID string) error {
	f.revokedAll = append(f.revokedAll, userID)
	return nil
}

func (f *fakeSessions) MarkRevoked(_ context.Context, ids ...string) {
	f.marked = append(f.marked, ids...)
}

type fakeIssuer struct {
	claims []token.Claims
	err    error
}

func (f *fakeIssuer) Issue(c token.Claims) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.claims = append(f.claims, c)
	return "ACCESS-" + c.SessionID, nil
}

type fakeKeys struct{ rotated int }

func (f *fakeKeys) Rotate(context.Context) (token.Key, error) {
	f.rotated++
	return token.Key{ID: "kid-baru-0001", Status: token.StatusPending, CreatedAt: time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)}, nil
}

type fakeMFA struct {
	calls      []string // "method|user"
	verifyErr  error
	confirmErr error
	resetErr   error
	resetActor string
	resetUser  string
	codes      []string
}

func (f *fakeMFA) BeginEnrollment(_ context.Context, userID string) (sdkidentity.EnrollmentInfo, error) {
	f.calls = append(f.calls, "begin|"+userID)
	return sdkidentity.EnrollmentInfo{Secret: "SECRETB32", OTPAuthURI: "otpauth://totp/x"}, nil
}

func (f *fakeMFA) ConfirmEnrollment(_ context.Context, userID, _ string) ([]string, error) {
	f.calls = append(f.calls, "confirm|"+userID)
	if f.confirmErr != nil {
		return nil, f.confirmErr
	}
	return []string{"AAAAA-BBBBB"}, nil
}

func (f *fakeMFA) Verify(_ context.Context, userID, _ string) error {
	f.calls = append(f.calls, "verify|"+userID)
	return f.verifyErr
}

func (f *fakeMFA) UseRecoveryCode(_ context.Context, userID, _ string) error {
	f.calls = append(f.calls, "recovery|"+userID)
	return f.verifyErr
}

func (f *fakeMFA) RegenerateRecoveryCodes(_ context.Context, userID string) ([]string, error) {
	f.calls = append(f.calls, "regen|"+userID)
	return []string{"CCCCC-DDDDD"}, nil
}

func (f *fakeMFA) Reset(_ context.Context, actor, userID string) error {
	f.calls = append(f.calls, "reset|"+userID)
	f.resetActor, f.resetUser = actor, userID
	return f.resetErr
}

type fakeStatus struct {
	st          MFAStatus
	err         error
	invalidated []string
	order       []string
}

func (f *fakeStatus) Status(context.Context, string) (MFAStatus, error) {
	f.order = append(f.order, "status")
	return f.st, f.err
}

func (f *fakeStatus) Invalidate(_ tenancy.TenantID, userID string) {
	f.order = append(f.order, "invalidate")
	f.invalidated = append(f.invalidated, userID)
}

type fakeReset struct {
	requested []string // "email|locale|ip"
	completed []string
	err       error
}

func (f *fakeReset) Request(ctx context.Context, email, locale string) error {
	f.requested = append(f.requested, email+"|"+locale)
	return f.err
}

func (f *fakeReset) Complete(_ context.Context, raw, _ string) error {
	f.completed = append(f.completed, raw)
	return f.err
}

type fakeSettings struct {
	got []string // "url|updatedBy"
	err error
}

func (f *fakeSettings) SetPublicURL(_ context.Context, raw, by string) error {
	f.got = append(f.got, raw+"|"+by)
	return f.err
}

type fakeAuditor struct{ events []AuditEvent }

func (f *fakeAuditor) Record(_ context.Context, e AuditEvent) { f.events = append(f.events, e) }

// fakeStore: penyimpanan memori untuk pengguna, sesi, dan role.
type fakeStore struct {
	users       map[string]store.User // id -> user
	roles       map[string][]string   // userID -> role keys
	sessionCnt  map[string]int        // userID -> jumlah sesi (semua, termasuk tercabut)
	grantErr    error
	activateErr error    // galat paksa untuk SetUserStatus(active)
	revoked     []string // "user|role" yang dicabut lewat RevokeRole
	changed     []string // "user|except"
	changeRev   []string
	statusSet   map[string]string
	hashSet     map[string]string
	lastLogins  []string
}

func newFakeStore() *fakeStore {
	return &fakeStore{users: map[string]store.User{}, roles: map[string][]string{}, sessionCnt: map[string]int{},
		statusSet: map[string]string{}, hashSet: map[string]string{}}
}

func (s *fakeStore) addUser(id, email, hash string) {
	s.users[id] = store.User{ID: id, Email: email, Name: "Uji", Status: store.UserActive, PasswordHash: hash}
}

func (s *fakeStore) GetUser(_ context.Context, _ tenancy.TenantID, id string) (store.User, error) {
	if u, ok := s.users[id]; ok {
		return u, nil
	}
	return store.User{}, store.ErrNotFound
}

func (s *fakeStore) FindUserByEmail(_ context.Context, _ tenancy.TenantID, email string) (store.User, error) {
	for _, u := range s.users {
		if u.Email == store.NormalizeEmail(email) {
			return u, nil
		}
	}
	return store.User{}, store.ErrNotFound
}

func (s *fakeStore) CreateUser(_ context.Context, _ tenancy.TenantID, in store.NewUser) (store.User, error) {
	for _, u := range s.users {
		if u.Email == in.Email {
			return store.User{}, store.ErrEmailTaken
		}
	}
	id := uuid.NewString()
	u := store.User{ID: id, Email: in.Email, Name: in.Name, Status: in.Status, PasswordHash: in.PasswordHash}
	s.users[id] = u
	return u, nil
}

func (s *fakeStore) SetUserStatus(_ context.Context, _ tenancy.TenantID, userID, status string) error {
	if status == store.UserActive && s.activateErr != nil {
		return s.activateErr
	}
	s.statusSet[userID] = status
	if u, ok := s.users[userID]; ok {
		u.Status = status
		s.users[userID] = u
	}
	return nil
}

func (s *fakeStore) SetPasswordHash(_ context.Context, _ tenancy.TenantID, userID, hash string) error {
	s.hashSet[userID] = hash
	if u, ok := s.users[userID]; ok {
		u.PasswordHash = hash
		s.users[userID] = u
	}
	return nil
}

func (s *fakeStore) ChangePassword(_ context.Context, _ tenancy.TenantID, userID, newHash, except string, _ time.Time) ([]string, error) {
	s.changed = append(s.changed, userID+"|"+except+"|"+newHash)
	return s.changeRev, nil
}

func (s *fakeStore) TouchLastLogin(_ context.Context, _ tenancy.TenantID, userID string, _ time.Time) error {
	s.lastLogins = append(s.lastLogins, userID)
	return nil
}

func (s *fakeStore) GrantRole(_ context.Context, _ tenancy.TenantID, userID, roleKey string, _ time.Time) (bool, error) {
	if s.grantErr != nil {
		return false, s.grantErr
	}
	if slices.Contains(s.roles[userID], roleKey) {
		return false, nil
	}
	s.roles[userID] = append(s.roles[userID], roleKey)
	return true, nil
}

func (s *fakeStore) HasRole(_ context.Context, _ tenancy.TenantID, userID, roleKey string) (bool, error) {
	return slices.Contains(s.roles[userID], roleKey), nil
}

func (s *fakeStore) RevokeRole(_ context.Context, _ tenancy.TenantID, userID, roleKey string) (bool, error) {
	s.revoked = append(s.revoked, userID+"|"+roleKey)
	before := len(s.roles[userID])
	s.roles[userID] = slices.DeleteFunc(slices.Clone(s.roles[userID]), func(r string) bool { return r == roleKey })
	return len(s.roles[userID]) < before, nil
}

func (s *fakeStore) UserRoleKeys(_ context.Context, _ tenancy.TenantID, userID string) ([]string, error) {
	return slices.Clone(s.roles[userID]), nil
}

// WithEmailLock pada fake cukup menjalankan fn (tes fake berjalan serial); serialisasi sungguhan diuji terhadap PostgreSQL.
func (s *fakeStore) WithEmailLock(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}

func (s *fakeStore) CountUserSessions(_ context.Context, _ tenancy.TenantID, userID string) (int, error) {
	return s.sessionCnt[userID], nil
}

var errBoom = errors.New("boom: detail internal rahasia")

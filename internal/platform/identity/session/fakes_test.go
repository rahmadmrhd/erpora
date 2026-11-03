package session_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/identity/session"
	"erpora/internal/platform/identity/store"
	"erpora/internal/platform/tenancy"
)

var (
	tenantA = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"))
	tenantB = tenancy.TenantID(uuid.MustParse("0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c"))
)

// fakeClock adalah jam palsu yang aman dipakai bersamaan.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}
func (c *fakeClock) Set(t time.Time) { c.mu.Lock(); defer c.mu.Unlock(); c.t = t }

// fakeNotifier adalah token.Notifier in-process bernama kanal (tanpa muatan, seperti antarmuka aslinya).
type fakeNotifier struct {
	mu        sync.Mutex
	next      int
	subs      map[string]map[int]func(context.Context)
	published map[string]int
}

func newFakeNotifier() *fakeNotifier {
	return &fakeNotifier{subs: map[string]map[int]func(context.Context){}, published: map[string]int{}}
}

func (n *fakeNotifier) Publish(ctx context.Context, channel string) {
	n.mu.Lock()
	n.published[channel]++
	var fns []func(context.Context)
	for _, f := range n.subs[channel] {
		fns = append(fns, f)
	}
	n.mu.Unlock()
	for _, f := range fns {
		f(ctx)
	}
}

func (n *fakeNotifier) Subscribe(channel string, fn func(context.Context)) func() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.subs[channel] == nil {
		n.subs[channel] = map[int]func(context.Context){}
	}
	id := n.next
	n.next++
	n.subs[channel][id] = fn
	return func() { n.mu.Lock(); defer n.mu.Unlock(); delete(n.subs[channel], id) }
}

func (n *fakeNotifier) count(channel string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.published[channel]
}

func (n *fakeNotifier) subscribers(channel string) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.subs[channel])
}

// syncBuf adalah bytes.Buffer yang aman dipakai bersamaan (penampung log).
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// fakeStore memenuhi session.Store dan menegakkan aturan yang sama dengan store.Store sungguhan (bentuk UUID,
// validasi sesi dan token, FK komposit tenant, hash unik, tepi rotasi atomik bersyarat): fake yang longgar
// menyembunyikan bug.
type fakeStore struct {
	mu        sync.Mutex
	users     map[string]store.User
	members   map[memberKey]string
	sessions  map[string]*fakeSession
	tokens    map[string]*fakeToken
	hashes    map[string]string // hash(hex) -> token id
	listCalls atomic.Int64
}

type memberKey struct {
	t tenancy.TenantID
	u string
}

type fakeSession struct {
	t tenancy.TenantID
	s store.Session
}

type fakeToken struct {
	t tenancy.TenantID
	r store.RefreshToken
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users: map[string]store.User{}, members: map[memberKey]string{},
		sessions: map[string]*fakeSession{}, tokens: map[string]*fakeToken{}, hashes: map[string]string{},
	}
}

var _ session.Store = (*fakeStore)(nil)

func validUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

func (f *fakeStore) addUser(t tenancy.TenantID, status string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.NewString()
	f.users[id] = store.User{ID: id, Email: id + "@example.test", Status: status}
	f.members[memberKey{t, id}] = store.MembershipActive
	return id
}

func (f *fakeStore) setStatus(id, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	u := f.users[id]
	u.Status = status
	f.users[id] = u
}

func (f *fakeStore) GetUser(_ context.Context, t tenancy.TenantID, id string) (store.User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !validUUID(id) {
		return store.User{}, store.ErrNotFound
	}
	if _, ok := f.members[memberKey{t, id}]; !ok {
		return store.User{}, store.ErrNotFound
	}
	u, ok := f.users[id]
	if !ok {
		return store.User{}, store.ErrNotFound
	}
	return u, nil
}

func (f *fakeStore) GetMembership(_ context.Context, t tenancy.TenantID, userID string) (store.Membership, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.members[memberKey{t, userID}]
	if !ok {
		return store.Membership{}, store.ErrNotFound
	}
	return store.Membership{UserID: userID, Status: st}, nil
}

func validReason(r string) bool { return r != "" && len([]rune(r)) <= 64 }

func (f *fakeStore) validateSession(sess store.Session) bool {
	if !validUUID(sess.ID) || !validUUID(sess.UserID) {
		return false
	}
	switch sess.ClientKind {
	case store.ClientWeb, store.ClientMobile, store.ClientDesktop:
	default:
		return false
	}
	switch sess.AuthMethod {
	case "password", "oidc", "pat", "client_credentials":
	default:
		return false
	}
	return len(sess.UserAgent) <= 512 && len(sess.IP) <= 45 && !sess.AbsoluteExpiresAt.IsZero() && len(sess.AMR) >= 1 && len(sess.AMR) <= 8
}

func (f *fakeStore) validateToken(rt store.RefreshToken) bool {
	return validUUID(rt.ID) && validUUID(rt.SessionID) && validUUID(rt.FamilyID) && len(rt.TokenHash) == 32 && !rt.ExpiresAt.IsZero() &&
		(rt.ParentID == "" || validUUID(rt.ParentID))
}

// insertToken berasumsi f.mu terkunci.
func (f *fakeStore) insertTokenLocked(t tenancy.TenantID, rt store.RefreshToken) error {
	if !f.validateToken(rt) {
		return store.ErrInvalidArgument
	}
	if _, dup := f.hashes[fmt.Sprintf("%x", rt.TokenHash)]; dup {
		return errors.New("fake: token_hash duplikat")
	}
	if s, ok := f.sessions[rt.SessionID]; !ok || s.t != t {
		return errors.New("fake: FK sesi")
	}
	if rt.ParentID != "" {
		p, ok := f.tokens[rt.ParentID]
		if !ok || p.r.FamilyID != rt.FamilyID || p.r.SessionID != rt.SessionID {
			return errors.New("fake: FK family/sesi induk")
		}
	}
	rt.TokenHash = append([]byte(nil), rt.TokenHash...)
	f.tokens[rt.ID] = &fakeToken{t: t, r: rt}
	f.hashes[fmt.Sprintf("%x", rt.TokenHash)] = rt.ID
	return nil
}

func (f *fakeStore) CreateSessionWithRefresh(_ context.Context, t tenancy.TenantID, sess store.Session, rt store.RefreshToken) error {
	if rt.SessionID != sess.ID {
		return store.ErrInvalidArgument
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.validateSession(sess) || !f.validateToken(rt) {
		return store.ErrInvalidArgument
	}
	if f.members[memberKey{t, sess.UserID}] != store.MembershipActive {
		return store.ErrNotFound
	}
	if _, dup := f.sessions[sess.ID]; dup {
		return errors.New("fake: sesi duplikat")
	}
	sess.AMR = append([]string(nil), sess.AMR...)
	f.sessions[sess.ID] = &fakeSession{t: t, s: sess}
	if err := f.insertTokenLocked(t, rt); err != nil {
		delete(f.sessions, sess.ID)
		return err
	}
	return nil
}

func (f *fakeStore) GetSession(_ context.Context, t tenancy.TenantID, id string) (store.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok || s.t != t || !validUUID(id) {
		return store.Session{}, store.ErrNotFound
	}
	return s.s, nil
}

func (f *fakeStore) FindRefreshTokenByHash(_ context.Context, t tenancy.TenantID, hash []byte) (store.RefreshToken, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(hash) != 32 {
		return store.RefreshToken{}, store.ErrNotFound
	}
	id, ok := f.hashes[fmt.Sprintf("%x", hash)]
	if !ok || f.tokens[id].t != t {
		return store.RefreshToken{}, store.ErrNotFound
	}
	return f.tokens[id].r, nil
}

func (f *fakeStore) RotateRefreshToken(_ context.Context, t tenancy.TenantID, usedID string, at time.Time, next store.RefreshToken) (bool, error) {
	if !validUUID(usedID) {
		return false, nil
	}
	if next.ParentID != usedID {
		return false, store.ErrInvalidArgument
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	tok, ok := f.tokens[usedID]
	if !ok || tok.t != t || tok.r.UsedAt != nil {
		return false, nil
	}
	if s := f.sessions[tok.r.SessionID]; s == nil || s.t != t || s.s.RevokedAt != nil {
		return false, nil
	}
	// galat penyimpanan token pengganti membatalkan seluruh rotasi (token lama tidak terbakar)
	if err := f.insertTokenLocked(t, next); err != nil {
		return false, err
	}
	u := at.UTC()
	tok.r.UsedAt = &u
	return true, nil
}

func (f *fakeStore) revokeLocked(s *fakeSession, reason string, at time.Time) {
	u := at.UTC()
	s.s.RevokedAt = &u
	s.s.RevokeReason = reason
}

func (f *fakeStore) RevokeSession(_ context.Context, t tenancy.TenantID, id, reason string, at time.Time) (bool, error) {
	if !validReason(reason) {
		return false, store.ErrInvalidArgument
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok || s.t != t || s.s.RevokedAt != nil {
		return false, nil
	}
	f.revokeLocked(s, reason, at)
	return true, nil
}

func (f *fakeStore) RevokeUserSessions(_ context.Context, t tenancy.TenantID, userID, except, reason string, at time.Time) ([]string, error) {
	if !validReason(reason) {
		return nil, store.ErrInvalidArgument
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for id, s := range f.sessions {
		if s.t == t && s.s.UserID == userID && s.s.RevokedAt == nil && id != except {
			f.revokeLocked(s, reason, at)
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (f *fakeStore) RevokeSessionsOfFamily(_ context.Context, t tenancy.TenantID, familyID, reason string, at time.Time) ([]string, error) {
	if !validReason(reason) {
		return nil, store.ErrInvalidArgument
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	seen := map[string]bool{}
	for _, tok := range f.tokens {
		if tok.t != t || tok.r.FamilyID != familyID || seen[tok.r.SessionID] {
			continue
		}
		seen[tok.r.SessionID] = true
		if s := f.sessions[tok.r.SessionID]; s != nil && s.t == t && s.s.RevokedAt == nil {
			f.revokeLocked(s, reason, at)
			ids = append(ids, s.s.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (f *fakeStore) ListRevokedSessionsSince(_ context.Context, t tenancy.TenantID, since time.Time) ([]store.RevokedSession, error) {
	f.listCalls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.RevokedSession
	for _, s := range f.sessions {
		if s.t == t && s.s.RevokedAt != nil && !s.s.RevokedAt.Before(since) {
			out = append(out, store.RevokedSession{ID: s.s.ID, RevokedAt: *s.s.RevokedAt})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// failingLoader selalu gagal memuat (untuk uji galat Load).
type failingLoader struct{ calls atomic.Int64 }

func (l *failingLoader) ListRevokedSessionsSince(context.Context, tenancy.TenantID, time.Time) ([]store.RevokedSession, error) {
	l.calls.Add(1)
	return nil, errors.New("loader: gagal")
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

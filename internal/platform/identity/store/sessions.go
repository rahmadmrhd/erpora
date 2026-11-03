package store

import (
	"context"
	"encoding/json"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"erpora/internal/platform/tenancy"
)

type sessionRow struct {
	ID                uuid.UUID  `gorm:"column:id"`
	TenantID          uuid.UUID  `gorm:"column:tenant_id"`
	UserID            uuid.UUID  `gorm:"column:user_id"`
	ClientKind        string     `gorm:"column:client_kind"`
	AuthMethod        string     `gorm:"column:auth_method"`
	AMR               []byte     `gorm:"column:amr"`
	UserAgent         string     `gorm:"column:user_agent"`
	IP                string     `gorm:"column:ip"`
	CreatedAt         time.Time  `gorm:"column:created_at"`
	LastSeenAt        time.Time  `gorm:"column:last_seen_at"`
	AbsoluteExpiresAt time.Time  `gorm:"column:absolute_expires_at"`
	RevokedAt         *time.Time `gorm:"column:revoked_at"`
	RevokeReason      *string    `gorm:"column:revoke_reason"`
}

func (sessionRow) TableName() string { return "auth_sessions" }

func (r sessionRow) session() Session {
	var amr []string
	_ = json.Unmarshal(r.AMR, &amr)
	s := Session{
		ID: idString(r.ID), UserID: idString(r.UserID), ClientKind: r.ClientKind, AuthMethod: r.AuthMethod, AMR: amr,
		UserAgent: r.UserAgent, IP: r.IP, CreatedAt: r.CreatedAt.UTC(), LastSeenAt: r.LastSeenAt.UTC(),
		AbsoluteExpiresAt: r.AbsoluteExpiresAt.UTC(), RevokedAt: utcPtr(r.RevokedAt),
	}
	if r.RevokeReason != nil {
		s.RevokeReason = *r.RevokeReason
	}
	return s
}

type refreshRow struct {
	ID        uuid.UUID  `gorm:"column:id"`
	TenantID  uuid.UUID  `gorm:"column:tenant_id"`
	SessionID uuid.UUID  `gorm:"column:session_id"`
	FamilyID  uuid.UUID  `gorm:"column:family_id"`
	ParentID  *uuid.UUID `gorm:"column:parent_id"`
	TokenHash []byte     `gorm:"column:token_hash"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UsedAt    *time.Time `gorm:"column:used_at"`
	ExpiresAt time.Time  `gorm:"column:expires_at"`
}

func (refreshRow) TableName() string { return "refresh_tokens" }

func (r refreshRow) token() RefreshToken {
	t := RefreshToken{
		ID: idString(r.ID), SessionID: idString(r.SessionID), FamilyID: idString(r.FamilyID), TokenHash: r.TokenHash,
		CreatedAt: r.CreatedAt.UTC(), UsedAt: utcPtr(r.UsedAt), ExpiresAt: r.ExpiresAt.UTC(),
	}
	if r.ParentID != nil {
		t.ParentID = idString(*r.ParentID)
	}
	return t
}

func validReason(r string) bool { return r != "" && utf8.RuneCountInString(r) <= 64 }

// sessionArgs memvalidasi Session dan menyiapkan kolom amr (jsonb).
func sessionArgs(sess Session) (id, user uuid.UUID, amr string, ok bool) {
	id, ok1 := parseID(sess.ID)
	user, ok2 := parseID(sess.UserID)
	if !ok1 || !ok2 {
		return id, user, "", false
	}
	switch sess.ClientKind {
	case ClientWeb, ClientMobile, ClientDesktop:
	default:
		return id, user, "", false
	}
	switch sess.AuthMethod {
	case "password", "oidc", "pat", "client_credentials":
	default:
		return id, user, "", false
	}
	if len(sess.UserAgent) > 512 || len(sess.IP) > 45 || sess.AbsoluteExpiresAt.IsZero() || len(sess.AMR) < 1 || len(sess.AMR) > 8 {
		return id, user, "", false
	}
	b, err := json.Marshal(sess.AMR)
	if err != nil {
		return id, user, "", false
	}
	return id, user, string(b), true
}

func insertSession(tx *gorm.DB, t tenancy.TenantID, sess Session) error {
	id, user, amr, ok := sessionArgs(sess)
	if !ok {
		return ErrInvalidArgument
	}
	created, seen := sess.CreatedAt, sess.LastSeenAt
	if created.IsZero() {
		created = nowUTC()
	}
	if seen.IsZero() {
		seen = created
	}
	// hanya anggota aktif tenant t yang dapat memiliki sesi di tenant itu (juga ditegakkan FK komposit)
	n, err := exec(tx, `INSERT INTO auth_sessions (id, tenant_id, user_id, client_kind, auth_method, amr, user_agent, ip, created_at, last_seen_at, absolute_expires_at)
		SELECT ?::uuid, m.tenant_id, m.user_id, ?::text, ?::text, ?::jsonb, ?::text, ?::text, ?::timestamptz, ?::timestamptz, ?::timestamptz
		FROM tenant_memberships m WHERE m.tenant_id = ? AND m.user_id = ? AND m.status = 'active'`,
		id, sess.ClientKind, sess.AuthMethod, amr, sess.UserAgent, sess.IP, created.UTC(), seen.UTC(), sess.AbsoluteExpiresAt.UTC(), tid(t), user)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func insertRefresh(tx *gorm.DB, t tenancy.TenantID, rt RefreshToken) error {
	id, ok1 := parseID(rt.ID)
	sid, ok2 := parseID(rt.SessionID)
	fam, ok3 := parseID(rt.FamilyID)
	if !ok1 || !ok2 || !ok3 || len(rt.TokenHash) != 32 || rt.ExpiresAt.IsZero() {
		return ErrInvalidArgument
	}
	var parent any
	if rt.ParentID != "" {
		p, ok := parseID(rt.ParentID)
		if !ok {
			return ErrInvalidArgument
		}
		parent = p
	}
	created := rt.CreatedAt
	if created.IsZero() {
		created = nowUTC()
	}
	_, err := exec(tx, `INSERT INTO refresh_tokens (id, tenant_id, session_id, family_id, parent_id, token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, id, tid(t), sid, fam, parent, rt.TokenHash, created.UTC(), rt.ExpiresAt.UTC())
	return err
}

// CreateSession membuat sesi. ErrNotFound bila pengguna bukan anggota aktif tenant t.
func (s *Store) CreateSession(ctx context.Context, t tenancy.TenantID, sess Session) error {
	if err := insertSession(s.db(ctx), t, sess); err != nil {
		if err == ErrNotFound || err == ErrInvalidArgument {
			return err
		}
		return wrap("buat sesi", err)
	}
	return nil
}

// CreateSessionWithRefresh membuat sesi dan token refresh akarnya dalam satu transaksi (login). rt.SessionID harus
// sama dengan sess.ID.
func (s *Store) CreateSessionWithRefresh(ctx context.Context, t tenancy.TenantID, sess Session, rt RefreshToken) error {
	if rt.SessionID != sess.ID {
		return ErrInvalidArgument
	}
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		if err := insertSession(tx, t, sess); err != nil {
			return err
		}
		return insertRefresh(tx, t, rt)
	})
	if err != nil {
		if err == ErrNotFound || err == ErrInvalidArgument {
			return err
		}
		return wrap("buat sesi dan refresh token", err)
	}
	return nil
}

// GetSession memuat sesi tenant t; ErrNotFound bila tidak ada atau milik tenant lain.
func (s *Store) GetSession(ctx context.Context, t tenancy.TenantID, id string) (Session, error) {
	sid, ok := parseID(id)
	if !ok {
		return Session{}, ErrNotFound
	}
	var r sessionRow
	res := s.db(ctx).Raw(`SELECT * FROM auth_sessions WHERE id = ? AND tenant_id = ?`, sid, tid(t)).Scan(&r)
	if res.Error != nil {
		return Session{}, wrap("muat sesi", res.Error)
	}
	if res.RowsAffected == 0 {
		return Session{}, ErrNotFound
	}
	return r.session(), nil
}

// ListUserSessions memuat sesi aktif (belum dicabut, belum melewati batas absolut) milik pengguna di tenant t.
func (s *Store) ListUserSessions(ctx context.Context, t tenancy.TenantID, userID string) ([]Session, error) {
	uid, ok := parseID(userID)
	if !ok {
		return nil, nil
	}
	var rows []sessionRow
	if err := s.db(ctx).Raw(`SELECT * FROM auth_sessions WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL
		AND absolute_expires_at > now() ORDER BY created_at, id`, tid(t), uid).Scan(&rows).Error; err != nil {
		return nil, wrap("muat sesi pengguna", err)
	}
	out := make([]Session, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.session())
	}
	return out, nil
}

// TouchSession memperbarui last_seen_at.
func (s *Store) TouchSession(ctx context.Context, t tenancy.TenantID, id string, at time.Time) error {
	sid, ok := parseID(id)
	if !ok {
		return ErrNotFound
	}
	n, err := exec(s.db(ctx), `UPDATE auth_sessions SET last_seen_at = ? WHERE id = ? AND tenant_id = ?`, at.UTC(), sid, tid(t))
	if err != nil {
		return wrap("perbarui sesi", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSessionAMR mengganti klaim amr sesi (mis. ["pwd","otp"] setelah verifikasi MFA) selama sesi belum tercabut;
// false bila sesi tidak ada, milik tenant lain, atau sudah tercabut.
func (s *Store) SetSessionAMR(ctx context.Context, t tenancy.TenantID, id string, amr []string) (bool, error) {
	if len(amr) < 1 || len(amr) > 8 {
		return false, ErrInvalidArgument
	}
	sid, ok := parseID(id)
	if !ok {
		return false, nil
	}
	b, err := json.Marshal(amr)
	if err != nil {
		return false, ErrInvalidArgument
	}
	n, err := exec(s.db(ctx), `UPDATE auth_sessions SET amr = ?::jsonb WHERE id = ? AND tenant_id = ? AND revoked_at IS NULL`, string(b), sid, tid(t))
	if err != nil {
		return false, wrap("ubah amr sesi", err)
	}
	return n == 1, nil
}

// RevokeSession mencabut sesi (bersyarat atomik: hanya bila belum tercabut). false bila tidak ada, milik tenant
// lain, atau sudah tercabut.
func (s *Store) RevokeSession(ctx context.Context, t tenancy.TenantID, id, reason string, at time.Time) (bool, error) {
	if !validReason(reason) {
		return false, ErrInvalidArgument
	}
	sid, ok := parseID(id)
	if !ok {
		return false, nil
	}
	n, err := exec(s.db(ctx), `UPDATE auth_sessions SET revoked_at = ?, revoke_reason = ? WHERE id = ? AND tenant_id = ? AND revoked_at IS NULL`,
		at.UTC(), reason, sid, tid(t))
	if err != nil {
		return false, wrap("cabut sesi", err)
	}
	return n == 1, nil
}

func revokeUserSessions(tx *gorm.DB, t tenancy.TenantID, uid uuid.UUID, except string, reason string, at time.Time) ([]string, error) {
	q := `UPDATE auth_sessions SET revoked_at = ?, revoke_reason = ? WHERE tenant_id = ? AND user_id = ? AND revoked_at IS NULL`
	args := []any{at.UTC(), reason, tid(t), uid}
	if except != "" {
		ex, ok := parseID(except)
		if !ok {
			return nil, ErrInvalidArgument
		}
		q += ` AND id <> ?`
		args = append(args, ex)
	}
	return pluckIDs(tx, q+` RETURNING id::text`, args...)
}

// RevokeUserSessions mencabut semua sesi aktif pengguna di tenant t kecuali exceptSessionID (kosong = tanpa
// pengecualian) dan mengembalikan id sesi yang baru tercabut (untuk disiarkan ke daftar tercabut).
func (s *Store) RevokeUserSessions(ctx context.Context, t tenancy.TenantID, userID, exceptSessionID, reason string, at time.Time) ([]string, error) {
	if !validReason(reason) {
		return nil, ErrInvalidArgument
	}
	uid, ok := parseID(userID)
	if !ok {
		return nil, nil
	}
	ids, err := revokeUserSessions(s.db(ctx), t, uid, exceptSessionID, reason, at)
	if err != nil {
		if err == ErrInvalidArgument {
			return nil, err
		}
		return nil, wrap("cabut sesi pengguna", err)
	}
	return ids, nil
}

// RevokeSessionsOfFamily mencabut semua sesi aktif yang memiliki token pada family itu (deteksi reuse) dan
// mengembalikan id sesi yang baru tercabut.
func (s *Store) RevokeSessionsOfFamily(ctx context.Context, t tenancy.TenantID, familyID, reason string, at time.Time) ([]string, error) {
	if !validReason(reason) {
		return nil, ErrInvalidArgument
	}
	fam, ok := parseID(familyID)
	if !ok {
		return nil, nil
	}
	ids, err := pluckIDs(s.db(ctx), `UPDATE auth_sessions SET revoked_at = ?, revoke_reason = ? WHERE tenant_id = ? AND revoked_at IS NULL
		AND id IN (SELECT session_id FROM refresh_tokens WHERE tenant_id = ? AND family_id = ?) RETURNING id::text`,
		at.UTC(), reason, tid(t), tid(t), fam)
	if err != nil {
		return nil, wrap("cabut sesi family", err)
	}
	return ids, nil
}

// ListRevokedSessionsSince memuat sesi tenant t yang dicabut pada atau setelah since (muat awal daftar tercabut di
// memori saat startup: since = sekarang dikurangi umur access token + leeway).
func (s *Store) ListRevokedSessionsSince(ctx context.Context, t tenancy.TenantID, since time.Time) ([]RevokedSession, error) {
	var rows []struct {
		ID        string    `gorm:"column:id"`
		RevokedAt time.Time `gorm:"column:revoked_at"`
	}
	if err := s.db(ctx).Raw(`SELECT id::text AS id, revoked_at FROM auth_sessions WHERE tenant_id = ? AND revoked_at >= ? ORDER BY revoked_at, id`,
		tid(t), since.UTC()).Scan(&rows).Error; err != nil {
		return nil, wrap("muat sesi tercabut", err)
	}
	out := make([]RevokedSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, RevokedSession{ID: r.ID, RevokedAt: r.RevokedAt.UTC()})
	}
	return out, nil
}

// InsertRefreshToken menyimpan token refresh (hash 32 byte). Anak harus satu family dan satu sesi dengan induknya
// (FK komposit di database).
func (s *Store) InsertRefreshToken(ctx context.Context, t tenancy.TenantID, rt RefreshToken) error {
	if err := insertRefresh(s.db(ctx), t, rt); err != nil {
		if err == ErrInvalidArgument {
			return err
		}
		return wrap("simpan refresh token", err)
	}
	return nil
}

// FindRefreshTokenByHash memuat token menurut hash SHA-256 pada tenant t; ErrNotFound bila tidak dikenal.
func (s *Store) FindRefreshTokenByHash(ctx context.Context, t tenancy.TenantID, hash []byte) (RefreshToken, error) {
	if len(hash) != 32 {
		return RefreshToken{}, ErrNotFound
	}
	var r refreshRow
	res := s.db(ctx).Raw(`SELECT * FROM refresh_tokens WHERE tenant_id = ? AND token_hash = ?`, tid(t), hash).Scan(&r)
	if res.Error != nil {
		return RefreshToken{}, wrap("muat refresh token", res.Error)
	}
	if res.RowsAffected == 0 {
		return RefreshToken{}, ErrNotFound
	}
	return r.token(), nil
}

// MarkRefreshTokenUsed menandai token terpakai secara bersyarat atomik (UPDATE ... WHERE used_at IS NULL): tepat
// satu pemanggil serentak mendapat true. false berarti sudah terpakai (indikasi reuse), tidak ada, atau milik
// tenant lain. Kedaluwarsa dan status sesi diperiksa pemanggil dari baris yang dimuatnya.
func (s *Store) MarkRefreshTokenUsed(ctx context.Context, t tenancy.TenantID, id string, at time.Time) (bool, error) {
	rid, ok := parseID(id)
	if !ok {
		return false, nil
	}
	n, err := exec(s.db(ctx), `UPDATE refresh_tokens SET used_at = ? WHERE id = ? AND tenant_id = ? AND used_at IS NULL`, at.UTC(), rid, tid(t))
	if err != nil {
		return false, wrap("tandai refresh token terpakai", err)
	}
	return n == 1, nil
}

// RotateRefreshToken adalah rotasi atomik dalam satu transaksi: menandai usedID terpakai (bersyarat used_at IS NULL
// dan sesinya belum tercabut) lalu menyimpan token pengganti next (next.ParentID wajib usedID). Mengembalikan false
// tanpa efek bila kalah (sudah terpakai, sesi tercabut, tidak ada, atau milik tenant lain); galat (mis. pelanggaran
// konsistensi family/sesi) membatalkan seluruh transaksi sehingga token lama tidak terbakar.
func (s *Store) RotateRefreshToken(ctx context.Context, t tenancy.TenantID, usedID string, at time.Time, next RefreshToken) (bool, error) {
	rid, ok := parseID(usedID)
	if !ok {
		return false, nil
	}
	if next.ParentID != usedID {
		return false, ErrInvalidArgument
	}
	won := false
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		n, err := exec(tx, `UPDATE refresh_tokens rt SET used_at = ? WHERE rt.id = ? AND rt.tenant_id = ? AND rt.used_at IS NULL
			AND EXISTS (SELECT 1 FROM auth_sessions s WHERE s.id = rt.session_id AND s.tenant_id = rt.tenant_id AND s.revoked_at IS NULL)`,
			at.UTC(), rid, tid(t))
		if err != nil || n == 0 {
			return err
		}
		if err := insertRefresh(tx, t, next); err != nil {
			return err
		}
		won = true
		return nil
	})
	if err != nil {
		if err == ErrInvalidArgument {
			return false, err
		}
		return false, wrap("rotasi refresh token", err)
	}
	return won, nil
}

// UpgradeSession menaikkan faktor autentikasi sesi (mis. ["pwd"] menjadi ["pwd","otp"] setelah MFA) dan MENGGANTI
// token refresh dalam satu transaksi: pembaruan amr bersyarat atomik (compare-and-set terhadap oldAMR, sesi belum
// tercabut), seluruh token refresh sesi yang belum terpakai dihapus, dan next disimpan sebagai akar baru pada family
// yang SAMA (next.FamilyID diabaikan bila sesi sudah punya family; next.ParentID harus kosong). Dengan begitu token
// pra-upgrade menjadi tak dikenal (ditolak seragam tanpa mencabut sesi, bukan reuse) sementara reuse leluhur lama
// tetap mencabut family. false tanpa efek bila kalah: amr sudah berubah (upgrade serentak: tepat satu menang), sesi
// tercabut, tidak ada, atau milik tenant lain. Galat membatalkan seluruh transaksi.
func (s *Store) UpgradeSession(ctx context.Context, t tenancy.TenantID, sessionID string, oldAMR, newAMR []string, next RefreshToken) (bool, error) {
	sid, ok := parseID(sessionID)
	if !ok {
		return false, nil
	}
	if next.SessionID != sessionID || next.ParentID != "" || len(oldAMR) < 1 || len(oldAMR) > 8 || len(newAMR) < 1 || len(newAMR) > 8 {
		return false, ErrInvalidArgument
	}
	oldJSON, err1 := json.Marshal(oldAMR)
	newJSON, err2 := json.Marshal(newAMR)
	if err1 != nil || err2 != nil {
		return false, ErrInvalidArgument
	}
	won := false
	err := s.db(ctx).Transaction(func(tx *gorm.DB) error {
		n, err := exec(tx, `UPDATE auth_sessions SET amr = ?::jsonb WHERE id = ? AND tenant_id = ? AND revoked_at IS NULL AND amr = ?::jsonb`,
			string(newJSON), sid, tid(t), string(oldJSON))
		if err != nil || n == 0 {
			return err
		}
		fams, err := pluckIDs(tx, `SELECT family_id::text FROM refresh_tokens WHERE session_id = ? AND tenant_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, sid, tid(t))
		if err != nil {
			return err
		}
		if len(fams) == 1 {
			next.FamilyID = fams[0]
		}
		if _, err := exec(tx, `DELETE FROM refresh_tokens WHERE session_id = ? AND tenant_id = ? AND used_at IS NULL`, sid, tid(t)); err != nil {
			return err
		}
		if err := insertRefresh(tx, t, next); err != nil {
			return err
		}
		won = true
		return nil
	})
	if err != nil {
		if err == ErrInvalidArgument {
			return false, err
		}
		return false, wrap("naikkan sesi", err)
	}
	return won, nil
}

// CountUserSessions menghitung SEMUA sesi (aktif maupun tercabut) milik pengguna di tenant t; 0 untuk ID cacat atau
// pengguna tanpa sesi. Dipakai untuk memastikan akun belum pernah dipakai masuk (pemulihan create-admin).
func (s *Store) CountUserSessions(ctx context.Context, t tenancy.TenantID, userID string) (int, error) {
	uid, ok := parseID(userID)
	if !ok {
		return 0, nil
	}
	var n int64
	if err := s.db(ctx).Raw(`SELECT count(*) FROM auth_sessions WHERE tenant_id = ? AND user_id = ?`, tid(t), uid).Scan(&n).Error; err != nil {
		return 0, wrap("hitung sesi", err)
	}
	return int(n), nil
}

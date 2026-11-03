// Package store adalah persistensi identitas PRD-001 C1: satu-satunya pemakai GORM untuk tabel users,
// tenant_memberships, user_preferences, signing_keys, auth_sessions, refresh_tokens, user_mfa_factors,
// user_recovery_codes, password_resets, login_attempts (control DB; migrasi control 0005_identity) dan roles,
// user_roles (tenant DB; migrasi tenant 0003_roles_min). Skema hanya lewat migrasi goose (tanpa AutoMigrate).
//
// # Aturan umum
//
//   - Semua ID di API adalah string UUID kanonik (36 karakter). ID yang bukan UUID tidak pernah mencapai database
//     dan diperlakukan sebagai tidak ditemukan.
//   - Tenant selalu parameter eksplisit (tenancy.TenantID, diambil pemanggil dari context). Pengguna (users) global;
//     tabel itu hanya terlihat lewat tenant_memberships tenant pemanggil. Sesi, refresh token, reset sandi, dan
//     percobaan login membawa tenant_id dan hanya milik anggota tenant itu (FK komposit di database). Baris tenant
//     lain: ErrNotFound atau false, tidak pernah galat yang membedakan.
//   - Pengecualian: signing_keys global platform (satu JWKS; klaim tid menyatakan tenant), sehingga metodenya tidak
//     menerima tenant dan tidak menyaring tenant_id.
//   - Semua waktu masuk dan keluar UTC (timestamptz). Pemanggil menyuplai at (jam bisa diinjeksi pada tes).
//   - Operasi sekali pakai bersifat bersyarat atomik di database (UPDATE ... WHERE used_at IS NULL, dst.) dan
//     mengembalikan bool: tepat satu pemanggil serentak mendapat true; false bukan galat.
//   - Rahasia: secret TOTP dan kunci privat JWT diterima dan dikembalikan sebagai bytea hasil crypto.Sealer (store
//     tidak membukanya). Pemanggil disarankan mengikat AAD ke konteks, mis. "user_mfa_factors|secret|<user_id>"
//     dan "signing_keys|private|<kid>". Token refresh, token reset, dan kode pemulihan hanya berupa hash 32 byte.
//   - Galat sentinel: ErrNotFound, ErrEmailTaken, ErrConflict, ErrInvalidArgument, ErrInvalidTransition.
//
// # Metode (dipakai C1.4 sampai C1.12)
//
// Pengguna dan keanggotaan:
//
//	CreateUser(ctx, t, NewUser) (User, error)               // atomik user + keanggotaan; ErrEmailTaken
//	GetUser(ctx, t, id) (User, error)
//	FindUserByEmail(ctx, t, email) (User, error)            // email dinormalisasi (NormalizeEmail)
//	GetMembership(ctx, t, userID) (Membership, error)
//	AddMembership(ctx, t, userID, status) (created bool, error)
//	SetPasswordHash(ctx, t, userID, hash) error
//	TouchLastLogin(ctx, t, userID, at) error
//	SetUserStatus(ctx, t, userID, status) error             // active|disabled
//	LockUser(ctx, t, userID, at) (bool, error)              // bersyarat: hanya dari active
//	UnlockUser(ctx, t, userID) (bool, error)                // SELALU hapus login_attempts tenant itu; true bila locked->active
//	GetPreferences(ctx, t, userID) (Preferences, error)
//	UpsertPreferences(ctx, t, userID, locale, timezone) error
//
// Kunci penandatangan (global; kontrak KeyStore C1.5):
//
//	ListSigningKeys(ctx) ([]SigningKey, error)              // terurut created_at, kid
//	InsertSigningKey(ctx, SigningKey) error                 // kid unik: ErrConflict; cap waktu status diisi CreatedAt bila kosong
//	TransitionSigningKey(ctx, id, from, to, at) (bool, error)
//	  // satu UPDATE bersyarat status == from; false bila tidak cocok; ErrInvalidTransition untuk tepi terlarang
//	  // (diizinkan: pending->active, active->retiring, retiring->retired, pending->retired). Tidak ada indeks
//	  // "satu active": urutan promosi/demosi adalah tanggung jawab service.
//	ValidKeyTransition(from, to) bool
//
//	Batasan yang harus diikuti pemanggil KeyStore (C1.5), ditegakkan store dan CHECK database:
//	  - kid wajib cocok ^[A-Za-z0-9_-]{8,64}$ (tanpa '.', ':', '=', '/'; mis. base64url tanpa padding). Selain itu
//	    InsertSigningKey mengembalikan ErrInvalidArgument.
//	  - PublicKey wajib tepat 32 byte (kunci publik Ed25519 mentah, bukan PKIX/DER 44 byte);
//	    PrivateKeySealed 1..4096 byte (hasil Sealer).
//	  - TransitionSigningKey mengembalikan ErrInvalidTransition (bukan false) untuk tepi di luar empat tepi di atas.
//	  - Status pending tidak boleh membawa ActivatedAt/RetiringAt/RetiredAt; untuk status lain cap waktu kosong diisi
//	    CreatedAt oleh InsertSigningKey.
//
// Sesi dan refresh token:
//
//	CreateSession(ctx, t, Session) error
//	CreateSessionWithRefresh(ctx, t, Session, RefreshToken) error   // atomik (login)
//	GetSession(ctx, t, id) (Session, error)
//	ListUserSessions(ctx, t, userID) ([]Session, error)             // aktif saja
//	TouchSession(ctx, t, id, at) error
//	SetSessionAMR(ctx, t, id, amr []string) (bool, error)           // mis. ["pwd","otp"]; false bila tercabut
//	RevokeSession(ctx, t, id, reason, at) (bool, error)             // bersyarat revoked_at IS NULL
//	RevokeUserSessions(ctx, t, userID, exceptSessionID, reason, at) ([]string, error)
//	RevokeSessionsOfFamily(ctx, t, familyID, reason, at) ([]string, error)   // deteksi reuse
//	ListRevokedSessionsSince(ctx, t, since) ([]RevokedSession, error)        // muat awal daftar tercabut
//	InsertRefreshToken(ctx, t, RefreshToken) error
//	FindRefreshTokenByHash(ctx, t, hash) (RefreshToken, error)
//	MarkRefreshTokenUsed(ctx, t, id, at) (bool, error)              // bersyarat used_at IS NULL
//	RotateRefreshToken(ctx, t, usedID, at, next) (bool, error)
//	  // atomik: tandai usedID terpakai (used_at IS NULL dan sesi belum tercabut) lalu simpan next (ParentID == usedID);
//	  // false = kalah; galat = rollback total (token lama tidak terbakar).
//	UpgradeSession(ctx, t, sessionID, oldAMR, newAMR, next) (bool, error)
//	  // atomik (PRD-001 Q27): compare-and-set amr (oldAMR -> newAMR, sesi belum tercabut), hapus refresh token belum
//	  // terpakai milik sesi, sisipkan next sebagai akar (ParentID kosong) di family yang sama; false = kalah/tercabut.
//	CountUserSessions(ctx, t, userID) (int, error)                  // SEMUA sesi pengguna, termasuk tercabut
//
// MFA:
//
//	GetMFAFactor(ctx, t, userID) (MFAFactor, error)
//	UpsertPendingMFAFactor(ctx, t, userID, secretSealed, at) (bool, error)  // false bila sudah terkonfirmasi
//	ConfirmMFAFactor(ctx, t, userID, step, at) (bool, error)        // set last_used_step = step konfirmasi
//	AdvanceMFAStep(ctx, t, userID, step) (bool, error)              // true hanya bila step > last_used_step
//	ResetMFA(ctx, t, userID) error                                  // hapus faktor + recovery code
//	ReplaceRecoveryCodes(ctx, t, userID, hashes, at) error          // atomik; hash 32 byte unik
//	UseRecoveryCode(ctx, t, userID, hash, at) (bool, error)         // bersyarat used_at IS NULL
//	CountUnusedRecoveryCodes(ctx, t, userID) (int, error)
//
// Reset dan ganti sandi:
//
//	InsertPasswordReset(ctx, t, PasswordReset) error
//	FindPasswordResetByHash(ctx, t, hash) (PasswordReset, error)
//	ConsumePasswordReset(ctx, t, id, at) (bool, error)              // used_at IS NULL dan expires_at > at
//	InvalidatePasswordResets(ctx, t, userID, at) (int64, error)     // batalkan tautan lama sebelum menerbitkan yang baru
//	ResetPassword(ctx, t, resetID, newHash, at) (ResetResult, bool, error)
//	  // atomik: pakai tautan, ganti sandi, buka locked, batalkan tautan lain, cabut SEMUA sesi, hapus login_attempts.
//	ChangePassword(ctx, t, userID, newHash, exceptSessionID, at) ([]string, error)
//	  // atomik: ganti sandi dan cabut sesi lain; mengembalikan id sesi yang tercabut.
//
// Percobaan login dan lockout (Q16):
//
//	RecordLoginAttempt(ctx, t, userID, ip, kind, success, at) error // kind: AttemptPassword|AttemptMFA
//	ReserveLoginAttempt(ctx, t, userID, ip, kind, at, since, admit) (PairState, bool, int64, error)
//	  // atomik per pasangan (kunci advisory transaksional): baca keadaan, admit(keadaan), lalu catat SATU kegagalan bila diterima
//	ReleaseLoginAttempt(ctx, t, userID, id) error                   // hapus reservasi yang ternyata bukan kegagalan
//	PairFailureState(ctx, t, userID, ip) (PairState, error)         // gagal sejak sukses terakhir pasangan (akun, IP), tanpa batas waktu
//	PairFailureStateSince(ctx, t, userID, ip, since) (PairState, error) // hanya gagal >= since (24 jam, 3 Nov 2026); since nol = tanpa batas
//	CrossIPFailures(ctx, t, userID, since) (CrossIPState, error)    // gagal lintas IP; tidak direset sukses
//	PruneLoginAttempts(ctx, t, before) (int64, error)
//	  // Hanya pengguna yang dikenal dapat dicatat (user_id NOT NULL); email tak dikenal ditangani pemanggil
//	  // (batas laju per IP/identitas sasaran). Catat sukses hanya setelah autentikasi selesai penuh.
//
// Peran (tenant DB lewat TenantConnector):
//
//	GrantRole(ctx, t, userID, roleKey, at) (bool, error)
//	RevokeRole(ctx, t, userID, roleKey) (bool, error)
//	UserRoleKeys(ctx, t, userID) ([]string, error)
//	HasRole(ctx, t, userID, roleKey) (bool, error)
//	CountRoleHolders(ctx, t, roleKey) (int, error)
package store

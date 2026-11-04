// Package httpapi berisi handler HTTP modul core (endpoint autentikasi, PRD-001 FR-101/102/106/107). Foldernya bernama
// http, tetapi package dinamai httpapi agar tidak bentrok dengan net/http.
//
// Handler sengaja tipis: seluruh keputusan (kredensial, lockout, sesi, MFA, batas laju) ada di identity.Service milik
// platform, yang diambil dari context (identity.ServiceFrom; dipasang platform hanya untuk modul core). Pengguna dan
// sesi pada endpoint terautentikasi SELALU berasal dari Principal di context, tidak pernah dari body, query, atau path,
// sehingga tidak ada titik masuk ID (IDOR). Refresh token hanya lewat cookie HttpOnly (cookie.go), tidak pernah di body.
package httpapi

import (
	"errors"
	"net/http"
	"time"

	"erpora/sdk/httpx"
	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/identity"
)

// Handlers menampung handler HTTP modul core.
type Handlers struct {
	now func() time.Time
}

// New membuat Handlers.
func New() *Handlers { return &Handlers{now: time.Now} }

// WithClock mengganti jam (untuk pengujian umur cookie).
func (h *Handlers) WithClock(now func() time.Time) *Handlers {
	h.now = now
	return h
}

// --- DTO permintaan (bentuk body; metadata request diisi handler, bukan klien) ---

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type forgotRequest struct {
	Email string `json:"email"`
}

type resetRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

type changeRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

type codeRequest struct {
	Code string `json:"code"`
}

// --- DTO respons ---

// sessionDTO adalah hasil login/refresh/verifikasi MFA. Refresh token TIDAK ada di sini dengan sengaja.
type sessionDTO struct {
	AccessToken string            `json:"accessToken"`
	ExpiresIn   int               `json:"expiresIn"`
	MFAState    identity.MFAState `json:"mfaState"`
}

type enrollmentDTO struct {
	Secret     string `json:"secret"`
	OTPAuthURI string `json:"otpauthUri"`
}

type codesDTO struct {
	Codes []string `json:"codes"`
}

// confirmDTO: recovery code (tampil sekali) dan, bila penaikan sesi berhasil, sesi otp baru (Q27).
type confirmDTO struct {
	Codes   []string    `json:"codes"`
	Session *sessionDTO `json:"session,omitempty"`
}

func toSession(r identity.LoginResult) sessionDTO {
	return sessionDTO{AccessToken: r.AccessToken, ExpiresIn: r.ExpiresIn, MFAState: r.MFAState}
}

// service mengambil identity.Service dari context. Tanpa itu perakitan platform salah (modul core wajib menerimanya):
// 500 generik dan sebabnya dicatat di log; tidak pernah diteruskan ke klien.
func (h *Handlers) service(w http.ResponseWriter, r *http.Request) (identity.Service, bool) {
	svc, ok := identity.ServiceFrom(r.Context())
	if !ok {
		httpx.RespondError(w, r, &httpx.Error{
			Status: http.StatusInternalServerError, Code: "internal_error", Message: "Terjadi kesalahan pada server.",
			Err: errors.New("identity.Service tidak terpasang di context modul core"),
		})
		return nil, false
	}
	return svc, true
}

// NoStore membungkus handler auth: semua respons (sukses maupun galat) membawa Cache-Control: no-store (ADR-0013 §1;
// respons memuat token, atau terkait kredensial).
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// Login: POST /api/core/auth/login.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	req, err := httpx.DecodeJSON[loginRequest](w, r)
	if err != nil {
		httpx.RespondError(w, r, err)
		return
	}
	res, err := svc.Login(r.Context(), identity.LoginInput{
		Email: req.Email, Password: req.Password, ClientIP: httpx.ClientIP(r.Context()), UserAgent: r.UserAgent(),
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	h.writeSession(w, res)
}

// Refresh: POST /api/core/auth/refresh. Refresh token dari cookie (bukan body); Origin dari header untuk pemeriksaan
// CSRF cookie (ADR-0013 §1). Token ditolak: cookie dihapus dan 401 generik.
func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	res, err := svc.Refresh(r.Context(), identity.RefreshInput{
		RefreshToken: refreshCookie(r), Origin: r.Header.Get("Origin"), ClientIP: httpx.ClientIP(r.Context()), UserAgent: r.UserAgent(),
	})
	if err != nil {
		if isInvalidRefresh(err) {
			clearRefreshCookie(w)
		}
		respondError(w, r, err)
		return
	}
	h.writeSession(w, res)
}

// Logout: POST /api/core/auth/logout. Mencabut sesi Principal sendiri; cookie refresh dihapus.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	if err := svc.Logout(r.Context()); err != nil {
		respondError(w, r, err)
		return
	}
	clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// ForgotPassword: POST /api/core/auth/password/forgot. Selalu 204 kecuali 429 (tidak membedakan akun ada atau tidak).
func (h *Handlers) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	req, err := httpx.DecodeJSON[forgotRequest](w, r)
	if err != nil {
		httpx.RespondError(w, r, err)
		return
	}
	loc, _ := sdki18n.LocaleFrom(r.Context())
	if err := svc.ForgotPassword(r.Context(), identity.ForgotInput{
		Email: req.Email, Locale: string(loc), ClientIP: httpx.ClientIP(r.Context()), UserAgent: r.UserAgent(),
	}); err != nil {
		respondError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ResetPassword: POST /api/core/auth/password/reset (tautan sekali pakai dari email).
func (h *Handlers) ResetPassword(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	req, err := httpx.DecodeJSON[resetRequest](w, r)
	if err != nil {
		httpx.RespondError(w, r, err)
		return
	}
	if err := svc.ResetPassword(r.Context(), identity.ResetInput{
		Token: req.Token, NewPassword: req.NewPassword, ClientIP: httpx.ClientIP(r.Context()), UserAgent: r.UserAgent(),
	}); err != nil {
		respondError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ChangePassword: POST /api/core/auth/password/change. Hanya untuk Principal sendiri.
func (h *Handlers) ChangePassword(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	req, err := httpx.DecodeJSON[changeRequest](w, r)
	if err != nil {
		httpx.RespondError(w, r, err)
		return
	}
	if err := svc.ChangePassword(r.Context(), identity.ChangeInput{
		CurrentPassword: req.CurrentPassword, NewPassword: req.NewPassword,
		ClientIP: httpx.ClientIP(r.Context()), UserAgent: r.UserAgent(),
	}); err != nil {
		respondError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// EnrollMFA: POST /api/core/auth/mfa/enroll (bahan pendaftaran TOTP, tampil sekali).
func (h *Handlers) EnrollMFA(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	info, err := svc.BeginMFAEnrollment(r.Context())
	if err != nil {
		respondError(w, r, err)
		return
	}
	httpx.WriteData(w, http.StatusOK, enrollmentDTO{Secret: info.Secret, OTPAuthURI: info.OTPAuthURI})
}

// ConfirmMFA: POST /api/core/auth/mfa/confirm. Mengaktifkan faktor, menaikkan sesi ke otp, dan merotasi refresh token
// (Q27): cookie ditulis ulang; respons memuat recovery code (tampil sekali) dan access token otp.
func (h *Handlers) ConfirmMFA(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	req, err := httpx.DecodeJSON[codeRequest](w, r)
	if err != nil {
		httpx.RespondError(w, r, err)
		return
	}
	res, err := svc.ConfirmMFA(r.Context(), identity.ConfirmMFAInput{
		Code: req.Code, ClientIP: httpx.ClientIP(r.Context()), UserAgent: r.UserAgent(),
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	out := confirmDTO{Codes: res.Codes}
	if out.Codes == nil {
		out.Codes = []string{}
	}
	if res.Session.AccessToken != "" {
		s := toSession(res.Session)
		out.Session = &s
		h.setCookieIfRotated(w, res.Session)
	}
	httpx.WriteData(w, http.StatusOK, out)
}

// VerifyMFA: POST /api/core/auth/mfa/verify (kode TOTP pada sesi yang menunggu verifikasi).
func (h *Handlers) VerifyMFA(w http.ResponseWriter, r *http.Request) { h.verify(w, r, false) }

// RecoverMFA: POST /api/core/auth/mfa/recovery (recovery code sebagai pengganti kode TOTP).
func (h *Handlers) RecoverMFA(w http.ResponseWriter, r *http.Request) { h.verify(w, r, true) }

func (h *Handlers) verify(w http.ResponseWriter, r *http.Request, recovery bool) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	req, err := httpx.DecodeJSON[codeRequest](w, r)
	if err != nil {
		httpx.RespondError(w, r, err)
		return
	}
	res, err := svc.VerifyMFA(r.Context(), identity.VerifyInput{
		Code: req.Code, Recovery: recovery, ClientIP: httpx.ClientIP(r.Context()), UserAgent: r.UserAgent(),
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	h.writeSession(w, res)
}

// RegenerateRecoveryCodes: POST /api/core/auth/mfa/recovery-codes (kode lama hangus; tampil sekali).
func (h *Handlers) RegenerateRecoveryCodes(w http.ResponseWriter, r *http.Request) {
	svc, ok := h.service(w, r)
	if !ok {
		return
	}
	rc, err := svc.RegenerateRecoveryCodes(r.Context())
	if err != nil {
		respondError(w, r, err)
		return
	}
	codes := rc.Codes
	if codes == nil {
		codes = []string{}
	}
	httpx.WriteData(w, http.StatusOK, codesDTO{Codes: codes})
}

// writeSession menulis cookie refresh (bila ada token baru) lalu body sesi tanpa refresh token.
func (h *Handlers) writeSession(w http.ResponseWriter, res identity.LoginResult) {
	h.setCookieIfRotated(w, res)
	httpx.WriteData(w, http.StatusOK, toSession(res))
}

func (h *Handlers) setCookieIfRotated(w http.ResponseWriter, res identity.LoginResult) {
	if res.RefreshToken != "" {
		setRefreshCookie(w, res.RefreshToken, res.RefreshExpiresAt, h.now())
	}
}

func isInvalidRefresh(err error) bool { return errors.Is(err, identity.ErrInvalidRefresh) }

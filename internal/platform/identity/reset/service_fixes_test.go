package reset_test

import (
	"net/textproto"
	"strings"
	"testing"

	"erpora/internal/platform/identity/reset"
	"erpora/internal/platform/identity/store"
)

// B-4: alamat tersimpan yang tidak dapat dikirimi (tak lolos aturan sdk/mail) tidak boleh menghanguskan tautan yang
// masih berlaku; tidak ada tautan baru dan tidak ada surat; log tanpa email.
func TestRequestUnsendableAddressKeepsExistingLink(t *testing.T) {
	const badEmail = "budi@localhost" // domain tanpa titik: ditolak sdkmail.Validate
	h := newHarness(t, hopts{})
	u := h.st.addUser(t, badEmail, store.UserActive, "$old")
	// Tautan lama yang masih berlaku (mis. diterbitkan sebelum alamat berubah).
	h.st.mu.Lock()
	h.st.resets["old"] = &store.PasswordReset{ID: "old", UserID: u.ID, TokenHash: make([]byte, 32), CreatedAt: h.clk.Now(), ExpiresAt: h.clk.Now().Add(reset.TokenTTL)}
	h.st.mu.Unlock()
	invalidations, inserts := h.st.callCount("InvalidatePasswordResets"), h.st.callCount("InsertPasswordReset")

	if err := h.request(ipCtx("198.51.100.7"), badEmail, ""); err != nil {
		t.Fatalf("dari luar harus seragam: %v", err)
	}
	if n := len(h.sender.messages()); n != 0 {
		t.Fatalf("surat terkirim ke alamat tak sah: %d", n)
	}
	if h.st.callCount("InvalidatePasswordResets") != invalidations || h.st.callCount("InsertPasswordReset") != inserts {
		t.Fatal("tautan lama dibatalkan/tautan baru disimpan padahal alamat tak dapat dikirimi")
	}
	h.st.mu.Lock()
	old := h.st.resets["old"]
	h.st.mu.Unlock()
	if old == nil || old.UsedAt != nil {
		t.Fatalf("tautan lama harus tetap berlaku: %+v", old)
	}
	out := h.logs.String()
	if !strings.Contains(out, "tidak dapat dikirimi") || strings.Contains(out, badEmail) || strings.Contains(out, "localhost") {
		t.Errorf("harus ada peringatan tanpa email:\n%s", out)
	}
}

// B-1: galat pengiriman berupa balasan server SMTP dicatat hanya kodenya (teks sering menggemakan alamat penerima).
func TestRequestSendFailureLogsOnlySMTPCode(t *testing.T) {
	for _, tc := range []struct {
		code int
		msg  string
	}{{550, "5.1.1 <" + userEmail + ">: User unknown"}, {450, "4.2.0 <" + userEmail + "> greylisted"}} {
		h := newHarness(t, hopts{})
		h.st.addUser(t, userEmail, store.UserActive, "$old")
		h.sender.err = &textproto.Error{Code: tc.code, Msg: tc.msg}
		if err := h.request(ipCtx("198.51.100.7"), userEmail, ""); err != nil {
			t.Fatalf("kegagalan kirim tidak boleh terlihat dari luar: %v", err)
		}
		out := h.logs.String()
		if strings.Contains(out, userEmail) || strings.Contains(out, "User unknown") || strings.Contains(out, "greylisted") {
			t.Errorf("log memuat teks balasan server / email (%d):\n%s", tc.code, out)
		}
		if !strings.Contains(out, "SMTP") || !strings.Contains(out, string(rune('0'+tc.code/100))) {
			t.Errorf("log harus memuat kode SMTP %d:\n%s", tc.code, out)
		}
	}
	// Galat berkode dari driver (mail.ReplyError) pun hanya kodenya.
	h := newHarness(t, hopts{})
	h.st.addUser(t, userEmail, store.UserActive, "$old")
	h.sender.err = codedErr{code: 554}
	_ = h.request(ipCtx("198.51.100.7"), userEmail, "")
	if out := h.logs.String(); !strings.Contains(out, "554") || strings.Contains(out, userEmail) {
		t.Errorf("log = %s", out)
	}
}

type codedErr struct{ code int }

func (e codedErr) Error() string { return "teks server rahasia " + userEmail }
func (e codedErr) SMTPCode() int { return e.code }

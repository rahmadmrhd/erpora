package identity_test

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"erpora/sdk/identity"
)

// secretCase memuat satu nilai bawaan rahasia, rahasia yang harus tersamar, dan bidang tak sensitif
// yang harus tetap terlihat.
type secretCase struct {
	name    string
	value   any
	secrets []string
	visible []string
}

func secretCases() []secretCase {
	return []secretCase{
		{"LoginInput", identity.LoginInput{Email: "a@example.com", Password: "S3cr3t-pw", ClientIP: "10.1.2.3", UserAgent: "UA-visible"},
			[]string{"S3cr3t-pw"}, []string{"a@example.com", "10.1.2.3", "UA-visible"}},
		{"RefreshInput", identity.RefreshInput{RefreshToken: "S3cr3t-refresh", ClientIP: "10.1.2.3", UserAgent: "UA-visible"},
			[]string{"S3cr3t-refresh"}, []string{"10.1.2.3", "UA-visible"}},
		{"ResetInput", identity.ResetInput{Token: "S3cr3t-token", NewPassword: "S3cr3t-newpw", ClientIP: "10.1.2.3", UserAgent: "UA-visible"},
			[]string{"S3cr3t-token", "S3cr3t-newpw"}, []string{"10.1.2.3", "UA-visible"}},
		{"ChangeInput", identity.ChangeInput{CurrentPassword: "S3cr3t-old", NewPassword: "S3cr3t-new", ClientIP: "10.1.2.3", UserAgent: "UA-visible"},
			[]string{"S3cr3t-old", "S3cr3t-new"}, []string{"10.1.2.3", "UA-visible"}},
		{"LoginResult", identity.LoginResult{AccessToken: "S3cr3t-access", ExpiresIn: 600, MFAState: identity.MFAVerifyRequired,
			RefreshToken: "S3cr3t-refresh", RefreshExpiresAt: time.Unix(1700000000, 0)},
			[]string{"S3cr3t-access", "S3cr3t-refresh"}, []string{"600", "verify_required"}},
		{"EnrollmentInfo", identity.EnrollmentInfo{Secret: "S3cr3t-totp", OTPAuthURI: "otpauth://totp/x?secret=S3cr3t-totp"},
			[]string{"S3cr3t-totp"}, nil},
		{"RecoveryCodes", identity.RecoveryCodes{Codes: []string{"S3cr3t-rc1", "S3cr3t-rc2"}},
			[]string{"S3cr3t-rc1", "S3cr3t-rc2"}, nil},
		{"VerifyInput", identity.VerifyInput{Code: "S3cr3t-123456", Recovery: true, ClientIP: "10.1.2.3", UserAgent: "UA-visible"},
			[]string{"S3cr3t-123456"}, []string{"10.1.2.3", "UA-visible"}},
		{"ConfirmMFAInput", identity.ConfirmMFAInput{Code: "S3cr3t-123456", ClientIP: "10.1.2.3", UserAgent: "UA-visible"},
			[]string{"S3cr3t-123456"}, []string{"10.1.2.3", "UA-visible"}},
		{"CreateAdminInput", identity.CreateAdminInput{Email: "admin@example.com", Name: "Nama-Visible", Password: "S3cr3t-pw"},
			[]string{"S3cr3t-pw"}, []string{"admin@example.com", "Nama-Visible"}},
	}
}

func check(t *testing.T, label, out string, c secretCase) {
	t.Helper()
	for _, s := range c.secrets {
		if strings.Contains(out, s) {
			t.Errorf("%s %s: rahasia %q bocor: %s", c.name, label, s, out)
		}
	}
	if !strings.Contains(out, "****") {
		t.Errorf("%s %s: tidak ada penyamar ****: %s", c.name, label, out)
	}
	for _, v := range c.visible {
		if !strings.Contains(out, v) {
			t.Errorf("%s %s: bidang tak sensitif %q hilang: %s", c.name, label, v, out)
		}
	}
}

func TestSecretsMaskedInFmt(t *testing.T) {
	for _, c := range secretCases() {
		for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%d", "%x", "%q", "%10.3v"} {
			check(t, verb, fmt.Sprintf(verb, c.value), c)
		}
	}
}

func TestSecretsMaskedInSlog(t *testing.T) {
	for _, c := range secretCases() {
		var text, js bytes.Buffer
		slog.New(slog.NewTextHandler(&text, nil)).Info("m", "v", c.value)
		slog.New(slog.NewJSONHandler(&js, nil)).Info("m", "v", c.value)
		check(t, "slog text", text.String(), c)
		check(t, "slog json", js.String(), c)
	}
}

// Mengimplementasi LogValuer lewat nilai dan pointer (pemanggil sering mencatat &input).
func TestLogValuerImplemented(t *testing.T) {
	for _, c := range secretCases() {
		if _, ok := c.value.(slog.LogValuer); !ok {
			t.Errorf("%s tidak mengimplementasi slog.LogValuer", c.name)
		}
		if _, ok := c.value.(fmt.Stringer); !ok {
			t.Errorf("%s tidak mengimplementasi fmt.Stringer", c.name)
		}
		if _, ok := c.value.(fmt.GoStringer); !ok {
			t.Errorf("%s tidak mengimplementasi fmt.GoStringer", c.name)
		}
	}
}

// Pointer ke struct (umum pada handler) juga tersamar, dan nilai aslinya tidak berubah.
func TestSecretsMaskedViaPointer(t *testing.T) {
	in := &identity.LoginInput{Email: "a@example.com", Password: "S3cr3t-pw"}
	c := secretCase{name: "*LoginInput", secrets: []string{"S3cr3t-pw"}, visible: []string{"a@example.com"}}
	for _, verb := range []string{"%v", "%+v", "%#v"} {
		check(t, verb, fmt.Sprintf(verb, in), c)
	}
	var text bytes.Buffer
	slog.New(slog.NewTextHandler(&text, nil)).Info("m", "v", in)
	check(t, "slog ptr", text.String(), c)
	if in.Password != "S3cr3t-pw" {
		t.Fatal("nilai asli berubah")
	}
}

package identity_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"erpora/sdk/httpx"
	"erpora/sdk/identity"
)

// Format menutup semua verba, termasuk di dalam slice, map, dan struct pembungkus.
func TestSecretsMaskedNestedInFmt(t *testing.T) {
	type wrapper struct {
		In  identity.LoginInput
		Res identity.LoginResult
	}
	in := identity.LoginInput{Email: "a@example.com", Password: "S3cr3t-pw"}
	res := identity.LoginResult{AccessToken: "S3cr3t-access", RefreshToken: "S3cr3t-refresh"}
	vals := []any{
		[]identity.LoginInput{in},
		map[string]identity.LoginResult{"k": res},
		wrapper{In: in, Res: res},
	}
	for _, v := range vals {
		for _, verb := range []string{"%v", "%+v", "%#v", "%d", "%x", "%q"} {
			out := fmt.Sprintf(verb, v)
			for _, s := range []string{"S3cr3t-pw", "S3cr3t-access", "S3cr3t-refresh"} {
				if strings.Contains(out, s) {
					t.Errorf("%s %T: %q bocor: %s", verb, v, s, out)
				}
			}
		}
	}
}

// slog JSON handler men-marshal nilai di dalam slice/map lewat json.Marshal; tipe input harus tersamar.
func TestSecretsMaskedInSlogJSONNested(t *testing.T) {
	in := identity.LoginInput{Email: "a@example.com", Password: "S3cr3t-pw"}
	adm := identity.CreateAdminInput{Email: "admin@example.com", Password: "S3cr3t-adm"}
	var buf bytes.Buffer
	l := slog.New(slog.NewJSONHandler(&buf, nil))
	l.Info("m", "list", []identity.LoginInput{in}, "map", map[string]identity.CreateAdminInput{"x": adm}, "ptr", &in)
	out := buf.String()
	for _, s := range []string{"S3cr3t-pw", "S3cr3t-adm"} {
		if strings.Contains(out, s) {
			t.Errorf("%q bocor: %s", s, out)
		}
	}
	if !strings.Contains(out, "a@example.com") {
		t.Errorf("bidang tak sensitif hilang: %s", out)
	}
}

// Tipe yang hanya berperan sebagai input menyamarkan diri pada json.Marshal.
func TestInputTypesMarshalMasked(t *testing.T) {
	cases := []struct {
		v       any
		secrets []string
		visible []string
	}{
		{identity.LoginInput{Email: "a@example.com", Password: "S3cr3t-pw"}, []string{"S3cr3t-pw"}, []string{`"email":"a@example.com"`, `"password":"****"`}},
		{identity.RefreshInput{RefreshToken: "S3cr3t-r"}, []string{"S3cr3t-r"}, nil},
		{identity.ResetInput{Token: "S3cr3t-t", NewPassword: "S3cr3t-n"}, []string{"S3cr3t-t", "S3cr3t-n"}, []string{`"token":"****"`, `"newPassword":"****"`}},
		{identity.ChangeInput{CurrentPassword: "S3cr3t-c", NewPassword: "S3cr3t-n"}, []string{"S3cr3t-c", "S3cr3t-n"}, []string{`"currentPassword":"****"`}},
		{identity.ConfirmMFAInput{Code: "S3cr3t-1"}, []string{"S3cr3t-1"}, []string{`"code":"****"`}},
		{identity.VerifyInput{Code: "S3cr3t-1", Recovery: true}, []string{"S3cr3t-1"}, []string{`"recovery":true`}},
		{identity.CreateAdminInput{Email: "x@example.com", Name: "Nama", Password: "S3cr3t-p"}, []string{"S3cr3t-p"}, []string{`"name":"Nama"`, `"password":"****"`}},
		{&identity.LoginInput{Password: "S3cr3t-pw"}, []string{"S3cr3t-pw"}, nil},
	}
	for _, c := range cases {
		raw, err := json.Marshal(c.v)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range c.secrets {
			if strings.Contains(string(raw), s) {
				t.Errorf("%T: %q bocor: %s", c.v, s, raw)
			}
		}
		for _, w := range c.visible {
			if !strings.Contains(string(raw), w) {
				t.Errorf("%T: tanpa %s: %s", c.v, w, raw)
			}
		}
	}
}

// Tipe hasil tetap dapat di-marshal untuk respons (access token harus sampai ke klien).
func TestResultTypesMarshalForResponse(t *testing.T) {
	raw, _ := json.Marshal(identity.LoginResult{AccessToken: "acc", ExpiresIn: 1})
	if !strings.Contains(string(raw), `"accessToken":"acc"`) {
		t.Errorf("LoginResult: %s", raw)
	}
	raw, _ = json.Marshal(identity.EnrollmentInfo{Secret: "SEC", OTPAuthURI: "otpauth://x"})
	if !strings.Contains(string(raw), `"secret":"SEC"`) {
		t.Errorf("EnrollmentInfo: %s", raw)
	}
	raw, _ = json.Marshal(identity.ConfirmMFAResult{Codes: []string{"c1"},
		Session: identity.LoginResult{AccessToken: "acc2", ExpiresIn: 1, RefreshToken: "S3cr3t-refresh-cookie"}})
	if !strings.Contains(string(raw), `"codes":["c1"]`) || !strings.Contains(string(raw), `"accessToken":"acc2"`) ||
		strings.Contains(string(raw), "S3cr3t-refresh-cookie") {
		t.Errorf("ConfirmMFAResult: %s", raw)
	}
	raw, _ = json.Marshal(identity.RecoveryCodes{Codes: []string{"c1"}})
	if !strings.Contains(string(raw), `"codes":["c1"]`) {
		t.Errorf("RecoveryCodes: %s", raw)
	}
}

// Penyamaran saat marshal tidak boleh merusak decode body request: httpx.DecodeJSON harus tetap mengisi
// field sandi dan token dengan nilai asli.
func TestInputTypesDecodeStillWorks(t *testing.T) {
	post := func(body string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		return r
	}
	got, err := httpx.DecodeJSON[identity.LoginInput](httptest.NewRecorder(), post(`{"email":"a@example.com","password":"real-pw"}`))
	if err != nil || got.Password != "real-pw" || got.Email != "a@example.com" {
		t.Fatalf("LoginInput: %+v err=%v", got, err)
	}
	rs, err := httpx.DecodeJSON[identity.ResetInput](httptest.NewRecorder(), post(`{"token":"tok","newPassword":"npw"}`))
	if err != nil || rs.Token != "tok" || rs.NewPassword != "npw" {
		t.Fatalf("ResetInput: %+v err=%v", rs, err)
	}
	ch, err := httpx.DecodeJSON[identity.ChangeInput](httptest.NewRecorder(), post(`{"currentPassword":"o","newPassword":"n"}`))
	if err != nil || ch.CurrentPassword != "o" || ch.NewPassword != "n" {
		t.Fatalf("ChangeInput: %+v err=%v", ch, err)
	}
	vf, err := httpx.DecodeJSON[identity.VerifyInput](httptest.NewRecorder(), post(`{"code":"123456","recovery":true}`))
	if err != nil || vf.Code != "123456" || !vf.Recovery {
		t.Fatalf("VerifyInput: %+v err=%v", vf, err)
	}
	// json.Unmarshal biasa juga tetap normal.
	var in identity.CreateAdminInput
	if err := json.Unmarshal([]byte(`{"email":"e","name":"n","password":"p"}`), &in); err != nil || in.Password != "p" {
		t.Fatalf("CreateAdminInput: %+v err=%v", in, err)
	}
}

package identity_test

import (
	"context"
	"reflect"
	"testing"

	"erpora/sdk/identity"
)

// fullService menetapkan bentuk persis antarmuka Service (kompilasi gagal bila berubah tanpa sengaja).
type fullService struct{}

func (fullService) Login(context.Context, identity.LoginInput) (identity.LoginResult, error) {
	return identity.LoginResult{}, nil
}
func (fullService) Refresh(context.Context, identity.RefreshInput) (identity.LoginResult, error) {
	return identity.LoginResult{}, nil
}

// Logout tanpa sessionID: sesi diturunkan dari Principal di ctx (M-9, tanpa jalur IDOR).
func (fullService) Logout(context.Context) error                               { return nil }
func (fullService) ForgotPassword(context.Context, identity.ForgotInput) error { return nil }
func (fullService) ResetPassword(context.Context, identity.ResetInput) error   { return nil }
func (fullService) ChangePassword(context.Context, identity.ChangeInput) error { return nil }
func (fullService) BeginMFAEnrollment(context.Context) (identity.EnrollmentInfo, error) {
	return identity.EnrollmentInfo{}, nil
}
func (fullService) ConfirmMFA(context.Context, identity.ConfirmMFAInput) (identity.RecoveryCodes, error) {
	return identity.RecoveryCodes{}, nil
}
func (fullService) VerifyMFA(context.Context, identity.VerifyInput) (identity.LoginResult, error) {
	return identity.LoginResult{}, nil
}
func (fullService) RegenerateRecoveryCodes(context.Context) (identity.RecoveryCodes, error) {
	return identity.RecoveryCodes{}, nil
}

var _ identity.Service = fullService{}

type fullAdmin struct{}

func (fullAdmin) CreateAdmin(context.Context, identity.CreateAdminInput) (identity.UserSummary, error) {
	return identity.UserSummary{}, nil
}
func (fullAdmin) RotateSigningKeys(context.Context) (identity.KeyInfo, error) {
	return identity.KeyInfo{}, nil
}
func (fullAdmin) ResetMFA(context.Context, string) error         { return nil }
func (fullAdmin) UnlockAccount(context.Context, string) error    { return nil }
func (fullAdmin) LogoutAllDevices(context.Context, string) error { return nil }

var _ identity.Admin = fullAdmin{}

// Satu pola metadata request: setiap input yang diterima dari jaringan membawa ClientIP dan UserAgent
// bertag json:"-" (diisi handler dari request, tidak pernah dari body klien).
func TestRequestMetadataConsistent(t *testing.T) {
	inputs := []any{
		identity.LoginInput{}, identity.RefreshInput{}, identity.ForgotInput{}, identity.ResetInput{},
		identity.ChangeInput{}, identity.VerifyInput{}, identity.ConfirmMFAInput{},
	}
	for _, in := range inputs {
		rt := reflect.TypeOf(in)
		for _, name := range []string{"ClientIP", "UserAgent"} {
			f, ok := rt.FieldByName(name)
			if !ok {
				t.Errorf("%s tidak punya field %s", rt.Name(), name)
				continue
			}
			if f.Tag.Get("json") != "-" {
				t.Errorf("%s.%s harus bertag json:\"-\", got %q", rt.Name(), name, f.Tag.Get("json"))
			}
		}
	}
}

func TestForgotInputFields(t *testing.T) {
	rt := reflect.TypeOf(identity.ForgotInput{})
	for _, name := range []string{"Email", "Locale"} {
		if _, ok := rt.FieldByName(name); !ok {
			t.Errorf("ForgotInput tanpa %s", name)
		}
	}
	if f, _ := rt.FieldByName("Locale"); f.Tag.Get("json") != "-" {
		t.Errorf("Locale diisi handler (Accept-Language), harus json:\"-\"")
	}
}

package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	sdkidentity "erpora/sdk/identity"
	"erpora/sdk/module"
)

// fakeAdmin merekam panggilan Admin dan mengembalikan hasil/galat yang diatur tes.
type fakeAdmin struct {
	created   []sdkidentity.CreateAdminInput
	createErr error
	rotateErr error
	key       sdkidentity.KeyInfo
	resetMFA  []string
	unlocked  []string
	publicURL []string
	err       error // galat umum untuk reset/unlock/set URL
	rotated   int
}

func (f *fakeAdmin) CreateAdmin(_ context.Context, in sdkidentity.CreateAdminInput) (sdkidentity.UserSummary, error) {
	f.created = append(f.created, in)
	if f.createErr != nil {
		return sdkidentity.UserSummary{}, f.createErr
	}
	return sdkidentity.UserSummary{ID: "u-1", Email: strings.ToLower(in.Email), Name: in.Name}, nil
}

func (f *fakeAdmin) RotateSigningKeys(context.Context) (sdkidentity.KeyInfo, error) {
	f.rotated++
	return f.key, f.rotateErr
}
func (f *fakeAdmin) ResetMFA(_ context.Context, email string) error {
	f.resetMFA = append(f.resetMFA, email)
	return f.err
}
func (f *fakeAdmin) UnlockAccount(_ context.Context, email string) error {
	f.unlocked = append(f.unlocked, email)
	return f.err
}
func (f *fakeAdmin) LogoutAllDevices(context.Context, string) error { return nil }
func (f *fakeAdmin) SetPublicURL(_ context.Context, u string) error {
	f.publicURL = append(f.publicURL, u)
	return f.err
}

const testPassword = "kata-sandi-cli-kuat-2026"

func byName(t *testing.T, name string) module.Command {
	t.Helper()
	for _, c := range Commands() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("command %q tidak ada", name)
	return module.Command{}
}

func run(t *testing.T, name string, a *fakeAdmin, flags map[string]string, stdin string) (string, error) {
	t.Helper()
	ctx := context.Background()
	if a != nil {
		ctx = sdkidentity.WithAdmin(ctx, a)
	}
	var out bytes.Buffer
	var in io.Reader
	if stdin != "-" {
		in = strings.NewReader(stdin)
	}
	err := byName(t, name).Run(ctx, module.Invocation{Flags: flags, Out: &out, In: in})
	return out.String(), err
}

func TestCommands_Declared(t *testing.T) {
	want := map[string][]string{
		"create-admin": {"email", "name"},
		"rotate-keys":  nil,
		"reset-mfa":    {"email"},
		"unlock":       {"email"},
		"settings:set": {"key", "value"},
	}
	got := Commands()
	if len(got) != len(want) {
		t.Fatalf("jumlah command = %d, want %d", len(got), len(want))
	}
	for _, c := range got {
		flags, ok := want[c.Name]
		if !ok {
			t.Errorf("command tak terduga %q", c.Name)
			continue
		}
		if c.Short == "" || c.Run == nil {
			t.Errorf("%s: Short/Run wajib", c.Name)
		}
		var names []string
		for _, f := range c.Flags {
			names = append(names, f.Name)
			// Tidak boleh ada flag yang membawa rahasia: sandi hanya dari prompt/stdin.
			for _, bad := range []string{"password", "pass", "secret", "token", "sandi"} {
				if strings.Contains(strings.ToLower(f.Name), bad) {
					t.Errorf("%s: flag %q tidak boleh membawa rahasia", c.Name, f.Name)
				}
			}
		}
		if strings.Join(names, ",") != strings.Join(flags, ",") {
			t.Errorf("%s: flag = %v, want %v", c.Name, names, flags)
		}
	}
}

func TestCreateAdmin_ReadsPasswordFromStdinNotArgs(t *testing.T) {
	a := &fakeAdmin{}
	out, err := run(t, "create-admin", a, map[string]string{"email": "Admin@Example.test", "name": "Budi"}, testPassword+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.created) != 1 || a.created[0].Email != "Admin@Example.test" || a.created[0].Name != "Budi" || a.created[0].Password != testPassword {
		t.Fatalf("CreateAdmin dipanggil dengan %+v", a.created)
	}
	if strings.Contains(out, testPassword) || !strings.Contains(out, "admin@example.test") {
		t.Errorf("keluaran = %q (harus memuat email, tanpa sandi)", out)
	}
}

func TestCreateAdmin_StdinLineEndingsAndFirstLineOnly(t *testing.T) {
	for name, in := range map[string]string{
		"CRLF":           testPassword + "\r\n",
		"tanpa newline":  testPassword,
		"baris pertama":  testPassword + "\nbaris-kedua\n",
		"spasi bermakna": "  " + testPassword + "  \n",
	} {
		t.Run(name, func(t *testing.T) {
			a := &fakeAdmin{}
			if _, err := run(t, "create-admin", a, map[string]string{"email": "a@b.test"}, in); err != nil {
				t.Fatal(err)
			}
			want := testPassword
			if name == "spasi bermakna" {
				want = "  " + testPassword + "  " // sandi tidak di-trim
			}
			if a.created[0].Password != want {
				t.Errorf("sandi terbaca %q, want %q", a.created[0].Password, want)
			}
		})
	}
}

func TestCreateAdmin_DefaultNameAndRequiredEmail(t *testing.T) {
	a := &fakeAdmin{}
	if _, err := run(t, "create-admin", a, map[string]string{"email": "a@b.test"}, testPassword+"\n"); err != nil {
		t.Fatal(err)
	}
	if a.created[0].Name != DefaultAdminName {
		t.Errorf("nama bawaan = %q, want %q", a.created[0].Name, DefaultAdminName)
	}
	for _, flags := range []map[string]string{{}, {"email": "  "}} {
		a = &fakeAdmin{}
		if _, err := run(t, "create-admin", a, flags, testPassword+"\n"); err == nil || !strings.Contains(err.Error(), "--email") {
			t.Errorf("tanpa email harus menolak menyebut --email: %v", err)
		}
		if len(a.created) != 0 {
			t.Error("Admin tidak boleh dipanggil tanpa email")
		}
	}
}

func TestCreateAdmin_PasswordInputErrors(t *testing.T) {
	a := &fakeAdmin{}
	if _, err := run(t, "create-admin", a, map[string]string{"email": "a@b.test"}, "-"); err == nil {
		t.Error("tanpa stdin harus galat")
	}
	for name, in := range map[string]string{"kosong": "", "newline saja": "\n", "terlalu panjang": strings.Repeat("x", maxPasswordInput+10) + "\n"} {
		a = &fakeAdmin{}
		out, err := run(t, "create-admin", a, map[string]string{"email": "a@b.test"}, in)
		if err == nil {
			t.Errorf("%s: harus galat", name)
		}
		if len(a.created) != 0 {
			t.Errorf("%s: Admin tidak boleh dipanggil", name)
		}
		if strings.Contains(out+errString(err), "xxxx") {
			t.Errorf("%s: masukan sandi bocor ke keluaran/galat", name)
		}
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestCreateAdmin_ErrorsDoNotEchoPassword(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"kebijakan": {&sdkidentity.PasswordPolicyError{Codes: []string{"too_short", "common"}}, "too_short"},
		"email":     {sdkidentity.ErrEmailTaken, "sudah dipakai"},
		"input":     {sdkidentity.ErrInvalidInput, "tidak valid"},
		"forbidden": {sdkidentity.ErrForbidden, "ditolak"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a := &fakeAdmin{createErr: c.err}
			out, err := run(t, "create-admin", a, map[string]string{"email": "a@b.test"}, testPassword+"\n")
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want memuat %q", err, c.want)
			}
			if strings.Contains(out+err.Error(), testPassword) {
				t.Error("sandi bocor ke keluaran atau galat")
			}
		})
	}
}

func TestRotateKeys_ReportsPendingAndActivation(t *testing.T) {
	act := time.Date(2026, 11, 4, 12, 5, 0, 0, time.UTC)
	a := &fakeAdmin{key: sdkidentity.KeyInfo{KeyID: "kid-1", Algorithm: "EdDSA", Status: sdkidentity.KeyStatusPending,
		CreatedAt: act.Add(-5 * time.Minute), ActivatesAt: act}}
	out, err := run(t, "rotate-keys", a, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kid-1", "EdDSA", "pending", act.Format(time.RFC3339), "serve"} {
		if !strings.Contains(out, want) {
			t.Errorf("keluaran tidak memuat %q:\n%s", want, out)
		}
	}
	if a.rotated != 1 {
		t.Errorf("RotateSigningKeys dipanggil %d kali", a.rotated)
	}
}

func TestRotateKeys_ErrorAndMissingAdmin(t *testing.T) {
	a := &fakeAdmin{rotateErr: sdkidentity.ErrForbidden}
	if _, err := run(t, "rotate-keys", a, nil, ""); err == nil || !strings.Contains(err.Error(), "ditolak") {
		t.Errorf("err = %v", err)
	}
	// Tanpa Admin di context (modul bukan core / stack salah rakit): galat jelas, bukan panic.
	if _, err := run(t, "rotate-keys", nil, nil, ""); err == nil || !strings.Contains(err.Error(), "Admin") {
		t.Errorf("tanpa Admin: %v", err)
	}
}

func TestResetMFAAndUnlock(t *testing.T) {
	for _, name := range []string{"reset-mfa", "unlock"} {
		t.Run(name, func(t *testing.T) {
			a := &fakeAdmin{}
			out, err := run(t, name, a, map[string]string{"email": "Orang@Example.test"}, "")
			if err != nil {
				t.Fatal(err)
			}
			got := append(a.resetMFA, a.unlocked...)
			if len(got) != 1 || got[0] != "Orang@Example.test" || !strings.Contains(out, "Orang@Example.test") {
				t.Errorf("panggilan = %v, keluaran = %q", got, out)
			}
			if _, err := run(t, name, a, map[string]string{}, ""); err == nil || !strings.Contains(err.Error(), "--email") {
				t.Errorf("tanpa email: %v", err)
			}
			a.err = sdkidentity.ErrNotFound
			if _, err := run(t, name, a, map[string]string{"email": "x@y.test"}, ""); err == nil || !strings.Contains(err.Error(), "tidak ditemukan") {
				t.Errorf("sasaran tak ada: %v", err)
			}
		})
	}
}

func TestSettingsSet_OnlyPublicURLKey(t *testing.T) {
	a := &fakeAdmin{}
	out, err := run(t, "settings:set", a, map[string]string{"key": "platform.public_url", "value": "https://erp.example.com"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.publicURL) != 1 || a.publicURL[0] != "https://erp.example.com" || !strings.Contains(out, "platform.public_url") {
		t.Errorf("panggilan = %v, keluaran = %q", a.publicURL, out)
	}
	for name, flags := range map[string]map[string]string{
		"kunci lain":    {"key": "platform.other", "value": "x"},
		"tanpa kunci":   {"value": "https://erp.example.com"},
		"tanpa nilai":   {"key": "platform.public_url"},
		"nilai spasi":   {"key": "platform.public_url", "value": "   "},
		"kunci berbeda": {"key": "Platform.Public_URL", "value": "https://x.test"},
	} {
		a = &fakeAdmin{}
		if _, err := run(t, "settings:set", a, flags, ""); err == nil {
			t.Errorf("%s: harus galat", name)
		}
		if len(a.publicURL) != 0 {
			t.Errorf("%s: Admin tidak boleh dipanggil", name)
		}
	}
	a = &fakeAdmin{err: errors.Join(sdkidentity.ErrInvalidInput, errors.New("https wajib di production"))}
	if _, err := run(t, "settings:set", a, map[string]string{"key": "platform.public_url", "value": "http://x.test"}, ""); err == nil || !strings.Contains(err.Error(), "https wajib") {
		t.Errorf("galat validasi harus diteruskan ke operator: %v", err)
	}
}

// Stdin non-terminal (mis. mintty/Git Bash di Windows tanpa winpty) tidak punya prompt: pengguna harus diberi tahu di
// stderr bahwa sandi dibaca satu baris dan TIDAK disembunyikan. stdout tetap bersih untuk skrip yang memakai pipe.
func TestCreateAdmin_NonTTYStdinWritesNoticeToErrNotOut(t *testing.T) {
	a := &fakeAdmin{}
	var out, errw bytes.Buffer
	err := byName(t, "create-admin").Run(sdkidentity.WithAdmin(context.Background(), a), module.Invocation{
		Flags: map[string]string{"email": "admin@example.test"}, Out: &out, Err: &errw, In: strings.NewReader(testPassword + "\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	notice := errw.String()
	for _, want := range []string{"stdin", "tidak disembunyikan", "winpty"} {
		if !strings.Contains(notice, want) {
			t.Errorf("pemberitahuan stderr %q tidak memuat %q", notice, want)
		}
	}
	if strings.Contains(notice, testPassword) || strings.Contains(out.String(), testPassword) {
		t.Errorf("sandi bocor: stderr=%q stdout=%q", notice, out.String())
	}
	if strings.Contains(out.String(), "tidak disembunyikan") {
		t.Errorf("pemberitahuan tidak boleh ke stdout: %q", out.String())
	}
}

// Err nil (pemanggil tanpa stderr) tidak boleh membuat panik.
func TestCreateAdmin_NonTTYStdinWithoutErrWriter(t *testing.T) {
	if _, err := run(t, "create-admin", &fakeAdmin{}, map[string]string{"email": "admin@example.test"}, testPassword+"\n"); err != nil {
		t.Fatal(err)
	}
}

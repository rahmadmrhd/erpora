package identity

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
)

// Struct pembawa rahasia (sandi, token, kode MFA, secret TOTP) mengimplementasi fmt.Formatter (semua
// verba: %v %+v %#v %s %d %x %q ...), fmt.Stringer, fmt.GoStringer, dan slog.LogValuer agar pencetakan
// tak sengaja menyamarkan nilai rahasia menjadi "****" sementara bidang tak sensitif tetap terlihat.
//
// Serialisasi JSON: tipe yang HANYA berperan sebagai input (LoginInput, RefreshInput, ResetInput,
// ChangeInput, ConfirmMFAInput, VerifyInput, CreateAdminInput) juga menyamarkan diri pada json.Marshal
// (mis. slog JSON handler yang men-marshal isi slice/map); decoding tidak terpengaruh karena tidak ada
// UnmarshalJSON. Tipe hasil (LoginResult, EnrollmentInfo, RecoveryCodes) tetap di-marshal apa adanya
// karena memang dikirim ke klien sebagai respons; satu-satunya jalur penyamaran mereka adalah
// Formatter/LogValuer, jadi jangan menaruhnya mentah di dalam slice/map yang dicatat ke slog JSON handler.
// Batasan: redaksi Format/MarshalJSON tidak berlaku bila nilai pembawa rahasia menjadi field TAK DIEKSPOR
// dari struct lain yang dicetak dengan %+v (fmt tidak dapat memanggil Format pada field tak terekspor
// karena CanInterface false), jadi jangan menyimpan tipe-tipe ini sebagai field tak terekspor yang dicetak.
// Ini jaring pengaman terakhir; kode tetap tidak boleh sengaja mencatat input mentah.

const masked = "****"

type attr struct {
	key    string
	val    any
	secret bool
}

func secret(key string) attr       { return attr{key: key, secret: true} }
func plain(key string, v any) attr { return attr{key: key, val: v} }
func renderAttrs(name string, as []attr) string {
	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, a := range as {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(a.key)
		b.WriteByte(':')
		if a.secret {
			b.WriteString(masked)
		} else {
			fmt.Fprint(&b, a.val)
		}
	}
	b.WriteByte('}')
	return b.String()
}

func logAttrs(as []attr) slog.Value {
	out := make([]slog.Attr, 0, len(as))
	for _, a := range as {
		if a.secret {
			out = append(out, slog.String(a.key, masked))
		} else {
			out = append(out, slog.Any(a.key, a.val))
		}
	}
	return slog.GroupValue(out...)
}

func (x LoginInput) attrs() []attr {
	return []attr{plain("Email", x.Email), secret("Password"), plain("ClientIP", x.ClientIP), plain("UserAgent", x.UserAgent)}
}
func (x LoginInput) String() string             { return renderAttrs("LoginInput", x.attrs()) }
func (x LoginInput) GoString() string           { return x.String() }
func (x LoginInput) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x LoginInput) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x RefreshInput) attrs() []attr {
	return []attr{secret("RefreshToken"), plain("ClientIP", x.ClientIP), plain("UserAgent", x.UserAgent)}
}
func (x RefreshInput) String() string             { return renderAttrs("RefreshInput", x.attrs()) }
func (x RefreshInput) GoString() string           { return x.String() }
func (x RefreshInput) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x RefreshInput) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x ResetInput) attrs() []attr {
	return []attr{secret("Token"), secret("NewPassword"), plain("ClientIP", x.ClientIP), plain("UserAgent", x.UserAgent)}
}
func (x ResetInput) String() string             { return renderAttrs("ResetInput", x.attrs()) }
func (x ResetInput) GoString() string           { return x.String() }
func (x ResetInput) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x ResetInput) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x ChangeInput) attrs() []attr {
	return []attr{secret("CurrentPassword"), secret("NewPassword"), plain("ClientIP", x.ClientIP), plain("UserAgent", x.UserAgent)}
}
func (x ChangeInput) String() string             { return renderAttrs("ChangeInput", x.attrs()) }
func (x ChangeInput) GoString() string           { return x.String() }
func (x ChangeInput) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x ChangeInput) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x LoginResult) attrs() []attr {
	return []attr{secret("AccessToken"), plain("ExpiresIn", x.ExpiresIn), plain("MFAState", x.MFAState),
		secret("RefreshToken"), plain("RefreshExpiresAt", x.RefreshExpiresAt)}
}
func (x LoginResult) String() string             { return renderAttrs("LoginResult", x.attrs()) }
func (x LoginResult) GoString() string           { return x.String() }
func (x LoginResult) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x LoginResult) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x EnrollmentInfo) attrs() []attr              { return []attr{secret("Secret"), secret("OTPAuthURI")} }
func (x EnrollmentInfo) String() string             { return renderAttrs("EnrollmentInfo", x.attrs()) }
func (x EnrollmentInfo) GoString() string           { return x.String() }
func (x EnrollmentInfo) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x EnrollmentInfo) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x RecoveryCodes) attrs() []attr              { return []attr{secret("Codes")} }
func (x RecoveryCodes) String() string             { return renderAttrs("RecoveryCodes", x.attrs()) }
func (x RecoveryCodes) GoString() string           { return x.String() }
func (x RecoveryCodes) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x RecoveryCodes) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x ConfirmMFAInput) attrs() []attr {
	return []attr{secret("Code"), plain("ClientIP", x.ClientIP), plain("UserAgent", x.UserAgent)}
}
func (x ConfirmMFAInput) String() string             { return renderAttrs("ConfirmMFAInput", x.attrs()) }
func (x ConfirmMFAInput) GoString() string           { return x.String() }
func (x ConfirmMFAInput) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x ConfirmMFAInput) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x VerifyInput) attrs() []attr {
	return []attr{secret("Code"), plain("Recovery", x.Recovery), plain("ClientIP", x.ClientIP), plain("UserAgent", x.UserAgent)}
}
func (x VerifyInput) String() string             { return renderAttrs("VerifyInput", x.attrs()) }
func (x VerifyInput) GoString() string           { return x.String() }
func (x VerifyInput) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x VerifyInput) LogValue() slog.Value       { return logAttrs(x.attrs()) }

func (x CreateAdminInput) attrs() []attr {
	return []attr{plain("Email", x.Email), plain("Name", x.Name), secret("Password")}
}
func (x CreateAdminInput) String() string             { return renderAttrs("CreateAdminInput", x.attrs()) }
func (x CreateAdminInput) GoString() string           { return x.String() }
func (x CreateAdminInput) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, x.String()) }
func (x CreateAdminInput) LogValue() slog.Value       { return logAttrs(x.attrs()) }

// MarshalJSON tipe input: bentuk sama dengan serialisasi biasa, nilai rahasia diganti "****". Tipe
// `plain` menghapus method sehingga tidak rekursif dan tag json tetap berlaku.
func (x LoginInput) MarshalJSON() ([]byte, error) {
	type plain LoginInput
	x.Password = masked
	return json.Marshal(plain(x))
}

func (x RefreshInput) MarshalJSON() ([]byte, error) {
	type plain RefreshInput
	x.RefreshToken = masked
	return json.Marshal(plain(x))
}

func (x ResetInput) MarshalJSON() ([]byte, error) {
	type plain ResetInput
	x.Token, x.NewPassword = masked, masked
	return json.Marshal(plain(x))
}

func (x ChangeInput) MarshalJSON() ([]byte, error) {
	type plain ChangeInput
	x.CurrentPassword, x.NewPassword = masked, masked
	return json.Marshal(plain(x))
}

func (x ConfirmMFAInput) MarshalJSON() ([]byte, error) {
	type plain ConfirmMFAInput
	x.Code = masked
	return json.Marshal(plain(x))
}

func (x VerifyInput) MarshalJSON() ([]byte, error) {
	type plain VerifyInput
	x.Code = masked
	return json.Marshal(plain(x))
}

func (x CreateAdminInput) MarshalJSON() ([]byte, error) {
	type plain CreateAdminInput
	x.Password = masked
	return json.Marshal(plain(x))
}

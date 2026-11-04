package mail_test

import (
	"errors"
	"testing"

	"erpora/sdk/mail"
)

// Byte UTF-8 tidak valid (mis. 0xFF, 0xC3 0x28) tidak boleh lolos ke Subject, Text, HTML, maupun alamat:
// iterasi rune mengubahnya menjadi U+FFFD (kategori So) sehingga pemeriksaan karakter kontrol saja tidak cukup.
func TestValidateRejectsInvalidUTF8(t *testing.T) {
	invalid := map[string]string{
		"0xFF":      string([]byte{0xff}),
		"0xC3 0x28": string([]byte{0xc3, 0x28}),
		"terpotong": string([]byte{0xe2, 0x82}),
	}
	base := func() mail.Message {
		return mail.Message{To: []string{"a@example.com"}, Subject: "x", Text: "t"}
	}
	for name, bad := range invalid {
		subj := base()
		subj.Subject = "a" + bad + "b"
		text := base()
		text.Text = "a" + bad + "b"
		html := base()
		html.HTML = "<p>a" + bad + "b</p>"
		to := base()
		to.To = []string{"a" + bad + "@example.com"}
		for field, m := range map[string]mail.Message{"Subject": subj, "Text": text, "HTML": html, "To": to} {
			if err := mail.Validate(m); !errors.Is(err, mail.ErrInvalidMessage) {
				t.Errorf("%s/%s: err = %v, want ErrInvalidMessage", name, field, err)
			}
		}
	}
	// UTF-8 sah (non-ASCII) pada isi dan subjek tetap diterima.
	ok := base()
	ok.Subject = "Müller"
	ok.Text = "isi: 例え 👍"
	ok.HTML = "<p>é</p>"
	if err := mail.Validate(ok); err != nil {
		t.Errorf("UTF-8 sah ditolak: %v", err)
	}
}

package mail_test

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"erpora/sdk/mail"
)

type rec struct{ got []mail.Message }

func (r *rec) Send(_ context.Context, m mail.Message) error { r.got = append(r.got, m); return nil }

func TestMailSendWithoutSenderFailsClosed(t *testing.T) {
	err := mail.Send(context.Background(), mail.Message{To: []string{"a@example.com"}, Subject: "x", Text: "y"})
	if !errors.Is(err, mail.ErrNoSender) {
		t.Fatalf("err = %v, want ErrNoSender", err)
	}
	if _, ok := mail.From(context.Background()); ok {
		t.Fatal("ctx kosong tidak boleh membawa sender")
	}
	if _, ok := mail.From(mail.WithSender(context.Background(), nil)); ok {
		t.Fatal("sender nil dianggap tidak ada")
	}
}

func TestMailSendDelivers(t *testing.T) {
	r := &rec{}
	ctx := mail.WithSender(context.Background(), r)
	m := mail.Message{To: []string{"a@example.com"}, Subject: "Halo", Text: "isi"}
	if err := mail.Send(ctx, m); err != nil {
		t.Fatal(err)
	}
	if len(r.got) != 1 || r.got[0].Subject != "Halo" {
		t.Fatalf("got %+v", r.got)
	}
}

func TestMailSendRejectsHeaderInjection(t *testing.T) {
	r := &rec{}
	ctx := mail.WithSender(context.Background(), r)
	cases := map[string]mail.Message{
		"subject LF":   {To: []string{"a@example.com"}, Subject: "x\nBcc: evil@example.com", Text: "t"},
		"subject CR":   {To: []string{"a@example.com"}, Subject: "x\rBcc: evil@example.com", Text: "t"},
		"to CRLF":      {To: []string{"a@example.com\r\nBcc: evil@example.com"}, Subject: "x", Text: "t"},
		"to LF second": {To: []string{"a@example.com", "b@example.com\n"}, Subject: "x", Text: "t"},
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if err := mail.Send(ctx, m); !errors.Is(err, mail.ErrInvalidMessage) {
				t.Fatalf("err = %v, want ErrInvalidMessage", err)
			}
		})
	}
	if len(r.got) != 0 {
		t.Fatalf("pesan tidak valid terkirim: %+v", r.got)
	}
}

func TestMailSendRejectsEmptyRecipients(t *testing.T) {
	ctx := mail.WithSender(context.Background(), &rec{})
	for _, to := range [][]string{nil, {}, {""}, {"  "}} {
		err := mail.Send(ctx, mail.Message{To: to, Subject: "x", Text: "t"})
		if !errors.Is(err, mail.ErrInvalidMessage) {
			t.Errorf("to=%q err = %v, want ErrInvalidMessage", to, err)
		}
	}
}

func TestMailSendRejectsOversize(t *testing.T) {
	ctx := mail.WithSender(context.Background(), &rec{})
	base := mail.Message{To: []string{"a@example.com"}, Subject: "x", Text: "t"}

	big := base
	big.Text = strings.Repeat("a", mail.MaxBodyBytes+1)
	if err := mail.Send(ctx, big); !errors.Is(err, mail.ErrInvalidMessage) {
		t.Errorf("body besar err = %v", err)
	}
	long := base
	long.Subject = strings.Repeat("a", mail.MaxSubjectLen+1)
	if err := mail.Send(ctx, long); !errors.Is(err, mail.ErrInvalidMessage) {
		t.Errorf("subject panjang err = %v", err)
	}
	many := base
	many.To = make([]string, mail.MaxRecipients+1)
	for i := range many.To {
		many.To[i] = "a@example.com"
	}
	if err := mail.Send(ctx, many); !errors.Is(err, mail.ErrInvalidMessage) {
		t.Errorf("penerima banyak err = %v", err)
	}
}

// Pesan tanpa subjek atau tanpa isi ditolak: tidak ada email kosong.
func TestMailSendRejectsEmptyContent(t *testing.T) {
	ctx := mail.WithSender(context.Background(), &rec{})
	if err := mail.Send(ctx, mail.Message{To: []string{"a@example.com"}, Subject: "x"}); !errors.Is(err, mail.ErrInvalidMessage) {
		t.Errorf("tanpa isi err = %v", err)
	}
	if err := mail.Send(ctx, mail.Message{To: []string{"a@example.com"}, Text: "t"}); !errors.Is(err, mail.ErrInvalidMessage) {
		t.Errorf("tanpa subjek err = %v", err)
	}
}

func TestMailValidateRecipientAddresses(t *testing.T) {
	bad := []string{
		"Nama <a@example.com>",        // nama tampilan
		"<a@example.com>",             // sudut
		"a@example.com,b@example.com", // koma: dua alamat dalam satu entri
		"a@example.com b@example.com", // ruang
		" a@example.com",              // ruang awal
		"a@example.com ",              // ruang akhir
		"tanpa-at",                    // bukan alamat
		"a@example.com\x00",           // NUL
		"a\t@example.com",             // tab
		"a@example.com\x7f",           // DEL
	}
	for _, to := range bad {
		err := mail.Validate(mail.Message{To: []string{to}, Subject: "x", Text: "t"})
		if !errors.Is(err, mail.ErrInvalidMessage) {
			t.Errorf("to=%q err = %v, want ErrInvalidMessage", to, err)
		}
	}
	if err := mail.Validate(mail.Message{To: []string{"a@example.com", "b.c+d@example.org"}, Subject: "x", Text: "t"}); err != nil {
		t.Errorf("alamat polos sah ditolak: %v", err)
	}
}

func TestMailValidateRejectsControlCharsInSubject(t *testing.T) {
	for _, subj := range []string{"a\x00b", "a\tb", "a\x1bb", "a\x7fb", "a\u0085b"} {
		err := mail.Validate(mail.Message{To: []string{"a@example.com"}, Subject: subj, Text: "t"})
		if !errors.Is(err, mail.ErrInvalidMessage) {
			t.Errorf("subject=%q err = %v, want ErrInvalidMessage", subj, err)
		}
	}
}

// Send memvalidasi sebelum mendelegasikan; driver yang dipanggil langsung lewat Sender.Send wajib memakai Validate.
func TestMailSendUsesValidate(t *testing.T) {
	r := &rec{}
	ctx := mail.WithSender(context.Background(), r)
	err := mail.Send(ctx, mail.Message{To: []string{"Nama <a@example.com>"}, Subject: "x", Text: "t"})
	if !errors.Is(err, mail.ErrInvalidMessage) || len(r.got) != 0 {
		t.Fatalf("err=%v terkirim=%d", err, len(r.got))
	}
}

type ptrSender struct{}

func (*ptrSender) Send(context.Context, mail.Message) error { return nil }

type mapSender map[string]string

func (mapSender) Send(context.Context, mail.Message) error { return nil }

type funcSender func()

func (funcSender) Send(context.Context, mail.Message) error { return nil }

// Nil bertipe (pointer/map/func) di balik antarmuka tidak boleh dianggap ada dan tidak boleh panik.
func TestMailFromRejectsTypedNil(t *testing.T) {
	var p *ptrSender
	var m mapSender
	var f funcSender
	for name, s := range map[string]mail.Sender{"ptr": p, "map": m, "func": f} {
		ctx := mail.WithSender(context.Background(), s)
		if _, ok := mail.From(ctx); ok {
			t.Errorf("%s: nil bertipe dianggap sender", name)
		}
		if err := mail.Send(ctx, mail.Message{To: []string{"a@example.com"}, Subject: "x", Text: "t"}); !errors.Is(err, mail.ErrNoSender) {
			t.Errorf("%s: err = %v, want ErrNoSender", name, err)
		}
	}
	// sender non-nil bertipe nilable tetap sah
	if _, ok := mail.From(mail.WithSender(context.Background(), &ptrSender{})); !ok {
		t.Error("pointer non-nil ditolak")
	}
	if _, ok := mail.From(mail.WithSender(context.Background(), mapSender{})); !ok {
		t.Error("map non-nil ditolak")
	}
}

// D9: hanya stdlib.
func TestOnlyStdlibImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			first, _, _ := strings.Cut(path, "/")
			if strings.Contains(first, ".") || first == "erpora" {
				t.Errorf("%s mengimpor non-stdlib %s", name, path)
			}
		}
	}
}

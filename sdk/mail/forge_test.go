package mail_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"erpora/sdk/mail"
)

// forgeKey meniru trik modul jahat tanpa import unsafe: menelusuri rantai ctx (context.valueCtx),
// mengambil tipe kunci tak diekspor milik paket mail lewat refleksi, lalu membuat nilainya dengan
// reflect.New. Mengembalikan kunci tiruan dan tipe nilai yang disimpan paket di bawah kunci itu.
func forgeKey(t *testing.T, legit context.Context, keyTypeName string) (key any, valType reflect.Type) {
	t.Helper()
	c := legit
	for c != nil {
		v := reflect.ValueOf(c)
		if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
			break
		}
		s := v.Elem()
		if s.Type().String() == "context.valueCtx" {
			kt := s.FieldByName("key").Elem().Type()
			if kt.Name() == keyTypeName {
				return reflect.New(kt).Elem().Interface(), s.FieldByName("val").Elem().Type()
			}
		}
		next := s.FieldByName("Context")
		if !next.IsValid() {
			break
		}
		c = next.Interface().(context.Context)
	}
	t.Fatalf("kunci %s tidak ditemukan di rantai context", keyTypeName)
	return nil, nil
}

// zeroBox membuat nilai nol bertipe nilai yang disimpan paket di context (hasil reflect.New: field tak
// diekspor tetap nol, jadi penanda sah tidak terisi). Ini hanya menguji penolakan oleh penanda ok; bukan
// bukti kekebalan terhadap kode yang memakai unsafe (lihat catatan batas di komentar paket sdk/identity).
func zeroBox(valType reflect.Type) any {
	if valType.Kind() == reflect.Pointer {
		return reflect.New(valType.Elem()).Interface()
	}
	return reflect.New(valType).Elem().Interface()
}

func okMessage() mail.Message {
	return mail.Message{To: []string{"a@example.com"}, Subject: "x", Text: "t"}
}

// Sender palsu (mentah, pointer, kotak nol) yang dipasang dengan kunci tiruan ditolak oleh From dan
// Send gagal tertutup (ErrNoSender); pemalsu tidak pernah menerima pesan. Sender sah tetap terbaca.
func TestFromRejectsForgedSender(t *testing.T) {
	real := &rec{}
	legit := mail.WithSender(context.Background(), real)
	key, valType := forgeKey(t, legit, "senderKey")

	attacker := &rec{}
	forgedValues := map[string]any{
		"Sender mentah":       attacker,
		"pointer ke Sender":   &attacker,
		"nilai antarmuka nil": nil,
		"kotak nol":           zeroBox(valType),
	}
	for name, forged := range forgedValues {
		t.Run(name, func(t *testing.T) {
			onEmpty := context.WithValue(context.Background(), key, forged)
			if s, ok := mail.From(onEmpty); ok {
				t.Fatalf("Sender palsu diterima: %v", s)
			}
			if err := mail.Send(onEmpty, okMessage()); !errors.Is(err, mail.ErrNoSender) {
				t.Fatalf("Send tanpa Sender sah: err = %v, want ErrNoSender", err)
			}
			// Di atas ctx sah: pembayang tidak diterima dan tidak jatuh ke Sender sah (gagal tertutup).
			onLegit := context.WithValue(legit, key, forged)
			if s, ok := mail.From(onLegit); ok {
				t.Fatalf("Sender palsu di atas ctx sah diterima: %v", s)
			}
			if err := mail.Send(onLegit, okMessage()); !errors.Is(err, mail.ErrNoSender) {
				t.Fatalf("Send di atas pembayang: err = %v, want ErrNoSender", err)
			}
		})
	}
	if len(attacker.got) != 0 || len(real.got) != 0 {
		t.Fatalf("pesan terkirim lewat jalur palsu: attacker=%d real=%d", len(attacker.got), len(real.got))
	}

	if s, ok := mail.From(legit); !ok || s != mail.Sender(real) {
		t.Fatalf("Sender sah tidak terbaca: %v %v", s, ok)
	}
	if err := mail.Send(legit, okMessage()); err != nil || len(real.got) != 1 {
		t.Fatalf("Send sah gagal: err=%v terkirim=%d", err, len(real.got))
	}
}

// Kunci tiruan tidak boleh memberi Sender yang dapat dipakai: nilai di bawahnya bukan Sender sehingga
// pemanggil tidak dapat memanggil Send langsung (melewati Validate).
func TestForgedKeyCannotReadSender(t *testing.T) {
	legit := mail.WithSender(context.Background(), &rec{})
	key, _ := forgeKey(t, legit, "senderKey")
	raw := legit.Value(key)
	if raw == nil {
		t.Fatal("kunci tiruan tidak menemukan apa pun (uji tidak bermakna)")
	}
	if _, ok := raw.(mail.Sender); ok {
		t.Fatal("nilai di bawah kunci tiruan memenuhi Sender; Send dapat dipanggil langsung")
	}
	rv := reflect.ValueOf(raw)
	if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
		t.Fatalf("kotak bukan pointer ke struct: %T", raw)
	}
	// Seluruh field kotak tak diekspor (desain; tidak boleh diekspor tanpa sengaja).
	for i := 0; i < rv.Elem().NumField(); i++ {
		if rv.Elem().Type().Field(i).IsExported() {
			t.Errorf("field kotak %q diekspor", rv.Elem().Type().Field(i).Name)
		}
	}
}

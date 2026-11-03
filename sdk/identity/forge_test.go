package identity_test

import (
	"context"
	"reflect"
	"testing"

	"erpora/sdk/identity"
)

// valueCtxEntry menelusuri rantai ctx (seperti modul jahat yang memakai refleksi tanpa unsafe) dan
// mengembalikan kunci serta nilai pada context.valueCtx yang tipe kuncinya bernama keyTypeName.
func valueCtxEntry(t *testing.T, ctx context.Context, keyTypeName string) (key reflect.Type, val reflect.Type) {
	t.Helper()
	c := ctx
	for c != nil {
		v := reflect.ValueOf(c)
		if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Struct {
			break
		}
		s := v.Elem()
		if s.Type().String() == "context.valueCtx" {
			kt := s.FieldByName("key").Elem().Type()
			if kt.Name() == keyTypeName {
				return kt, s.FieldByName("val").Elem().Type()
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

// forgeKey membuat nilai kunci context bertipe tak diekspor milik paket identity (trik refleksi).
func forgeKey(t *testing.T, legit context.Context, keyTypeName string) (key any, valType reflect.Type) {
	t.Helper()
	kt, vt := valueCtxEntry(t, legit, keyTypeName)
	return reflect.New(kt).Elem().Interface(), vt
}

// Principal palsu yang dipasang dengan kunci hasil tiruan refleksi harus ditolak; yang sah tetap terbaca.
func TestFromContextRejectsForgedPrincipal(t *testing.T) {
	legit := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "u1", SessionID: "s1", AMR: []string{"pwd"}})
	key, valType := forgeKey(t, legit, "principalKey")

	forgedValues := map[string]any{
		"Principal mentah":  identity.Principal{UserID: "evil", Kind: identity.KindHuman},
		"pointer Principal": &identity.Principal{UserID: "evil"},
		"kotak nol":         zeroBox(valType), // *principalBox tanpa penanda sah
	}
	for name, forged := range forgedValues {
		t.Run(name, func(t *testing.T) {
			onEmpty := context.WithValue(context.Background(), key, forged)
			if p, ok := identity.FromContext(onEmpty); ok || p.UserID != "" {
				t.Fatalf("Principal palsu diterima: %+v ok=%v", p, ok)
			}
			if _, err := identity.RequirePrincipal(onEmpty); err == nil {
				t.Fatal("RequirePrincipal menerima Principal palsu")
			}
			// Dipasang di atas ctx sah: tetap gagal tertutup (tidak jatuh ke nilai lain, tidak menerima palsu).
			onLegit := context.WithValue(legit, key, forged)
			if p, ok := identity.FromContext(onLegit); ok {
				t.Fatalf("Principal palsu di atas ctx sah diterima: %+v", p)
			}
		})
	}

	got, ok := identity.FromContext(legit)
	if !ok || got.UserID != "u1" || got.SessionID != "s1" || len(got.AMR) != 1 {
		t.Fatalf("Principal sah tidak terbaca: %+v ok=%v", got, ok)
	}
}

// Kunci tiruan tidak boleh membocorkan nilai sah: ctx.Value(kunciPalsu) bukan Principal/Service/Admin
// yang dapat dipakai, melainkan kotak bertipe tak diekspor yang field-nya semuanya tak diekspor.
func TestForgedKeyCannotReadValues(t *testing.T) {
	ctx := context.Background()
	ctx = identity.WithPrincipal(ctx, identity.Principal{UserID: "u1"})
	ctx = identity.WithService(ctx, &fakeService{})
	ctx = identity.WithAdmin(ctx, &fakeAdmin{})

	for _, name := range []string{"principalKey", "serviceKey", "adminKey"} {
		key, _ := forgeKey(t, ctx, name)
		raw := ctx.Value(key)
		if raw == nil {
			t.Fatalf("%s: kunci tiruan tidak menemukan apa pun (uji tidak bermakna)", name)
		}
		if _, ok := raw.(identity.Principal); ok {
			t.Errorf("%s: nilai mentah berupa Principal", name)
		}
		if _, ok := raw.(identity.Service); ok {
			t.Errorf("%s: nilai mentah memenuhi Service", name)
		}
		if _, ok := raw.(identity.Admin); ok {
			t.Errorf("%s: nilai mentah memenuhi Admin", name)
		}
		rv := reflect.ValueOf(raw)
		if rv.Kind() != reflect.Pointer || rv.Elem().Kind() != reflect.Struct {
			t.Fatalf("%s: kotak bukan pointer ke struct: %T", name, raw)
		}
		// Seluruh field kotak tak diekspor (properti desain; jangan diekspor tanpa sengaja).
		for i := 0; i < rv.Elem().NumField(); i++ {
			if rv.Elem().Type().Field(i).IsExported() {
				t.Errorf("%s: field kotak %q diekspor", name, rv.Elem().Type().Field(i).Name)
			}
		}
	}
}

func TestServiceFromRejectsForged(t *testing.T) {
	legit := identity.WithService(context.Background(), &fakeService{})
	key, valType := forgeKey(t, legit, "serviceKey")

	forgedValues := map[string]any{
		"implementasi Service mentah": &fakeService{},
		"kotak nol":                   zeroBox(valType),
	}
	for name, forged := range forgedValues {
		t.Run(name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), key, forged)
			if s, ok := identity.ServiceFrom(ctx); ok {
				t.Fatalf("Service palsu diterima: %v", s)
			}
			if s, ok := identity.ServiceFrom(context.WithValue(legit, key, forged)); ok {
				t.Fatalf("Service palsu di atas ctx sah diterima: %v", s)
			}
		})
	}
	if _, ok := identity.ServiceFrom(legit); !ok {
		t.Fatal("Service sah tidak terbaca")
	}
}

func TestAdminFromRejectsForged(t *testing.T) {
	legit := identity.WithAdmin(context.Background(), &fakeAdmin{})
	key, valType := forgeKey(t, legit, "adminKey")

	forgedValues := map[string]any{
		"implementasi Admin mentah": &fakeAdmin{},
		"kotak nol":                 zeroBox(valType),
	}
	for name, forged := range forgedValues {
		t.Run(name, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), key, forged)
			if a, ok := identity.AdminFrom(ctx); ok {
				t.Fatalf("Admin palsu diterima: %v", a)
			}
			if a, ok := identity.AdminFrom(context.WithValue(legit, key, forged)); ok {
				t.Fatalf("Admin palsu di atas ctx sah diterima: %v", a)
			}
		})
	}
	if _, ok := identity.AdminFrom(legit); !ok {
		t.Fatal("Admin sah tidak terbaca")
	}
}

// zeroBox membuat nilai nol bertipe nilai yang disimpan paket di context lewat reflect.New (field tak
// diekspor tetap nol, jadi penanda sah tidak terisi). Ini hanya menguji penolakan oleh penanda ok untuk
// pemalsuan murni-refleksi; BUKAN bukti kekebalan terhadap kode yang memakai unsafe (lihat
// TestKnownLimitationUnsafeReflectForgery).
func zeroBox(valType reflect.Type) any {
	if valType.Kind() == reflect.Pointer {
		return reflect.New(valType.Elem()).Interface()
	}
	return reflect.New(valType).Elem().Interface()
}

// TestKnownLimitationUnsafeReflectForgery MENDOKUMENTASIKAN batas yang diketahui, bukan jaminan keamanan:
// kode yang memperoleh unsafe.Pointer (di sini lewat reflect.Value.UnsafePointer, tanpa menyebut paket
// unsafe) dapat mengisi kotak nol lewat reflect.NewAt dan melewati penanda ok, serta memutasi kotak sah
// di tempat. Ini batas bahasa Go; mitigasinya aturan arsitektur IDENT (archcheck) yang menandai
// ekspresi unsafe.Pointer di kode modul. Tes ini sengaja menegaskan bahwa serangan BERHASIL; bila suatu
// saat Go menutupnya, tes ini melapor Skip dengan pesan agar dihapus (bukan regresi).
func TestKnownLimitationUnsafeReflectForgery(t *testing.T) {
	legit := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "u1"})
	key, valType := forgeKey(t, legit, "principalKey")

	// Layout sama dengan principalBox{p Principal; ok bool}, tetapi dengan field diekspor agar dapat di-Set.
	layout := reflect.StructOf([]reflect.StructField{
		{Name: "P", Type: reflect.TypeOf(identity.Principal{})},
		{Name: "Ok", Type: reflect.TypeOf(true)},
	})
	fill := func(box any, p identity.Principal) {
		w := reflect.NewAt(layout, reflect.ValueOf(box).UnsafePointer()).Elem()
		w.Field(0).Set(reflect.ValueOf(p))
		w.Field(1).SetBool(true)
	}

	// 1) Kotak nol palsu yang diisi lewat NewAt diterima FromContext.
	forged := zeroBox(valType)
	fill(forged, identity.Principal{UserID: "evil"})
	got, ok := identity.FromContext(context.WithValue(context.Background(), key, forged))
	if !ok || got.UserID != "evil" {
		t.Skipf("serangan NewAt tidak lagi berhasil (got=%+v ok=%v): Go menutup celah; hapus tes ini", got, ok)
	}

	// 2) Kotak sah dapat dimutasi di tempat sehingga principal berubah untuk semua yang berbagi ctx.
	shared := identity.WithPrincipal(context.Background(), identity.Principal{UserID: "victim"})
	fill(shared.Value(key), identity.Principal{UserID: "evil"})
	if p, _ := identity.FromContext(shared); p.UserID != "evil" {
		t.Skipf("mutasi in-place tidak lagi berhasil (p=%+v): hapus tes ini", p)
	}
}

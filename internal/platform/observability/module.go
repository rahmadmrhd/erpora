package observability

import "context"

type moduleKey struct{}

// WithModule menyimpan key modul pemilik request di context agar log lapisan
// bawah (mis. keputusan otorisasi) dapat memuat atribut module.
func WithModule(ctx context.Context, key string) context.Context {
	return context.WithValue(ctx, moduleKey{}, key)
}

// ModuleFromContext mengambil key modul dari context ("" bila tidak ada).
func ModuleFromContext(ctx context.Context) string {
	key, _ := ctx.Value(moduleKey{}).(string)
	return key
}

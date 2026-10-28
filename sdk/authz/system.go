package authz

import "context"

type systemKey struct{}

// AsSystem menandai ctx sebagai berjalan dengan hak sistem untuk alasan tertentu (mis. hook lifecycle
// atau command CLI). reason wajib diisi agar setiap pemakaian dapat diaudit; reason kosong panic
// karena itu kesalahan pemrograman. Pemakai (platform maupun internal/application modul) wajib
// mencatat pemakaian ini di log beserta reason.
func AsSystem(ctx context.Context, reason string) context.Context {
	if reason == "" {
		panic("authz: AsSystem membutuhkan reason")
	}
	return context.WithValue(ctx, systemKey{}, reason)
}

// SystemReason mengembalikan alasan hak sistem pada ctx; ok false bila ctx tidak berhak sistem.
func SystemReason(ctx context.Context) (reason string, ok bool) {
	r, ok := ctx.Value(systemKey{}).(string)
	return r, ok
}

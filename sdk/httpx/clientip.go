package httpx

import "context"

type clientIPKey struct{}

// WithClientIP memasang IP klien pada ctx. Dipanggil platform (httpserver) untuk setiap request dengan alamat peer
// TCP (r.RemoteAddr tanpa port); header X-Forwarded-For dan sejenisnya TIDAK dipercaya selama konfigurasi proxy
// tepercaya belum ada. Handler modul membacanya lewat ClientIP untuk mengisi metadata request (mis.
// identity.LoginInput.ClientIP) alih-alih membaca header sendiri, sehingga klien tidak dapat memalsukannya.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// ClientIP mengembalikan IP klien yang dipasang platform; "" bila tidak ada (mis. di luar rantai HTTP platform).
func ClientIP(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

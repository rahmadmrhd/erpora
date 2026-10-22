package observability

import "context"

type requestIDKey struct{}

// WithRequestID menyimpan request id di context agar TraceHandler menambahkannya ke setiap log.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestIDFromContext mengambil request id dari context.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(requestIDKey{}).(string)
	return id, ok && id != ""
}

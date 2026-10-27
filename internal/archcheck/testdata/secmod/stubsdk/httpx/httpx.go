// Package httpx adalah stub minimal SDK untuk fixture arch test.
package httpx

import "net/http"

// DecodeJSON meniru signature httpx.DecodeJSON asli.
func DecodeJSON[T any](w http.ResponseWriter, r *http.Request, opts ...any) (T, error) {
	var zero T
	return zero, nil
}

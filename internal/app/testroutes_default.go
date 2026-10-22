//go:build !integration

package app

import "erpora/internal/platform/httpserver"

// registerTestRoutes tidak mendaftarkan apa pun pada build normal (release/devtools).
func registerTestRoutes(*httpserver.Router) {}

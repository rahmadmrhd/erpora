// Package templates menyematkan template generator (text/template) developer tools. Hanya dikompilasi ke binary dev.
package templates

import "embed"

// FS berisi seluruh template; nama file berakhiran .tmpl dan path-nya dipakai sebagai nama template.
//
//go:embed all:migration all:module all:entity all:usecase all:contract all:command all:repository
var FS embed.FS

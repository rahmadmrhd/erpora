package config

import (
	"net/url"
	"strings"
)

// RedactDSN mengganti password pada DSN dengan "****".
func RedactDSN(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return "<invalid dsn>"
	}
	if u.User == nil {
		return u.String()
	}
	if _, has := u.User.Password(); !has {
		return u.String()
	}
	// url.UserPassword akan meng-escape "****"; susun manual agar terbaca.
	u.User = url.User(u.User.Username())
	name := u.User.String()
	return strings.Replace(u.String(), "//"+name+"@", "//"+name+":****@", 1)
}

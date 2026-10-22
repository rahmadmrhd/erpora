package config

import (
	"net/url"
	"strings"
)

const (
	redacted   = "****"
	invalidDSN = "<invalid dsn>"
)

// secretKeys adalah kunci DSN (huruf kecil) yang nilainya rahasia.
var secretKeys = map[string]bool{"password": true, "sslpassword": true}

// RedactDSN menyamarkan password pada DSN (bentuk URL postgres://, termasuk password di query,
// maupun bentuk keyword=value). DSN yang tidak dapat di-parse TIDAK pernah dikembalikan apa adanya.
func RedactDSN(dsn string) string {
	dsn = strings.TrimSpace(dsn)
	if isPostgresURL(dsn) {
		u, err := url.Parse(dsn)
		if err != nil {
			return invalidDSN
		}
		if u.User != nil {
			if _, has := u.User.Password(); has {
				// url.UserPassword akan meng-escape "****"; susun manual agar terbaca.
				u.User = url.User(u.User.Username())
				name := u.User.String()
				u.RawQuery = redactQuery(u.RawQuery)
				return strings.Replace(u.String(), "//"+name+"@", "//"+name+":"+redacted+"@", 1)
			}
		}
		u.RawQuery = redactQuery(u.RawQuery)
		return u.String()
	}
	toks, ok := parseKV(dsn)
	if !ok {
		return invalidDSN
	}
	out := dsn
	for i := len(toks) - 1; i >= 0; i-- {
		if secretKeys[strings.ToLower(toks[i].key)] {
			out = out[:toks[i].start] + redacted + out[toks[i].end:]
		}
	}
	return out
}

// DSNParts mengembalikan host[:port] (untuk pesan error) dan semua nilai rahasia yang ada di DSN.
// Host "?" bila DSN tidak dapat di-parse.
func DSNParts(dsn string) (host string, secrets []string) {
	dsn = strings.TrimSpace(dsn)
	add := func(s string) {
		if s == "" {
			return
		}
		for _, e := range secrets {
			if e == s {
				return
			}
		}
		secrets = append(secrets, s)
	}
	if isPostgresURL(dsn) {
		u, err := url.Parse(dsn)
		if err != nil {
			return "?", nil
		}
		host = u.Host
		if u.User != nil {
			if pw, has := u.User.Password(); has {
				add(pw)
				add(url.PathEscape(pw))
			}
		}
		for _, part := range strings.Split(u.RawQuery, "&") {
			k, v, _ := strings.Cut(part, "=")
			if ku, err := url.QueryUnescape(k); err == nil && secretKeys[strings.ToLower(ku)] {
				if vu, err := url.QueryUnescape(v); err == nil {
					add(vu)
				}
				add(v)
			}
		}
		if host == "" {
			host = "?"
		}
		return host, secrets
	}
	toks, ok := parseKV(dsn)
	if !ok {
		return "?", nil
	}
	var h, port string
	for _, t := range toks {
		switch k := strings.ToLower(t.key); {
		case secretKeys[k]:
			add(t.val)
		case k == "host" && h == "":
			h = t.val
		case k == "hostaddr" && h == "":
			h = t.val
		case k == "port":
			port = t.val
		}
	}
	if h == "" {
		return "?", secrets
	}
	if port != "" {
		h += ":" + port
	}
	return h, secrets
}

func isPostgresURL(s string) bool {
	l := strings.ToLower(s)
	return strings.HasPrefix(l, "postgres://") || strings.HasPrefix(l, "postgresql://")
}

func redactQuery(raw string) string {
	if raw == "" {
		return raw
	}
	parts := strings.Split(raw, "&")
	for i, p := range parts {
		k, _, has := strings.Cut(p, "=")
		if !has {
			continue
		}
		if ku, err := url.QueryUnescape(k); err == nil && secretKeys[strings.ToLower(ku)] {
			parts[i] = k + "=" + redacted
		}
	}
	return strings.Join(parts, "&")
}

type kvToken struct {
	key, val   string
	start, end int // rentang nilai mentah (termasuk kutip) di string asal
}

// parseKV mem-parse DSN bentuk "key=value key2='nilai berspasi'" (nilai boleh memakai \ sebagai escape).
func parseKV(s string) ([]kvToken, bool) {
	var toks []kvToken
	i := 0
	isSpace := func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
	for {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if i >= len(s) {
			break
		}
		ks := i
		for i < len(s) && s[i] != '=' && !isSpace(s[i]) {
			i++
		}
		key := s[ks:i]
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		if key == "" || i >= len(s) || s[i] != '=' {
			return nil, false
		}
		i++
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		start := i
		var val strings.Builder
		if i < len(s) && s[i] == '\'' {
			i++
			closed := false
			for i < len(s) {
				c := s[i]
				if c == '\\' && i+1 < len(s) {
					val.WriteByte(s[i+1])
					i += 2
					continue
				}
				if c == '\'' {
					i++
					closed = true
					break
				}
				val.WriteByte(c)
				i++
			}
			if !closed {
				return nil, false
			}
		} else {
			for i < len(s) && !isSpace(s[i]) {
				if s[i] == '\\' && i+1 < len(s) {
					val.WriteByte(s[i+1])
					i += 2
					continue
				}
				val.WriteByte(s[i])
				i++
			}
		}
		toks = append(toks, kvToken{key: key, val: val.String(), start: start, end: i})
	}
	if len(toks) == 0 {
		return nil, false
	}
	return toks, true
}

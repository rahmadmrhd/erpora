package reset

import (
	"bytes"
	"embed"
	"fmt"
	htmltemplate "html/template"
	"io/fs"
	"sort"
	"strings"
	texttemplate "text/template"

	sdkmail "erpora/sdk/mail"
)

//go:embed templates/*/*.tmpl
var templateFS embed.FS

// templateData adalah data templat email reset.
type templateData struct {
	AppName        string
	Name           string
	Link           string
	ExpiresMinutes int
}

type bundle struct {
	subject *texttemplate.Template
	text    *texttemplate.Template
	html    *htmltemplate.Template
}

type templates struct {
	byLocale map[string]bundle
	order    []string
}

func loadTemplates() (templates, error) {
	dirs, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return templates{}, fmt.Errorf("reset: baca templat: %w", err)
	}
	t := templates{byLocale: map[string]bundle{}}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		loc := d.Name()
		read := func(name string) (string, error) {
			b, err := fs.ReadFile(templateFS, "templates/"+loc+"/"+name)
			return string(b), err
		}
		sub, err := read("subject.txt.tmpl")
		if err != nil {
			return templates{}, fmt.Errorf("reset: templat %s: %w", loc, err)
		}
		txt, err := read("body.txt.tmpl")
		if err != nil {
			return templates{}, fmt.Errorf("reset: templat %s: %w", loc, err)
		}
		htm, err := read("body.html.tmpl")
		if err != nil {
			return templates{}, fmt.Errorf("reset: templat %s: %w", loc, err)
		}
		var b bundle
		if b.subject, err = texttemplate.New(loc + "/subject").Option("missingkey=error").Parse(sub); err != nil {
			return templates{}, fmt.Errorf("reset: templat %s subjek: %w", loc, err)
		}
		if b.text, err = texttemplate.New(loc + "/text").Option("missingkey=error").Parse(txt); err != nil {
			return templates{}, fmt.Errorf("reset: templat %s teks: %w", loc, err)
		}
		if b.html, err = htmltemplate.New(loc + "/html").Option("missingkey=error").Parse(htm); err != nil {
			return templates{}, fmt.Errorf("reset: templat %s html: %w", loc, err)
		}
		t.byLocale[loc] = b
		t.order = append(t.order, loc)
	}
	if _, ok := t.byLocale[defaultLocale]; !ok {
		return templates{}, fmt.Errorf("reset: templat locale bawaan %q tidak ada", defaultLocale)
	}
	sort.Strings(t.order)
	return t, nil
}

// SupportedLocales mengembalikan kode locale yang punya templat email reset (urut abjad).
func SupportedLocales() []string {
	t, err := loadTemplates()
	if err != nil {
		return nil
	}
	return append([]string(nil), t.order...)
}

// pickLocale memilih locale email: preferensi pengguna, lalu parameter (mis. Accept-Language), lalu bawaan "id".
// Hanya locale yang punya templat diterima; nilai lain (termasuk yang jahat) jatuh ke berikutnya. Nilai tidak pernah
// dipakai sebagai jalur berkas: ia hanya kunci peta.
func pickLocale(pref, param string) string {
	for _, c := range []string{pref, param} {
		if l := normalizeLocale(c); l != "" {
			if _, ok := supported[l]; ok {
				return l
			}
		}
	}
	return defaultLocale
}

// supported adalah himpunan locale bawaan yang dibangun dari templat tertanam saat inisialisasi paket.
var supported = func() map[string]struct{} {
	m := map[string]struct{}{}
	for _, l := range SupportedLocales() {
		m[l] = struct{}{}
	}
	return m
}()

// normalizeLocale mengubah "EN-us" atau "en_US" menjadi "en"; masukan kosong atau terlalu panjang menjadi "".
func normalizeLocale(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 35 {
		return ""
	}
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		s = s[:i]
	}
	return s
}

// render menghasilkan Message (tanpa To) untuk locale yang didukung.
func (t templates) render(locale string, d templateData) (sdkmail.Message, error) {
	b, ok := t.byLocale[locale]
	if !ok {
		b = t.byLocale[defaultLocale]
	}
	var sub, txt, htm bytes.Buffer
	if err := b.subject.Execute(&sub, d); err != nil {
		return sdkmail.Message{}, err
	}
	if err := b.text.Execute(&txt, d); err != nil {
		return sdkmail.Message{}, err
	}
	if err := b.html.Execute(&htm, d); err != nil {
		return sdkmail.Message{}, err
	}
	return sdkmail.Message{Subject: strings.TrimSpace(sub.String()), Text: txt.String(), HTML: htm.String()}, nil
}

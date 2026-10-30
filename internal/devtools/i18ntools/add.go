package i18ntools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"erpora/internal/platform/i18n"
)

var localeCodePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

// AddOptions mengonfigurasi Add.
type AddOptions struct {
	// Locale adalah kode bahasa baru (pola registri, mis. "ms" atau "en-GB").
	Locale string
	// Name adalah nama asli bahasa untuk registri bila bahasa belum terdaftar (default: kode).
	Name string
	// Fallback adalah bahasa fallback bila bahasa belum terdaftar (default: bahasa default registri).
	Fallback string
	// Modules membatasi namespace: "ui", "platform", atau modul (direktori atau slug). Kosong = semua
	// namespace (platform, ui, dan setiap modul yang punya katalog).
	Modules []string
}

// AddResult merangkum perubahan Add; semua path relatif root repo dengan pemisah "/".
type AddResult struct {
	RegistryUpdated  bool
	Created          []string
	Skipped          []string // berkas katalog yang sudah ada (tidak ditimpa)
	ManifestsUpdated []string
}

// Add membuat kerangka katalog bahasa baru dari bahasa default tiap namespace terpilih (Requirement 8.8):
// struktur dan urutan kunci sama, nilai diberi awalan TodoPrefix agar mudah dicari dan dihitung belum
// diterjemahkan oleh status. Bahasa ditambahkan ke registri bila belum ada, dan ke locales manifest modul
// yang mendeklarasikan i18n. Idempoten: berkas yang sudah ada tidak ditimpa dan registri/manifest tidak
// ditulis ulang bila isinya sudah memuat bahasa itu. Semua validasi dilakukan sebelum menulis apa pun.
func Add(root string, opts AddOptions) (*AddResult, error) {
	if !localeCodePattern.MatchString(opts.Locale) {
		return nil, &ValidationError{Msg: fmt.Sprintf("kode bahasa %q tidak sesuai pola %s", opts.Locale, localeCodePattern)}
	}
	rep := &Report{}
	ws, err := load(root, rep)
	if err != nil {
		return nil, err
	}
	if ws.reg == nil {
		return nil, &ValidationError{Msg: "registri bahasa tidak valid; jalankan i18n:check"}
	}
	targets, err := selectNamespaces(ws, opts.Modules)
	if err != nil {
		return nil, err
	}

	// Bahasa baru: validasi fallback dan susun entri registri sebelum ada tulis.
	var registryText []byte
	registered := ws.reg.Has(opts.Locale)
	if !registered {
		fb := opts.Fallback
		if fb == "" {
			fb = ws.reg.Default
		}
		if !ws.reg.Has(fb) {
			return nil, &ValidationError{Msg: fmt.Sprintf("fallback %q tidak terdaftar di registri bahasa", fb)}
		}
		name := opts.Name
		if name == "" {
			name = opts.Locale
		}
		registryText, err = appendRegistryEntry(root, opts.Locale, name, fb)
		if err != nil {
			return nil, err
		}
	}

	// Bangun semua isi lebih dulu (kegagalan membaca bahasa default membatalkan seluruh operasi).
	type write struct {
		rel     string
		content []byte
		created bool // berkas katalog baru
	}
	var writes []write
	res := &AddResult{}
	for _, n := range targets {
		rel := relSlash(n.Dir, opts.Locale+".json")
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			res.Skipped = append(res.Skipped, rel)
		} else {
			base := n.Catalogs[n.Default]
			if base == nil || !base.OK {
				return nil, &ValidationError{Msg: fmt.Sprintf("katalog bahasa default namespace %q (%s) tidak valid atau tidak ada; jalankan i18n:check", n.Name, relSlash(n.Dir, n.Default+".json"))}
			}
			src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(base.File))) //nolint:gosec // path di root repo
			if err != nil {
				return nil, fmt.Errorf("baca %s: %w", base.File, err)
			}
			skel, err := skeleton(src)
			if err != nil {
				return nil, fmt.Errorf("kerangka dari %s: %w", base.File, err)
			}
			writes = append(writes, write{rel: rel, content: skel, created: true})
		}
		if n.Kind == kindModule {
			mf, changed, err := addLocaleToManifest(root, n.ModuleDir, opts.Locale)
			if err != nil {
				return nil, err
			}
			if changed {
				writes = append(writes, write{rel: relSlash(n.ModuleDir, "module.json"), content: mf})
				res.ManifestsUpdated = append(res.ManifestsUpdated, relSlash(n.ModuleDir, "module.json"))
			}
		}
	}
	if registryText != nil {
		writes = append([]write{{rel: RegistryPath, content: registryText}}, writes...)
		res.RegistryUpdated = true
	}
	for _, w := range writes {
		p := filepath.Join(root, filepath.FromSlash(w.rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return nil, fmt.Errorf("buat direktori %s: %w", filepath.Dir(w.rel), err)
		}
		if err := os.WriteFile(p, w.content, 0o600); err != nil {
			return nil, fmt.Errorf("tulis %s: %w", w.rel, err)
		}
		if w.created {
			res.Created = append(res.Created, w.rel)
		}
	}
	return res, nil
}

// selectNamespaces memilih namespace menurut argumen (kosong = semua).
func selectNamespaces(ws *workspace, names []string) ([]*loadedNS, error) {
	if len(names) == 0 {
		return ws.ns, nil
	}
	var out []*loadedNS
	for _, name := range names {
		var hit *loadedNS
		for _, n := range ws.ns {
			if n.Name == name || (n.ModuleDir != "" && path0(n.ModuleDir) == name) {
				hit = n
				break
			}
		}
		if hit == nil {
			var known []string
			for _, n := range ws.ns {
				known = append(known, n.Name)
			}
			return nil, &ValidationError{Msg: fmt.Sprintf("namespace atau modul %q tidak dikenal atau tidak punya katalog i18n; yang ada: %s", name, strings.Join(known, ", "))}
		}
		if !slices.Contains(out, hit) {
			out = append(out, hit)
		}
	}
	return out, nil
}

func path0(moduleDir string) string { return moduleDir[strings.LastIndex(moduleDir, "/")+1:] }

// appendRegistryEntry menyisipkan entri bahasa ke akhir array "locales" secara tekstual agar format berkas
// registri tidak berubah, lalu memastikan hasilnya lolos ParseRegistry.
func appendRegistryEntry(root, code, name, fallback string) ([]byte, error) {
	p := filepath.Join(root, filepath.FromSlash(RegistryPath))
	src, err := os.ReadFile(p) //nolint:gosec // path tetap di root repo
	if err != nil {
		return nil, configErr("registri bahasa %s tidak dapat dibaca: %v", RegistryPath, err)
	}
	entry := fmt.Sprintf(`{ "code": %s, "name": %s, "fallback": %s }`, jsonString(code), jsonString(name), jsonString(fallback))
	out, err := appendToArray(src, []string{"locales"}, entry)
	if err != nil {
		return nil, fmt.Errorf("sisipkan ke %s: %w", RegistryPath, err)
	}
	if _, err := i18n.ParseRegistry(out); err != nil {
		return nil, &ValidationError{Msg: fmt.Sprintf("registri hasil penambahan %q tidak valid: %v", code, err)}
	}
	return out, nil
}

// addLocaleToManifest menambahkan bahasa ke i18n.locales module.json bila modul mendeklarasikan i18n dan
// belum memuatnya. Modul tanpa bagian i18n tidak diubah (module:check yang melaporkannya).
func addLocaleToManifest(root, moduleDir, locale string) (out []byte, changed bool, err error) {
	p := filepath.Join(root, filepath.FromSlash(moduleDir), "module.json")
	src, err := os.ReadFile(p) //nolint:gosec // path dari manifest yang sudah dibaca
	if err != nil {
		return nil, false, fmt.Errorf("baca %s/module.json: %w", moduleDir, err)
	}
	var m struct {
		I18n *struct {
			Locales []string `json:"locales"`
		} `json:"i18n"`
	}
	if err := json.Unmarshal(src, &m); err != nil {
		return nil, false, fmt.Errorf("%s/module.json: %w", moduleDir, err)
	}
	if m.I18n == nil || slices.Contains(m.I18n.Locales, locale) {
		return nil, false, nil
	}
	out, err = appendToArray(src, []string{"i18n", "locales"}, jsonString(locale))
	if err != nil {
		return nil, false, fmt.Errorf("%s/module.json: %w", moduleDir, err)
	}
	return out, true, nil
}

// ---- edit JSON tekstual ----

type member struct {
	key              string
	valStart, valEnd int
}

// scanObject membaca anggota objek JSON teratas dan posisi bytenya.
func scanObject(src []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(src))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("JSON tidak valid: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("JSON harus berupa objek")
	}
	var out []member
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("JSON tidak valid: %w", err)
		}
		key, ok := kt.(string)
		if !ok {
			return nil, errors.New("JSON tidak valid: kunci bukan string")
		}
		valStart := int(dec.InputOffset())
		for valStart < len(src) && strings.IndexByte(": \t\r\n", src[valStart]) >= 0 {
			valStart++
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("JSON tidak valid: %w", err)
		}
		out = append(out, member{key: key, valStart: valStart, valEnd: int(dec.InputOffset())})
	}
	return out, nil
}

// appendToArray menambahkan elemen (teks JSON) di akhir array pada jalur objek bersarang path, mempertahankan
// format sekitarnya: array satu baris diberi pemisah yang sama dengan elemen sebelumnya, array banyak baris
// memakai indentasi elemen terakhir.
func appendToArray(src []byte, path []string, elem string) ([]byte, error) {
	lo, hi := 0, len(src)
	for _, seg := range path {
		ms, err := scanObject(src[lo:hi])
		if err != nil {
			return nil, err
		}
		var found *member
		for i := range ms {
			if ms[i].key == seg {
				found = &ms[i]
			}
		}
		if found == nil {
			return nil, fmt.Errorf("anggota %q tidak ada", seg)
		}
		lo, hi = lo+found.valStart, lo+found.valEnd
	}
	arr := src[lo:hi]
	if len(arr) < 2 || arr[0] != '[' || arr[len(arr)-1] != ']' {
		return nil, errors.New("nilai bukan array")
	}
	closePos := hi - 1
	body := bytes.TrimRight(src[lo:closePos], " \t\r\n")
	empty := len(bytes.TrimSpace(arr[1:len(arr)-1])) == 0
	var ins string
	switch {
	case empty:
		ins = elem
	case !bytes.Contains(arr, []byte("\n")):
		sep := ","
		if bytes.Contains(arr, []byte(", ")) {
			sep = ", "
		}
		ins = sep + elem
	default:
		lineStart := bytes.LastIndexByte(body, '\n') + 1
		indent := leadingSpace(body[lineStart:])
		closeStart := bytes.LastIndexByte(src[:closePos], '\n') + 1
		closeIndent := leadingSpace(src[closeStart:closePos])
		ins = ",\n" + indent + elem + "\n" + closeIndent
		var buf bytes.Buffer
		buf.Write(src[:lo+len(body)])
		buf.WriteString(ins)
		buf.Write(src[closePos:])
		return buf.Bytes(), nil
	}
	var buf bytes.Buffer
	buf.Write(src[:lo+len(body)])
	buf.WriteString(ins)
	buf.Write(src[closePos:])
	return buf.Bytes(), nil
}

func leadingSpace(b []byte) string {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	return string(b[:i])
}

func jsonString(s string) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

// ---- kerangka katalog ----

// skeleton membuat salinan katalog src dengan struktur dan urutan kunci yang sama, nilai diberi TodoPrefix,
// indentasi dua spasi dan newline di akhir. src harus sudah lolos ParseCatalog.
func skeleton(src []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimPrefix(src, []byte{0xEF, 0xBB, 0xBF})))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("akar katalog harus berupa objek")
	}
	var buf bytes.Buffer
	if err := skeletonObject(dec, &buf, 0); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// skeletonObject menulis objek yang token '{'-nya sudah dibaca, sampai dan termasuk '}'.
func skeletonObject(dec *json.Decoder, buf *bytes.Buffer, depth int) error {
	buf.WriteByte('{')
	first := true
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := kt.(string)
		if !ok {
			return errors.New("kunci bukan string")
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.WriteString("\n" + strings.Repeat("  ", depth+1) + jsonString(key) + ": ")
		vt, err := dec.Token()
		if err != nil {
			return err
		}
		switch v := vt.(type) {
		case string:
			buf.WriteString(jsonString(TodoPrefix + v))
		case json.Delim:
			if v != '{' {
				return errors.New("nilai harus string atau objek")
			}
			if err := skeletonObject(dec, buf, depth+1); err != nil {
				return err
			}
		default:
			return errors.New("nilai harus string atau objek")
		}
	}
	if _, err := dec.Token(); err != nil { // '}'
		return err
	}
	if !first {
		buf.WriteString("\n" + strings.Repeat("  ", depth))
	}
	buf.WriteByte('}')
	return nil
}

package i18ntools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"erpora/internal/platform/i18n"
	sdki18n "erpora/sdk/i18n"
	"erpora/sdk/module"
)

var namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// Jenis namespace.
const (
	kindPlatform = "platform"
	kindUI       = "ui"
	kindModule   = "module"
)

// namespaceInfo menjelaskan satu namespace katalog dan di mana katalognya berada.
type namespaceInfo struct {
	Name string
	Kind string
	// Dir adalah direktori katalog relatif root repo (pemisah "/").
	Dir string
	// ModuleDir adalah "modules/<dir>" untuk modul; kosong untuk platform dan ui.
	ModuleDir string
	// Locales adalah bahasa yang diperiksa, urutan deklarasi/registri.
	Locales []string
	// Default adalah bahasa dasar kunci namespace ini.
	Default string
}

// catalog adalah hasil memuat satu berkas katalog.
type catalog struct {
	Locale string
	File   string // relatif root repo, "/"
	Exists bool
	Flat   sdki18n.Flat // nil bila berkas tidak ada atau tidak valid
	OK     bool         // berkas ada dan lolos ParseCatalog
}

// loadedNS adalah satu namespace dengan katalog-katalognya.
type loadedNS struct {
	namespaceInfo
	Catalogs map[string]*catalog
}

// workspace adalah hasil memuat seluruh katalog repo. reg nil berarti registri tidak valid dan tidak ada
// namespace yang dimuat (temuan sudah dicatat di laporan).
type workspace struct {
	root string
	reg  *i18n.Registry
	ns   []*loadedNS
}

func relSlash(parts ...string) string { return path.Join(parts...) }

// load memuat registri, manifest modul, dan semua katalog dari filesystem di root, mencatat temuan
// struktural ke rep. Kegagalan prasyarat dikembalikan sebagai *ConfigError.
func load(root string, rep *Report) (*workspace, error) {
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return nil, configErr("root repo %q tidak ditemukan atau bukan direktori", root)
	}
	ws := &workspace{root: root}

	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(RegistryPath))) //nolint:gosec // path tetap di root repo
	if err != nil {
		return nil, configErr("registri bahasa %s tidak dapat dibaca: %v", RegistryPath, err)
	}
	reg, err := i18n.ParseRegistry(data)
	if err != nil {
		rep.add(Finding{Severity: SeverityError, Code: CodeInvalidRegistry, File: RegistryPath, Detail: err.Error()})
		return ws, nil
	}
	ws.reg = reg

	infos, err := discover(root, reg, rep)
	if err != nil {
		return nil, err
	}
	for _, info := range infos {
		l := &loadedNS{namespaceInfo: info, Catalogs: map[string]*catalog{}}
		for _, loc := range info.Locales {
			l.Catalogs[loc] = loadCatalog(root, reg, info, loc, rep)
		}
		ws.ns = append(ws.ns, l)
	}
	return ws, nil
}

// discover menentukan namespace: platform, ui, lalu modul yang mendeklarasikan bagian i18n di manifest.
func discover(root string, reg *i18n.Registry, rep *Report) ([]namespaceInfo, error) {
	var out []namespaceInfo
	for _, b := range []struct{ name, kind, dir string }{
		{sdki18n.NamespacePlatform, kindPlatform, PlatformCatalogDir},
		{sdki18n.NamespaceUI, kindUI, UICatalogDir},
	} {
		info := namespaceInfo{Name: b.name, Kind: b.kind, Dir: b.dir, Locales: reg.Codes(), Default: reg.Default}
		out = append(out, info)
		warnUndeclaredFiles(root, info, rep)
	}

	manifests, err := filepath.Glob(filepath.Join(root, "modules", "*", "module.json"))
	if err != nil {
		return nil, configErr("glob manifest modul: %v", err)
	}
	sort.Strings(manifests)
	for _, mf := range manifests {
		dirName := filepath.Base(filepath.Dir(mf))
		modDir := relSlash("modules", dirName)
		raw, err := os.ReadFile(mf) //nolint:gosec // path dari glob di root repo
		if err != nil {
			return nil, configErr("baca %s/module.json: %v", modDir, err)
		}
		m, err := module.ParseManifest(raw)
		if err != nil {
			return nil, configErr("%s/module.json: %v", modDir, err)
		}
		catDir := relSlash(modDir, "i18n")
		if m.I18n == nil {
			if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(catDir))); err == nil && st.IsDir() {
				rep.add(Finding{Severity: SeverityWarning, Code: CodeUndeclaredI18n, Namespace: m.Slug, File: catDir,
					Detail: "modul membawa direktori i18n/ tetapi manifest tidak punya bagian \"i18n\"; katalognya tidak diperiksa"})
			}
			continue
		}
		manifestFile := relSlash(modDir, "module.json")
		if m.Slug == sdki18n.NamespaceUI || m.Slug == sdki18n.NamespacePlatform || !namespacePattern.MatchString(m.Slug) {
			rep.add(Finding{Severity: SeverityError, Code: CodeInvalidNamespace, Namespace: m.Slug, File: manifestFile,
				Detail: fmt.Sprintf("slug %q tidak sah sebagai namespace i18n (pola %s; ui dan platform tercadang)", m.Slug, namespacePattern)})
			continue
		}
		info := namespaceInfo{Name: m.Slug, Kind: kindModule, Dir: catDir, ModuleDir: modDir, Default: m.I18n.DefaultLocale}
		if !slices.Contains(m.I18n.Locales, m.I18n.DefaultLocale) {
			rep.add(Finding{Severity: SeverityError, Code: CodeInvalidManifest, Namespace: m.Slug, File: manifestFile,
				Detail: fmt.Sprintf("defaultLocale %q tidak ada di locales %v", m.I18n.DefaultLocale, m.I18n.Locales)})
			info.Default = ""
		}
		for _, loc := range m.I18n.Locales {
			if !reg.Has(loc) {
				rep.add(Finding{Severity: SeverityError, Code: CodeUnregisteredLocale, Namespace: m.Slug, Locale: loc, File: manifestFile,
					Detail: fmt.Sprintf("bahasa %q tidak terdaftar di registri bahasa (%s)", loc, RegistryPath)})
				if loc == info.Default {
					info.Default = ""
				}
				continue
			}
			info.Locales = append(info.Locales, loc)
		}
		out = append(out, info)
		warnUndeclaredFiles(root, info, rep)
	}
	return out, nil
}

// warnUndeclaredFiles melaporkan berkas <bahasa>.json di direktori katalog yang tidak termasuk bahasa namespace.
func warnUndeclaredFiles(root string, info namespaceInfo, rep *Report) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(info.Dir)))
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		loc := strings.TrimSuffix(name, ".json")
		if slices.Contains(info.Locales, loc) {
			continue
		}
		rep.add(Finding{Severity: SeverityWarning, Code: CodeUndeclaredFile, Namespace: info.Name, Locale: loc,
			File: relSlash(info.Dir, name), Detail: "berkas katalog bahasa yang tidak terdaftar/dideklarasikan untuk namespace ini; diabaikan"})
	}
}

// loadCatalog membaca dan mem-parse satu berkas katalog, mencatat temuan.
func loadCatalog(root string, reg *i18n.Registry, info namespaceInfo, loc string, rep *Report) *catalog {
	file := relSlash(info.Dir, loc+".json")
	c := &catalog{Locale: loc, File: file}
	f, err := os.Open(filepath.Join(root, filepath.FromSlash(file))) //nolint:gosec // path dari registri/manifest tervalidasi
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if sev, ok := missingFileSeverity(reg, info, loc); ok {
				rep.add(Finding{Severity: sev, Code: CodeMissingFile, Namespace: info.Name, Locale: loc, File: file,
					Detail: "berkas katalog tidak ditemukan"})
			}
			return c
		}
		rep.add(Finding{Severity: SeverityError, Code: sdki18n.CodeInvalidJSON, Namespace: info.Name, Locale: loc, File: file, Detail: "gagal membuka berkas: " + err.Error()})
		return c
	}
	defer func() { _ = f.Close() }()
	c.Exists = true
	flat, issues := sdki18n.ParseCatalog(f, sdki18n.DefaultLimits())
	if len(issues) > 0 {
		for _, is := range issues {
			code := is.Code
			if code == sdki18n.CodeInvalidSegment && strings.Contains(is.Key, "::") {
				code = CodeInvalidNamespace
			}
			rep.add(Finding{Severity: SeverityError, Code: code, Namespace: info.Name, Locale: loc, File: file, Key: is.Key, Detail: is.Detail})
		}
		return c
	}
	c.Flat, c.OK = flat, true
	return c
}

// missingFileSeverity: berkas bahasa default selalu galat; modul wajib menyediakan bahasa yang dideklarasikannya
// (galat); platform/ui pada bahasa terdaftar non-default hanya peringatan, dan tidak dilaporkan untuk bahasa partial.
func missingFileSeverity(reg *i18n.Registry, info namespaceInfo, loc string) (Severity, bool) {
	if loc == info.Default || info.Kind == kindModule {
		return SeverityError, true
	}
	if li, ok := reg.Info(loc); ok && li.Partial {
		return "", false
	}
	return SeverityWarning, true
}

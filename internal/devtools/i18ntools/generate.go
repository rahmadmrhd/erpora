package i18ntools

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	sdki18n "erpora/sdk/i18n"
)

// Target berkas hasil generate (relatif root repo). Modul hanya dibangkitkan bila punya direktori web/.
const (
	// platformKeysPath memuat augmentasi namespace yang dimiliki platform-web: "platform" dan "ui".
	platformKeysPath = "packages/platform-web/src/i18n/keys.platform.gen.ts"
	// uiKeysPath memuat tipe kunci namespace "ui" untuk paket @erpora/ui sendiri (tanpa augmentasi).
	uiKeysPath       = "packages/ui/src/i18n/keys.gen.ts"
	moduleKeysSuffix = "web/src/i18n/keys.gen.ts"

	// maxLine adalah lebar baris Biome (biome.json lineWidth).
	maxLine = 100

	richTagType = "RichTag"
	// uiRichType: @erpora/ui adalah daun (F4) dan tidak mengimpor paket repo lain, sehingga tag rich memakai
	// tipe fungsi yang sama secara struktural dengan RichTag.
	uiRichType = "(chunks: ReactNode) => ReactNode"
)

// GenFile adalah satu berkas hasil generate; Path relatif root repo dengan "/".
type GenFile struct {
	Path    string
	Content []byte
}

var identPattern = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

// nsKeys adalah bentuk argumen seluruh kunci satu namespace (terurut menurut kunci).
type nsKeys struct {
	name      string
	source    string // berkas katalog bahasa default, relatif root repo
	entries   []entry
	needsRich bool
}

type entry struct {
	key   string
	props []prop
}

// prop adalah satu properti bentuk argumen; alts terisi untuk gabungan literal (select) agar dapat dipecah.
type prop struct {
	name, typ string
	alts      []string
}

// Generate membangkitkan, tanpa menulis, berkas kunci terketik (Requirement 7.1) dari katalog bahasa default
// tiap namespace. Nilai tiap kunci adalah BENTUK ARGUMEN: {} tanpa argumen, string | number untuk {x}, number
// untuk plural, gabungan literal untuk select, dan tag rich bertipe RichTag.
//
//   - modules/<dir>/web/src/i18n/keys.gen.ts (modul ber-web/): augmentasi I18nCatalogs milik "@erpora/platform-web".
//   - packages/platform-web/src/i18n/keys.platform.gen.ts: augmentasi namespace "platform" dan "ui" lewat
//     modul relatif "./types" (paket tidak dapat me-resolve namanya sendiri).
//   - packages/ui/src/i18n/keys.gen.ts: UiCatalog dan UiKey (tanpa augmentasi; @erpora/ui tidak boleh
//     mengimpor @erpora/platform-web, F4).
//
// Katalog harus bebas galat struktural (*ValidationError bila tidak). Hasil deterministik (kunci terurut),
// terurut menurut Path, dan sudah dalam format Biome.
func Generate(root string) ([]GenFile, error) {
	rep := &Report{}
	ws, err := load(root, rep)
	if err != nil {
		return nil, err
	}
	rep.finalize(false)
	if ws.reg == nil || rep.Failed() {
		return nil, &ValidationError{Msg: fmt.Sprintf("katalog memiliki %d galat; jalankan i18n:check untuk rincian", rep.Errors)}
	}
	var files []GenFile
	var platformNS []nsKeys
	for _, n := range ws.ns {
		base := n.Catalogs[n.Default]
		if base == nil || !base.OK {
			continue
		}
		rich := richTagType
		if n.Kind == kindUI {
			rich = uiRichType
		}
		keys, err := collect(n.Name, base.File, base.Flat, rich)
		if err != nil {
			return nil, fmt.Errorf("namespace %s: %w", n.Name, err)
		}
		switch n.Kind {
		case kindPlatform:
			platformNS = append(platformNS, keys)
		case kindUI:
			// ui dua kali: tipe kunci untuk paket ui sendiri dan augmentasi di platform-web (tag rich RichTag).
			files = append(files, GenFile{Path: uiKeysPath, Content: renderStandalone(keys)})
			pk, err := collect(n.Name, base.File, base.Flat, richTagType)
			if err != nil {
				return nil, fmt.Errorf("namespace %s: %w", n.Name, err)
			}
			platformNS = append(platformNS, pk)
		default:
			if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(n.ModuleDir), "web")); err != nil || !st.IsDir() {
				continue
			}
			files = append(files, GenFile{
				Path:    relSlash(n.ModuleDir, moduleKeysSuffix),
				Content: renderAugmentation("@erpora/platform-web", `import type { RichTag } from "@erpora/platform-web";`, []nsKeys{keys}),
			})
		}
	}
	if len(platformNS) > 0 {
		sort.SliceStable(platformNS, func(i, j int) bool { return platformNS[i].name == "platform" && platformNS[j].name != "platform" })
		files = append(files, GenFile{
			Path:    platformKeysPath,
			Content: renderAugmentation("./types", `import type { RichTag } from "./types";`, platformNS),
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// collect menurunkan bentuk argumen setiap kunci dari katalog bahasa default.
func collect(ns, source string, flat sdki18n.Flat, richType string) (nsKeys, error) {
	keys := make([]string, 0, len(flat))
	for k := range flat {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := nsKeys{name: ns, source: source}
	for _, k := range keys {
		msg, err := sdki18n.Compile(flat[k], sdki18n.DefaultLimits())
		if err != nil {
			return nsKeys{}, fmt.Errorf("kunci %q: %w", k, err)
		}
		var ps []prop
		for _, p := range msg.Params() {
			ps = append(ps, paramProp(p, richType))
			if p.Kind == sdki18n.KindTag {
				out.needsRich = true
			}
		}
		out.entries = append(out.entries, entry{key: k, props: ps})
	}
	return out, nil
}

func header(nss []nsKeys) string {
	srcs := make([]string, len(nss))
	for i, n := range nss {
		srcs[i] = n.source
	}
	return fmt.Sprintf("// Dibangkitkan oleh `erpora i18n:generate` dari %s. JANGAN DIEDIT.\n", strings.Join(srcs, " dan "))
}

// renderAugmentation menulis berkas yang memperluas I18nCatalogs pada modul module dengan namespace nss.
func renderAugmentation(module, richImport string, nss []nsKeys) []byte {
	var b bytes.Buffer
	b.WriteString(header(nss))
	needsRich := false
	for _, n := range nss {
		needsRich = needsRich || n.needsRich
	}
	if needsRich {
		b.WriteString(richImport + "\n")
	} else {
		b.WriteString("export {};\n")
	}
	fmt.Fprintf(&b, "\ndeclare module %s {\n  interface I18nCatalogs {\n", jsonString(module))
	for _, n := range nss {
		fmt.Fprintf(&b, "    %s: {\n", propKey(n.name, identPattern.MatchString(n.name)))
		bare := allBare(n.entries)
		for _, e := range n.entries {
			b.WriteString(renderEntry(6, propKey(e.key, bare), e.props))
		}
		b.WriteString("    };\n")
	}
	b.WriteString("  }\n}\n")
	return b.Bytes()
}

// renderStandalone menulis UiCatalog dan UiKey (paket ui tidak mengaugmentasi platform-web).
func renderStandalone(n nsKeys) []byte {
	var b bytes.Buffer
	b.WriteString(header([]nsKeys{n}))
	b.WriteString("// Paket ui adalah daun (F4) dan tidak memperluas I18nCatalogs; augmentasi namespace \"ui\" ada di\n")
	b.WriteString("// packages/platform-web/src/i18n/keys.platform.gen.ts.\n")
	if n.needsRich {
		b.WriteString("import type { ReactNode } from \"react\";\n")
	}
	b.WriteString("\nexport interface UiCatalog {\n")
	bare := allBare(n.entries)
	for _, e := range n.entries {
		b.WriteString(renderEntry(2, propKey(e.key, bare), e.props))
	}
	b.WriteString("}\n\n/** Kunci lengkap namespace ui. */\nexport type UiKey = keyof UiCatalog;\n")
	return b.Bytes()
}

func allBare(entries []entry) bool {
	for _, e := range entries {
		if !identPattern.MatchString(e.key) {
			return false
		}
	}
	return true
}

// propKey menulis kunci properti: tanpa kutip bila semua kunci pada objeknya pengenal sah (seperti Biome
// quoteProperties=asNeeded), selain itu dikutip.
func propKey(k string, bare bool) string {
	if bare && identPattern.MatchString(k) {
		return k
	}
	return jsonString(k)
}

func paramProp(p sdki18n.Param, richType string) prop {
	switch p.Kind {
	case sdki18n.KindPlural:
		return prop{name: p.Name, typ: "number"}
	case sdki18n.KindSelect:
		lits := make([]string, len(p.Options))
		for i, o := range p.Options {
			lits[i] = jsonString(o)
		}
		return prop{name: p.Name, typ: strings.Join(lits, " | "), alts: lits}
	case sdki18n.KindTag:
		return prop{name: p.Name, typ: richType}
	default:
		return prop{name: p.Name, typ: "string | number"}
	}
}

// renderEntry menulis satu kunci pada indentasi indent, dipecah seperti Biome bila melebihi lebar baris.
func renderEntry(indent int, key string, props []prop) string {
	pad := strings.Repeat(" ", indent)
	if len(props) == 0 {
		return pad + key + ": {};\n"
	}
	allIdent := true
	for _, p := range props {
		if !identPattern.MatchString(p.name) {
			allIdent = false
			break
		}
	}
	parts := make([]string, len(props))
	for i, p := range props {
		parts[i] = propKey(p.name, allIdent) + ": " + p.typ
	}
	line := pad + key + ": { " + strings.Join(parts, "; ") + " };"
	if len([]rune(line)) <= maxLine {
		return line + "\n"
	}
	var b strings.Builder
	b.WriteString(pad + key + ": {\n")
	inner := strings.Repeat(" ", indent+2)
	for _, p := range props {
		name := propKey(p.name, allIdent)
		pl := inner + name + ": " + p.typ + ";"
		if len([]rune(pl)) <= maxLine || len(p.alts) < 2 {
			b.WriteString(pl + "\n")
			continue
		}
		b.WriteString(inner + name + ":\n")
		for j, alt := range p.alts {
			b.WriteString(inner + "  | " + alt)
			if j == len(p.alts)-1 {
				b.WriteString(";")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString(pad + "};\n")
	return b.String()
}

// WriteGenerated menulis berkas hasil Generate (LF).
func WriteGenerated(root string, files []GenFile) error {
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			return err
		}
		if err := os.WriteFile(p, bytes.ReplaceAll(f.Content, []byte("\r\n"), []byte("\n")), 0o644); err != nil { //nolint:gosec // berkas sumber repo, bukan rahasia
			return err
		}
	}
	return nil
}

// CheckGenerated mengembalikan path (terurut) yang isinya berbeda atau belum ada di disk, ditambah
// modules/*/web/src/i18n/keys.gen.ts yatim (modul tidak lagi punya katalog). Perbedaan CRLF/LF diabaikan.
func CheckGenerated(root string, files []GenFile) ([]string, error) {
	want := map[string]bool{}
	var stale []string
	for _, f := range files {
		want[f.Path] = true
		disk, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path))) //nolint:gosec // path dari daftar generate
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		if err != nil || !bytes.Equal(bytes.ReplaceAll(disk, []byte("\r\n"), []byte("\n")), f.Content) {
			stale = append(stale, f.Path)
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, "modules", "*", filepath.FromSlash(moduleKeysSuffix)))
	if err != nil {
		return nil, err
	}
	for _, m := range matches {
		rel, err := filepath.Rel(root, m)
		if err != nil {
			return nil, err
		}
		if rel = filepath.ToSlash(rel); !want[rel] {
			stale = append(stale, rel)
		}
	}
	sort.Strings(stale)
	return stale, nil
}

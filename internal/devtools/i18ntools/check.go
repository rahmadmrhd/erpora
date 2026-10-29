package i18ntools

import (
	"fmt"
	"math"
	"sort"
	"strings"

	sdki18n "erpora/sdk/i18n"
)

// CheckOptions mengonfigurasi Check.
type CheckOptions struct {
	// Strict mengubah semua peringatan menjadi galat (Requirement 8.4).
	Strict bool
}

// Check memeriksa semua katalog di root repo (Requirement 8): galat struktural (JSON/ICU, kunci ganda,
// tabrakan perataan, segmen terlarang, HTML, namespace, registri, deklarasi manifest) dan peringatan
// ketidaksesuaian antar bahasa terhadap bahasa default namespace (kunci hilang/berlebih, argumen berbeda).
// Kegagalan prasyarat dikembalikan sebagai *ConfigError.
func Check(root string, opts CheckOptions) (*Report, error) {
	rep := &Report{}
	ws, err := load(root, rep)
	if err != nil {
		return nil, err
	}
	if ws.reg != nil {
		for _, n := range ws.ns {
			parity(n, ws, rep)
		}
	}
	rep.finalize(opts.Strict)
	return rep, nil
}

// parity membandingkan setiap bahasa non-default dengan bahasa default namespace.
func parity(n *loadedNS, ws *workspace, rep *Report) {
	base := n.Catalogs[n.Default]
	if base == nil || !base.OK {
		return // bahasa default rusak/hilang sudah dilaporkan; perbandingan tidak bermakna
	}
	baseMsgs := compileAll(base.Flat)
	for _, loc := range n.Locales {
		if loc == n.Default {
			continue
		}
		c := n.Catalogs[loc]
		if c == nil || !c.OK {
			continue
		}
		partial := false
		if li, ok := ws.reg.Info(loc); ok {
			partial = li.Partial
		}
		add := func(code, key, detail string) {
			rep.add(Finding{Severity: SeverityWarning, Code: code, Namespace: n.Name, Locale: loc, File: c.File, Key: key, Detail: detail})
		}
		for _, k := range sortedKeys(base.Flat) {
			src, ok := c.Flat[k]
			if !ok {
				if !partial {
					add(CodeMissingKey, k, fmt.Sprintf("kunci ada di bahasa default %q tetapi tidak di %q", n.Default, loc))
				}
				continue
			}
			bm, ok1 := baseMsgs[k]
			om, err := sdki18n.Compile(src, sdki18n.DefaultLimits())
			if ok1 && err == nil && !sdki18n.SameParams(bm.Params(), om.Params()) {
				add(CodeParamMismatch, k, fmt.Sprintf("argumen berbeda dari bahasa default %q: default %s, %s %s",
					n.Default, describeParams(bm.Params()), loc, describeParams(om.Params())))
			}
		}
		for _, k := range sortedKeys(c.Flat) {
			if _, ok := base.Flat[k]; !ok {
				add(CodeExtraKey, k, fmt.Sprintf("kunci tidak ada di bahasa default %q", n.Default))
			}
		}
	}
}

func compileAll(f sdki18n.Flat) map[string]sdki18n.Message {
	out := make(map[string]sdki18n.Message, len(f))
	for k, src := range f {
		if m, err := sdki18n.Compile(src, sdki18n.DefaultLimits()); err == nil {
			out[k] = m
		}
	}
	return out
}

func sortedKeys(f sdki18n.Flat) []string {
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func describeParams(ps []sdki18n.Param) string {
	if len(ps) == 0 {
		return "[tanpa argumen]"
	}
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.Name + ":" + p.Kind.String()
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// StatusRow adalah kelengkapan satu bahasa pada satu namespace. Translated menghitung kunci bahasa default
// yang ada di bahasa itu dan nilainya bukan penanda TODO hasil i18n:add.
type StatusRow struct {
	Namespace  string  `json:"namespace"`
	Locale     string  `json:"locale"`
	Default    bool    `json:"default"`
	Partial    bool    `json:"partial"`
	Total      int     `json:"total"`
	Translated int     `json:"translated"`
	Percent    float64 `json:"percent"`
}

// ValidationError berarti katalog tidak valid sehingga operasi tidak dapat dilanjutkan (exit 2).
type ValidationError struct{ Msg string }

// Error mengimplementasi error.
func (e *ValidationError) Error() string { return e.Msg }

// TodoPrefix menandai nilai yang dibuat i18n:add dan belum diterjemahkan.
const TodoPrefix = "TODO: "

// Status menghitung kelengkapan per bahasa per namespace (Requirement 8.2). Katalog harus bebas galat
// struktural; bila tidak, *ValidationError (jalankan i18n:check untuk rincian).
func Status(root string) ([]StatusRow, error) {
	rep := &Report{}
	ws, err := load(root, rep)
	if err != nil {
		return nil, err
	}
	rep.finalize(false)
	if rep.Failed() || ws.reg == nil {
		return nil, &ValidationError{Msg: fmt.Sprintf("katalog memiliki %d galat; jalankan i18n:check untuk rincian", rep.Errors)}
	}
	var rows []StatusRow
	for _, n := range ws.ns {
		base := n.Catalogs[n.Default]
		if base == nil || !base.OK {
			continue
		}
		for _, loc := range n.Locales {
			row := StatusRow{Namespace: n.Name, Locale: loc, Default: loc == n.Default, Total: len(base.Flat)}
			if li, ok := ws.reg.Info(loc); ok {
				row.Partial = li.Partial
			}
			if c := n.Catalogs[loc]; c != nil && c.OK {
				for k := range base.Flat {
					if v, ok := c.Flat[k]; ok && !strings.HasPrefix(v, TodoPrefix) {
						row.Translated++
					}
				}
			}
			row.Percent = 100
			if row.Total > 0 {
				row.Percent = math.Round(float64(row.Translated)*1000/float64(row.Total)) / 10
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

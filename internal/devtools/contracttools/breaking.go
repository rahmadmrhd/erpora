package contracttools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/oasdiff/oasdiff/checker"
	"github.com/oasdiff/oasdiff/diff"
	"github.com/oasdiff/oasdiff/load"
	"golang.org/x/mod/semver"

	"erpora/internal/platform/apicontract"
)

// Change adalah satu perubahan hasil oasdiff. Level: ERR | WARN (INFO tidak dilaporkan).
type Change struct{ ID, Level, Method, Path, Text string }

// BreakingResult adalah hasil satu target (platform atau modul).
type BreakingResult struct {
	Target, BaseTag string
	Changes         []Change
	Skipped         string // "belum pernah dirilis" bila tidak ada tag rilis
	Approved        bool   // label + MAJOR naik (keputusan rencana 12)
}

// BreakingOptions mengatur CheckBreaking.
type BreakingOptions struct {
	Only     string // "" = semua; "platform" atau satu modul (Dir, slug, atau key)
	Approved bool   // dari env ERP_API_BREAKING_APPROVED=true (label PR api-breaking-approved)
}

const skippedUnreleased = "belum pernah dirilis"

var releaseTagRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// LatestReleaseTag mengembalikan tag semver tertinggi berbentuk "<prefix>vX.Y.Z" (prerelease dan build diabaikan);
// "" bila tidak ada. Root memakai prefix "", modul "modules/<dir>/".
func LatestReleaseTag(tags []string, prefix string) string {
	best := ""
	for _, t := range tags {
		if !strings.HasPrefix(t, prefix) {
			continue
		}
		v := t[len(prefix):]
		if !releaseTagRe.MatchString(v) {
			continue
		}
		if best == "" || semver.Compare(v, best[len(prefix):]) > 0 {
			best = t
		}
	}
	return best
}

// Failed melaporkan apakah ada target dengan perubahan breaking yang belum disetujui.
func Failed(rs []BreakingResult) bool {
	for _, r := range rs {
		if len(r.Changes) > 0 && !r.Approved {
			return true
		}
	}
	return false
}

type breakTarget struct {
	name, prefix, doc, version string
}

// CheckBreaking membandingkan kontrak terbundel HEAD dengan kontrak baseline pada tag rilis terakhir setiap target
// (platform: tag "vX.Y.Z"; modul: "modules/<dir>/vX.Y.Z"). Baseline dibaca dari pohon tag yang sama (termasuk erp:common
// dan dokumen dependensi) lewat GitReader. Perubahan berlevel WARN atau lebih dilaporkan (api-guidelines section 14.2
// melarang menghapus field apa pun). Target tanpa tag dilewati. Approved bila opts.Approved dan major version
// manifest HEAD lebih besar daripada major tag baseline; syarat bukti 6 bulan diperiksa reviewer, bukan alat.
func CheckBreaking(ctx context.Context, repo *Repo, git GitReader, opts BreakingOptions) ([]BreakingResult, error) {
	targets := []breakTarget{{name: "platform", prefix: "", doc: "platform", version: platformVersion(repo)}}
	for _, m := range repo.Modules {
		if m.HasContract {
			targets = append(targets, breakTarget{name: m.Dir, prefix: "modules/" + m.Dir + "/", doc: m.Slug, version: m.Version})
		}
	}
	if opts.Only != "" {
		var pick []breakTarget
		for _, t := range targets {
			if t.name == opts.Only || t.doc == opts.Only {
				pick = append(pick, t)
			} else if m, ok := repo.Module(opts.Only); ok && m.Dir == t.name {
				pick = append(pick, t)
			}
		}
		if len(pick) == 0 {
			return nil, fmt.Errorf("target %q tidak dikenal (platform atau modul: %s)", opts.Only, repo.ModuleNames())
		}
		targets = pick
	}

	var out []BreakingResult
	for _, t := range targets {
		res, err := checkTarget(ctx, repo, git, t, opts)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.name, err)
		}
		out = append(out, res)
	}
	return out, nil
}

func checkTarget(ctx context.Context, repo *Repo, git GitReader, t breakTarget, opts BreakingOptions) (BreakingResult, error) {
	res := BreakingResult{Target: t.name}
	tags, err := git.Tags(ctx, t.prefix+"v*")
	if err != nil {
		return res, err
	}
	res.BaseTag = LatestReleaseTag(tags, t.prefix)
	if res.BaseTag == "" {
		res.Skipped = skippedUnreleased
		return res, nil
	}
	baseSet, err := loadBaseline(ctx, repo, git, res.BaseTag, t.doc)
	if errors.Is(err, fs.ErrNotExist) {
		res.Skipped = "kontrak belum ada pada " + res.BaseTag
		return res, nil
	}
	if err != nil {
		return res, err
	}
	baseSpec, err := baseSet.Bundle(t.doc)
	if err != nil {
		return res, fmt.Errorf("bundel baseline: %w", err)
	}
	headSpec, err := repo.Set.Bundle(t.doc)
	if err != nil {
		return res, fmt.Errorf("bundel HEAD: %w", err)
	}
	res.Changes, err = diffSpecs(baseSpec, headSpec, t.name)
	if err != nil {
		return res, err
	}
	if opts.Approved && len(res.Changes) > 0 {
		res.Approved = major(t.version) > major(strings.TrimPrefix(res.BaseTag, t.prefix))
	}
	return res, nil
}

// major mengembalikan angka MAJOR dari "X.Y.Z" atau "vX.Y.Z" (-1 bila tidak valid).
func major(v string) int {
	v = strings.TrimPrefix(v, "v")
	n, err := strconv.Atoi(strings.SplitN(v, ".", 2)[0])
	if err != nil {
		return -1
	}
	return n
}

// platformVersion membaca info.version kontrak platform HEAD (versi kontrak platform; root tidak punya manifest).
func platformVersion(repo *Repo) string {
	if d, ok := repo.Set.Doc("platform"); ok {
		if info, ok := d.Spec["info"].(map[string]any); ok {
			if v, ok := info["version"].(string); ok {
				return v
			}
		}
	}
	return ""
}

// loadBaseline memuat dokumen kontrak pada rev: common, doc, dan seluruh dokumen yang dirujuk lewat erp: (transitif).
// Path file dipetakan lewat repo.Files (HEAD); modul yang direktorinya dipindah tidak didukung.
func loadBaseline(ctx context.Context, repo *Repo, git GitReader, rev, doc string) (*apicontract.Set, error) {
	queue := []string{"common", doc}
	loaded := map[string]*apicontract.Doc{}
	var order []*apicontract.Doc
	for i := 0; i < len(queue); i++ {
		name := queue[i]
		if loaded[name] != nil {
			continue
		}
		rel, ok := repo.Files[name]
		if !ok {
			return nil, fmt.Errorf("dokumen %q dirujuk kontrak pada %s tetapi tidak dikenal di HEAD", name, rev)
		}
		data, err := git.Show(ctx, rev, rel)
		if err != nil {
			return nil, err
		}
		d, err := apicontract.Parse(name, data)
		if err != nil {
			return nil, fmt.Errorf("%s pada %s: %w", rel, rev, err)
		}
		loaded[name] = d
		order = append(order, d)
		queue = append(queue, d.ExternalRefs()...)
	}
	return apicontract.NewSet(order...)
}

// diffSpecs menjalankan oasdiff (diff + pemeriksaan kompatibilitas) atas dua spec terbundel.
func diffSpecs(base, head map[string]any, name string) ([]Change, error) {
	load1, err := loadSpecInfo(base, "baseline/"+name)
	if err != nil {
		return nil, fmt.Errorf("baseline: %w", err)
	}
	load2, err := loadSpecInfo(head, "head/"+name)
	if err != nil {
		return nil, fmt.Errorf("HEAD: %w", err)
	}
	d, sources, err := diff.GetWithOperationsSourcesMap(diff.NewConfig(), load1, load2)
	if err != nil {
		return nil, fmt.Errorf("oasdiff diff: %w", err)
	}
	found := checker.CheckBackwardCompatibilityUntilLevel(checker.NewConfig(checker.GetAllChecks()), d, sources, checker.WARN)
	localizer := checker.NewDefaultLocalizer()
	var out []Change
	for _, c := range found {
		level := ""
		switch c.GetLevel() {
		case checker.ERR:
			level = "ERR"
		case checker.WARN:
			level = "WARN"
		default:
			continue
		}
		out = append(out, Change{ID: c.GetId(), Level: level, Method: c.GetOperation(), Path: c.GetPath(), Text: c.GetUncolorizedText(localizer)})
	}
	return out, nil
}

// loadSpecInfo memuat spec ke kin-openapi lewat loader baru (cache dokumen tidak boleh dibagi antar spec).
func loadSpecInfo(spec map[string]any, name string) (*load.SpecInfo, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	return load.NewSpecInfoFromData(openapi3.NewLoader(), data, name)
}

package check

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"golang.org/x/mod/semver"

	"erpora/internal/devtools/contracttools"
	"erpora/internal/devtools/metadatatools/load"
	"erpora/internal/devtools/metadatatools/resolve"
)

// GitReader membaca tag dan isi berkas dari riwayat git (metadata pada tag rilis terakhir modul; Requirement 12.1,
// design D-15). Antarmuka kecil ini memungkinkan git palsu pada tes, seperti contracttools.GitReader.
type GitReader interface {
	// Tags mengembalikan tag yang cocok dengan pola (git tag --list).
	Tags(ctx context.Context, pattern string) ([]string, error)
	// Files mengembalikan jalur berkas (rekursif, garis miring biasa) di bawah dir pada rev.
	Files(ctx context.Context, rev, dir string) ([]string, error)
	// Show mengembalikan isi path pada rev.
	Show(ctx context.Context, rev, path string) ([]byte, error)
}

type execGit struct {
	contracttools.GitReader
	root string
}

// ExecGit membaca git lewat binary git di PATH pada repo root (Tags dan Show memakai helper contracttools).
func ExecGit(root string) GitReader {
	return execGit{GitReader: contracttools.ExecGit(root), root: root}
}

func (g execGit) Files(ctx context.Context, rev, dir string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-tree", "-r", "--name-only", rev, "--", dir) //nolint:gosec // argumen dari kode devtools; rev dari tag repo
	cmd.Dir = g.root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s -- %s: %w", rev, dir, err)
	}
	var files []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			files = append(files, l)
		}
	}
	return files, nil
}

func semverOf(v string) string {
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

func majorOf(v string) int {
	var n int
	_, _ = fmt.Sscanf(strings.TrimPrefix(semver.Major(semverOf(v)), "v"), "%d", &n)
	return n
}

// isMajorVersion menyatakan v berbentuk N.0.0 (rilis MAJOR murni).
func isMajorVersion(v string) bool {
	sv := semverOf(v)
	return semver.IsValid(sv) && semver.Canonical(sv) == semver.Major(sv)+".0.0" && semver.Prerelease(sv) == ""
}

// checkRemoveAfterStatic memeriksa deprecated.since dan removeAfter pada satu field (Requirement 12.4): removeAfter
// hanya bersama deprecated dan harus versi MAJOR (N.0.0) yang lebih besar daripada MAJOR since ("MAJOR berikutnya
// atau sesudahnya"). Tidak membutuhkan git.
func (x *rctx) checkRemoveAfterStatic(f *resolve.Field) {
	ra := f.Str("removeAfter")
	if ra == "" {
		return
	}
	dep, ok := f.Attrs["deprecated"].(map[string]any)
	if !ok {
		x.err(f, "removeAfter", CodeRemoveAfterWithoutDeprecated,
			"removeAfter %s pada field %s hanya sah bersama deprecated: {since, replacedBy?} (Requirement 12.1, 12.4)", ra, f.Name)
		return
	}
	since, _ := dep["since"].(string)
	if !isMajorVersion(ra) || !semver.IsValid(semverOf(since)) || majorOf(ra) <= majorOf(since) {
		x.err(f, "removeAfter", CodeRemoveAfterNotMajor,
			"removeAfter %s pada field %s harus versi MAJOR (N.0.0) berikutnya atau sesudahnya dari deprecated.since %s: penghapusan kolom hanya pada rilis MAJOR (K18, Requirement 12.4)", ra, f.Name, since)
	}
}

// checkReleaseBaselines membandingkan metadata saat ini dengan metadata pada tag rilis terakhir tiap modul
// (modules/<dir>/vX.Y.Z; modul tanpa tag belum punya konsumen sehingga tidak dibandingkan): field yang ada pada rilis lalu hilang hanya sah bila rilis
// itu sudah menandainya deprecated dengan removeAfter dan versi manifest yang sedang disiapkan adalah rilis MAJOR
// (lebih besar dari MAJOR rilis terakhir) yang memenuhi removeAfter (Requirement 12.1, 12.4, design D-15, D-35).
// Kegagalan membaca baseline hanya berupa peringatan; contracts:breaking dan mesin selisih tetap pengaman kedua.
func (c *checker) checkReleaseBaselines(in Input) {
	if in.Git == nil {
		return
	}
	ctx := context.Background()
	for _, m := range in.Modules {
		if len(m.Docs) == 0 {
			continue
		}
		prefix := strings.TrimSuffix(m.Dir, "/") + "/"
		tags, err := in.Git.Tags(ctx, prefix+"v*")
		if err != nil {
			c.add(Problem{Severity: SeverityWarning, File: m.Dir, Code: CodeReleaseBaselineUnavailable, Module: m.Manifest.Slug,
				Message: "tag rilis modul tidak dapat dibaca sehingga penghapusan field tidak dibandingkan dengan rilis sebelumnya: " + err.Error()})
			continue
		}
		tag := contracttools.LatestReleaseTag(tags, prefix)
		if tag == "" {
			// Modul tanpa tag rilis belum punya konsumen: field/kolom boleh dihapus langsung tanpa deprecated, removeAfter,
			// atau gerbang MAJOR (keputusan pemilik produk, 2 Nov 2026; Requirement 12.4 hanya berlaku sejak ada tag rilis).
			c.rep.Unreleased[m.Manifest.Slug] = true
			continue
		}
		c.compareWithRelease(in, m, tag)
	}
}

func (c *checker) compareWithRelease(in Input, m Module, tag string) {
	ctx := context.Background()
	warn := func(code, format string, args ...any) {
		c.add(Problem{Severity: SeverityWarning, File: m.Dir, Code: code, Module: m.Manifest.Slug, Message: fmt.Sprintf(format, args...)})
	}
	files, err := in.Git.Files(ctx, tag, strings.TrimSuffix(m.Dir, "/")+"/metadata")
	if err != nil {
		warn(CodeReleaseBaselineUnavailable, "metadata pada tag %s tidak dapat dibaca: %v", tag, err)
		return
	}
	var docs []*load.Document
	bad := 0
	for _, p := range files {
		if !strings.HasSuffix(p, ".yaml") {
			continue
		}
		raw, err := in.Git.Show(ctx, tag, p)
		if err != nil {
			warn(CodeReleaseBaselineUnavailable, "berkas %s pada tag %s tidak dapat dibaca: %v", p, tag, err)
			return
		}
		d, probs := load.Parse(tag+":"+p, raw)
		if len(probs) > 0 {
			bad++
			continue
		}
		docs = append(docs, d)
	}
	if bad > 0 {
		warn(CodeReleaseBaselineInvalid, "%d berkas metadata pada tag %s tidak lolos pemuatan (skema berubah sejak rilis?) sehingga perbandingan dilewati", bad, tag)
		return
	}
	if len(docs) == 0 {
		return // modul belum bermetadata pada rilis itu
	}
	base := c.baselineCatalog(in, m, docs)
	res := base.ResolveAll()
	c.baselines[m.Manifest.Slug] = res
	curVersion := m.Manifest.Version
	baseVersion := strings.TrimPrefix(tag, strings.TrimSuffix(m.Dir, "/")+"/v")
	for _, old := range res.Resources {
		if old.Module != m.Manifest.Slug {
			continue
		}
		cur := c.res.Resource(old.Key)
		if cur == nil {
			continue // resource baru dihapus atau gagal diselesaikan (dilaporkan pemeriksa lain)
		}
		doc := c.cat.Doc(old.Key)
		for _, of := range old.Fields {
			if of.ProtectedBy != "" || of.IsStatusAxis || cur.Field(of.Name) != nil {
				continue
			}
			c.reportRemoval(m, doc, cur, of, tag, baseVersion, curVersion)
		}
	}
}

// reportRemoval menilai satu field yang hilang terhadap rilis terakhir.
func (c *checker) reportRemoval(m Module, doc *load.Document, cur *resolve.Resolved, of *resolve.Field, tag, baseVersion, curVersion string) {
	file, line, col := cur.Source.File, 1, 1
	if doc != nil {
		file = doc.File
		p := doc.Pos("/fields")
		line, col = p.Line, p.Column
	}
	emit := func(code, format string, args ...any) {
		c.add(Problem{Severity: SeverityError, File: file, Line: line, Column: col, Pointer: "/fields/" + of.Name, Code: code,
			Message: fmt.Sprintf(format, args...), Module: m.Manifest.Slug, Resource: cur.Key})
	}
	dep, deprecated := of.Attrs["deprecated"].(map[string]any)
	if !deprecated {
		emit(CodeFieldRemovedWithoutDeprecation,
			"field %s ada pada rilis %s tetapi dihapus tanpa pernah ditandai deprecated: tandai deprecated: {since, replacedBy?} dan removeAfter pada satu rilis, hapus pada rilis MAJOR berikutnya (K18, Requirement 12.1)", of.Name, tag)
		return
	}
	ra := of.Str("removeAfter")
	if ra == "" {
		emit(CodeFieldRemovedNoRemoveAfter,
			"field %s ditandai deprecated (since %v) pada rilis %s tetapi tidak menyatakan removeAfter: penghapusan kolom butuh removeAfter berupa versi MAJOR (Requirement 12.4)", of.Name, dep["since"], tag)
		return
	}
	cv := semverOf(curVersion)
	if !semver.IsValid(cv) || majorOf(curVersion) <= majorOf(baseVersion) || semver.Compare(cv, semverOf(ra)) < 0 {
		emit(CodeFieldRemovedBeforeMajor,
			"field %s hanya boleh dihapus pada rilis MAJOR modul yang memenuhi removeAfter %s: versi manifest %q belum memenuhi (rilis terakhir %s; K18, Requirement 12.4)", of.Name, ra, curVersion, baseVersion)
	}
}

// baselineCatalog membangun katalog untuk menyelesaikan metadata baseline modul m: dokumen baseline modul itu,
// dokumen saat ini untuk modul lain, dan metadata bawaan SDK.
func (c *checker) baselineCatalog(in Input, m Module, docs []*load.Document) *resolve.Catalog {
	cat := resolve.NewCatalog()
	cat.AddBuiltin()
	addModules(cat, in, func(mod Module) []*load.Document {
		if mod.Manifest.Slug == m.Manifest.Slug {
			return docs
		}
		return mod.Docs
	}, nil)
	return cat
}

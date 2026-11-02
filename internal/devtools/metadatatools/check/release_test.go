package check

import (
	"context"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"testing"
)

// fakeGit adalah pembaca git palsu: pohon berkas per rev (tag) di memori.
type fakeGit struct {
	tags    []string
	trees   map[string]map[string]string // rev -> path -> isi
	tagsErr error
	shown   []string
}

func (g *fakeGit) Tags(_ context.Context, pattern string) ([]string, error) {
	if g.tagsErr != nil {
		return nil, g.tagsErr
	}
	prefix := strings.TrimSuffix(pattern, "*")
	var out []string
	for _, t := range g.tags {
		if strings.HasPrefix(t, prefix) {
			out = append(out, t)
		}
	}
	return out, nil
}

func (g *fakeGit) Files(_ context.Context, rev, dir string) ([]string, error) {
	tree, ok := g.trees[rev]
	if !ok {
		return nil, fmt.Errorf("rev %s tidak dikenal", rev)
	}
	var out []string
	for p := range tree {
		if strings.HasPrefix(p, dir+"/") {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (g *fakeGit) Show(_ context.Context, rev, path string) ([]byte, error) {
	g.shown = append(g.shown, rev+":"+path)
	tree, ok := g.trees[rev]
	if !ok {
		return nil, fmt.Errorf("rev %s tidak dikenal", rev)
	}
	b, ok := tree[path]
	if !ok {
		return nil, fmt.Errorf("%s:%s: %w", rev, path, fs.ErrNotExist)
	}
	return []byte(b), nil
}

const tagPrefix = "modules/m/"

// released membuat git palsu dengan satu tag rilis berisi metadata resource m.r (baseline).
func released(tag, baselineFields string) *fakeGit {
	return &fakeGit{
		tags: []string{tagPrefix + tag},
		trees: map[string]map[string]string{
			tagPrefix + tag: {"modules/m/metadata/r.resource.yaml": resource("m.r", "fields:\n"+baselineFields)},
		},
	}
}

func currentModule(t *testing.T, version, currentFields string) Module {
	t.Helper()
	m := mod(t, "m", nil, f("modules/m/metadata/r.resource.yaml", resource("m.r", "fields:\n"+currentFields)))
	m.Manifest.Version = version
	return m
}

func withGit(t *testing.T, g GitReader, m Module) *Report {
	t.Helper()
	return Run(Input{Modules: []Module{m}, Git: g})
}

const (
	keepField = "  keep: {type: string, maxLength: 10}\n"
	oldPhone  = "  phone: {type: string, maxLength: 32}\n"
)

func TestRemovedFieldWithoutDeprecationInReleaseIsRejected(t *testing.T) {
	g := released("v1.0.0", keepField+oldPhone)
	rep := withGit(t, g, currentModule(t, "1.1.0", keepField))
	p := wantErr(t, rep, CodeFieldRemovedWithoutDeprecation, "/fields/phone")
	if !strings.Contains(p.Message, "modules/m/v1.0.0") || !strings.Contains(p.Message, "deprecated") {
		t.Errorf("pesan harus menyebut tag dan deprecated: %q", p.Message)
	}
	if p.File != "modules/m/metadata/r.resource.yaml" || p.Resource != "m.r" || p.Module != "m" {
		t.Errorf("lokasi = %+v", p)
	}
	// field hanya dihapus di working tree; ditandai deprecated baru di working tree tidak menolong (harus pada rilis).
	cur := currentModule(t, "2.0.0", keepField)
	wantErr(t, withGit(t, g, cur), CodeFieldRemovedWithoutDeprecation, "/fields/phone")
}

func TestRemovedFieldNeedsRemoveAfterInRelease(t *testing.T) {
	g := released("v1.4.0", keepField+"  phone: {type: string, maxLength: 32, deprecated: {since: \"1.4.0\"}}\n")
	rep := withGit(t, g, currentModule(t, "2.0.0", keepField))
	wantErr(t, rep, CodeFieldRemovedNoRemoveAfter, "/fields/phone")
}

func TestRemovalOnlyOnMajorReleaseSatisfyingRemoveAfter(t *testing.T) {
	baseline := keepField + "  phone: {type: string, maxLength: 32, deprecated: {since: \"1.4.0\", replacedBy: phoneE164}, removeAfter: \"2.0.0\"}\n  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone}\n"
	cur := keepField + "  phoneE164: {type: string, maxLength: 20, nullable: true}\n"
	tests := []struct {
		name    string
		tag     string
		version string
		ok      bool
	}{
		{"MAJOR memenuhi removeAfter", "v1.4.0", "2.0.0", true},
		{"MAJOR lebih besar dari removeAfter", "v1.4.0", "3.0.0", true},
		{"versi belum dinaikkan", "v1.4.0", "1.4.0", false},
		{"MINOR naik bukan MAJOR", "v1.4.0", "1.9.0", false},
		{"sebelum removeAfter", "v1.4.0", "1.5.0", false},
		{"MAJOR kurang dari removeAfter", "v0.9.0", "1.0.0", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := withGit(t, released(tc.tag, baseline), currentModule(t, tc.version, cur))
			if tc.ok {
				noErrors(t, rep)
				return
			}
			p := wantErr(t, rep, CodeFieldRemovedBeforeMajor, "/fields/phone")
			if !strings.Contains(p.Message, "MAJOR") || !strings.Contains(p.Message, "2.0.0") {
				t.Errorf("pesan harus menyebut MAJOR dan removeAfter: %q", p.Message)
			}
		})
	}
	t.Run("removeAfter 3.0.0 belum terpenuhi oleh 2.0.0", func(t *testing.T) {
		b := keepField + "  phone: {type: string, maxLength: 32, deprecated: {since: \"1.4.0\"}, removeAfter: \"3.0.0\"}\n"
		rep := withGit(t, released("v2.0.0", b), currentModule(t, "2.0.0", keepField))
		wantErr(t, rep, CodeFieldRemovedBeforeMajor, "/fields/phone")
		noErrors(t, withGit(t, released("v2.0.0", b), currentModule(t, "3.0.0", keepField)))
	})
}

func TestNoBaselineOrNoRemovalIsClean(t *testing.T) {
	cur := currentModule(t, "1.0.0", keepField)
	t.Run("modul tanpa tag dikecualikan", func(t *testing.T) {
		g := &fakeGit{tags: []string{"modules/other/v1.0.0"}, trees: map[string]map[string]string{}}
		noErrors(t, withGit(t, g, cur))
		for _, p := range withGit(t, g, cur).Problems {
			if strings.HasPrefix(p.Code, "release_") {
				t.Errorf("tanpa tag tidak boleh ada temuan rilis: %+v", p)
			}
		}
	})
	t.Run("tanpa Git sama sekali", func(t *testing.T) {
		noErrors(t, Run(Input{Modules: []Module{cur}}))
	})
	t.Run("field tetap ada, deprecated sah", func(t *testing.T) {
		g := released("v1.0.0", keepField+oldPhone)
		c := currentModule(t, "1.1.0", keepField+"  phone: {type: string, maxLength: 32, deprecated: {since: \"1.1.0\"}, removeAfter: \"2.0.0\"}\n")
		noErrors(t, withGit(t, g, c))
	})
	t.Run("field baru dan perubahan lain bukan penghapusan", func(t *testing.T) {
		g := released("v1.0.0", keepField)
		c := currentModule(t, "1.1.0", keepField+"  extra: {type: text, nullable: true}\n")
		noErrors(t, withGit(t, g, c))
	})
	t.Run("resource baru tanpa pembanding", func(t *testing.T) {
		g := released("v1.0.0", keepField)
		m := mod(t, "m", nil, f("modules/m/metadata/r.resource.yaml", resource("m.r", "fields:\n"+keepField)),
			f("modules/m/metadata/n.resource.yaml", resource("m.n", "fields:\n  a: {type: text}\n")))
		m.Manifest.Version = "1.1.0"
		noErrors(t, withGit(t, g, m))
	})
}

func TestLatestReleaseTagIsTheBaseline(t *testing.T) {
	g := &fakeGit{
		tags: []string{tagPrefix + "v1.0.0", tagPrefix + "v1.10.0", tagPrefix + "v1.2.0", tagPrefix + "v1.11.0-rc1", "modules/other/v9.9.9"},
		trees: map[string]map[string]string{
			tagPrefix + "v1.0.0":  {"modules/m/metadata/r.resource.yaml": resource("m.r", "fields:\n"+keepField+oldPhone)},
			tagPrefix + "v1.2.0":  {"modules/m/metadata/r.resource.yaml": resource("m.r", "fields:\n"+keepField+oldPhone)},
			tagPrefix + "v1.10.0": {"modules/m/metadata/r.resource.yaml": resource("m.r", "fields:\n"+keepField)},
		},
	}
	// v1.10.0 (tertinggi, bukan prarilis) tidak punya phone: menghapusnya sekarang bukan penghapusan.
	noErrors(t, withGit(t, g, currentModule(t, "1.11.0", keepField)))
	for _, s := range g.shown {
		if !strings.HasPrefix(s, tagPrefix+"v1.10.0:") {
			t.Errorf("baseline harus dibaca dari tag terakhir saja: %s", s)
		}
	}
}

func TestRemovedFieldFromSharedBlockIsDetected(t *testing.T) {
	g := &fakeGit{
		tags: []string{tagPrefix + "v1.0.0"},
		trees: map[string]map[string]string{tagPrefix + "v1.0.0": {
			"modules/m/metadata/blk.abstract.yaml": abstract("m.blk", "fields:\n  street: {type: text}\n  line2: {type: text, nullable: true}\n"),
			"modules/m/metadata/r.resource.yaml":   resource("m.r", "include:\n  - {source: m.blk, prefix: billing}\n"),
		}},
	}
	m := mod(t, "m", nil,
		f("modules/m/metadata/blk.abstract.yaml", abstract("m.blk", "fields:\n  street: {type: text}\n")),
		f("modules/m/metadata/r.resource.yaml", resource("m.r", "include:\n  - {source: m.blk, prefix: billing}\n")))
	m.Manifest.Version = "1.1.0"
	wantErr(t, withGit(t, g, m), CodeFieldRemovedWithoutDeprecation, "/fields/billingLine2")
}

func TestRenameWithoutDeprecationOfOldFieldIsStillRejected(t *testing.T) {
	g := released("v1.0.0", keepField+oldPhone)
	cur := keepField + "  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone}\n"
	rep := withGit(t, g, currentModule(t, "1.1.0", cur))
	// migrateFrom merujuk field yang sudah tidak ada dan penghapusan tanpa deprecated pada rilis.
	wantErr(t, rep, CodeFieldRemovedWithoutDeprecation, "/fields/phone")
	wantErr(t, rep, CodeMigrateFromUnknown, "/fields/phoneE164")
}

func TestReleaseBaselineProblemsAreWarningsNotErrors(t *testing.T) {
	t.Run("git tidak dapat dibaca", func(t *testing.T) {
		g := &fakeGit{tagsErr: fmt.Errorf("bukan repositori git")}
		rep := withGit(t, g, currentModule(t, "1.0.0", keepField))
		noErrors(t, rep)
		wantWarning(t, rep, CodeReleaseBaselineUnavailable, "")
	})
	t.Run("baseline tidak valid", func(t *testing.T) {
		g := &fakeGit{tags: []string{tagPrefix + "v1.0.0"}, trees: map[string]map[string]string{
			tagPrefix + "v1.0.0": {"modules/m/metadata/r.resource.yaml": "schemaVersion: \"1\"\nkind: resource\nkey: m.r\nbogus: 1\n"},
		}}
		rep := withGit(t, g, currentModule(t, "1.0.0", keepField))
		noErrors(t, rep)
		wantWarning(t, rep, CodeReleaseBaselineInvalid, "")
	})
	t.Run("tag tanpa metadata (modul belum bermetadata saat itu)", func(t *testing.T) {
		g := &fakeGit{tags: []string{tagPrefix + "v1.0.0"}, trees: map[string]map[string]string{tagPrefix + "v1.0.0": {"modules/m/module.json": "{}"}}}
		rep := withGit(t, g, currentModule(t, "1.0.0", keepField))
		noErrors(t, rep)
		for _, p := range rep.Problems {
			if strings.HasPrefix(p.Code, "release_") {
				t.Errorf("tag tanpa metadata tidak boleh menghasilkan temuan: %+v", p)
			}
		}
	})
	t.Run("versi manifest tidak sah", func(t *testing.T) {
		g := released("v1.0.0", keepField+oldPhone)
		rep := withGit(t, g, currentModule(t, "bukan-semver", keepField))
		wantErr(t, rep, CodeFieldRemovedWithoutDeprecation, "/fields/phone")
	})
}

// ---- pemeriksaan statis deprecated dan removeAfter (tanpa git) ----

func TestDeprecatedAndRemoveAfterStaticRules(t *testing.T) {
	tests := []struct {
		name  string
		attrs string
		code  string // "" = lolos
	}{
		{"deprecated tanpa removeAfter sah", `type: string, maxLength: 5, deprecated: {since: "1.4.0"}`, ""},
		{"removeAfter MAJOR berikutnya", `type: string, maxLength: 5, deprecated: {since: "1.4.0"}, removeAfter: "2.0.0"`, ""},
		{"removeAfter MAJOR sesudahnya", `type: string, maxLength: 5, deprecated: {since: "1.4.0"}, removeAfter: "4.0.0"`, ""},
		{"removeAfter tanpa deprecated", `type: string, maxLength: 5, removeAfter: "2.0.0"`, CodeRemoveAfterWithoutDeprecated},
		{"removeAfter bukan MAJOR (minor)", `type: string, maxLength: 5, deprecated: {since: "1.4.0"}, removeAfter: "1.5.0"`, CodeRemoveAfterNotMajor},
		{"removeAfter bukan MAJOR (patch)", `type: string, maxLength: 5, deprecated: {since: "1.4.0"}, removeAfter: "2.0.1"`, CodeRemoveAfterNotMajor},
		{"removeAfter MAJOR sama dengan since", `type: string, maxLength: 5, deprecated: {since: "1.4.0"}, removeAfter: "1.0.0"`, CodeRemoveAfterNotMajor},
		{"removeAfter sebelum since", `type: string, maxLength: 5, deprecated: {since: "3.2.0"}, removeAfter: "2.0.0"`, CodeRemoveAfterNotMajor},
		{"removeAfter MAJOR sama dengan MAJOR since", `type: string, maxLength: 5, deprecated: {since: "2.0.0"}, removeAfter: "2.0.0"`, CodeRemoveAfterNotMajor},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rep := single(t, oneField(tc.attrs))
			if tc.code == "" {
				noErrors(t, rep)
				return
			}
			wantErr(t, rep, tc.code, "/fields/x")
		})
	}
	t.Run("replacedBy harus field yang ada", func(t *testing.T) {
		wantErr(t, single(t, oneField(`type: string, maxLength: 5, deprecated: {since: "1.0.0", replacedBy: ghost}`)), CodeFieldDeprecatedTarget, "/fields/x")
	})
}

func TestMigrateFromPairsRemainIdentical(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", `fields:
  phone: {type: string, maxLength: 32, deprecated: {since: "1.0.0"}, fieldGroup: contact, classification: confidential}
  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone, fieldGroup: other, classification: internal}
`)))
	wantErr(t, rep, CodeMigrateFromGroup, "/fields/phoneE164")
	wantErr(t, rep, CodeMigrateFromClassification, "/fields/phoneE164")
}

// Keputusan 2 Nov 2026 (Requirement 12.4): modul tanpa tag rilis belum punya konsumen sehingga boleh menghapus
// field langsung tanpa deprecated, removeAfter, atau gerbang MAJOR; modul bertag tetap ketat.
func TestUnreleasedModuleMayRemoveFieldsDirectly(t *testing.T) {
	cur := currentModule(t, "0.1.0", keepField) // phone tidak pernah dirilis lalu dihapus
	t.Run("tanpa tag: lolos dan ditandai Unreleased", func(t *testing.T) {
		g := &fakeGit{tags: []string{"modules/other/v1.0.0"}, trees: map[string]map[string]string{}}
		rep := withGit(t, g, cur)
		noErrors(t, rep)
		if !rep.Unreleased["m"] {
			t.Errorf("Unreleased = %v; modul m belum bertag", rep.Unreleased)
		}
	})
	t.Run("bertag: penghapusan tanpa deprecated ditolak dan tidak Unreleased", func(t *testing.T) {
		g := released("v1.0.0", keepField+oldPhone)
		rep := withGit(t, g, currentModule(t, "1.1.0", keepField))
		wantErr(t, rep, CodeFieldRemovedWithoutDeprecation, "/fields/phone")
		if rep.Unreleased["m"] {
			t.Error("modul bertag tidak boleh Unreleased")
		}
	})
	t.Run("tag tak terbaca: tidak dianggap Unreleased", func(t *testing.T) {
		rep := withGit(t, &fakeGit{tagsErr: fmt.Errorf("git rusak")}, cur)
		if rep.Unreleased["m"] {
			t.Error("kegagalan membaca tag tidak boleh membuka kelonggaran")
		}
	})
}

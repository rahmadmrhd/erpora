package contracttools

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLatestReleaseTag(t *testing.T) {
	tags := []string{
		"v1.9.0", "v1.10.0", "v2.0.0-rc.1", "v1.2.3-beta", "vx", "v1.10",
		"modules/example/v3.0.0", "modules/example/v1.0.0", "modules/other/v9.0.0", "release-1",
	}
	if got := LatestReleaseTag(tags, ""); got != "v1.10.0" {
		t.Errorf("root = %q, want v1.10.0 (v1.10.0 > v1.9.0; prerelease dan prefix modul diabaikan)", got)
	}
	if got := LatestReleaseTag(tags, "modules/example/"); got != "modules/example/v3.0.0" {
		t.Errorf("example = %q, want modules/example/v3.0.0", got)
	}
	if got := LatestReleaseTag(tags, "modules/none/"); got != "" {
		t.Errorf("tanpa tag = %q, want kosong", got)
	}
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=", "GIT_CONFIG_SYSTEM=", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.test",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// releasedRepo membuat repo git sementara berisi kontrak example dan exampleaddon yang sudah di-tag modules/example/v1.0.0.
func releasedRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git tidak ada")
	}
	root := t.TempDir()
	copyRepoSubset(t, root, "example", "exampleaddon")
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "rilis awal")
	git(t, root, "tag", "modules/example/v1.0.0")
	return root
}

func breaking(t *testing.T, root string, opts BreakingOptions) []BreakingResult {
	t.Helper()
	repo, err := LoadRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	rs, err := CheckBreaking(context.Background(), repo, ExecGit(root), opts)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func resultOf(t *testing.T, rs []BreakingResult, target string) BreakingResult {
	t.Helper()
	for _, r := range rs {
		if r.Target == target {
			return r
		}
	}
	t.Fatalf("target %q tidak ada di %+v", target, rs)
	return BreakingResult{}
}

const noteTitle = "        title:\n          type: string\n          x-erp-classification: internal\n"

// AC FR-23: menghapus field respons terdeteksi sebagai breaking.
func TestCheckBreaking_RemovedFieldFails(t *testing.T) {
	root := releasedRepo(t)
	editFile(t, root, "modules/example/openapi.yaml",
		[2]string{noteTitle, ""},
		[2]string{"required: [id, title, version, createdAt, updatedAt]", "required: [id, version, createdAt, updatedAt]"})
	rs := breaking(t, root, BreakingOptions{})
	ex := resultOf(t, rs, "example")
	if ex.BaseTag != "modules/example/v1.0.0" || len(ex.Changes) == 0 {
		t.Fatalf("example = %+v, want >= 1 Change terhadap modules/example/v1.0.0", ex)
	}
	if !Failed(rs) {
		t.Error("Failed harus true")
	}
	for _, c := range ex.Changes {
		if c.Level != "ERR" && c.Level != "WARN" {
			t.Errorf("level %q tidak dilaporkan", c.Level)
		}
	}
}

func TestCheckBreaking_OptionalFieldPasses(t *testing.T) {
	root := releasedRepo(t)
	editFile(t, root, "modules/example/openapi.yaml",
		[2]string{"        version: { type: integer, format: int64 }\n        createdAt:", "        version: { type: integer, format: int64 }\n        pinned: { type: boolean }\n        createdAt:"})
	rs := breaking(t, root, BreakingOptions{})
	ex := resultOf(t, rs, "example")
	if len(ex.Changes) != 0 || ex.Skipped != "" {
		t.Errorf("properti respons opsional baru tidak boleh breaking: %+v", ex)
	}
	if Failed(rs) {
		t.Error("Failed harus false")
	}
}

func TestCheckBreaking_UnreleasedSkipped(t *testing.T) {
	root := releasedRepo(t)
	rs := breaking(t, root, BreakingOptions{})
	for _, target := range []string{"exampleaddon", "platform"} {
		if got := resultOf(t, rs, target).Skipped; got != "belum pernah dirilis" {
			t.Errorf("%s: Skipped = %q, want \"belum pernah dirilis\"", target, got)
		}
	}
}

func TestCheckBreaking_ApprovedMajor(t *testing.T) {
	root := releasedRepo(t)
	editFile(t, root, "modules/example/openapi.yaml",
		[2]string{noteTitle, ""},
		[2]string{"required: [id, title, version, createdAt, updatedAt]", "required: [id, version, createdAt, updatedAt]"})

	// Label saja tanpa kenaikan MAJOR manifest: tetap gagal.
	editFile(t, root, "modules/example/module.json", [2]string{`"version": "1.0.0"`, `"version": "1.1.0"`})
	rs := breaking(t, root, BreakingOptions{Approved: true})
	if r := resultOf(t, rs, "example"); r.Approved || !Failed(rs) {
		t.Errorf("manifest 1.1.0 + label harus tetap gagal: %+v", r)
	}

	// Label + MAJOR naik: diizinkan.
	editFile(t, root, "modules/example/module.json", [2]string{`"version": "1.1.0"`, `"version": "2.0.0"`})
	rs = breaking(t, root, BreakingOptions{Approved: true})
	if r := resultOf(t, rs, "example"); !r.Approved || len(r.Changes) == 0 || Failed(rs) {
		t.Errorf("manifest 2.0.0 + label harus diizinkan: %+v", r)
	}

	// MAJOR naik tanpa label: tetap gagal.
	rs = breaking(t, root, BreakingOptions{})
	if !Failed(rs) {
		t.Error("tanpa label harus gagal")
	}
}

// recordingGit mencatat setiap Show agar test bisa memastikan baseline dibaca dari tag, bukan dari HEAD.
type recordingGit struct {
	GitReader
	shown []string
}

func (r *recordingGit) Show(ctx context.Context, rev, path string) ([]byte, error) {
	r.shown = append(r.shown, rev+":"+path)
	return r.GitReader.Show(ctx, rev, path)
}

func TestCheckBreaking_BaselineResolvesErpRefsAtTag(t *testing.T) {
	root := releasedRepo(t)
	// common.yaml di HEAD berbeda dengan di tag (perubahan kompatibel); modul tidak berubah.
	editFile(t, root, "contracts/openapi/common.yaml", [2]string{"description: UUID (v7 untuk ID baru).", "description: UUID (v7 untuk ID baru), diubah setelah rilis."})
	repo, err := LoadRepo(root)
	if err != nil {
		t.Fatal(err)
	}
	rg := &recordingGit{GitReader: ExecGit(root)}
	rs, err := CheckBreaking(context.Background(), repo, rg, BreakingOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ex := resultOf(t, rs, "example")
	if len(ex.Changes) != 0 || Failed(rs) {
		t.Errorf("perubahan common yang kompatibel tidak boleh dianggap breaking: %+v", ex)
	}
	want := "modules/example/v1.0.0:contracts/openapi/common.yaml"
	found := false
	for _, s := range rg.shown {
		if s == want {
			found = true
		}
	}
	if !found {
		t.Errorf("baseline tidak membaca common.yaml dari tag; Show: %v", rg.shown)
	}
	for _, s := range rg.shown {
		if !strings.HasPrefix(s, "modules/example/v1.0.0:") {
			t.Errorf("Show memakai rev selain tag baseline: %s", s)
		}
	}
}

// Spike keputusan 12: kin-openapi harus dapat memuat seluruh kontrak terbundel berversi 3.1.0 (termasuk type array),
// dan diff atas dua salinan identik tidak melaporkan perubahan.
func TestOasdiff_LoadsRealBundles(t *testing.T) {
	repo, err := LoadRepo(realRoot)
	if err != nil {
		t.Fatal(err)
	}
	for name := range repo.Files {
		spec, err := repo.Set.Bundle(name)
		if err != nil {
			t.Fatalf("Bundle(%s): %v", name, err)
		}
		changes, err := diffSpecs(spec, spec, name)
		if err != nil {
			t.Errorf("%s: oasdiff gagal memuat/diff bundel 3.1.0: %v", name, err)
		}
		if len(changes) != 0 {
			t.Errorf("%s: diff identik melaporkan %v", name, changes)
		}
	}
}

func TestExecGit_ShowMissingIsNotExist(t *testing.T) {
	root := releasedRepo(t)
	_, err := ExecGit(root).Show(context.Background(), "modules/example/v1.0.0", "tidak/ada.yaml")
	if err == nil || !isNotExist(err) {
		t.Errorf("err = %v, want fs.ErrNotExist", err)
	}
	tags, err := ExecGit(root).Tags(context.Background(), "modules/example/v*")
	if err != nil || len(tags) != 1 || tags[0] != "modules/example/v1.0.0" {
		t.Errorf("tags = %v err = %v", tags, err)
	}
}

func isNotExist(err error) bool { return errors.Is(err, fs.ErrNotExist) }

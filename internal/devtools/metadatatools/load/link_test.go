package load

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const leakDoc = "schemaVersion: \"1\"\nkind: resource\nkey: leak.thing\nfields:\n  a: {type: text}\n"

// writeTree menulis berkas uji di bawah dir.
func writeTree(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// link membuat tautan simbolik; pada Windows tanpa hak simbolik memakai junction untuk direktori.
func link(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err == nil {
		return
	}
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", name, target).CombinedOutput(); err == nil {
			return
		} else {
			t.Skipf("tidak dapat membuat tautan/junction: %v %s", err, out)
		}
	}
	t.Skip("tidak dapat membuat tautan simbolik")
}

func TestLoadFSRejectsLinkedMetadataDirectory(t *testing.T) {
	outside := t.TempDir()
	writeTree(t, outside, "x.resource.yaml", leakDoc)

	t.Run("direktori metadata adalah tautan ke luar", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, "modules/m/module.json", "{}")
		link(t, outside, filepath.Join(root, "modules", "m", "metadata"))
		docs, probs := LoadFS(os.DirFS(root), "modules/m/metadata")
		if len(docs) != 0 {
			t.Fatalf("dokumen dari direktori bertautan tidak boleh dimuat: %d", len(docs))
		}
		if !hasProblem(probs, CodeSymlink) {
			t.Fatalf("temuan = %v", probs)
		}
		for _, p := range probs {
			if strings.Contains(p.Message, "leak.thing") {
				t.Errorf("isi berkas bocor ke pesan: %s", p.Message)
			}
		}
	})
	t.Run("induk direktori metadata adalah tautan", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, outside, "metadata/y.resource.yaml", leakDoc)
		writeTree(t, root, "modules/real/module.json", "{}")
		link(t, outside, filepath.Join(root, "modules", "evil"))
		docs, probs := LoadFS(os.DirFS(root), "modules/evil/metadata")
		if len(docs) != 0 || !hasProblem(probs, CodeSymlink) {
			t.Fatalf("docs=%d temuan=%v", len(docs), probs)
		}
	})
	t.Run("subdirektori bertautan di dalam metadata ditolak", func(t *testing.T) {
		root := t.TempDir()
		writeTree(t, root, "modules/m/metadata/ok.resource.yaml", strings.Replace(leakDoc, "leak.thing", "m.ok", 1))
		link(t, outside, filepath.Join(root, "modules", "m", "metadata", "sub"))
		docs, probs := LoadFS(os.DirFS(root), "modules/m/metadata")
		if len(docs) != 1 || docs[0].Key != "m.ok" {
			t.Fatalf("hanya berkas asli yang dimuat: %v", docs)
		}
		if !hasProblem(probs, CodeSymlink) {
			t.Fatalf("temuan = %v", probs)
		}
	})
}

func hasProblem(ps []Problem, code string) bool {
	for _, p := range ps {
		if p.Code == code {
			return true
		}
	}
	return false
}

func TestSchemaAndScalarMessagesDoNotEchoValues(t *testing.T) {
	const secret = "SUPERSECRETVALUE"
	tests := map[string]string{
		"pola key":                   "schemaVersion: \"1\"\nkind: resource\nkey: " + secret + "\nfields:\n  a: {type: text}\n",
		"bilangan di luar jangkauan": "schemaVersion: \"1\"\nkind: resource\nkey: m.r\nfields:\n  a: {type: string, maxLength: 99999999999999999999999999" + secret[:0] + "}\n",
		"tipe field":                 "schemaVersion: \"1\"\nkind: resource\nkey: m.r\nfields:\n  a: {type: " + secret + "}\n",
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			_, probs := Parse("x.resource.yaml", []byte(src))
			if len(probs) == 0 {
				t.Fatal("seharusnya ada temuan")
			}
			for _, p := range probs {
				if strings.Contains(p.Message, secret) || strings.Contains(p.Message, "99999999999999999999") {
					t.Errorf("pesan memuat nilai YAML: %s", p)
				}
				if p.Line == 0 || p.File == "" {
					t.Errorf("lokasi hilang: %+v", p)
				}
			}
		})
	}
}

package architecture

import (
	"os"
	"strings"
	"testing"
)

// TestAgentsMDMatchesClaudeMD menjaga AGENTS.md (dibaca Codex, termasuk review PR) identik dengan CLAUDE.md.
func TestAgentsMDMatchesClaudeMD(t *testing.T) {
	read := func(name string) string {
		t.Helper()
		b, err := os.ReadFile("../../" + name)
		if err != nil {
			t.Fatalf("baca %s: %v", name, err)
		}
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	if read("CLAUDE.md") != read("AGENTS.md") {
		t.Fatal("AGENTS.md berbeda dari CLAUDE.md; salin isi CLAUDE.md ke AGENTS.md (keduanya harus identik)")
	}
}

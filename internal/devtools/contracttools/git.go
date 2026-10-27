package contracttools

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
)

// GitReader membaca tag dan isi file dari riwayat git.
type GitReader interface {
	// Tags mengembalikan tag yang cocok dengan pola (`git tag --list <pattern>`).
	Tags(ctx context.Context, pattern string) ([]string, error)
	// Show mengembalikan isi path pada rev (`git show <rev>:<path>`); tidak ada -> error yang memuat fs.ErrNotExist.
	Show(ctx context.Context, rev, path string) ([]byte, error)
}

type execGit struct{ root string }

// ExecGit membaca git lewat binary `git` di PATH pada repo root.
func ExecGit(root string) GitReader { return execGit{root: root} }

func (g execGit) run(ctx context.Context, args ...string) ([]byte, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // argumen dari kode devtools; rev dari tag repo
	cmd.Dir = g.root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return out, stderr.String(), err
}

func (g execGit) Tags(ctx context.Context, pattern string) ([]string, error) {
	out, stderr, err := g.run(ctx, "tag", "--list", pattern)
	if err != nil {
		return nil, fmt.Errorf("git tag --list %s: %w: %s", pattern, err, strings.TrimSpace(stderr))
	}
	var tags []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			tags = append(tags, l)
		}
	}
	return tags, nil
}

func (g execGit) Show(ctx context.Context, rev, path string) ([]byte, error) {
	// Keberadaan path ditentukan dari keluaran `git ls-tree` (kosong = tidak ada), bukan dari teks pesan error git yang
	// bergantung locale (K-10). rev yang tidak valid membuat ls-tree gagal (exit non-nol) dan dilaporkan sebagai error biasa.
	listed, stderr, err := g.run(ctx, "ls-tree", "--name-only", rev, "--", path)
	if err != nil {
		return nil, fmt.Errorf("git ls-tree %s -- %s: %w: %s", rev, path, err, strings.TrimSpace(stderr))
	}
	if strings.TrimSpace(string(listed)) == "" {
		return nil, fmt.Errorf("%s:%s: %w", rev, path, fs.ErrNotExist)
	}
	out, stderr, err := g.run(ctx, "show", rev+":"+path)
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w: %s", rev, path, err, strings.TrimSpace(stderr))
	}
	return out, nil
}

package contracttools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// BunRunner menjalankan `bun <args>` di dir dan mengembalikan stdout; stderr diteruskan ke stderr proses.
type BunRunner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

type execBun struct{ path string }

// FindBun mencari bun di PATH.
func FindBun() (BunRunner, error) {
	p, err := exec.LookPath("bun")
	if err != nil {
		return nil, errors.New("bun tidak ditemukan; jalankan `erpora doctor`")
	}
	return execBun{path: p}, nil
}

func (b execBun) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, b.path, args...) //nolint:gosec // bun dari PATH, argumen dari kode devtools
	cmd.Dir = dir
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bun %v: %w", args, err)
	}
	return out, nil
}

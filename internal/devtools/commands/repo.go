package commands

import (
	"errors"
	"os"
	"path/filepath"
)

// repoRoot mencari root repo (direktori yang memuat go.work) dari direktori kerja ke atas.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("root repo tidak ditemukan (go.work); jalankan dari dalam repo ERPora")
		}
		dir = parent
	}
}

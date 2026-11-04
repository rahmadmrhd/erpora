package migrate

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"erpora/internal/devtools/gen"
)

// FileName menyusun nama berkas migrasi berikutnya NNNN_<nama>.sql (konvensi make:migration) untuk FS
// direktori migrations modul. Nama harus snake_case (^[a-z][a-z0-9_]{0,62}$); direktori kosong atau tidak ada
// dimulai dari 0001.
func FileName(fsys fs.FS, name string) (string, error) {
	snake, err := gen.ParseSnakeName(name)
	if err != nil {
		return "", err
	}
	n, err := gen.NextMigrationNumber(fsys)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%04d_%s.sql", n, snake), nil
}

// HasTodo melaporkan apakah isi migrasi masih memuat penanda TodoMarker.
func HasTodo(sql string) bool { return strings.Contains(sql, TodoMarker) }

// TodoRef menunjuk satu penanda TODO pada berkas migrasi.
type TodoRef struct {
	File string
	Line int
}

// FindTodos memindai berkas .sql di akar fsys (direktori migrations modul) dan mengembalikan setiap baris
// yang memuat TodoMarker, terurut. Dipakai metadata:check: migrasi bangkitan yang belum dilengkapi
// menggagalkan pemeriksaan (Requirement 11.3).
func FindTodos(fsys fs.FS) ([]TodoRef, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []TodoRef
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		b, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, TodoMarker) {
				out = append(out, TodoRef{File: e.Name(), Line: i + 1})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

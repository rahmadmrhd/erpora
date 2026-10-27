package gen

import (
	"fmt"
	"os"
	"path/filepath"
)

// MakeMigration membangun Plan satu file migrasi goose. platform "" = migrasi modul (modules/<dir>/migrations);
// "control" atau "tenant" = migrasi platform (migrations/<target>; argumen module diabaikan). Modul dengan
// migrations.strategy "none" ditolak dengan instruksi.
func MakeMigration(root, module, name string, platform string) (Plan, error) {
	snake, err := ParseSnakeName(name)
	if err != nil {
		return Plan{}, err
	}
	var dir, tmpl string
	data := map[string]string{"Name": snake}
	switch platform {
	case "":
		ref, err := ResolveModule(root, module)
		if err != nil {
			return Plan{}, err
		}
		if ref.Strategy != "versioned" {
			return Plan{}, fmt.Errorf("modul %s memakai migrations.strategy %q: ubah menjadi \"versioned\" di modules/%s/module.json "+
				"dan tambahkan Migrations() (go:embed migrations/*.sql) di module.go sebelum membuat migrasi", ref.Key, ref.Strategy, ref.Dir)
		}
		dir = "modules/" + ref.Dir + "/migrations"
		tmpl = "migration/module.sql.tmpl"
		data["Slug"] = ref.Slug
	case "control", "tenant":
		dir = "migrations/" + platform
		tmpl = "migration/platform.sql.tmpl"
		data["Target"] = platform
	default:
		return Plan{}, fmt.Errorf("--platform %q tidak dikenal; pilih control atau tenant", platform)
	}
	n, err := NextMigrationNumber(os.DirFS(filepath.Join(root, filepath.FromSlash(dir))))
	if err != nil {
		if os.IsNotExist(err) {
			n = 1
		} else {
			return Plan{}, err
		}
	}
	content, err := Render(tmpl, data)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Ops: []FileOp{{Path: fmt.Sprintf("%s/%04d_%s.sql", dir, n, snake), Kind: OpCreate, Content: content}}}, nil
}

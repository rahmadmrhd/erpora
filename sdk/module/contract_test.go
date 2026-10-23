package module_test

import (
	"context"
	"io/fs"
	"net/http"
	"testing"

	"erpora/sdk/module"
)

type fakeModule struct{}

func (fakeModule) Manifest() module.Manifest                                 { return module.Manifest{} }
func (fakeModule) Register(module.Registrar) error                           { return nil }
func (fakeModule) Migrations() fs.FS                                         { return nil }
func (fakeModule) Install(context.Context, module.Env) error                 { return nil }
func (fakeModule) Upgrade(context.Context, module.Env, string, string) error { return nil }
func (fakeModule) OnEnable(context.Context, module.Env) error                { return nil }
func (fakeModule) OnDisable(context.Context, module.Env) error               { return nil }
func (fakeModule) Health(context.Context, module.Env) error                  { return nil }
func (fakeModule) SeedDev(context.Context, module.Env) error                 { return nil }

type fakeRegistrar struct{ commands []module.Command }

func (*fakeRegistrar) Handle(method, path, permission string, h http.Handler) {}
func (*fakeRegistrar) Public(method, path string, h http.Handler)             {}
func (r *fakeRegistrar) Command(c module.Command)                             { r.commands = append(r.commands, c) }

var (
	_ module.Module          = fakeModule{}
	_ module.MigrationSource = fakeModule{}
	_ module.Installer       = fakeModule{}
	_ module.Upgrader        = fakeModule{}
	_ module.EnableHook      = fakeModule{}
	_ module.DisableHook     = fakeModule{}
	_ module.HealthChecker   = fakeModule{}
	_ module.DevSeeder       = fakeModule{}
	_ module.Registrar       = (*fakeRegistrar)(nil)
)

// Registry wajib menolak Command dengan Run nil; zero value memang nil.
func TestCommand_ZeroValueRunNil(t *testing.T) {
	if (module.Command{}).Run != nil {
		t.Fatal("Command{}.Run harus nil")
	}
}

package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"erpora/internal/platform/config"
	"erpora/internal/platform/modules/registry"
)

func coreCommand(t *testing.T, name string) registry.CommandInfo {
	t.Helper()
	reg, err := BuildRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range reg.Commands() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("command %s tidak terdaftar", name)
	return registry.CommandInfo{}
}

// Command core:* menyegel/membuka data identitas: tanpa ERP_SECRET_KEY valid ditolak SEBELUM database dibuka (exit 2),
// di semua mode termasuk development. Command modul lain tidak terpengaruh (tes integrasi mencakupnya).
func TestRunModuleCommand_CoreRequiresSecretKey(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, name := range []string{"core:create-admin", "core:rotate-keys", "core:reset-mfa", "core:unlock", "core:settings:set"} {
		for _, env := range []config.Env{config.EnvDevelopment, config.EnvProduction} {
			t.Run(name+"/"+string(env), func(t *testing.T) {
				var out bytes.Buffer
				err := RunModuleCommand(context.Background(), config.Config{Env: env}, log, CommandRun{
					Command: coreCommand(t, name), Flags: map[string]string{"email": "a@b.test"}, Out: &out,
					In: strings.NewReader("kata-sandi-uji-panjang-2026\n"), Actor: "cli:tester",
				})
				var xe *ExitError
				if !errors.As(err, &xe) || xe.Code != 2 || !strings.Contains(err.Error(), "ERP_SECRET_KEY") {
					t.Fatalf("err = %v, want exit 2 menyebut ERP_SECRET_KEY", err)
				}
				if out.Len() != 0 {
					t.Errorf("tidak boleh ada keluaran: %q", out.String())
				}
			})
		}
	}
}

package app

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"erpora/internal/platform/config"
	"erpora/modules/example"
	"erpora/sdk/module"
)

// M6: katalog rusak membuat Run berhenti dengan ExitError{Code: 2} sebelum akses database.
func TestRun_BrokenCatalogExits2BeforeDB(t *testing.T) {
	orig := compiledModulesFn
	compiledModulesFn = func() []module.Module { return []module.Module{example.New(), example.New()} } // key ganda
	defer func() { compiledModulesFn = orig }()

	cfg := config.Config{
		DatabaseURL: "postgres://u:p@127.0.0.1:1/db?sslmode=disable", // port tertutup: akses DB akan gagal berbeda
		Env:         config.EnvDevelopment, LogFormat: "text", TenancyMode: "single", HTTPAddr: "127.0.0.1:0",
		AuthzStub: "deny",
	}
	err := Run(context.Background(), cfg, io.Discard)
	var xe *ExitError
	if !errors.As(err, &xe) || xe.Code != 2 {
		t.Fatalf("err = %v, diharapkan *ExitError{Code: 2}", err)
	}
	if !strings.Contains(err.Error(), "katalog modul tidak valid") || !strings.Contains(err.Error(), "duplicate_key") {
		t.Errorf("pesan = %q", err.Error())
	}
}

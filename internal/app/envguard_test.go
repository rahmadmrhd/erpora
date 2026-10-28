package app

import (
	"errors"
	"testing"

	"erpora/internal/platform/config"
)

func TestDecideDevWrite_Table(t *testing.T) {
	envs := []config.Env{config.EnvDevelopment, config.EnvProduction}
	markers := []struct {
		marker config.Env
		has    bool
	}{{"", false}, {config.EnvDevelopment, true}, {config.EnvProduction, true}}

	// Binary release tidak pernah ditolak, apa pun kombinasinya.
	for _, m := range markers {
		for _, e := range envs {
			if err := decideDevWrite("release", m.marker, m.has, e); err != nil {
				t.Errorf("release marker=%q has=%v env=%s: err = %v, want nil", m.marker, m.has, e, err)
			}
		}
	}

	cases := []struct {
		name    string
		marker  config.Env
		has     bool
		env     config.Env
		refused bool
	}{
		{"marker production, env development", config.EnvProduction, true, config.EnvDevelopment, true},
		{"marker production, env production", config.EnvProduction, true, config.EnvProduction, true},
		{"marker development, env production (marker menentukan)", config.EnvDevelopment, true, config.EnvProduction, false},
		{"marker development, env development", config.EnvDevelopment, true, config.EnvDevelopment, false},
		{"tanpa marker, env production (keputusan 4)", "", false, config.EnvProduction, true},
		{"tanpa marker, env development", "", false, config.EnvDevelopment, false},
	}
	for _, c := range cases {
		err := decideDevWrite("dev", c.marker, c.has, c.env)
		if c.refused {
			if !errors.Is(err, ErrProductionDatabase) {
				t.Errorf("%s: err = %v, want ErrProductionDatabase", c.name, err)
			}
		} else if err != nil {
			t.Errorf("%s: err = %v, want nil", c.name, err)
		}
	}
}

func TestDestructiveEnvAllowed(t *testing.T) {
	if err := DestructiveEnvAllowed(config.Config{Env: config.EnvDevelopment}); err != nil {
		t.Errorf("development: %v", err)
	}
	err := DestructiveEnvAllowed(config.Config{Env: config.EnvProduction})
	var xe *ExitError
	if !errors.As(err, &xe) || xe.Code != 3 || !errors.Is(err, ErrNotDevelopment) {
		t.Errorf("production: err = %v, want ExitError{3, ErrNotDevelopment}", err)
	}
}

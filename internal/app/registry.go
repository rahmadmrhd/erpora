package app

import (
	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/modules/admin"
	"erpora/internal/platform/modules/registry"
	"erpora/sdk/module"
)

// compiledModulesFn dapat diganti test untuk mensimulasikan katalog rusak.
var compiledModulesFn = compiledModules

// BuildRegistry memvalidasi katalog modul terkompilasi. Dipakai serve, inspect,
// dan test keamanan; tidak membutuhkan database maupun environment.
func BuildRegistry() (*registry.Registry, error) {
	reg, _, err := buildRegistry()
	return reg, err
}

// buildRegistry seperti BuildRegistry, ditambah API manajemen modul (route platform.core) yang di-bind
// ke lifecycle.Service oleh assemble.
func buildRegistry() (*registry.Registry, *admin.API, error) {
	api := admin.New()
	reg, err := registry.Build(registry.Options{
		PlatformVersion:      buildinfo.PlatformVersion,
		SDKVersion:           module.SDKVersion,
		Platform:             api.Register,
		PlatformCapabilities: admin.Capabilities,
	}, compiledModulesFn()...)
	return reg, api, err
}

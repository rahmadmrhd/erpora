package app

import (
	"erpora/internal/platform/buildinfo"
	"erpora/internal/platform/modules/registry"
	"erpora/sdk/module"
)

// compiledModulesFn dapat diganti test untuk mensimulasikan katalog rusak.
var compiledModulesFn = compiledModules

// BuildRegistry memvalidasi katalog modul terkompilasi. Dipakai serve, inspect,
// dan test keamanan; tidak membutuhkan database maupun environment.
func BuildRegistry() (*registry.Registry, error) {
	return registry.Build(registry.Options{
		PlatformVersion: buildinfo.PlatformVersion,
		SDKVersion:      module.SDKVersion,
	}, compiledModulesFn()...)
}

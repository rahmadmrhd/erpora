package app

import (
	"slices"

	"erpora/contracts"
	"erpora/internal/platform/buildinfo"
	i18nadmin "erpora/internal/platform/i18n/admin"
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

// platformAPIs mengumpulkan handler route platform.core yang di-bind ke layanan oleh assemble (route
// didaftarkan lebih dulu lewat registry, layanannya baru ada setelah perakitan).
type platformAPIs struct {
	Modules *admin.API
	I18n    *i18nadmin.API
}

// buildRegistry seperti BuildRegistry, ditambah API route platform.core: manajemen modul (di-bind ke
// lifecycle.Service) dan katalog i18n (di-bind ke i18n.Service) oleh assemble.
func buildRegistry() (*registry.Registry, *platformAPIs, error) {
	apis := &platformAPIs{Modules: admin.New(), I18n: i18nadmin.New()}
	reg, err := registry.Build(registry.Options{
		PlatformVersion: buildinfo.PlatformVersion,
		SDKVersion:      module.SDKVersion,
		Platform: func(r module.Registrar) error {
			if err := apis.Modules.Register(r); err != nil {
				return err
			}
			return apis.I18n.Register(r)
		},
		PlatformCapabilities: append(slices.Clone(admin.Capabilities), i18nadmin.Capabilities...),
		PlatformContract:     contracts.PlatformOpenAPI,
	}, compiledModulesFn()...)
	return reg, apis, err
}

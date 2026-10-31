package desks

import "erpora/sdk/module"

// PlatformDesk adalah Desk bawaan platform (D-9): pseudo-modul platform.core tidak punya module.json, jadi Desk-nya
// didefinisikan di Go dengan tipe yang sama dengan Desk modul. Menu "Modul" membuka /platform/modules. Label ada di
// katalog namespace "platform" (diperiksa platform_test.go; registry tidak memeriksa label Desk platform).
// Dipasang lewat registry.Options.PlatformDesk.
func PlatformDesk() *module.Desk {
	return &module.Desk{
		Label: "desk.title",
		Icon:  "layers",
		Menus: []module.DeskMenu{
			{ID: "modules", Label: "desk.modules", Icon: "boxes", To: "/modules", Permission: "platform.modules.read"},
		},
	}
}

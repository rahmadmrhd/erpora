package app

import (
	"erpora/modules/example"
	"erpora/modules/exampleaddon"
	"erpora/modules/stockopname"
	"erpora/sdk/module"
)

// compiledModules adalah modul yang dikompilasi ke binary. Baris baru
// ditambahkan di atas penanda (PRD-000 FR-22.3); dependensi kontrak antar
// modul dirakit eksplisit di sini.
func compiledModules() []module.Module {
	ex := example.New()
	return []module.Module{
		ex,
		exampleaddon.New(ex.Notes()),
		stockopname.New(),
		// erp:generated:modules
	}
}

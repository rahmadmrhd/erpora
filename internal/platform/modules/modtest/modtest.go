// Package modtest menyediakan pembantu uji untuk membangun deklarasi resource manifest (PRD-001 FR-110) dari
// daftar permission datar. Hanya dipakai test; bukan bagian API modul.
package modtest

import (
	"strings"

	"erpora/sdk/module"
)

// Resources mengelompokkan permission berbentuk "<slug>.<resource>.<aksi>" menjadi resource manifest: aksi standar
// masuk actions, selebihnya custom_actions; setiap resource mendapat satu grup field default. Urutan resource dan
// aksi mengikuti kemunculan pertama. Permission yang bukan tiga segmen diabaikan.
func Resources(perms ...string) []module.Resource {
	out := []module.Resource{}
	index := map[string]int{}
	for _, p := range perms {
		i := strings.LastIndex(p, ".")
		if i < 0 {
			continue
		}
		key, action := p[:i], p[i+1:]
		if !strings.Contains(key, ".") {
			continue
		}
		at, ok := index[key]
		if !ok {
			name := key[strings.Index(key, ".")+1:]
			out = append(out, module.Resource{
				Key: key, Label: "resource." + name,
				FieldGroups: []module.FieldGroup{{Key: "general", Label: "fieldGroup.general", Default: true}},
			})
			at = len(out) - 1
			index[key] = at
		}
		if module.IsStandardAction(action) {
			out[at].Actions = append(out[at].Actions, action)
		} else {
			out[at].CustomActions = append(out[at].CustomActions, module.CustomAction{Key: action, Label: "action." + action})
		}
	}
	for i := range out {
		if out[i].Actions == nil {
			out[i].Actions = []string{}
		}
	}
	return out
}

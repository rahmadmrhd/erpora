package gen

import (
	"bytes"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"erpora/sdk/module"
)

// EnsureManifestPermission memastikan permission perm ("<slug>.<resource>.<aksi>") adalah turunan resources
// module.json (PRD-001 FR-110: capabilities diturunkan dari resource x aksi). Bila belum: aksi standar ditambahkan
// ke actions resource yang sudah ada, atau resource baru (satu grup field default, kunci label "resource.<nama>")
// ditambahkan ke resources. Edit bersifat tekstual (format dan urutan kunci terjaga). newResource berisi kunci
// resource baru (kosong bila tidak ada) agar pemanggil menambah label ke katalog i18n. Aksi non-standar dan
// manifest yang tidak mengikuti format templat menghasilkan galat berisi petunjuk deklarasi manual.
func EnsureManifestPermission(man []byte, perm string) (out []byte, newResource string, err error) {
	m, err := module.ParseManifest(man)
	if err != nil {
		return nil, "", err
	}
	if slices.Contains(m.Permissions(), perm) {
		return man, "", nil
	}
	i := strings.LastIndex(perm, ".")
	if i < 0 {
		return nil, "", fmt.Errorf("permission %q tidak berbentuk <resource>.<aksi>", perm)
	}
	resKey, action := perm[:i], perm[i+1:]
	manual := "deklarasikan di \"resources\" module.json (beserta label i18n) secara manual"
	if !module.IsStandardAction(action) {
		return nil, "", fmt.Errorf("aksi %q pada permission %q bukan aksi standar (%s): %s sebagai custom_actions", action, perm, strings.Join(module.StandardActions, ", "), manual)
	}

	if slices.ContainsFunc(m.Resources, func(r module.Resource) bool { return r.Key == resKey }) {
		re := regexp.MustCompile(`("key":\s*` + regexp.QuoteMeta(jsonString(resKey)) + `[^{}]*?"actions":\s*\[)([^\]]*)\]`)
		loc := re.FindSubmatchIndex(man)
		if loc == nil {
			return nil, "", fmt.Errorf("resource %q tidak dapat diubah otomatis (susunan module.json tidak sesuai templat): %s", resKey, manual)
		}
		inner := strings.TrimSpace(string(man[loc[4]:loc[5]]))
		add := jsonString(action)
		if inner != "" {
			add = ", " + add
		}
		at := loc[4] + len(strings.TrimRight(string(man[loc[4]:loc[5]]), " \t\n\r"))
		return splice(man, at, add), "", nil
	}

	_, name, _ := strings.Cut(resKey, ".")
	obj := "{\n      \"key\": " + jsonString(resKey) + ",\n      \"label\": " + jsonString("resource."+name) + ",\n      \"actions\": [" +
		jsonString(action) + "],\n      \"field_groups\": [{ \"key\": \"general\", \"label\": \"fieldGroup.general\", \"classification\": \"internal\", \"default\": true }]\n    }"
	ms, err := scanObject(man)
	if err != nil {
		return nil, "", err
	}
	res, ok := findMember(ms, "resources")
	if !ok {
		return nil, "", fmt.Errorf("module.json tidak punya \"resources\": %s", manual)
	}
	raw := man[res.valStart:res.valEnd]
	body := bytes.TrimSpace(raw[1 : len(raw)-1])
	if len(body) == 0 {
		return splice(man, res.valStart+1, "\n    "+obj+"\n  "), resKey, nil
	}
	last := res.valStart + 1 + len(bytes.TrimRight(raw[1:len(raw)-1], " \t\n\r"))
	return splice(man, last, ",\n    "+obj), resKey, nil
}

func splice(src []byte, at int, ins string) []byte {
	out := make([]byte, 0, len(src)+len(ins))
	out = append(out, src[:at]...)
	out = append(out, ins...)
	return append(out, src[at:]...)
}

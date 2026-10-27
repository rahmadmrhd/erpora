package apicontract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// bundleURL adalah URL sintetis dokumen bundel di compiler; tidak pernah diakses (tanpa jaringan atau file).
const bundleURL = "erp-bundle://contract.json"

// ResponseSchema mengompilasi skema respons op untuk status (jatuh ke "default" bila status tidak didokumentasikan;
// tidak ada keduanya -> error) dari Bundle(op.Doc) dengan santhosh-tekuri/jsonschema/v6 (draft 2020-12).
// Kata kunci x-erp-* diabaikan; format (uuid, date-time, dst.) ditegakkan. Respons tanpa body bukan skema -> error.
func (s *Set) ResponseSchema(op Operation, status int) (*jsonschema.Schema, error) {
	bundle, err := s.Bundle(op.Doc)
	if err != nil {
		return nil, err
	}
	base := "/paths/" + escapePointerSeg(op.Path) + "/" + strings.ToLower(op.Method) + "/responses/"
	ptr := base + strconv.Itoa(status)
	node, ok := walkPointer(bundle, ptr)
	if !ok {
		ptr = base + "default"
		if node, ok = walkPointer(bundle, ptr); !ok {
			return nil, fmt.Errorf("%s %s: respons %d tidak didokumentasikan dan tidak ada default", op.Method, op.Path, status)
		}
	}
	// Ikuti $ref respons komponen (hasil bundel hanya berisi rujukan lokal).
	for i := 0; ; i++ {
		m, _ := node.(map[string]any)
		ref, isRef := m["$ref"].(string)
		if !isRef {
			break
		}
		if i >= 16 || !strings.HasPrefix(ref, "#/") {
			return nil, fmt.Errorf("%s %s: rantai $ref respons tidak valid (%q)", op.Method, op.Path, ref)
		}
		ptr = ref[1:]
		if node, ok = walkPointer(bundle, ptr); !ok {
			return nil, fmt.Errorf("%s %s: $ref respons %q tidak ditemukan", op.Method, op.Path, ref)
		}
	}
	content, _ := node.(map[string]any)["content"].(map[string]any)
	mt := ""
	if _, ok := content["application/json"]; ok {
		mt = "application/json"
	} else {
		types := make([]string, 0, len(content))
		for k := range content {
			types = append(types, k)
		}
		sort.Strings(types)
		if len(types) > 0 {
			mt = types[0]
		}
	}
	if mt == "" {
		return nil, fmt.Errorf("%s %s: respons %d tidak punya body", op.Method, op.Path, status)
	}
	if _, ok := content[mt].(map[string]any)["schema"]; !ok {
		return nil, fmt.Errorf("%s %s: respons %d (%s) tidak punya schema", op.Method, op.Path, status, mt)
	}
	ptr += "/content/" + escapePointerSeg(mt) + "/schema"

	raw, err := json.Marshal(bundle)
	if err != nil {
		return nil, err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.AssertFormat()
	if err := c.AddResource(bundleURL, doc); err != nil {
		return nil, err
	}
	sch, err := c.Compile(bundleURL + "#" + ptr)
	if err != nil {
		return nil, fmt.Errorf("%s %s respons %d: kompilasi skema gagal: %w", op.Method, op.Path, status, err)
	}
	return sch, nil
}

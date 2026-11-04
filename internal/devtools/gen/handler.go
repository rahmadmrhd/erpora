package gen

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// HandlerOptions mengatur make:handler.
type HandlerOptions struct {
	Module, Name string // Name PascalCase
	Route        string // "METHOD /path" relatif modul, mis. "GET /items/{id}"
	Permission   string // "<slug>.<resource>.<action>"
}

var (
	kebabSegRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)
	permRe     = func(slug string) *regexp.Regexp {
		return regexp.MustCompile(`^` + regexp.QuoteMeta(slug) + `\.[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
	}
)

// handlerKind menentukan bentuk handler dari method dan segmen path (api-guidelines section 2).
func handlerKind(method, path string) (kind string, hasID bool, err error) {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "//") || strings.HasSuffix(path, "/") || strings.ContainsAny(path, "?# \t") {
		return "", false, fmt.Errorf("path %q tidak valid: harus diawali '/', bersih, tanpa trailing slash, query, atau spasi", path)
	}
	segs := strings.Split(path[1:], "/")
	for i, s := range segs {
		if s == "{id}" && i == 1 {
			continue
		}
		if !kebabSegRe.MatchString(s) {
			return "", false, fmt.Errorf("segmen %q pada %s tidak valid: kebab-case huruf kecil, atau tepat {id} sebagai segmen kedua (api-guidelines section 2)", s, path)
		}
	}
	if !strings.HasSuffix(segs[0], "s") {
		return "", false, fmt.Errorf("koleksi %q harus jamak (api-guidelines section 2); resource tunggal ditulis manual dengan x-erp-singleton", segs[0])
	}
	switch len(segs) {
	case 1:
		switch method {
		case http.MethodGet:
			return "get_collection", false, nil
		case http.MethodPost:
			return "post_create", false, nil
		}
		return "", false, fmt.Errorf("%s %s tidak didukung: koleksi hanya GET (daftar) dan POST (buat)", method, path)
	case 2:
		if segs[1] != "{id}" {
			return "", false, fmt.Errorf("path %s tidak didukung: bentuknya /<koleksi>, /<koleksi>/{id}, atau /<koleksi>/{id}/<aksi>", path)
		}
		switch method {
		case http.MethodGet:
			return "get_item", true, nil
		case http.MethodPatch:
			return "patch", true, nil
		case http.MethodDelete:
			return "delete", true, nil
		}
		return "", false, fmt.Errorf("%s %s tidak didukung: resource hanya GET, PATCH, DELETE; aksi memakai POST %s/<aksi> (api-guidelines section 2, 3)", method, path, path)
	case 3:
		if segs[1] == "{id}" && method == http.MethodPost {
			return "post_action", true, nil
		}
	}
	return "", false, fmt.Errorf("%s %s tidak didukung: bentuknya /<koleksi>, /<koleksi>/{id}, atau POST /<koleksi>/{id}/<aksi>", method, path)
}

// handlerData adalah data template make:handler.
type handlerData struct {
	Mod            layerMod
	Name, Camel    string
	Method         string
	Path           string
	BasePath       string
	Kind           string
	HasID          bool
	HasBody        bool
	BodyRequired   bool
	Permission     string
	OperationID    string
	SuccessCode    string
	SampleBody     string
	RequestExample string
	Decode         string
	FailFunc       string
}

// MakeHandler membangun Plan satu endpoint: handler + test (OpCreate), registrasi route di module.go, resource/aksi di
// module.json (permission = turunan resource x aksi, FR-110), dan operasi + skema di openapi.yaml (yaml.Node). Validasi (method, path, permission, operasi sudah ada)
// dilakukan sebelum Plan. Handler ber-ID selalu disertai kerangka test IDOR.
func MakeHandler(root string, o HandlerOptions) (Plan, error) {
	ref, mod, err := resolveLayerModule(root, o.Module)
	if err != nil {
		return Plan{}, err
	}
	t, err := ParseTypeName(o.Name)
	if err != nil {
		return Plan{}, err
	}
	method, path, ok := strings.Cut(o.Route, " ")
	method, path = strings.TrimSpace(method), strings.TrimSpace(path)
	if !ok || path == "" || strings.HasPrefix(method, "/") {
		return Plan{}, fmt.Errorf("route %q tidak valid: bentuknya \"METHOD /path\", mis. \"GET /items/{id}\"", o.Route)
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete:
	case http.MethodPut:
		return Plan{}, fmt.Errorf("PUT ditolak (api-guidelines section 3): pakai PATCH untuk perubahan sebagian, atau tandai x-erp-allow-put manual di kontrak")
	default:
		return Plan{}, fmt.Errorf("method %q tidak didukung; pilih GET, POST, PATCH, atau DELETE", method)
	}
	kind, hasID, err := handlerKind(method, path)
	if err != nil {
		return Plan{}, err
	}
	if !permRe(ref.Slug).MatchString(o.Permission) {
		return Plan{}, fmt.Errorf("permission %q tidak valid: harus berbentuk %s.<resource>.<action> (berprefiks slug modul)", o.Permission, ref.Slug)
	}

	fullPath := "/api/" + ref.Slug + path
	specPath := "modules/" + ref.Dir + "/openapi.yaml"
	spec, err := readRepoFile(root, specPath)
	if err != nil {
		return Plan{}, err
	}
	if exists, err := HasOperation(spec, fullPath, strings.ToLower(method)); err != nil {
		return Plan{}, err
	} else if exists {
		return Plan{}, fmt.Errorf("operasi %s %s sudah ada di %s", method, fullPath, specPath)
	}

	hasBody := kind == "post_create" || kind == "post_action" || kind == "patch"
	d := handlerData{
		Mod: mod, Name: t.Pascal, Camel: t.Camel, Method: method, Path: path, BasePath: "/api/" + ref.Slug, Kind: kind,
		HasID: hasID, HasBody: hasBody, BodyRequired: kind != "post_action", Permission: o.Permission,
		OperationID: t.Camel + moduleNamePascal(ref.Slug), SuccessCode: "200",
		Decode: "httpx.DecodeJSON[" + t.Pascal + "Request]",
	}
	switch kind {
	case "post_create":
		d.SuccessCode = "201"
		d.SampleBody = `{"title":"Contoh"}`
		d.RequestExample = "{ title: Contoh }"
	case "patch":
		d.SampleBody = `{"title":"Contoh"}`
		d.RequestExample = "{ title: Contoh }"
	case "post_action":
		d.SampleBody = `{"reason":"Contoh"}`
		d.RequestExample = "{ reason: Contoh }"
	case "delete":
		d.SuccessCode = "204"
	}
	d.FailFunc = failFunc(kind, t.Pascal)

	base := "modules/" + ref.Dir + "/internal/presentation/http/" + t.Snake
	files := [][2]string{
		{base + ".go", "handler/" + kind + ".go.tmpl"},
		{base + "_test.go", "handler/handler_test.go.tmpl"},
	}
	// Template test memakai Kind untuk memilih kerangka; selalu satu template.
	plan, err := renderOps(files, d)
	if err != nil {
		return Plan{}, err
	}

	// module.go: impor + registrasi route di atas penanda.
	modPath := "modules/" + ref.Dir + "/module.go"
	src, err := readRepoFile(root, modPath)
	if err != nil {
		return Plan{}, err
	}
	opts := "module.Classify(module.ClassInternal)"
	if hasBody {
		opts += ", module.MaxBodyBytes(64<<10)"
	}
	line := fmt.Sprintf("r.Handle(http.Method%s, %q, %q, httpapi.%s(), %s)", titleASCII(method), path, o.Permission, t.Pascal, opts)
	out, err := InsertAtMarker(modPath, src, "// erp:generated:routes", line)
	if err != nil {
		return Plan{}, err
	}
	for _, imp := range [][2]string{{"", "net/http"}, {"httpapi", ref.GoPath + "/internal/presentation/http"}, {"", "erpora/sdk/module"}} {
		if out, err = AddGoImport(modPath, out, imp[0], imp[1]); err != nil {
			return Plan{}, err
		}
	}
	plan.Ops = append(plan.Ops, FileOp{Path: modPath, Kind: OpModify, Content: out})

	// module.json: permission harus turunan resource x aksi (PRD-001 FR-110); resource/aksi baru ditambahkan, beserta
	// label resource di katalog i18n.
	manPath := "modules/" + ref.Dir + "/module.json"
	man, err := readRepoFile(root, manPath)
	if err != nil {
		return Plan{}, err
	}
	man, newResource, err := EnsureManifestPermission(man, o.Permission)
	if err != nil {
		return Plan{}, fmt.Errorf("%s: %w", manPath, err)
	}
	plan.Ops = append(plan.Ops, FileOp{Path: manPath, Kind: OpModify, Content: man})
	if newResource != "" {
		catOps, err := resourceLabelOps(root, ref.Dir, newResource)
		if err != nil {
			return Plan{}, err
		}
		plan.Ops = append(plan.Ops, catOps...)
	}

	// openapi.yaml: operasi dan skema.
	opText, err := Render("handler/operation.yaml.tmpl", d)
	if err != nil {
		return Plan{}, err
	}
	schemaText, err := Render("handler/schemas.yaml.tmpl", d)
	if err != nil {
		return Plan{}, err
	}
	if _, err := parseYAMLMapping(opText); err != nil {
		return Plan{}, fmt.Errorf("operasi hasil template: %w", err)
	}
	if spec, err = AddComponentSchemasText(spec, schemaText); err != nil {
		return Plan{}, err
	}
	if spec, err = AddOperationText(spec, fullPath, strings.ToLower(method), opText); err != nil {
		return Plan{}, err
	}
	plan.Ops = append(plan.Ops, FileOp{Path: specPath, Kind: OpModify, Content: spec})
	if ref.HasWeb {
		// Tipe TS kontrak modul di-commit; operasi baru membuatnya basi.
		plan.Post = append(plan.Post, generatePost(ref.Dir))
	}
	plan.NoForce = true
	return plan, nil
}

// moduleNamePascal mengubah slug (kebab) menjadi PascalCase, dipakai untuk operationId unik lintas modul.
func moduleNamePascal(slug string) string {
	var b strings.Builder
	for _, w := range strings.Split(slug, "-") {
		if w != "" {
			b.WriteString(strings.ToUpper(w[:1]) + w[1:])
		}
	}
	return b.String()
}

func titleASCII(method string) string {
	return method[:1] + strings.ToLower(method[1:])
}

// failFunc membangun fungsi uji yang selalu gagal dengan detail internal, sesuai tanda tangan titik sambung kind.
func failFunc(kind, name string) string {
	const boom = `errors.New("SELECT rahasia")`
	switch kind {
	case "get_item":
		return "func(context.Context, string) (" + name + "Data, error) { return " + name + "Data{}, " + boom + " }"
	case "get_collection":
		return "func(context.Context, int, int) ([]" + name + "Data, int64, error) { return nil, 0, " + boom + " }"
	case "post_create":
		return "func(context.Context, string, " + name + "Request) (" + name + "Data, error) { return " + name + "Data{}, " + boom + " }"
	case "post_action":
		return "func(context.Context, string, string, string, " + name + "Request) (" + name + "Data, error) { return " + name + "Data{}, " + boom + " }"
	case "patch":
		return "func(context.Context, string, string, " + name + "Request) (" + name + "Data, error) { return " + name + "Data{}, " + boom + " }"
	default: // delete
		return "func(context.Context, string, string) error { return " + boom + " }"
	}
}

// resourceLabelOps menambah label resource baru (resource.<nama>) dan grup field default (fieldGroup.general) ke
// katalog id dan en modul bila berkasnya ada; teks awal diturunkan dari nama resource agar dapat disunting.
func resourceLabelOps(root, dir, resourceKey string) ([]FileOp, error) {
	_, name, _ := strings.Cut(resourceKey, ".")
	title := titleCase(strings.NewReplacer("-", " ", "_", " ").Replace(name))
	var ops []FileOp
	for _, loc := range []struct{ code, general string }{{"id", "Umum"}, {"en", "General"}} {
		rel := "modules/" + dir + "/i18n/" + loc.code + ".json"
		src, err := readRepoFile(root, rel)
		if err != nil {
			continue // modul tanpa katalog bahasa ini: tidak ada yang diubah
		}
		out, err := AddJSONDependency(src, "resource", name, title)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		if out, err = AddJSONDependency(out, "fieldGroup", "general", loc.general); err != nil {
			return nil, fmt.Errorf("%s: %w", rel, err)
		}
		ops = append(ops, FileOp{Path: rel, Kind: OpModify, Content: out})
	}
	return ops, nil
}

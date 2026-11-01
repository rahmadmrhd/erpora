package generate

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"erpora/internal/devtools/metadatatools/check"
	"erpora/internal/devtools/metadatatools/naming"
	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/sdk/metadata"
)

var (
	enumValueRe   = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	resourceKeyRe = regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z][A-Za-z0-9]*$`)
	currencyRe    = regexp.MustCompile(`^[A-Z]{3}$`)
	canonDecRe    = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?$`)
	canonIntRe    = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)
	// goFieldNameReserved adalah anggota DTO yang tidak boleh ditimpa nama field.
	goFieldNameReserved = map[string]bool{"Present": true}
)

// maxSafeInt adalah batas bilangan bulat aman (2^53-1), selaras evaluator.
const maxSafeInt = 1<<53 - 1

var knownTypes = map[string]metadata.FieldType{
	"string": metadata.TypeString, "text": metadata.TypeText, "integer": metadata.TypeInteger,
	"decimal": metadata.TypeDecimal, "boolean": metadata.TypeBoolean, "date": metadata.TypeDate,
	"datetime": metadata.TypeDateTime, "enum": metadata.TypeEnum, "uuid": metadata.TypeUUID,
}

// fld adalah satu field yang siap dibangkitkan (atribut sudah dinormalkan dan divalidasi ulang).
type fld struct {
	Name   string
	GoName string
	Type   string

	Nullable, ReadOnly, Computed bool
	Required                     bool
	HasDefault                   bool
	Default                      any // nil, bool, string, json.Number (integer)

	MinLength, MaxLength int
	Pattern, Format      string
	Enum                 []string
	Extensible           bool
	Minimum, Maximum     string // teks kanonik ("" = tidak ada)
	Precision, Scale     int
	Currency             string

	Class, Group  string
	Filterable    bool
	Sortable      bool
	Groupable     bool
	Aggregatable  bool
	Deprecated    bool
	DeprecatedMap map[string]string // since, replacedBy
	MigrateFrom   string
	Description   string
	Label         string

	RequiredWhen, ReadonlyWhen, VisibleWhen *metadata.Condition
	// ServerCond menandai kondisi tingkat field yang menurut check hanya server (tidak ikut ke klien).
	ServerCond map[string]bool

	ClientVisible bool
	DomainType    string
	Widget        string
	Display       string
	Operators     []string
	AccessorKind  string
	AccessorExpr  any

	// Morph menandai kolom tipe morph: nilai berupa kunci resource manifest (bertitik), bukan enum snake_case.
	Morph bool
	// Column menandai kolom relasi sintetis (belongsTo: <nama>Id; morphTo: <nama>Type dan <nama>Id).
	Column bool
}

// Writable melaporkan apakah field menjadi masukan tulis (masuk DTO create dan update).
func (f *fld) Writable() bool { return !f.ReadOnly && !f.Computed }

type ruleModel struct {
	ID, Code, Field, Message string
	Assert                   *metadata.Condition
	ServerOnly               bool
}

type relModel struct {
	Name  string
	Attrs map[string]any
}

// resModel adalah resource yang siap dibangkitkan.
type resModel struct {
	Res          *resolve.Resolved
	Name         string // bagian nama pada kunci (camelCase)
	Pascal       string
	Snake        string
	Fields       []*fld
	Rules        []ruleModel
	LockWhen     *metadata.Condition
	LockServer   bool // lockWhen hanya server menurut check
	LabelTpl     string
	DefaultGroup string
	byName       map[string]*fld
}

func inputErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInput, fmt.Sprintf(format, a...))
}

func attrString(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func attrBool(m map[string]any, k string) bool {
	b, _ := m[k].(bool)
	return b
}

func attrInt(m map[string]any, k string) (int, bool, error) {
	v, ok := m[k]
	if !ok {
		return 0, false, nil
	}
	var n int64
	var err error
	switch t := v.(type) {
	case json.Number:
		n, err = t.Int64()
	case int:
		n = int64(t)
	case int64:
		n = t
	default:
		err = fmt.Errorf("bukan bilangan bulat")
	}
	if err != nil || n < 0 || n > 1<<31 {
		return 0, false, inputErr("atribut %s bukan bilangan bulat yang sah", k)
	}
	return int(n), true, nil
}

// numText mengembalikan teks bilangan dari json.Number atau string.
func numText(v any) (string, bool) {
	switch t := v.(type) {
	case json.Number:
		return t.String(), true
	case string:
		return t, true
	}
	return "", false
}

// canonicalDecimal menerima hanya string desimal kanonik (tanpa eksponen, tanpa nol di depan yang tak perlu).
func canonicalDecimal(s string) (string, bool) {
	if !canonDecRe.MatchString(s) {
		return "", false
	}
	if _, ok := metadata.ParseDecimal(s); !ok {
		return "", false
	}
	return s, true
}

func conditionOf(attrs map[string]any, key string) (*metadata.Condition, error) {
	raw, ok := attrs[key]
	if !ok || raw == nil {
		return nil, nil
	}
	c, err := metadata.ParseCondition(raw)
	if err != nil {
		return nil, inputErr("kondisi %s tidak sah: %v", key, err)
	}
	return c, nil
}

// normalizeDefault memvalidasi default menurut tipe dan mengembalikan nilai kanonik.
func normalizeDefault(typ string, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch typ {
	case "boolean":
		if b, ok := v.(bool); ok {
			return b, nil
		}
	case "integer":
		if s, ok := numText(v); ok && canonIntRe.MatchString(s) {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil && n <= maxSafeInt && n >= -maxSafeInt {
				return json.Number(s), nil
			}
		}
	case "decimal":
		if s, ok := numText(v); ok {
			if c, ok := canonicalDecimal(s); ok {
				return c, nil
			}
		}
	case "string", "text", "enum", "uuid", "date", "datetime":
		if s, ok := v.(string); ok {
			return s, nil
		}
	}
	return nil, inputErr("default tidak sesuai tipe %s", typ)
}

// newFld membangun fld dari atribut mentah. label bawaan dipakai bila atribut label tidak ada.
func newFld(name string, attrs map[string]any, defLabel string, group string, class string) (*fld, error) {
	if errs := naming.ValidateIdentifier(name); errs != nil {
		return nil, inputErr("nama field %q tidak sah: %v", name, errs)
	}
	typ := attrString(attrs, "type")
	if _, ok := knownTypes[typ]; !ok {
		return nil, inputErr("field %s: tipe %q tidak dikenal", name, typ)
	}
	if class == "highly_confidential" {
		return nil, inputErr("field %s berklasifikasi highly_confidential belum didukung (K17)", name)
	}
	if class != "public" && class != "internal" && class != "confidential" {
		return nil, inputErr("field %s: klasifikasi %q tidak sah", name, class)
	}
	f := &fld{
		Name: name, Type: typ, Class: class, Group: group,
		Nullable:      attrBool(attrs, "nullable"),
		Extensible:    attrBool(attrs, "extensible"),
		Filterable:    attrBool(attrs, "filterable"),
		Sortable:      attrBool(attrs, "sortable"),
		Groupable:     attrBool(attrs, "groupable"),
		Aggregatable:  attrBool(attrs, "aggregatable"),
		Pattern:       attrString(attrs, "pattern"),
		Format:        attrString(attrs, "format"),
		Currency:      attrString(attrs, "currency"),
		MigrateFrom:   attrString(attrs, "migrateFrom"),
		Description:   attrString(attrs, "description"),
		Label:         attrString(attrs, "label"),
		ClientVisible: true,
		DomainType:    attrString(attrs, "domainType"),
	}
	if f.Label == "" {
		f.Label = defLabel
	}
	if v, ok := attrs["clientVisible"].(bool); ok && !v {
		f.ClientVisible = false
	}
	if acc, ok := attrs["accessor"].(map[string]any); ok {
		f.Computed = true
		f.AccessorKind = attrString(acc, "kind")
		f.AccessorExpr = acc["expr"]
	}
	f.ReadOnly = attrBool(attrs, "readOnly") || f.Computed
	var err error
	if f.MinLength, _, err = attrInt(attrs, "minLength"); err != nil {
		return nil, err
	}
	if f.MaxLength, _, err = attrInt(attrs, "maxLength"); err != nil {
		return nil, err
	}
	if f.Precision, _, err = attrInt(attrs, "precision"); err != nil {
		return nil, err
	}
	if f.Scale, _, err = attrInt(attrs, "scale"); err != nil {
		return nil, err
	}
	if f.Pattern != "" {
		if perr := metadata.ValidatePattern(f.Pattern); perr != nil {
			return nil, inputErr("field %s: pattern di luar subset: %v", name, perr)
		}
	}
	if f.Format != "" && f.Format != "email" && f.Format != "url" {
		return nil, inputErr("field %s: format %q tidak dikenal", name, f.Format)
	}
	if f.Currency != "" && !currencyRe.MatchString(f.Currency) {
		return nil, inputErr("field %s: currency tidak sah", name)
	}
	for _, k := range []string{"minimum", "maximum"} {
		v, ok := attrs[k]
		if !ok {
			continue
		}
		s, ok := numText(v)
		if !ok {
			return nil, inputErr("field %s: %s bukan bilangan", name, k)
		}
		c, ok := canonicalDecimal(s)
		if !ok {
			return nil, inputErr("field %s: %s bukan bilangan desimal kanonik", name, k)
		}
		if k == "minimum" {
			f.Minimum = c
		} else {
			f.Maximum = c
		}
	}
	if raw, ok := attrs["values"].([]any); ok {
		for _, e := range raw {
			s, ok := e.(string)
			if !ok || !enumValueRe.MatchString(s) {
				return nil, inputErr("field %s: nilai enum tidak sah (harus snake_case)", name)
			}
			f.Enum = append(f.Enum, s)
		}
	}
	if typ == "enum" && len(f.Enum) == 0 {
		return nil, inputErr("field %s: enum tanpa nilai", name)
	}
	if d, ok := attrs["default"]; ok {
		nd, derr := normalizeDefault(typ, d)
		if derr != nil {
			return nil, fmt.Errorf("field %s: %w", name, derr)
		}
		f.HasDefault, f.Default = true, nd
	}
	if dep, ok := attrs["deprecated"].(map[string]any); ok {
		f.Deprecated = true
		f.DeprecatedMap = map[string]string{"since": attrString(dep, "since")}
		if r := attrString(dep, "replacedBy"); r != "" {
			f.DeprecatedMap["replacedBy"] = r
		}
	}
	for key, dst := range map[string]**metadata.Condition{
		"requiredWhen": &f.RequiredWhen, "readonlyWhen": &f.ReadonlyWhen, "visibleWhen": &f.VisibleWhen,
	} {
		c, cerr := conditionOf(attrs, key)
		if cerr != nil {
			return nil, fmt.Errorf("field %s: %w", name, cerr)
		}
		*dst = c
	}
	f.Required = f.Writable() && !f.Nullable && !f.HasDefault
	return f, nil
}

func relationColumns(res *resolve.Resolved, defaultGroup string, slug, name string) ([]*fld, error) {
	var out []*fld
	for _, rel := range res.Relations {
		typ := attrString(rel.Attrs, "type")
		required := attrBool(rel.Attrs, "required")
		mk := func(col, ftype string, enum []string) (*fld, error) {
			// Kolom tipe morph berisi kunci resource manifest (bertitik), bukan nilai enum snake_case; dibangun
			// sebagai string lalu ditandai enum agar tidak tunduk pada pola nilai enum.
			baseType := ftype
			if baseType == "enum" {
				baseType = "string"
			}
			attrs := map[string]any{"type": baseType, "nullable": !required}
			f, err := newFld(col, attrs, slug+"."+name+"."+col, defaultGroup, "internal")
			if err != nil {
				return nil, err
			}
			f.Column = true
			if ftype == "enum" {
				f.Type, f.Enum, f.Morph = "enum", enum, true
			}
			return f, nil
		}
		switch typ {
		case "belongsTo":
			f, err := mk(rel.Name+"Id", "uuid", nil)
			if err != nil {
				return nil, err
			}
			out = append(out, f)
		case "morphTo":
			var enum []string
			list, _ := rel.Attrs["types"].([]any)
			for _, e := range list {
				k, _ := e.(string)
				if !resourceKeyRe.MatchString(k) {
					return nil, inputErr("relasi %s: kunci tipe morph %q tidak sah", rel.Name, k)
				}
				mk2, err := naming.ManifestResourceKey(k)
				if err != nil {
					return nil, inputErr("relasi %s: %v", rel.Name, err)
				}
				enum = append(enum, mk2)
			}
			if len(enum) == 0 {
				return nil, inputErr("relasi %s: morphTo tanpa tipe", rel.Name)
			}
			ft, err := mk(rel.Name+"Type", "enum", enum)
			if err != nil {
				return nil, err
			}
			fi, err := mk(rel.Name+"Id", "uuid", nil)
			if err != nil {
				return nil, err
			}
			out = append(out, ft, fi)
		}
	}
	return out, nil
}

// buildModel membangun model satu resource dan memvalidasi ulang masukan (pertahanan berlapis: metadata:check
// seharusnya sudah menolak semuanya; di sini gagal tertutup).
func buildModel(res *resolve.Resolved, info *check.ResourceInfo) (*resModel, error) {
	slug, name, err := naming.SplitResourceKey(res.Key)
	if err != nil || !resourceKeyRe.MatchString(res.Key) {
		return nil, inputErr("kunci resource %q tidak sah", res.Key)
	}
	pascal, err := naming.ToPascal(name)
	if err != nil {
		return nil, inputErr("resource %s: %v", res.Key, err)
	}
	snake, err := naming.ToSnake(name)
	if err != nil {
		return nil, inputErr("resource %s: %v", res.Key, err)
	}
	m := &resModel{Res: res, Name: name, Pascal: pascal, Snake: snake, DefaultGroup: manifestGroup(res.DefaultGroup), byName: map[string]*fld{}}
	if m.DefaultGroup == "" {
		m.DefaultGroup = resolve.DefaultGroupName
	}
	for _, rf := range res.Fields {
		attrs := map[string]any{}
		for k, v := range rf.Attrs {
			attrs[k] = v
		}
		if rf.DomainType != "" {
			attrs["domainType"] = domainTypeRef(rf.DomainType)
		}
		group, class := manifestGroup(res.GroupOf(rf)), rf.Classification()
		if info != nil {
			// Accessor mewarisi klasifikasi tertinggi dan grup paling ketat dari field masukannya (Requirement 5.8).
			if g := info.Group[rf.Name]; g != "" {
				group = g
			}
			if c := info.Class[rf.Name]; c != "" {
				class = c
			}
		}
		f, err := newFld(rf.Name, attrs, slug+"."+name+"."+rf.Name, group, class)
		if err != nil {
			return nil, fmt.Errorf("resource %s: %w", res.Key, err)
		}
		if info != nil {
			for _, attr := range []string{"visibleWhen", "requiredWhen", "readonlyWhen"} {
				if info.ConditionScope["/fields/"+rf.Name+"/"+attr] == "server" {
					if f.ServerCond == nil {
						f.ServerCond = map[string]bool{}
					}
					f.ServerCond[attr] = true
				}
			}
		}
		f.Widget, f.Display, f.Operators = rf.Widget, rf.Display, append([]string(nil), rf.Operators...)
		m.Fields = append(m.Fields, f)
	}
	cols, err := relationColumns(res, m.DefaultGroup, slug, name)
	if err != nil {
		return nil, fmt.Errorf("resource %s: %w", res.Key, err)
	}
	m.Fields = append(m.Fields, cols...)
	goNames := map[string]string{}
	for _, f := range m.Fields {
		if _, dup := m.byName[f.Name]; dup {
			return nil, inputErr("resource %s: nama field %q ganda (termasuk kolom relasi)", res.Key, f.Name)
		}
		m.byName[f.Name] = f
		gn, gerr := naming.ToPascal(f.Name)
		if gerr != nil {
			return nil, inputErr("resource %s: %v", res.Key, gerr)
		}
		if goFieldNameReserved[gn] {
			return nil, inputErr("resource %s: nama field %q bertabrakan dengan anggota DTO", res.Key, f.Name)
		}
		if other, dup := goNames[gn]; dup {
			return nil, inputErr("resource %s: field %q dan %q menghasilkan nama Go yang sama", res.Key, f.Name, other)
		}
		goNames[gn] = f.Name
		f.GoName = gn
	}
	for _, r := range res.Rules {
		c, err := metadata.ParseCondition(r.Attrs["assert"])
		if err != nil {
			return nil, inputErr("resource %s: aturan %s: %v", res.Key, r.ID, err)
		}
		code, err := naming.ToSnake(r.ID)
		if err != nil {
			return nil, inputErr("resource %s: aturan %q: %v", res.Key, r.ID, err)
		}
		fieldName := attrString(r.Attrs, "field")
		if fieldName != "" {
			if errs := naming.ValidateIdentifier(fieldName); errs != nil {
				return nil, inputErr("resource %s: aturan %s: field tidak sah", res.Key, r.ID)
			}
		}
		m.Rules = append(m.Rules, ruleModel{
			ID: r.ID, Code: code, Field: fieldName, Message: attrString(r.Attrs, "message"),
			Assert: c, ServerOnly: attrString(r.Attrs, "scope") == "server" || (info != nil && info.RuleScope[r.ID] == "server"),
		})
	}
	if res.LockWhen != nil {
		c, err := metadata.ParseCondition(res.LockWhen)
		if err != nil {
			return nil, inputErr("resource %s: lockWhen: %v", res.Key, err)
		}
		m.LockWhen = c
		m.LockServer = info != nil && info.ConditionScope["/lockWhen"] == "server"
	}
	m.LabelTpl = res.LabelTemplate
	if m.LabelTpl != "" {
		if _, err := metadata.ParseLabelTemplate(m.LabelTpl); err != nil {
			return nil, inputErr("resource %s: labelTemplate tidak sah", res.Key)
		}
	}
	return m, nil
}

func domainTypeRef(key string) string {
	if rest, ok := strings.CutPrefix(key, resolve.SDKSlug+"."); ok {
		return "sdk:" + rest
	}
	return key
}

func (m *resModel) writable() []*fld {
	var out []*fld
	for _, f := range m.Fields {
		if f.Writable() {
			out = append(out, f)
		}
	}
	return out
}

// pascalValue mengubah nilai enum snake_case menjadi PascalCase ("net_15" menjadi "Net15"); "" bila nilai tidak
// memenuhi pola enum (tidak ada konstanta dibuat untuk nilai seperti itu).
func pascalValue(v string) string {
	if !enumValueRe.MatchString(v) {
		return ""
	}
	var sb strings.Builder
	for _, w := range strings.Split(v, "_") {
		if w == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	return sb.String()
}

// intText memformat bilangan bulat untuk keluaran; digunakan untuk batas yang sudah divalidasi.
func intText(s string) (string, bool) {
	if !canonIntRe.MatchString(s) {
		return "", false
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return "", false
	}
	return n.String(), true
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// manifestGroup memetakan kunci grup metadata (camelCase) ke kunci manifest (snake_case); kunci yang sudah
// berbentuk manifest (mis. grup default dari manifest) dikembalikan apa adanya.
func manifestGroup(g string) string {
	if naming.IdentifierRegexp().MatchString(g) {
		if s, err := naming.ToSnake(g); err == nil {
			return s
		}
	}
	return g
}

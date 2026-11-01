package check

import (
	"strconv"

	"erpora/internal/devtools/metadatatools/resolve"
	"erpora/sdk/metadata"
)

// allowedAttrs adalah atribut batasan tipe yang sah per tipe dasar (Requirement 2.1, 2.3).
var allowedAttrs = map[string]map[string]bool{
	"string":   {"minLength": true, "maxLength": true, "pattern": true, "format": true},
	"text":     {"minLength": true, "maxLength": true},
	"integer":  {"minimum": true, "maximum": true},
	"decimal":  {"precision": true, "scale": true, "currency": true, "minimum": true, "maximum": true},
	"boolean":  {},
	"date":     {},
	"datetime": {},
	"uuid":     {},
	"enum":     {"values": true, "extensible": true},
}

var constraintAttrs = []string{"minLength", "maxLength", "pattern", "format", "precision", "scale", "currency", "minimum", "maximum", "values", "extensible"}

var builtinTypes = map[string]bool{"string": true, "text": true, "integer": true, "decimal": true, "boolean": true, "date": true, "datetime": true, "uuid": true, "enum": true}

func (x *rctx) constraintsOf(f *resolve.Field) constraints {
	c := constraints{typ: metadata.FieldType(f.Type()), nullable: f.Bool("nullable")}
	if v, ok := atoiAttr(f.Attrs["minLength"]); ok {
		c.minLength, c.hasMinLen = v, true
	}
	if v, ok := atoiAttr(f.Attrs["maxLength"]); ok {
		c.maxLength, c.hasMaxLen = v, true
	}
	c.pattern = f.Str("pattern")
	if p, ok := atoiAttr(f.Attrs["precision"]); ok {
		c.precision = p
		if s, ok := atoiAttr(f.Attrs["scale"]); ok {
			c.scale, c.hasPrec = s, true
		}
	}
	c.minimum, c.maximum = f.Num("minimum"), f.Num("maximum")
	if vals, ok := f.Attrs["values"].([]any); ok {
		c.hasEnum = true
		for _, v := range vals {
			if s, ok := v.(string); ok {
				c.enumValues = append(c.enumValues, s)
			}
		}
	}
	return c
}

// checkField memeriksa tipe, batasan, default, klasifikasi, dan penanda satu field (Requirement 2).
func (x *rctx) checkField(f *resolve.Field) {
	typ := f.Type()
	if !builtinTypes[typ] {
		return // tipe domain yang gagal diselesaikan sudah dilaporkan resolver
	}
	allowed := allowedAttrs[typ]
	for _, a := range constraintAttrs {
		if _, has := f.Attrs[a]; has && !allowed[a] {
			x.err(f, a, CodeFieldAttrNotForType, "atribut %s tidak berlaku untuk tipe %s (field %s)", a, typ, f.Name)
		}
	}
	switch typ {
	case "decimal":
		_, hp := f.Attrs["precision"]
		_, hs := f.Attrs["scale"]
		if !hp || !hs {
			x.err(f, "", CodeFieldAttrRequired, "field %s bertipe decimal wajib menyatakan precision dan scale (numeric(p,s))", f.Name)
		}
		p, ok1 := atoiAttr(f.Attrs["precision"])
		s, ok2 := atoiAttr(f.Attrs["scale"])
		if ok1 && ok2 && s > p {
			x.err(f, "scale", CodeFieldScalePrecision, "scale %d melebihi precision %d pada field %s", s, p, f.Name)
		}
	case "enum":
		if _, has := f.Attrs["values"]; !has {
			x.err(f, "", CodeFieldAttrRequired, "field %s bertipe enum wajib menyatakan values", f.Name)
		}
	}
	// rentang
	if lo, ok1 := atoiAttr(f.Attrs["minLength"]); ok1 {
		if hi, ok2 := atoiAttr(f.Attrs["maxLength"]); ok2 && lo > hi {
			x.err(f, "minLength", CodeFieldRange, "minLength %d melebihi maxLength %d pada field %s", lo, hi, f.Name)
		}
	}
	mn, mx := f.Num("minimum"), f.Num("maximum")
	if mn != "" && mx != "" {
		a, ok1 := metadata.ParseDecimal(mn)
		b, ok2 := metadata.ParseDecimal(mx)
		if ok1 && ok2 && a.Cmp(b) > 0 {
			x.err(f, "minimum", CodeFieldRange, "minimum %s melebihi maximum %s pada field %s", mn, mx, f.Name)
		}
	}
	if pat := f.Str("pattern"); pat != "" {
		if err := metadata.ValidatePattern(pat); err != nil {
			x.err(f, "pattern", CodeFieldPatternInvalid, "pattern field %s di luar subset RE2 yang aman: %v", f.Name, err)
		}
	}
	if f.Bool("aggregatable") && typ != "integer" && typ != "decimal" {
		x.err(f, "aggregatable", CodeFieldAggregatableType, "aggregatable hanya sah untuk integer dan decimal, bukan %s (field %s)", typ, f.Name)
	}
	// default (literal dikanonisasi sesuai tipenya)
	if def, has := f.Attrs["default"]; has {
		if msg := x.constraintsOf(f).checkLiteral(def); msg != "" {
			x.err(f, "default", CodeFieldDefaultInvalid, "default field %s tidak sah: %s", f.Name, msg)
		}
	}
	if f.Str("classification") == "highly_confidential" {
		x.err(f, "classification", CodeFieldHighlyConfidential,
			"field %s berklasifikasi highly_confidential yang belum didukung sistem metadata (K17); dukungannya (enkripsi field dan pencarian eksak) adalah pengembangan berikutnya. Tangani kolom semacam itu dengan kode tulis tangan dan deklarasikan di unmanagedColumns", f.Name)
	}
	if IsSensitiveName(f.Name) {
		x.err(f, "", CodeFieldSensitiveName,
			"nama field %s tampak sensitif dan diperlakukan highly_confidential, yang belum didukung (K17); tangani sebagai kolom tulis tangan dan deklarasikan di unmanagedColumns", f.Name)
	}
	if f.ProtectedBy != "" && !f.Bool("readOnly") {
		x.err(f, "readOnly", CodeFieldReadOnlyRequired, "field bawaan %s dari %s wajib readOnly (Requirement 2.6)", f.Name, f.ProtectedBy)
	}
	if dep, ok := f.Attrs["deprecated"].(map[string]any); ok {
		if rb, _ := dep["replacedBy"].(string); rb != "" {
			if x.fields[rb] == nil {
				x.err(f, "deprecated", CodeFieldDeprecatedTarget, "deprecated.replacedBy %q pada field %s bukan field yang ada", rb, f.Name)
			}
		}
	}
}

func itoa(i int) string { return strconv.Itoa(i) }

package check

import "testing"

// K22: sdk:audit selalu disertakan otomatis; resource tanpa include tetap lolos pemeriksaan dan field bawaan
// terhitung pada info turunan.
func TestK22ResourceWithoutIncludeGetsAuditFields(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  title: {type: string, maxLength: 20}\n")))
	noErrors(t, rep)
	info := rep.Resource("m.r")
	for _, n := range []string{"id", "version", "createdAt", "updatedAt", "createdBy", "updatedBy"} {
		if info.Class[n] != "internal" || info.Group[n] != "default" {
			t.Errorf("%s = %q/%q", n, info.Class[n], info.Group[n])
		}
	}
	r := rep.Resolved.Resource("m.r")
	if r == nil || r.Field("id") == nil || !r.Field("createdBy").Bool("nullable") {
		t.Fatalf("field bawaan tidak ada atau createdBy tidak nullable: %v", r)
	}
}

func TestK22ExplicitAuditIncludeIsNoOp(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", "include:\n  - source: sdk:audit\nfields:\n  title: {type: string, maxLength: 20}\n")))
	noErrors(t, rep)
	noCode(t, rep, "duplicate_field")
}

func TestK22UserFieldNamedLikeAuditFieldIsRejected(t *testing.T) {
	rep := single(t, f("m/r.resource.yaml", resource("m.r", "fields:\n  version: {type: integer}\n")))
	wantErr(t, rep, "duplicate_field", "/fields/version")
}

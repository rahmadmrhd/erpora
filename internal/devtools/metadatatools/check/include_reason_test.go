package check

import (
	"strings"
	"testing"
)

func TestIncludeSetRequiresReason(t *testing.T) {
	src := f("m/costing.abstract.yaml", abstract("m.costing", "fields:\n  note: {type: text, nullable: true}\n"))
	mk := func(item string) *Report {
		return run(t, mod(t, "m", nil, src, f("m/r.resource.yaml", resource("m.r", "include:\n  - source: m.costing\n"+item))))
	}
	t.Run("set tanpa reason ditolak", func(t *testing.T) {
		rep := mk("    set:\n      note: {maxLength: 10}\n")
		p := wantErr(t, rep, CodeIncludeSetMissingReason, "/include")
		if !strings.Contains(p.Message, "reason") || p.Line == 0 {
			t.Errorf("pesan/lokasi: %+v", p)
		}
	})
	t.Run("reason kosong atau spasi ditolak", func(t *testing.T) {
		wantErr(t, mk("    reason: \"   \"\n    set:\n      note: {nullable: false}\n"), CodeIncludeSetMissingReason, "/include")
	})
	t.Run("set dengan reason lolos dan reason tercatat di model", func(t *testing.T) {
		rep := mk("    reason: Catatan wajib diisi pada dokumen ini\n    set:\n      note: {nullable: false}\n")
		noCode(t, rep, CodeIncludeSetMissingReason)
	})
	t.Run("include tanpa set tidak butuh reason", func(t *testing.T) {
		noCode(t, mk(""), CodeIncludeSetMissingReason)
	})
}

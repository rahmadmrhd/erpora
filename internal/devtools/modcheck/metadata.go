package modcheck

import (
	"fmt"
	"path/filepath"

	"erpora/internal/devtools/metadatatools/check"
)

// RuleMetadata adalah rule untuk kesamaan field group manifest dengan metadata resource (spec resource-metadata
// Requirement 7.2, P-1): module:check gagal bila field menyebut grup yang tidak ada di manifest atau manifest
// memuat daftar fields yang menyimpang dari metadata. Modul tanpa direktori metadata/ tidak terpengaruh;
// pemeriksaan metadata lainnya adalah tugas metadata:check.
const RuleMetadata = "METADATA"

func metadataFindings(root string, entries []entry) []Finding {
	picked := map[string]entry{}
	for _, e := range entries {
		if e.parseErr == nil && dirExists(filepath.Join(root, "modules", e.dir, "metadata")) {
			picked[e.manifest.Slug] = e
		}
	}
	if len(picked) == 0 {
		return nil
	}
	ws, _ := check.LoadWorkspace(root) // temuan pemuatan dilaporkan metadata:check
	rep := check.Run(ws)
	var out []Finding
	for _, p := range rep.Problems {
		if p.Severity != check.SeverityError || (p.Code != check.CodeManifestFieldsMismatch && p.Code != check.CodeManifestGroupUnknown) {
			continue
		}
		e, ok := picked[p.Module]
		if !ok {
			continue
		}
		loc := p.File
		if p.Line > 0 {
			loc = fmt.Sprintf("%s:%d", p.File, p.Line)
		}
		out = append(out, Finding{Module: label(e), Rule: RuleMetadata, Detail: fmt.Sprintf("%s: %s (%s)", loc, p.Message, p.Code)})
	}
	return out
}

// Package compatibility memeriksa kecocokan versi modul, platform, dan SDK
// terhadap constraint SemVer (PRD-000 FR-18).
package compatibility

import (
	"fmt"

	"github.com/Masterminds/semver/v3"
)

// Satisfies melaporkan apakah version memenuhi constraint. Versi pra-rilis
// hanya cocok bila constraint sendiri memuat pra-rilis. Error dikembalikan
// bila version atau constraint bukan SemVer yang valid.
func Satisfies(version, constraint string) (bool, error) {
	v, err := semver.StrictNewVersion(version)
	if err != nil {
		return false, fmt.Errorf("compatibility: versi %q tidak valid: %w", version, err)
	}
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return false, fmt.Errorf("compatibility: constraint %q tidak valid: %w", constraint, err)
	}
	return c.Check(v), nil
}

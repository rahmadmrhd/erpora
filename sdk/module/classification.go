package module

import "fmt"

// Classification adalah klasifikasi data (security-guidelines §2), urutan naik:
// public < internal < confidential < highly_confidential.
type Classification string

const (
	ClassPublic             Classification = "public"
	ClassInternal           Classification = "internal"
	ClassConfidential       Classification = "confidential"
	ClassHighlyConfidential Classification = "highly_confidential"
)

// ParseClassification menerima tepat empat nilai klasifikasi.
func ParseClassification(s string) (Classification, error) {
	c := Classification(s)
	if !c.Valid() {
		return "", fmt.Errorf("klasifikasi %q tidak dikenal (public, internal, confidential, highly_confidential)", s)
	}
	return c, nil
}

// Valid melaporkan apakah c salah satu dari empat kelas.
func (c Classification) Valid() bool { return c.Rank() >= 0 }

// Rank mengembalikan 0..3; nilai tidak valid atau kosong mengembalikan -1.
func (c Classification) Rank() int {
	switch c {
	case ClassPublic:
		return 0
	case ClassInternal:
		return 1
	case ClassConfidential:
		return 2
	case ClassHighlyConfidential:
		return 3
	}
	return -1
}

// AtLeast melaporkan apakah c valid dan setidaknya setinggi o.
func (c Classification) AtLeast(o Classification) bool {
	return c.Valid() && c.Rank() >= o.Rank()
}

// MaxClassification mengembalikan kelas tertinggi di antara yang valid; kosong bila tidak ada.
func MaxClassification(cs ...Classification) Classification {
	var best Classification
	for _, c := range cs {
		if c.Valid() && c.Rank() > best.Rank() {
			best = c
		}
	}
	return best
}

package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestDefaultLimits(t *testing.T) {
	want := Limits{
		MaxMessageLen: 2000, MaxDepth: 8, MaxOptions: 20, MaxArgs: 32,
		MaxKeys: 5000, MaxFileBytes: 1 << 20, MaxKeyLen: 200,
	}
	if got := DefaultLimits(); got != want {
		t.Fatalf("DefaultLimits() = %+v, want %+v", got, want)
	}
}

func TestLimitsNormalizeFillsNonPositive(t *testing.T) {
	got := Limits{MaxDepth: 3, MaxKeys: -1}.normalize()
	d := DefaultLimits()
	if got.MaxDepth != 3 {
		t.Errorf("MaxDepth = %d, want 3 (nilai positif dipertahankan)", got.MaxDepth)
	}
	if got.MaxKeys != d.MaxKeys || got.MaxMessageLen != d.MaxMessageLen || got.MaxFileBytes != d.MaxFileBytes {
		t.Errorf("bidang tidak positif harus diganti bawaan: %+v", got)
	}
}

func TestIssueError(t *testing.T) {
	i := Issue{File: "id.json", Key: "a.b", Code: CodeFlattenCollision, Detail: "dua jalur"}
	for _, want := range []string{CodeFlattenCollision, "id.json", "a.b", "dua jalur"} {
		if !strings.Contains(i.Error(), want) {
			t.Errorf("Error() = %q, harus memuat %q", i.Error(), want)
		}
	}
	var err error = i
	var got Issue
	if !errors.As(err, &got) || got.Code != CodeFlattenCollision {
		t.Error("Issue harus dapat diambil kembali lewat errors.As")
	}
}

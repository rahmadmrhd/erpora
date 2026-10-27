package runtime

import (
	"sync"
	"testing"
)

func TestApply_RejectsStaleRevision(t *testing.T) {
	s := NewSnapshot()
	if !s.Apply(tenantA, map[string]Status{"a": StatusEnabled}, 2) {
		t.Fatal("Apply pertama (tenant belum punya state) harus true")
	}
	if s.Apply(tenantA, map[string]Status{"a": StatusDisabled}, 1) {
		t.Error("revision basi (1 < 2) harus false")
	}
	if s.Status(tenantA, "a") != StatusEnabled || s.Revision(tenantA) != 2 {
		t.Error("pembaruan basi tidak boleh mengubah snapshot")
	}
	if s.Apply(tenantA, map[string]Status{"a": StatusDisabled}, 2) {
		t.Error("revision sama harus false")
	}
	if !s.Apply(tenantA, map[string]Status{"a": StatusDisabled}, 3) {
		t.Error("revision lebih baru harus true")
	}
	if s.Status(tenantA, "a") != StatusDisabled || s.Revision(tenantA) != 3 {
		t.Error("revision 3 tidak tercermin")
	}
}

func TestApply_FirstApplyAcceptsRevisionZero(t *testing.T) {
	s := NewSnapshot()
	if !s.Apply(tenantA, map[string]Status{"a": StatusAvailable}, 0) {
		t.Fatal("tenant tanpa state menerima revision 0")
	}
	if s.Apply(tenantA, map[string]Status{"a": StatusEnabled}, 0) {
		t.Error("revision 0 kedua harus ditolak")
	}
}

func TestApply_ConcurrentWritersMonotonic(t *testing.T) {
	s := NewSnapshot()
	var wg sync.WaitGroup
	for rev := int64(1); rev <= 50; rev++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st := Status("rev")
			if rev == 50 {
				st = StatusEnabled
			}
			s.Apply(tenantA, map[string]Status{"a": st}, rev)
		}()
	}
	wg.Wait()
	statuses, rev := s.Statuses(tenantA)
	if rev != 50 || statuses["a"] != StatusEnabled {
		t.Fatalf("revision = %d, status = %q; want 50/enabled", rev, statuses["a"])
	}
}

func TestStatuses_ReturnsCopy(t *testing.T) {
	s := NewSnapshot()
	s.Apply(tenantA, map[string]Status{"a": StatusEnabled}, 1)
	m, _ := s.Statuses(tenantA)
	m["a"] = StatusFailed
	if s.Status(tenantA, "a") != StatusEnabled {
		t.Error("Statuses harus mengembalikan salinan")
	}
}

func TestStatus_PlatformCoreAlwaysEnabled(t *testing.T) {
	s := NewSnapshot()
	if s.Status(tenantA, platformKey) != StatusEnabled {
		t.Error("platform.core selalu enabled")
	}
	s.Apply(tenantA, map[string]Status{platformKey: StatusDisabled}, 1)
	if s.Status(tenantA, platformKey) != StatusEnabled {
		t.Error("platform.core tidak boleh ditimpa")
	}
}

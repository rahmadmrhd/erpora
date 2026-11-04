package authz

import "testing"

func TestMapRecordCanonicalizesKeys(t *testing.T) {
	r := recordFromMap(map[string]any{"createdBy": "u1", "warehouse_id": "w1", "title": "x"})
	for field, want := range map[string]any{"created_by": "u1", "warehouse_id": "w1", "title": "x"} {
		if v, ok := r.AuthzValue(field); !ok || v != want {
			t.Errorf("AuthzValue(%q) = %v, %v", field, v, ok)
		}
	}
	if _, ok := r.AuthzValue("missing"); ok {
		t.Error("field tak ada harus ok=false")
	}
	// pemanggil memberi nama camelCase: dinormalisasi
	if v, ok := r.AuthzValue("createdBy"); !ok || v != "u1" {
		t.Errorf("camelCase: %v %v", v, ok)
	}
}

func TestMapRecordCollidingKeysDeny(t *testing.T) {
	r := recordFromMap(map[string]any{"createdBy": "u1", "created_by": "u2"})
	if _, ok := r.AuthzValue("created_by"); ok {
		t.Error("dua kunci yang runtuh ke nama kanonik sama harus ditolak (ok=false)")
	}
}

func TestMapRecordUnrecognizedKeyIgnored(t *testing.T) {
	r := recordFromMap(map[string]any{"taxID": "x"})
	if _, ok := r.AuthzValue("taxID"); ok {
		t.Error("nama tak dapat dikanonikalkan tidak boleh dicocokkan")
	}
}

func TestMapRecordNilMap(t *testing.T) {
	if _, ok := recordFromMap(nil).AuthzValue("a"); ok {
		t.Error("map nil tidak punya nilai")
	}
}

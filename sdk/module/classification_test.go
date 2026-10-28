package module

import "testing"

func TestClassification_Order(t *testing.T) {
	if !ClassHighlyConfidential.AtLeast(ClassConfidential) {
		t.Error("highly_confidential harus >= confidential")
	}
	if ClassInternal.AtLeast(ClassConfidential) {
		t.Error("internal tidak boleh >= confidential")
	}
	if Classification("").AtLeast(ClassPublic) {
		t.Error("kelas kosong tidak boleh >= public")
	}
	if got := MaxClassification(ClassInternal, ClassConfidential, ClassPublic); got != ClassConfidential {
		t.Errorf("MaxClassification = %q", got)
	}
	if got := MaxClassification(); got != "" {
		t.Errorf("MaxClassification() = %q, want kosong", got)
	}
	if _, err := ParseClassification("secret"); err == nil {
		t.Error("ParseClassification(secret) harus error")
	}
	got, err := ParseClassification("highly_confidential")
	if err != nil || got != ClassHighlyConfidential {
		t.Errorf("ParseClassification = %q, %v", got, err)
	}
	if Classification("secret").Rank() != -1 {
		t.Error("kelas tidak valid harus berperingkat -1")
	}
}

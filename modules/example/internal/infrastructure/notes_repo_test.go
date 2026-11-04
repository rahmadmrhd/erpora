package infrastructure

import (
	"testing"

	"github.com/google/uuid"
)

func TestUserConversions(t *testing.T) {
	if got := userString(nil); got != nil {
		t.Errorf("userString(nil) = %v", got)
	}
	id := uuid.MustParse("0193a6c2-0000-7000-8000-0000000000aa")
	if got := userString(&id); got == nil || *got != id.String() {
		t.Errorf("userString = %v", got)
	}
	if got, err := userUUID(nil); got != nil || err != nil {
		t.Errorf("userUUID(nil) = %v, %v", got, err)
	}
	s := id.String()
	if got, err := userUUID(&s); err != nil || got == nil || *got != id {
		t.Errorf("userUUID = %v, %v", got, err)
	}
	bad := "bukan-uuid"
	if _, err := userUUID(&bad); err == nil {
		t.Error("id pengguna tidak sah harus galat")
	}
}

func TestNoteRowToDomainKeepsNullAuditUsers(t *testing.T) {
	n := noteRow{ID: uuid.New(), Title: "x", Version: 1}.toDomain()
	if n.CreatedBy != nil || n.UpdatedBy != nil {
		t.Errorf("baris lama harus null: %+v", n)
	}
	id := uuid.New()
	n = noteRow{ID: uuid.New(), Title: "x", Version: 1, CreatedBy: &id, UpdatedBy: &id}.toDomain()
	if n.CreatedBy == nil || *n.CreatedBy != id.String() || n.UpdatedBy == nil {
		t.Errorf("baris berpengguna: %+v", n)
	}
}

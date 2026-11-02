package check

import "testing"

const (
	newPhone       = "  phoneE164: {type: string, maxLength: 20, nullable: true, migrateFrom: phone}\n"
	deprecatedOld  = "  phone: {type: string, maxLength: 32, deprecated: {since: \"1.4.0\", replacedBy: phoneE164}, removeAfter: \"2.0.0\"}\n"
	deprecatedOld2 = "  phone: {type: string, maxLength: 32, nullable: true, deprecated: {since: \"1.4.0\", replacedBy: phoneE164}, removeAfter: \"2.0.0\"}\n"
)

// Rename lintas rilis (K18, D-35): rilis MAJOR menghapus field lama yang deprecated pada rilis terakhir, dan
// migrateFrom pada field baru tetap sah supaya penjaga penghapusan dan pengecualian pemicu tetap dapat dibangkitkan.
func TestMigrateFromToRemovedFieldIsAllowedWhenBaselineDeprecated(t *testing.T) {
	g := released("v1.4.0", keepField+deprecatedOld2)
	rep := withGit(t, g, currentModule(t, "2.0.0", keepField+newPhone))
	noErrors(t, rep)
}

func TestMigrateFromToRemovedFieldRejectedWhenBaselineNotDeprecated(t *testing.T) {
	g := released("v1.4.0", keepField+oldPhone)
	rep := withGit(t, g, currentModule(t, "2.0.0", keepField+newPhone))
	wantErr(t, rep, CodeMigrateFromUnknown, "/fields/phoneE164")
}

func TestMigrateFromToUnknownFieldRejectedWhenBaselineLacksIt(t *testing.T) {
	g := released("v1.4.0", keepField)
	rep := withGit(t, g, currentModule(t, "2.0.0", keepField+newPhone))
	wantErr(t, rep, CodeMigrateFromUnknown, "/fields/phoneE164")
}

func TestMigrateFromToRemovedFieldWithoutBaselineIsWarning(t *testing.T) {
	// Modul tanpa tag rilis (belum pernah dirilis): tidak dapat diverifikasi, hanya peringatan.
	g := &fakeGit{}
	rep := withGit(t, g, currentModule(t, "0.1.0", keepField+newPhone))
	noErrors(t, rep)
	wantWarning(t, rep, CodeMigrateFromUnbaselined, "/fields/phoneE164")
}

func TestMigrateFromToExistingFieldIsUnchanged(t *testing.T) {
	g := released("v1.4.0", keepField+deprecatedOld)
	rep := withGit(t, g, currentModule(t, "1.5.0", keepField+deprecatedOld+newPhone))
	noErrors(t, rep)
}

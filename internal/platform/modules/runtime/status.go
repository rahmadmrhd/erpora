// Package runtime menyimpan snapshot status modul per tenant, guard status
// modul, dan pemasangan route modul ke router (PRD-000 FR-19).
package runtime

// Status adalah status siklus hidup modul untuk satu tenant.
type Status string

// Status modul.
const (
	StatusAvailable      Status = "available"
	StatusInstalled      Status = "installed"
	StatusEnabled        Status = "enabled"
	StatusDisabled       Status = "disabled"
	StatusUpgradePending Status = "upgrade_pending"
	StatusFailed         Status = "failed"
	StatusUninstalled    Status = "uninstalled"
	StatusMissing        Status = "missing"
	StatusIncompatible   Status = "incompatible"
)

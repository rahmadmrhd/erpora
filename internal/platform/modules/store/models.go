package store

import (
	"time"

	"github.com/google/uuid"
)

// Model GORM privat untuk tabel lifecycle (control DB) dan module_migrations (tenant DB).
// Kolom settings sengaja tidak dipetakan: nilai default DB ('{}') berlaku.

type installationRow struct {
	ID               uuid.UUID  `gorm:"column:id;primaryKey"`
	TenantID         uuid.UUID  `gorm:"column:tenant_id"`
	ModuleKey        string     `gorm:"column:module_key"`
	InstalledVersion *string    `gorm:"column:installed_version"`
	Status           string     `gorm:"column:status"`
	PreviousStatus   *string    `gorm:"column:previous_status"`
	LastOperationID  *uuid.UUID `gorm:"column:last_operation_id"`
	CreatedAt        time.Time  `gorm:"column:created_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at"`
}

func (installationRow) TableName() string { return "module_installations" }

type operationRow struct {
	ID          uuid.UUID  `gorm:"column:id;primaryKey"`
	TenantID    uuid.UUID  `gorm:"column:tenant_id"`
	ModuleKey   string     `gorm:"column:module_key"`
	Operation   string     `gorm:"column:operation"`
	FromVersion *string    `gorm:"column:from_version"`
	ToVersion   *string    `gorm:"column:to_version"`
	Status      string     `gorm:"column:status"`
	Steps       []byte     `gorm:"column:steps"`
	Error       []byte     `gorm:"column:error"`
	Actor       string     `gorm:"column:actor"`
	TraceID     *string    `gorm:"column:trace_id"`
	StartedAt   time.Time  `gorm:"column:started_at"`
	FinishedAt  *time.Time `gorm:"column:finished_at"`
}

func (operationRow) TableName() string { return "module_operations" }

type eventRow struct {
	ID          uuid.UUID  `gorm:"column:id;primaryKey"`
	TenantID    uuid.UUID  `gorm:"column:tenant_id"`
	ModuleKey   string     `gorm:"column:module_key"`
	Event       string     `gorm:"column:event"`
	Actor       string     `gorm:"column:actor"`
	OperationID *uuid.UUID `gorm:"column:operation_id"`
	Details     []byte     `gorm:"column:details"`
	OccurredAt  time.Time  `gorm:"column:occurred_at"`
}

func (eventRow) TableName() string { return "module_lifecycle_events" }

type moduleMigrationRow struct {
	ID              uuid.UUID  `gorm:"column:id;primaryKey"`
	TenantID        uuid.UUID  `gorm:"column:tenant_id"`
	ModuleKey       string     `gorm:"column:module_key"`
	ModuleVersion   string     `gorm:"column:module_version"`
	MigrationKey    string     `gorm:"column:migration_key"`
	Checksum        string     `gorm:"column:checksum"`
	AppliedAt       time.Time  `gorm:"column:applied_at"`
	ExecutionStatus string     `gorm:"column:execution_status"`
	OperationID     *uuid.UUID `gorm:"column:operation_id"`
}

func (moduleMigrationRow) TableName() string { return "module_migrations" }

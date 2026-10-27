package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"erpora/internal/platform/database"
	"erpora/internal/platform/tenancy"
)

// ErrNotFound dikembalikan bila record tidak ada, atau milik tenant lain (FR-119: 404, bukan 403).
var ErrNotFound = errors.New("store: tidak ditemukan")

// Installation adalah status tersimpan satu modul untuk satu tenant. uuid.Nil pada LastOperationID
// berarti belum ada; string kosong pada InstalledVersion/PreviousStatus disimpan sebagai NULL.
type Installation struct {
	TenantID                                            tenancy.TenantID
	ModuleKey, InstalledVersion, Status, PreviousStatus string
	LastOperationID                                     uuid.UUID
	CreatedAt, UpdatedAt                                time.Time
}

// Step adalah satu langkah operasi (module_operations.steps).
type Step struct {
	Name       string    `json:"name"`
	Status     string    `json:"status"` // running|succeeded|failed|skipped
	StartedAt  time.Time `json:"startedAt"`
	DurationMS int64     `json:"durationMs"`
	Error      string    `json:"error,omitempty"`
}

// OpError adalah error operasi (module_operations.error).
type OpError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Step    string         `json:"step,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

// Operation adalah catatan satu operasi lifecycle.
type Operation struct {
	ID                                                   uuid.UUID
	TenantID                                             tenancy.TenantID
	ModuleKey, Operation, FromVersion, ToVersion, Status string
	Steps                                                []Step
	Error                                                *OpError
	Actor, TraceID                                       string
	StartedAt                                            time.Time
	FinishedAt                                           *time.Time
}

// Event adalah satu baris module_lifecycle_events (append-only).
type Event struct {
	ID                      uuid.UUID
	TenantID                tenancy.TenantID
	ModuleKey, Event, Actor string
	OperationID             uuid.UUID
	Details                 map[string]any
	OccurredAt              time.Time
}

// ModuleMigration adalah jejak satu file migrasi modul di tenant DB.
type ModuleMigration struct {
	TenantID                                                          tenancy.TenantID
	ModuleKey, ModuleVersion, MigrationKey, Checksum, ExecutionStatus string
	OperationID                                                       uuid.UUID
}

// Finish menutup operasi pada Commit.
type Finish struct {
	OperationID uuid.UUID
	Status      string
	Error       *OpError
}

// Commit adalah satu unit atomik: installation + penutupan operasi + event + (opsional) revision.
// Installation harus memuat keadaan lengkap yang diinginkan; LastOperationID uuid.Nil = jangan diubah.
type Commit struct {
	Installation *Installation
	Finish       *Finish
	Events       []Event
	BumpRevision bool
}

// Store adalah persistensi lifecycle modul. Satu-satunya pemakai GORM untuk tabel lifecycle (D3b).
type Store struct {
	control database.ControlDB
	tenants database.TenantConnector
}

// New membuat Store.
func New(control database.ControlDB, tenants database.TenantConnector) *Store {
	return &Store{control: control, tenants: tenants}
}

func now() time.Time { return time.Now().UTC() }

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nilIfNilUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func tid(t tenancy.TenantID) uuid.UUID { return uuid.UUID(t) }

func (r installationRow) toModel() Installation {
	in := Installation{
		TenantID:         tenancy.TenantID(r.TenantID),
		ModuleKey:        r.ModuleKey,
		InstalledVersion: deref(r.InstalledVersion),
		Status:           r.Status,
		PreviousStatus:   deref(r.PreviousStatus),
		CreatedAt:        r.CreatedAt.UTC(),
		UpdatedAt:        r.UpdatedAt.UTC(),
	}
	if r.LastOperationID != nil {
		in.LastOperationID = *r.LastOperationID
	}
	return in
}

func (r operationRow) toModel() (Operation, error) {
	op := Operation{
		ID:          r.ID,
		TenantID:    tenancy.TenantID(r.TenantID),
		ModuleKey:   r.ModuleKey,
		Operation:   r.Operation,
		FromVersion: deref(r.FromVersion),
		ToVersion:   deref(r.ToVersion),
		Status:      r.Status,
		Actor:       r.Actor,
		TraceID:     deref(r.TraceID),
		StartedAt:   r.StartedAt.UTC(),
	}
	if r.FinishedAt != nil {
		f := r.FinishedAt.UTC()
		op.FinishedAt = &f
	}
	if len(r.Steps) > 0 {
		if err := json.Unmarshal(r.Steps, &op.Steps); err != nil {
			return Operation{}, fmt.Errorf("store: steps operasi %s: %w", r.ID, err)
		}
	}
	if len(r.Error) > 0 {
		var e OpError
		if err := json.Unmarshal(r.Error, &e); err != nil {
			return Operation{}, fmt.Errorf("store: error operasi %s: %w", r.ID, err)
		}
		op.Error = &e
	}
	return op, nil
}

func (r eventRow) toModel() (Event, error) {
	ev := Event{
		ID:         r.ID,
		TenantID:   tenancy.TenantID(r.TenantID),
		ModuleKey:  r.ModuleKey,
		Event:      r.Event,
		Actor:      r.Actor,
		OccurredAt: r.OccurredAt.UTC(),
	}
	if r.OperationID != nil {
		ev.OperationID = *r.OperationID
	}
	if len(r.Details) > 0 {
		if err := json.Unmarshal(r.Details, &ev.Details); err != nil {
			return Event{}, fmt.Errorf("store: details event %s: %w", r.ID, err)
		}
	}
	return ev, nil
}

func stepsJSON(steps []Step) ([]byte, error) {
	if steps == nil {
		steps = []Step{}
	}
	return json.Marshal(steps)
}

// Installations memuat semua status tersimpan tenant (urut module_key).
func (s *Store) Installations(ctx context.Context, t tenancy.TenantID) ([]Installation, error) {
	var rows []installationRow
	if err := s.control.WithContext(ctx).Where("tenant_id = ?", tid(t)).Order("module_key").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: muat installations: %w", err)
	}
	out := make([]Installation, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.toModel())
	}
	return out, nil
}

// Revision mengembalikan module_state_revision tenant; 0 bila belum ada baris.
func (s *Store) Revision(ctx context.Context, t tenancy.TenantID) (int64, error) {
	return revision(s.control.WithContext(ctx), t)
}

func revision(db *gorm.DB, t tenancy.TenantID) (int64, error) {
	var rev []int64
	if err := db.Table("tenant_state").Select("module_state_revision").Where("tenant_id = ?", tid(t)).Scan(&rev).Error; err != nil {
		return 0, fmt.Errorf("store: muat revision: %w", err)
	}
	if len(rev) == 0 {
		return 0, nil
	}
	return rev[0], nil
}

// StartOperation mencatat operasi baru (status running) dan, bila baris installation modul sudah ada,
// mengarahkan last_operation_id ke operasi ini.
func (s *Store) StartOperation(ctx context.Context, op Operation) error {
	if op.ID == uuid.Nil {
		return errors.New("store: operation ID wajib")
	}
	steps, err := stepsJSON(op.Steps)
	if err != nil {
		return err
	}
	started := op.StartedAt
	if started.IsZero() {
		started = now()
	}
	row := operationRow{
		ID: op.ID, TenantID: tid(op.TenantID), ModuleKey: op.ModuleKey, Operation: op.Operation,
		FromVersion: nilIfEmpty(op.FromVersion), ToVersion: nilIfEmpty(op.ToVersion),
		Status: op.Status, Steps: steps, Actor: op.Actor, TraceID: nilIfEmpty(op.TraceID),
		StartedAt: started.UTC(),
	}
	return s.control.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("store: catat operasi: %w", err)
		}
		if err := tx.Model(&installationRow{}).
			Where("tenant_id = ? AND module_key = ?", tid(op.TenantID), op.ModuleKey).
			Updates(map[string]any{"last_operation_id": op.ID, "updated_at": now()}).Error; err != nil {
			return fmt.Errorf("store: set last_operation_id: %w", err)
		}
		return nil
	})
}

// SaveSteps menimpa daftar langkah operasi (dipanggil sebelum dan sesudah setiap langkah).
func (s *Store) SaveSteps(ctx context.Context, t tenancy.TenantID, opID uuid.UUID, steps []Step) error {
	b, err := stepsJSON(steps)
	if err != nil {
		return err
	}
	res := s.control.WithContext(ctx).Model(&operationRow{}).
		Where("tenant_id = ? AND id = ?", tid(t), opID).Update("steps", b)
	if res.Error != nil {
		return fmt.Errorf("store: simpan steps: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// Commit menerapkan satu transaksi DB: upsert installation (FOR UPDATE), penutupan operasi, event, dan
// bila BumpRevision, tenant_state.module_state_revision + 1. Mengembalikan revision setelah commit.
func (s *Store) Commit(ctx context.Context, t tenancy.TenantID, c Commit) (int64, error) {
	var rev int64
	err := s.control.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if c.Installation != nil {
			if err := upsertInstallation(tx, t, *c.Installation); err != nil {
				return err
			}
		}
		if c.Finish != nil {
			if err := finishOperation(tx, t, *c.Finish); err != nil {
				return err
			}
		}
		for _, ev := range c.Events {
			if err := insertEvent(tx, t, ev); err != nil {
				return err
			}
		}
		if c.BumpRevision {
			if err := tx.Raw(`INSERT INTO tenant_state (tenant_id, module_state_revision) VALUES (?, 1)
				ON CONFLICT (tenant_id) DO UPDATE SET module_state_revision = tenant_state.module_state_revision + 1, updated_at = now()
				RETURNING module_state_revision`, tid(t)).Scan(&rev).Error; err != nil {
				return fmt.Errorf("store: naikkan revision: %w", err)
			}
			return nil
		}
		r, err := revision(tx, t)
		rev = r
		return err
	})
	if err != nil {
		return 0, err
	}
	return rev, nil
}

func upsertInstallation(tx *gorm.DB, t tenancy.TenantID, in Installation) error {
	var existing installationRow
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("tenant_id = ? AND module_key = ?", tid(t), in.ModuleKey).Take(&existing).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		ts := now()
		row := installationRow{
			ID: id, TenantID: tid(t), ModuleKey: in.ModuleKey,
			InstalledVersion: nilIfEmpty(in.InstalledVersion), Status: in.Status,
			PreviousStatus: nilIfEmpty(in.PreviousStatus), LastOperationID: nilIfNilUUID(in.LastOperationID),
			CreatedAt: ts, UpdatedAt: ts,
		}
		if err := tx.Create(&row).Error; err != nil {
			return fmt.Errorf("store: buat installation: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("store: kunci installation: %w", err)
	}
	upd := map[string]any{
		"installed_version": nilIfEmpty(in.InstalledVersion),
		"status":            in.Status,
		"previous_status":   nilIfEmpty(in.PreviousStatus),
		"updated_at":        now(),
	}
	if in.LastOperationID != uuid.Nil {
		upd["last_operation_id"] = in.LastOperationID
	}
	if err := tx.Model(&installationRow{}).Where("id = ?", existing.ID).Updates(upd).Error; err != nil {
		return fmt.Errorf("store: ubah installation: %w", err)
	}
	return nil
}

func finishOperation(tx *gorm.DB, t tenancy.TenantID, f Finish) error {
	upd := map[string]any{"status": f.Status, "finished_at": now()}
	if f.Error != nil {
		b, err := json.Marshal(f.Error)
		if err != nil {
			return err
		}
		upd["error"] = b
	}
	res := tx.Model(&operationRow{}).Where("tenant_id = ? AND id = ?", tid(t), f.OperationID).Updates(upd)
	if res.Error != nil {
		return fmt.Errorf("store: tutup operasi: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func insertEvent(tx *gorm.DB, t tenancy.TenantID, ev Event) error {
	id := ev.ID
	if id == uuid.Nil {
		var err error
		if id, err = uuid.NewV7(); err != nil {
			return err
		}
	}
	details := ev.Details
	if details == nil {
		details = map[string]any{}
	}
	b, err := json.Marshal(details)
	if err != nil {
		return err
	}
	at := ev.OccurredAt
	if at.IsZero() {
		at = now()
	}
	row := eventRow{
		ID: id, TenantID: tid(t), ModuleKey: ev.ModuleKey, Event: ev.Event, Actor: ev.Actor,
		OperationID: nilIfNilUUID(ev.OperationID), Details: b, OccurredAt: at.UTC(),
	}
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("store: catat event: %w", err)
	}
	return nil
}

// Operation memuat satu operasi; ErrNotFound juga bila milik tenant lain.
func (s *Store) Operation(ctx context.Context, t tenancy.TenantID, id uuid.UUID) (Operation, error) {
	var row operationRow
	err := s.control.WithContext(ctx).Where("tenant_id = ? AND id = ?", tid(t), id).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Operation{}, ErrNotFound
	}
	if err != nil {
		return Operation{}, fmt.Errorf("store: muat operasi: %w", err)
	}
	return row.toModel()
}

// Operations memuat operasi terbaru dulu; key kosong = semua modul; limit <= 0 = tanpa batas.
func (s *Store) Operations(ctx context.Context, t tenancy.TenantID, key string, limit int) ([]Operation, error) {
	q := s.control.WithContext(ctx).Where("tenant_id = ?", tid(t))
	if key != "" {
		q = q.Where("module_key = ?", key)
	}
	q = q.Order("started_at DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var rows []operationRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: muat operasi: %w", err)
	}
	return opsFromRows(rows)
}

// RunningOperations memuat operasi berstatus running milik tenant.
func (s *Store) RunningOperations(ctx context.Context, t tenancy.TenantID) ([]Operation, error) {
	var rows []operationRow
	if err := s.control.WithContext(ctx).Where("tenant_id = ? AND status = ?", tid(t), "running").
		Order("started_at, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("store: muat operasi running: %w", err)
	}
	return opsFromRows(rows)
}

func opsFromRows(rows []operationRow) ([]Operation, error) {
	out := make([]Operation, 0, len(rows))
	for _, r := range rows {
		op, err := r.toModel()
		if err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, nil
}

// Events memuat event terbaru dulu (halaman 1-based) beserta total; key kosong = semua modul.
func (s *Store) Events(ctx context.Context, t tenancy.TenantID, key string, page, pageSize int) ([]Event, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	q := s.control.WithContext(ctx).Model(&eventRow{}).Where("tenant_id = ?", tid(t))
	if key != "" {
		q = q.Where("module_key = ?", key)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("store: hitung event: %w", err)
	}
	var rows []eventRow
	if err := q.Order("occurred_at DESC, id DESC").Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("store: muat event: %w", err)
	}
	out := make([]Event, 0, len(rows))
	for _, r := range rows {
		ev, err := r.toModel()
		if err != nil {
			return nil, 0, err
		}
		out = append(out, ev)
	}
	return out, total, nil
}

// RecordModuleMigration mencatat jejak migrasi modul ke tenant DB lewat connector.
func (s *Store) RecordModuleMigration(ctx context.Context, m ModuleMigration) error {
	db, err := s.tenants.For(ctx, m.TenantID)
	if err != nil {
		return fmt.Errorf("store: tenant DB: %w", err)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	row := moduleMigrationRow{
		ID: id, TenantID: tid(m.TenantID), ModuleKey: m.ModuleKey, ModuleVersion: m.ModuleVersion,
		MigrationKey: m.MigrationKey, Checksum: m.Checksum, AppliedAt: now(),
		ExecutionStatus: m.ExecutionStatus, OperationID: nilIfNilUUID(m.OperationID),
	}
	if err := db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("store: catat module_migrations: %w", err)
	}
	return nil
}

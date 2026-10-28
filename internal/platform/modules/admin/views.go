package admin

import (
	"time"

	"github.com/google/uuid"

	"erpora/internal/platform/modules/lifecycle"
	"erpora/internal/platform/modules/store"
	"erpora/sdk/module"
)

// ModuleView adalah representasi JSON satu modul (camelCase; installedVersion dan lastOperationId null bila kosong).
type ModuleView struct {
	Key               string           `json:"key"`
	Slug              string           `json:"slug"`
	Name              string           `json:"name"`
	BinaryVersion     string           `json:"binaryVersion"`
	InstalledVersion  *string          `json:"installedVersion"`
	Status            string           `json:"status"`
	Required          bool             `json:"required"`
	Compatible        bool             `json:"compatible"`
	IncompatibleCodes []string         `json:"incompatibleCodes"`
	Dependencies      []DependencyView `json:"dependencies"`
	Dependents        []string         `json:"dependents"`
	LastOperationID   *string          `json:"lastOperationId"`
}

// DependencyView adalah satu dependensi manifest.
type DependencyView struct {
	Module   string `json:"module"`
	Version  string `json:"version"`
	Optional bool   `json:"optional"`
}

// StepView adalah satu langkah operasi.
type StepView struct {
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	StartedAt  string  `json:"startedAt"`
	DurationMS int64   `json:"durationMs"`
	Error      *string `json:"error"`
}

// FailureView adalah penyebab kegagalan operasi.
type FailureView struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Step    string `json:"step,omitempty"`
}

// OperationView adalah representasi JSON satu operasi lifecycle.
type OperationView struct {
	ID          string       `json:"id"`
	ModuleKey   string       `json:"moduleKey"`
	Operation   string       `json:"operation"`
	FromVersion *string      `json:"fromVersion"`
	ToVersion   *string      `json:"toVersion"`
	Status      string       `json:"status"`
	Steps       []StepView   `json:"steps"`
	Failure     *FailureView `json:"failure"`
	Actor       string       `json:"actor"`
	StartedAt   string       `json:"startedAt"`
	FinishedAt  *string      `json:"finishedAt"`
}

// EventView adalah representasi JSON satu event lifecycle.
type EventView struct {
	ID          string         `json:"id"`
	ModuleKey   string         `json:"moduleKey"`
	Event       string         `json:"event"`
	Actor       string         `json:"actor"`
	OperationID *string        `json:"operationId"`
	Details     map[string]any `json:"details"`
	OccurredAt  string         `json:"occurredAt"`
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func depViews(deps []module.Dependency) []DependencyView {
	out := make([]DependencyView, 0, len(deps))
	for _, d := range deps {
		out = append(out, DependencyView{Module: d.Module, Version: d.Version, Optional: d.Optional})
	}
	return out
}

func toModuleView(m lifecycle.ModuleState) ModuleView {
	v := ModuleView{
		Key: m.Key, Slug: m.Slug, Name: m.Name, BinaryVersion: m.BinaryVersion,
		InstalledVersion: strPtr(m.InstalledVersion), Status: string(m.Status), Required: m.Required,
		Compatible: m.Compatible, IncompatibleCodes: []string{}, Dependencies: depViews(m.Dependencies),
		Dependents: append([]string{}, m.Dependents...),
	}
	for _, is := range m.Incompatible {
		v.IncompatibleCodes = append(v.IncompatibleCodes, is.Code)
	}
	if m.LastOperation != nil {
		id := m.LastOperation.ID.String()
		v.LastOperationID = &id
	}
	return v
}

func toOperationView(op store.Operation) OperationView {
	v := OperationView{
		ID: op.ID.String(), ModuleKey: op.ModuleKey, Operation: op.Operation,
		FromVersion: strPtr(op.FromVersion), ToVersion: strPtr(op.ToVersion), Status: op.Status,
		Steps: make([]StepView, 0, len(op.Steps)), Actor: op.Actor, StartedAt: ts(op.StartedAt),
	}
	for _, s := range op.Steps {
		v.Steps = append(v.Steps, StepView{Name: s.Name, Status: s.Status, StartedAt: ts(s.StartedAt), DurationMS: s.DurationMS, Error: strPtr(s.Error)})
	}
	if op.Error != nil {
		v.Failure = &FailureView{Code: op.Error.Code, Message: op.Error.Message, Step: op.Error.Step}
	}
	if op.FinishedAt != nil {
		f := ts(*op.FinishedAt)
		v.FinishedAt = &f
	}
	return v
}

func toEventView(e store.Event) EventView {
	d := e.Details
	if d == nil {
		d = map[string]any{}
	}
	v := EventView{ID: e.ID.String(), ModuleKey: e.ModuleKey, Event: e.Event, Actor: e.Actor, Details: d, OccurredAt: ts(e.OccurredAt)}
	if e.OperationID != uuid.Nil {
		id := e.OperationID.String()
		v.OperationID = &id
	}
	return v
}

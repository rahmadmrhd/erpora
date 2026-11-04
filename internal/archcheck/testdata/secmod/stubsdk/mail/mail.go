// Package mail adalah stub minimal SDK untuk fixture arch test IDENT.
package mail

import "context"

// Message dan Sender meniru tipe asli.
type (
	Message struct{ To []string }
	Sender  interface {
		Send(ctx context.Context, m Message) error
	}
)

// WithSender meniru pemasang konteks milik platform.
func WithSender(ctx context.Context, s Sender) context.Context { return ctx }

// From dan Send adalah API yang sah dipakai modul.
func From(ctx context.Context) (Sender, bool) { return nil, false }

// Send meniru signature asli.
func Send(ctx context.Context, m Message) error { return nil }

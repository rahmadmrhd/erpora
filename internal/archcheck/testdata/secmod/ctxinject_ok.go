package secfixture

import (
	"context"

	"erpora/sdk/identity"
	"erpora/sdk/mail"
)

// Fixture: pemakaian yang sah, 0 pelanggaran IDENT: modul non-core hanya membaca Principal dan mengirim email.
func ctxInjectOK(ctx context.Context) error {
	if _, ok := identity.FromContext(ctx); !ok {
		return nil
	}
	_, _ = mail.From(ctx)
	return mail.Send(ctx, mail.Message{To: []string{"a@example.test"}})
}

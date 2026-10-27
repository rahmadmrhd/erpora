package database

import (
	"context"
	"errors"

	"erpora/internal/platform/tenancy"
	"erpora/sdk/data"
)

// DataProvider mengadaptasi TenantConnector menjadi data.Provider berdasarkan tenant di ctx.
// Session yang dikembalikan terikat ctx agar query ikut dibatalkan dan span DB menjadi anak span pemanggil.
func DataProvider(conn TenantConnector) data.Provider { return connectorProvider{conn: conn} }

type connectorProvider struct{ conn TenantConnector }

func (p connectorProvider) DB(ctx context.Context) (*DB, error) {
	id, ok := tenancy.FromContext(ctx)
	if !ok {
		return nil, data.ErrNoTenantContext
	}
	tdb, err := p.conn.For(ctx, id)
	if err != nil {
		return nil, err
	}
	if tdb.DB == nil {
		return nil, errors.New("database tenant belum tersedia")
	}
	return tdb.WithContext(ctx), nil
}

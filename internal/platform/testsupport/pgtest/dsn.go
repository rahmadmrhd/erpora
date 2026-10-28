// Package pgtest menyediakan PostgreSQL untuk test integrasi (container testcontainers, atau server
// eksternal lewat ERP_TEST_DATABASE_URL). Berkas ini berisi helper murni tanpa tag build.
package pgtest

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
)

// EnvExternal adalah env berisi DSN admin PostgreSQL eksternal; bila diset, pgtest tidak memakai container.
const EnvExternal = "ERP_TEST_DATABASE_URL"

// withDatabase mengganti nama database pada DSN berbentuk URL (postgres:// atau postgresql://).
func withDatabase(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", errors.New("DSN tidak dapat dibaca sebagai URL")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return "", errors.New("DSN harus berbentuk URL postgres://")
	}
	u.Path = "/" + name
	u.RawPath = ""
	return u.String(), nil
}

// uniqueDatabaseName menghasilkan nama database sementara yang unik dan aman dipakai sebagai identifier.
func uniqueDatabaseName() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "erpora_test_" + hex.EncodeToString(b)
}

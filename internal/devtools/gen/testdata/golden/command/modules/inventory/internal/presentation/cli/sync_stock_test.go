package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"erpora/sdk/module"
)

func TestSyncStockCommand(t *testing.T) {
	c := SyncStockCommand()
	if c.Name != "sync-stock" || c.Run == nil {
		t.Fatalf("command = %+v", c)
	}
	var out bytes.Buffer
	if err := c.Run(context.Background(), module.Invocation{Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "sync-stock") {
		t.Errorf("keluaran = %q", out.String())
	}
}

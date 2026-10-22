package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rahmadmrhd/erp-go/internal/platform/migrate"
)

func TestMigrateStatus_Format(t *testing.T) {
	var buf bytes.Buffer
	printStatus(&buf, []migrate.TargetStatus{
		{Target: migrate.Control, Current: 1, Latest: 1, Pending: 0},
		{Target: migrate.Tenant, Current: 0, Latest: 2, Pending: 2},
	})
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %q", lines)
	}
	if got := strings.Fields(lines[0]); strings.Join(got, " ") != "TARGET CURRENT LATEST PENDING" {
		t.Errorf("header = %q", lines[0])
	}
	if got := strings.Join(strings.Fields(lines[1]), " "); got != "control 1 1 0" {
		t.Errorf("row1 = %q", got)
	}
	if got := strings.Join(strings.Fields(lines[2]), " "); got != "tenant 0 2 2" {
		t.Errorf("row2 = %q", got)
	}
}

func TestMigrate_InRuntimeGroup(t *testing.T) {
	out, err := run(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "migrate") {
		t.Errorf("migrate missing from list:\n%s", out)
	}
}

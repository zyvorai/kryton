package atlas

import (
	"context"
	"testing"
)

func TestParseStorageClassNames(t *testing.T) {
	names := parseStorageClassNames(`{"items":[{"name":"rook-ceph-block"},{"name":"longhorn"}]}`)
	if len(names) != 2 || names[0] != "rook-ceph-block" {
		t.Fatalf("%v", names)
	}
	names = parseStorageClassNames(`[{"name":"zyvor-rbd-prod"}]`)
	if len(names) != 1 || names[0] != "zyvor-rbd-prod" {
		t.Fatalf("%v", names)
	}
}

func TestTestDisabled(t *testing.T) {
	r := Test(context.Background(), Config{Enabled: false})
	if r.Healthy {
		t.Fatal("expected not healthy when disabled")
	}
	if r.Configured {
		t.Fatal("expected not configured")
	}
}

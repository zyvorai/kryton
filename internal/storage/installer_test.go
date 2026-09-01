package storage

import "testing"

func TestValidateSetupRequest(t *testing.T) {
	if err := validateSetupRequest(SetupRequest{Backend: "longhorn"}); err != nil {
		t.Fatal(err)
	}
	if err := validateSetupRequest(SetupRequest{Backend: "rook-ceph", RookMode: "lab"}); err != nil {
		t.Fatal(err)
	}
	if err := validateSetupRequest(SetupRequest{Backend: "rook-ceph", RookMode: "device", Device: "/dev/sdb1"}); err != nil {
		t.Fatal(err)
	}
	if err := validateSetupRequest(SetupRequest{Backend: "rook-ceph", RookMode: "device"}); err == nil {
		t.Fatal("expected device required")
	}
	if err := validateSetupRequest(SetupRequest{Backend: "rook-ceph", RookMode: "device", Device: "/dev/sdb"}); err == nil {
		t.Fatal("expected sdb blocked")
	}
	if err := validateSetupRequest(SetupRequest{Backend: "other"}); err == nil {
		t.Fatal("expected backend error")
	}
}

func TestBuildSetupCommand(t *testing.T) {
	m := &SetupManager{
		snapshotsScript: "/scripts/enable-kubevirt-snapshots.sh",
		rookScript:      "/scripts/enable-rook-ceph.sh",
	}
	script, args, err := buildSetupCommand(m, SetupRequest{Backend: "longhorn"})
	if err != nil || script != m.snapshotsScript || len(args) != 2 || args[1] != "longhorn" {
		t.Fatalf("longhorn: script=%s args=%v err=%v", script, args, err)
	}
	script, args, err = buildSetupCommand(m, SetupRequest{Backend: "rook-ceph", RookMode: "pool-only"})
	if err != nil || script != m.rookScript || args[0] != "--pool-only" {
		t.Fatalf("pool-only: script=%s args=%v err=%v", script, args, err)
	}
	script, args, err = buildSetupCommand(m, SetupRequest{Backend: "rook-ceph", RookMode: "device", Device: "/dev/sdb1", WipeDevice: true})
	if err != nil || len(args) != 5 || args[4] != "--wipe-device" {
		t.Fatalf("device: args=%v err=%v", args, err)
	}
}

func TestBlockedDeviceReason(t *testing.T) {
	if blockedDeviceReason("/dev/sdb1") != "" {
		t.Fatal("sdb1 should be allowed")
	}
	if blockedDeviceReason("/dev/sdb") == "" {
		t.Fatal("sdb should be blocked")
	}
}

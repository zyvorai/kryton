package model

import "testing"

func TestValidateMachineSpec(t *testing.T) {
	good := MachineSpec{Name: "win-01", Image: "windows-server-2025", Compute: ComputeSpec{CPU: 4, MemoryMiB: 8192}, Disk: DiskSpec{SizeGiB: 80}}
	if err := ValidateMachineSpec(good); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	bad := good
	bad.Name = "Bad Name"
	if err := ValidateMachineSpec(bad); err == nil {
		t.Fatal("invalid name accepted")
	}
}

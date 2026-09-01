// Copyright 2026 Kryton contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

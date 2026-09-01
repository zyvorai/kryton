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

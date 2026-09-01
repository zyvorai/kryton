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

package doctor

import (
	"context"
	"testing"

	"github.com/zyvorai/kryton/internal/catalog"
	"github.com/zyvorai/kryton/internal/demo"
)

func TestRunDemoHealthy(t *testing.T) {
	cat, err := catalog.Load("")
	if err != nil {
		t.Fatal(err)
	}
	report := Run(context.Background(), Input{
		Provider: demo.New(),
		Catalog:  cat,
		AuthMode: "disabled",
		Projects: []string{"default"},
	})
	if !report.Healthy {
		t.Fatalf("expected healthy demo report: %+v", report.Findings)
	}
	if report.Provider != "demo" {
		t.Fatalf("provider=%s", report.Provider)
	}
	if len(report.Findings) < 3 {
		t.Fatalf("expected findings, got %d", len(report.Findings))
	}
}

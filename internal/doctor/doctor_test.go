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

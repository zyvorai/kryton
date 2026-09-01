package events

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
)

func TestFileStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")
	bus, err := New(10, "", "", path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	bus.Publish(context.Background(), "io.kryton.test", "machines/1", map[string]any{"project": "default"})
	bus2, err := New(10, "", "", path, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	items := bus2.List(10)
	if len(items) != 1 || items[0].Type != "io.kryton.test" {
		t.Fatalf("unexpected events: %#v", items)
	}
}

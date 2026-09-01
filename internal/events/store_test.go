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

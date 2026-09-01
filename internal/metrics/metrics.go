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

// Package metrics tracks HTTP request/error counters and per-operation
// counts and exposes them in Prometheus text-exposition format via an
// http.HandlerFunc, without pulling in the full Prometheus client library.
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
)

type Metrics struct {
	requests atomic.Uint64
	errors   atomic.Uint64
	mu       sync.RWMutex
	ops      map[string]uint64
}

func New() *Metrics { return &Metrics{ops: map[string]uint64{}} }
func (m *Metrics) Request(errorResponse bool) {
	m.requests.Add(1)
	if errorResponse {
		m.errors.Add(1)
	}
}
func (m *Metrics) Operation(name string) { m.mu.Lock(); m.ops[name]++; m.mu.Unlock() }

func (m *Metrics) Handler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP kryton_http_requests_total HTTP requests handled.\n# TYPE kryton_http_requests_total counter\nkryton_http_requests_total %d\n", m.requests.Load())
	fmt.Fprintf(w, "# HELP kryton_http_errors_total HTTP error responses.\n# TYPE kryton_http_errors_total counter\nkryton_http_errors_total %d\n", m.errors.Load())
	m.mu.RLock()
	keys := make([]string, 0, len(m.ops))
	for k := range m.ops {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintln(w, "# HELP kryton_operations_total Machine lifecycle operations.")
	fmt.Fprintln(w, "# TYPE kryton_operations_total counter")
	for _, k := range keys {
		fmt.Fprintf(w, "kryton_operations_total{operation=%q} %d\n", k, m.ops[k])
	}
	m.mu.RUnlock()
}

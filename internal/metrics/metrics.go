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

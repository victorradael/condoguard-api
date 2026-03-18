package middleware

import (
	"expvar"
	"net/http"
	"sync/atomic"
	"time"
)

// Metrics holds in-process counters exposed via expvar.
type Metrics struct {
	totalRequests int64
	errorRequests int64
	totalLatencyMs int64

	varRequests *expvar.Int
	varErrors   *expvar.Int
	varLatency  *expvar.Int
}

// NewMetrics registers and returns a new Metrics instance.
// expvar names are unique per process; duplicate registration is ignored.
func NewMetrics() *Metrics {
	m := &Metrics{}
	m.varRequests = expvarInt("http_requests_total")
	m.varErrors = expvarInt("http_errors_total")
	m.varLatency = expvarInt("http_latency_ms_total")
	return m
}

// Middleware wraps a handler to record request count, error count, and latency.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w}

		next.ServeHTTP(wrapped, r)

		status := wrapped.status
		if status == 0 {
			status = http.StatusOK
		}

		elapsed := time.Since(start).Milliseconds()

		atomic.AddInt64(&m.totalRequests, 1)
		atomic.AddInt64(&m.totalLatencyMs, elapsed)
		m.varRequests.Add(1)
		m.varLatency.Add(elapsed)

		if status >= 500 {
			atomic.AddInt64(&m.errorRequests, 1)
			m.varErrors.Add(1)
		}
	})
}

// TotalRequests returns the total number of requests handled.
func (m *Metrics) TotalRequests() int64 { return atomic.LoadInt64(&m.totalRequests) }

// ErrorRequests returns the count of 5xx responses.
func (m *Metrics) ErrorRequests() int64 { return atomic.LoadInt64(&m.errorRequests) }

// TotalLatencyMs returns the cumulative latency in milliseconds.
func (m *Metrics) TotalLatencyMs() int64 { return atomic.LoadInt64(&m.totalLatencyMs) }

// expvarInt gets or registers an expvar.Int by name.
func expvarInt(name string) *expvar.Int {
	if v := expvar.Get(name); v != nil {
		if i, ok := v.(*expvar.Int); ok {
			return i
		}
	}
	return expvar.NewInt(name)
}

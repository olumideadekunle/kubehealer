package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
)

// Registry manages Prometheus metrics for KubeHealer without requiring third-party runtime bloat.
type Registry struct {
	mu                  sync.RWMutex
	incidentsTotal      map[string]*int64 // key: reason:severity
	remediationsTotal   map[string]*int64 // key: action:status
	flappingSuppressions int64
	activeIncidents      int64
}

// NewRegistry constructs a thread-safe metrics registry.
func NewRegistry() *Registry {
	return &Registry{
		incidentsTotal:    make(map[string]*int64),
		remediationsTotal: make(map[string]*int64),
	}
}

// RecordIncident increments the total detected incident counter.
func (r *Registry) RecordIncident(reason, severity string) {
	key := fmt.Sprintf("%s:%s", reason, severity)
	r.mu.Lock()
	counter, exists := r.incidentsTotal[key]
	if !exists {
		var val int64
		counter = &val
		r.incidentsTotal[key] = counter
	}
	r.mu.Unlock()
	atomic.AddInt64(counter, 1)
}

// RecordRemediation tracks an action outcome.
func (r *Registry) RecordRemediation(action, status string) {
	key := fmt.Sprintf("%s:%s", action, status)
	r.mu.Lock()
	counter, exists := r.remediationsTotal[key]
	if !exists {
		var val int64
		counter = &val
		r.remediationsTotal[key] = counter
	}
	r.mu.Unlock()
	atomic.AddInt64(counter, 1)
}

// RecordFlappingSuppressed increments suppression circuit breaker count.
func (r *Registry) RecordFlappingSuppressed() {
	atomic.AddInt64(&r.flappingSuppressions, 1)
}

// Handler returns an HTTP handler serving Prometheus text exposition format.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		fmt.Fprintln(w, "# HELP kubehealer_up Indicates whether KubeHealer is running")
		fmt.Fprintln(w, "# TYPE kubehealer_up gauge")
		fmt.Fprintln(w, "kubehealer_up 1")

		fmt.Fprintln(w, "# HELP kubehealer_incidents_total Total number of detected cluster incidents")
		fmt.Fprintln(w, "# TYPE kubehealer_incidents_total counter")
		r.mu.RLock()
		for k, v := range r.incidentsTotal {
			var reason, severity string
			fmt.Sscanf(k, "%s:%s", &reason, &severity)
			fmt.Fprintf(w, "kubehealer_incidents_total{reason=\"%s\",severity=\"%s\"} %d\n", reason, severity, atomic.LoadInt64(v))
		}

		fmt.Fprintln(w, "# HELP kubehealer_remediations_total Total automated remediation actions taken")
		fmt.Fprintln(w, "# TYPE kubehealer_remediations_total counter")
		for k, v := range r.remediationsTotal {
			var action, status string
			fmt.Sscanf(k, "%s:%s", &action, &status)
			fmt.Fprintf(w, "kubehealer_remediations_total{action=\"%s\",status=\"%s\"} %d\n", action, status, atomic.LoadInt64(v))
		}
		r.mu.RUnlock()

		fmt.Fprintln(w, "# HELP kubehealer_flapping_suppressed_total Total pods suppressed by the anti-flapping circuit breaker")
		fmt.Fprintln(w, "# TYPE kubehealer_flapping_suppressed_total counter")
		fmt.Fprintf(w, "kubehealer_flapping_suppressed_total %d\n", atomic.LoadInt64(&r.flappingSuppressions))
	})
}

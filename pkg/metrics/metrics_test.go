package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetrics_Registry(t *testing.T) {
	reg := NewRegistry()
	reg.RecordIncident("CrashLoopBackOff", "CRITICAL")
	reg.RecordRemediation("RESTART_POD", "RESOLVED")
	reg.RecordFlappingSuppressed()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "kubehealer_up 1") {
		t.Errorf("missing kubehealer_up metric")
	}
	if !strings.Contains(body, "CrashLoopBackOff") {
		t.Errorf("missing incident metric for CrashLoopBackOff")
	}
	if !strings.Contains(body, "kubehealer_flapping_suppressed_total 1") {
		t.Errorf("missing flapping suppression metric")
	}
}

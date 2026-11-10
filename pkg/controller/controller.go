package controller

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/olumideadekunle/kubehealer/pkg/alert"
	"github.com/olumideadekunle/kubehealer/pkg/analyzer"
	"github.com/olumideadekunle/kubehealer/pkg/forensics"
	"github.com/olumideadekunle/kubehealer/pkg/metrics"
	"github.com/olumideadekunle/kubehealer/pkg/playbook"
	"github.com/olumideadekunle/kubehealer/pkg/types"
)

// Controller orchestrates anomaly detection, forensic triage, and automated healing.
type Controller struct {
	config     types.Config
	analyzer   *analyzer.Analyzer
	collector  *forensics.Collector
	engine     *playbook.Engine
	dispatcher *alert.Dispatcher
	metrics    *metrics.Registry

	mu             sync.RWMutex
	restartHistory map[string][]time.Time // key: namespace/podName -> timestamps
	recentIncidents []types.Incident
}

// New constructs an operational KubeHealer controller.
func New(
	cfg types.Config,
	collector *forensics.Collector,
	engine *playbook.Engine,
	dispatcher *alert.Dispatcher,
	registry *metrics.Registry,
) *Controller {
	return &Controller{
		config:         cfg,
		analyzer:       analyzer.New(cfg),
		collector:      collector,
		engine:         engine,
		dispatcher:     dispatcher,
		metrics:        registry,
		restartHistory: make(map[string][]time.Time),
	}
}

// ProcessPod evaluates a pod state against SRE rules and executes self-healing if needed.
func (c *Controller) ProcessPod(ctx context.Context, pod types.PodInfo, recentEvents []string) (*types.Incident, error) {
	podKey := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)

	// 1. Calculate restart count within the flapping window
	c.mu.Lock()
	now := time.Now().UTC()
	cutoff := now.Add(-c.config.FlappingWindow)
	
	// Filter old timestamps
	var validTimestamps []time.Time
	for _, t := range c.restartHistory[podKey] {
		if t.After(cutoff) {
			validTimestamps = append(validTimestamps, t)
		}
	}
	if pod.RestartCount > 0 {
		validTimestamps = append(validTimestamps, now)
	}
	c.restartHistory[podKey] = validTimestamps
	restartsInWindow := len(validTimestamps)
	c.mu.Unlock()

	// 2. Perform Root-Cause Analysis
	reason, severity, action, explanation := c.analyzer.Diagnose(pod, recentEvents, restartsInWindow)
	if action == types.ActionNone {
		return nil, nil // No anomaly detected
	}

	c.metrics.RecordIncident(string(reason), string(severity))

	// 3. Assemble Incident Record
	incident := types.Incident{
		ID:          fmt.Sprintf("INC-%s-%d", pod.Name, time.Now().Unix()),
		CreatedAt:   time.Now().UTC(),
		Pod:         pod,
		Reason:      reason,
		Severity:    severity,
		Status:      types.StatusSnapshotting,
		ActionTaken: action,
	}

	slog.Warn("Anomaly Detected",
		"incident_id", incident.ID,
		"reason", reason,
		"severity", severity,
		"action", action,
		"explanation", explanation,
	)

	// 4. Capture Forensics BEFORE changing state
	incident.Snapshot = c.collector.Capture(ctx, pod, reason)
	incident.Snapshot.DiagnosticAnalysis = explanation

	// 5. Execute Self-Healing Playbook
	if action == types.ActionSuppressFlapping {
		c.metrics.RecordFlappingSuppressed()
	}
	
	err := c.engine.Execute(ctx, &incident)
	c.metrics.RecordRemediation(string(action), string(incident.Status))

	// 6. Record and Notify
	c.mu.Lock()
	c.recentIncidents = append([]types.Incident{incident}, c.recentIncidents...)
	if len(c.recentIncidents) > 100 {
		c.recentIncidents = c.recentIncidents[:100] // Cap audit history
	}
	c.mu.Unlock()

	c.dispatcher.Notify(ctx, incident)

	return &incident, err
}

// GetRecentIncidents returns snapshot of the audit log.
func (c *Controller) GetRecentIncidents() []types.Incident {
	c.mu.RLock()
	defer c.mu.RUnlock()
	res := make([]types.Incident, len(c.recentIncidents))
	copy(res, c.recentIncidents)
	return res
}

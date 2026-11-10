package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/olumideadekunle/kubehealer/pkg/types"
)

// Dispatcher broadcasts incident lifecycle events to external notification targets.
type Dispatcher struct {
	config     types.Config
	httpClient *http.Client
}

// NewDispatcher creates a new alert dispatcher.
func NewDispatcher(cfg types.Config) *Dispatcher {
	return &Dispatcher{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Notify dispatches an incident report to all configured channels.
func (d *Dispatcher) Notify(ctx context.Context, incident types.Incident) {
	// 1. Console structured log
	slog.Info("INCIDENT REPORT",
		"id", incident.ID,
		"severity", incident.Severity,
		"status", incident.Status,
		"reason", incident.Reason,
		"pod", fmt.Sprintf("%s/%s", incident.Pod.Namespace, incident.Pod.Name),
		"action", incident.ActionTaken,
		"remediation", incident.RemediationLog,
	)

	// 2. Dispatch to Slack if configured
	if d.config.SlackWebhookURL != "" {
		go d.sendSlack(ctx, incident)
	}

	// 3. Dispatch to Generic Webhook if configured
	if d.config.WebhookURL != "" {
		go d.sendWebhook(ctx, incident)
	}
}

func (d *Dispatcher) sendSlack(ctx context.Context, incident types.Incident) {
	color := "#36a64f" // Green
	if incident.Severity == types.SeverityWarning {
		color = "#ecaa38" // Yellow
	} else if incident.Severity == types.SeverityCritical {
		color = "#d9534f" // Red
	}

	title := fmt.Sprintf("[%s] %s: %s/%s", incident.Severity, incident.Reason, incident.Pod.Namespace, incident.Pod.Name)

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color": color,
				"title": title,
				"fields": []map[string]interface{}{
					{"title": "Incident ID", "value": incident.ID, "short": true},
					{"title": "Cluster", "value": d.config.ClusterName, "short": true},
					{"title": "Status", "value": string(incident.Status), "short": true},
					{"title": "Action Taken", "value": string(incident.ActionTaken), "short": true},
					{"title": "Remediation Details", "value": incident.RemediationLog, "short": false},
					{"title": "Diagnostic Analysis", "value": incident.Snapshot.DiagnosticAnalysis, "short": false},
				},
				"footer": "KubeHealer Autonomous SRE Controller",
				"ts":     incident.CreatedAt.Unix(),
			},
		},
	}

	d.postJSON(ctx, d.config.SlackWebhookURL, payload)
}

func (d *Dispatcher) sendWebhook(ctx context.Context, incident types.Incident) {
	d.postJSON(ctx, d.config.WebhookURL, incident)
}

func (d *Dispatcher) postJSON(ctx context.Context, url string, payload interface{}) {
	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Failed to marshal webhook payload", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		slog.Error("Failed to construct webhook request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "KubeHealer-SRE-Operator/1.0")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		slog.Warn("Failed to deliver webhook notification", "url", url, "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		slog.Warn("Webhook responded with non-2xx status code", "status", resp.StatusCode)
	}
}

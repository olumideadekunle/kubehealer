package types

import (
	"time"
)

// Severity indicates the urgency of an incident.
type Severity string

const (
	SeverityInfo     Severity = "INFO"
	SeverityWarning  Severity = "WARNING"
	SeverityCritical Severity = "CRITICAL"
)

// IncidentReason represents the diagnosed root cause.
type IncidentReason string

const (
	ReasonCrashLoopBackOff IncidentReason = "CrashLoopBackOff"
	ReasonOOMKilled        IncidentReason = "OOMKilled"
	ReasonProbeFailure     IncidentReason = "UnhealthyLivenessProbe"
	ReasonImagePullBackOff IncidentReason = "ImagePullBackOff"
	ReasonEvicted          IncidentReason = "PodEvicted"
	ReasonDeadlockDetected IncidentReason = "DeadlockDetected"
	ReasonUnknown          IncidentReason = "UnknownFailure"
)

// RemediationAction represents the automated self-healing action.
type RemediationAction string

const (
	ActionNone               RemediationAction = "NONE"
	ActionRestartPod         RemediationAction = "RESTART_POD"
	ActionEvictPod           RemediationAction = "EVICT_POD"
	ActionRollbackDeployment RemediationAction = "ROLLBACK_DEPLOYMENT"
	ActionScaleDeployment    RemediationAction = "SCALE_DEPLOYMENT"
	ActionQuarantinePod      RemediationAction = "QUARANTINE_POD"
	ActionSuppressFlapping   RemediationAction = "SUPPRESS_FLAPPING"
)

// IncidentStatus represents the state machine of an incident triage.
type IncidentStatus string

const (
	StatusDetected    IncidentStatus = "DETECTED"
	StatusSnapshotting IncidentStatus = "SNAPSHOTTING"
	StatusRemediating IncidentStatus = "REMEDIATING"
	StatusResolved    IncidentStatus = "RESOLVED"
	StatusSuppressed  IncidentStatus = "SUPPRESSED"
	StatusFailed      IncidentStatus = "FAILED"
)

// PodInfo holds metadata of the affected Kubernetes pod.
type PodInfo struct {
	Name         string            `json:"name"`
	Namespace    string            `json:"namespace"`
	UID          string            `json:"uid"`
	NodeName     string            `json:"node_name"`
	OwnerKind    string            `json:"owner_kind"` // e.g. Deployment, ReplicaSet, StatefulSet
	OwnerName    string            `json:"owner_name"`
	Container    string            `json:"container"`
	RestartCount int32             `json:"restart_count"`
	ExitCode     int32             `json:"exit_code"`
	Phase        string            `json:"phase"`
	Labels       map[string]string `json:"labels,omitempty"`
}

// ForensicSnapshot captures pre-remediation diagnostic artifacts.
type ForensicSnapshot struct {
	CapturedAt         time.Time `json:"captured_at"`
	RecentLogs         string    `json:"recent_logs"`
	PreviousLogs       string    `json:"previous_logs,omitempty"`
	MemoryUsageBytes   int64     `json:"memory_usage_bytes,omitempty"`
	CPUUsageMillicores int64     `json:"cpu_usage_millicores,omitempty"`
	NodeCondition      string    `json:"node_condition,omitempty"`
	DiagnosticAnalysis string    `json:"diagnostic_analysis"`
}

// Incident models a discrete SRE event handled by KubeHealer.
type Incident struct {
	ID          string            `json:"id"`
	CreatedAt   time.Time         `json:"created_at"`
	ResolvedAt  *time.Time        `json:"resolved_at,omitempty"`
	Pod         PodInfo           `json:"pod"`
	Reason      IncidentReason    `json:"reason"`
	Severity    Severity          `json:"severity"`
	Status      IncidentStatus    `json:"status"`
	Snapshot    ForensicSnapshot  `json:"snapshot"`
	ActionTaken RemediationAction `json:"action_taken"`
	RemediationLog string         `json:"remediation_log"`
	Error       string            `json:"error,omitempty"`
}

// Config represents runtime controller configuration.
type Config struct {
	ClusterName         string        `yaml:"cluster_name" json:"cluster_name"`
	WatchNamespaces     []string      `yaml:"watch_namespaces" json:"watch_namespaces"`
	DryRun              bool          `yaml:"dry_run" json:"dry_run"`
	FlappingThreshold   int           `yaml:"flapping_threshold" json:"flapping_threshold"`       // Max restarts before suppression
	FlappingWindow      time.Duration `yaml:"flapping_window" json:"flapping_window"`             // Time window to count flapping (e.g. 10m)
	AutoRollbackEnabled bool          `yaml:"auto_rollback_enabled" json:"auto_rollback_enabled"` // Enable rollout undo for cascading crashes
	MetricsPort         int           `yaml:"metrics_port" json:"metrics_port"`
	SlackWebhookURL     string        `yaml:"slack_webhook_url" json:"slack_webhook_url"`
	WebhookURL          string        `yaml:"webhook_url" json:"webhook_url"`
	LogLevel            string        `yaml:"log_level" json:"log_level"`
}

// DefaultConfig provides sensible production defaults.
func DefaultConfig() Config {
	return Config{
		ClusterName:         "production-cluster",
		WatchNamespaces:     []string{"default", "prod", "staging"},
		DryRun:              false,
		FlappingThreshold:   5,
		FlappingWindow:      10 * time.Minute,
		AutoRollbackEnabled: true,
		MetricsPort:         8080,
		LogLevel:            "info",
	}
}

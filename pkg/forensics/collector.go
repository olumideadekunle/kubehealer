package forensics

import (
	"context"
	"fmt"
	"time"

	"github.com/olumideadekunle/kubehealer/pkg/types"
)

// LogProvider abstracts Kubernetes log extraction (supports real client-go and mock tests).
type LogProvider interface {
	GetRecentLogs(ctx context.Context, namespace, podName, container string, tailLines int) (string, error)
	GetPreviousLogs(ctx context.Context, namespace, podName, container string, tailLines int) (string, error)
	GetNodeCondition(ctx context.Context, nodeName string) (string, error)
}

// Collector assembles post-mortem incident evidence snapshots before remediation actions occur.
type Collector struct {
	provider LogProvider
}

// NewCollector constructs a new forensic evidence collector.
func NewCollector(provider LogProvider) *Collector {
	return &Collector{provider: provider}
}

// Capture compiles logs, node conditions, and failure diagnostics.
func (c *Collector) Capture(ctx context.Context, pod types.PodInfo, reason types.IncidentReason) types.ForensicSnapshot {
	snapshot := types.ForensicSnapshot{
		CapturedAt: time.Now().UTC(),
	}

	// 1. Capture recent container logs
	if c.provider != nil {
		logs, err := c.provider.GetRecentLogs(ctx, pod.Namespace, pod.Name, pod.Container, 50)
		if err == nil {
			snapshot.RecentLogs = logs
		} else {
			snapshot.RecentLogs = fmt.Sprintf("[Log capture unavailable: %v]", err)
		}

		// 2. Capture previous crash logs if restarted
		if pod.RestartCount > 0 {
			prevLogs, err := c.provider.GetPreviousLogs(ctx, pod.Namespace, pod.Name, pod.Container, 50)
			if err == nil && prevLogs != "" {
				snapshot.PreviousLogs = prevLogs
			}
		}

		// 3. Capture Node conditions
		if pod.NodeName != "" {
			cond, err := c.provider.GetNodeCondition(ctx, pod.NodeName)
			if err == nil {
				snapshot.NodeCondition = cond
			}
		}
	} else {
		snapshot.RecentLogs = "[No log provider configured]"
	}

	// 4. Synthesize diagnostic analysis
	switch reason {
	case types.ReasonOOMKilled:
		snapshot.DiagnosticAnalysis = fmt.Sprintf(
			"PROCESS TERMINATED BY KERNEL: Container '%s' hit cgroup memory limits. "+
				"Recommendation: Increase deployment resources.limits.memory by 25-50%% or investigate memory leak in application heap.",
			pod.Container,
		)
	case types.ReasonCrashLoopBackOff:
		snapshot.DiagnosticAnalysis = fmt.Sprintf(
			"APPLICATION RUNTIME CRASH: Container '%s' exited with code %d. "+
				"Inspect recent stderr trace above for unhandled exceptions or failed startup connections.",
			pod.Container, pod.ExitCode,
		)
	case types.ReasonProbeFailure:
		snapshot.DiagnosticAnalysis = fmt.Sprintf(
			"HEALTH CHECK TIMEOUT: Container '%s' did not respond to liveness probe. Check for thread deadlocks, high CPU throttling, or network saturation.",
			pod.Container,
		)
	default:
		snapshot.DiagnosticAnalysis = fmt.Sprintf("General anomaly recorded for pod %s/%s.", pod.Namespace, pod.Name)
	}

	return snapshot
}

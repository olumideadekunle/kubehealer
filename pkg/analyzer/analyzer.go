package analyzer

import (
	"fmt"
	"strings"

	"github.com/olumideadekunle/kubehealer/pkg/types"
)

// Analyzer inspects pod state and historical events to diagnose root cause.
type Analyzer struct {
	config types.Config
}

// New creates an initialized Analyzer instance.
func New(cfg types.Config) *Analyzer {
	return &Analyzer{config: cfg}
}

// Diagnose evaluates the pod info and determines the failure cause, severity, and proposed action.
func (a *Analyzer) Diagnose(pod types.PodInfo, recentEvents []string, restartHistoryCount int) (types.IncidentReason, types.Severity, types.RemediationAction, string) {
	// 1. Check Flapping Circuit Breaker
	if restartHistoryCount >= a.config.FlappingThreshold {
		return types.ReasonCrashLoopBackOff, types.SeverityCritical, types.ActionSuppressFlapping,
			fmt.Sprintf("Flapping threshold exceeded (%d restarts in %s). Automatic restarts suppressed to prevent cascading cluster storm.",
				restartHistoryCount, a.config.FlappingWindow)
	}

	// 2. Out of Memory (OOMKilled)
	// Linux SIGKILL 9 + 128 = 137
	if pod.ExitCode == 137 || containsAny(recentEvents, "OOMKilled", "out of memory", "cgroup killed") {
		return types.ReasonOOMKilled, types.SeverityCritical, types.ActionRestartPod,
			fmt.Sprintf("Container %s exceeded memory limits (exit code 137 - OOMKilled). Evicting dead pod and requesting node scheduler re-placement.", pod.Container)
	}

	// 3. ImagePullBackOff / Registry Failure
	if pod.Phase == "Pending" || containsAny(recentEvents, "ImagePullBackOff", "ErrImagePull", "manifest unknown") {
		return types.ReasonImagePullBackOff, types.SeverityWarning, types.ActionQuarantinePod,
			fmt.Sprintf("Container image pull failure in %s. Quarantining to prevent useless restart cycles.", pod.Container)
	}

	// 4. Node Eviction
	if pod.Phase == "Failed" || containsAny(recentEvents, "Evicted", "DiskPressure", "MemoryPressure") {
		return types.ReasonEvicted, types.SeverityCritical, types.ActionEvictPod,
			fmt.Sprintf("Pod evicted by kubelet due to node pressure. Purging tombstone pod.")
	}

	// 5. Unhealthy Liveness / Readiness Probes
	if containsAny(recentEvents, "Unhealthy", "Liveness probe failed", "Readiness probe failed", "connection refused") {
		return types.ReasonProbeFailure, types.SeverityWarning, types.ActionRestartPod,
			fmt.Sprintf("Container %s failed consecutive liveness/readiness health probes. Initiating graceful restart.", pod.Container)
	}

	// 6. Rapid Deployment CrashLoop -> Trigger Automated Rollback
	if pod.RestartCount >= 2 && pod.OwnerKind == "Deployment" && a.config.AutoRollbackEnabled && containsAny(recentEvents, "CrashLoopBackOff", "BackOff", "exit code 1") {
		return types.ReasonCrashLoopBackOff, types.SeverityCritical, types.ActionRollbackDeployment,
			fmt.Sprintf("Cascading crashloop detected on newly rolled out Deployment '%s'. Triggering zero-downtime rollback to previous stable ReplicaSet.", pod.OwnerName)
	}

	// 7. General CrashLoopBackOff
	if pod.RestartCount >= 1 || containsAny(recentEvents, "CrashLoopBackOff", "BackOff") {
		return types.ReasonCrashLoopBackOff, types.SeverityWarning, types.ActionRestartPod,
			fmt.Sprintf("Container %s entered CrashLoopBackOff (exit code %d). Performing graceful restart.", pod.Container, pod.ExitCode)
	}

	return types.ReasonUnknown, types.SeverityInfo, types.ActionNone, "No actionable anomaly detected."
}

func containsAny(slice []string, terms ...string) bool {
	for _, item := range slice {
		lowerItem := strings.ToLower(item)
		for _, term := range terms {
			if strings.Contains(lowerItem, strings.ToLower(term)) {
				return true
			}
		}
	}
	return false
}

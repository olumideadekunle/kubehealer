package playbook

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/olumideadekunle/kubehealer/pkg/types"
)

// KubeClient abstracts cluster mutating operations (supports production client-go and mock drivers).
type KubeClient interface {
	DeletePod(ctx context.Context, namespace, podName string, gracePeriodSeconds int64) error
	RollbackDeployment(ctx context.Context, namespace, deploymentName string) error
	AnnotatePod(ctx context.Context, namespace, podName string, annotations map[string]string) error
}

// Engine executes automated self-healing playbooks with safety constraints.
type Engine struct {
	client KubeClient
	config types.Config
}

// NewEngine creates an operational playbook execution engine.
func NewEngine(client KubeClient, cfg types.Config) *Engine {
	return &Engine{
		client: client,
		config: cfg,
	}
}

// Execute triggers the required remediation action.
func (e *Engine) Execute(ctx context.Context, incident *types.Incident) error {
	start := time.Now()

	if e.config.DryRun {
		slog.Info("[DRY-RUN] Simulating remediation action",
			"action", incident.ActionTaken,
			"pod", incident.Pod.Name,
			"namespace", incident.Pod.Namespace,
		)
		incident.Status = types.StatusResolved
		incident.RemediationLog = fmt.Sprintf("[DRY-RUN] Action %s simulated successfully.", incident.ActionTaken)
		return nil
	}

	incident.Status = types.StatusRemediating

	var err error
	switch incident.ActionTaken {
	case types.ActionRestartPod:
		err = e.restartPod(ctx, incident)
	case types.ActionRollbackDeployment:
		err = e.rollbackDeployment(ctx, incident)
	case types.ActionEvictPod:
		err = e.evictPod(ctx, incident)
	case types.ActionSuppressFlapping:
		err = e.suppressFlapping(ctx, incident)
	case types.ActionQuarantinePod:
		err = e.quarantinePod(ctx, incident)
	case types.ActionNone:
		incident.Status = types.StatusResolved
		incident.RemediationLog = "No remediation required."
		return nil
	default:
		return fmt.Errorf("unsupported remediation action: %s", incident.ActionTaken)
	}

	duration := time.Since(start)
	if err != nil {
		incident.Status = types.StatusFailed
		incident.Error = err.Error()
		return fmt.Errorf("playbook execution failed for %s: %w", incident.ActionTaken, err)
	}

	incident.Status = types.StatusResolved
	now := time.Now().UTC()
	incident.ResolvedAt = &now
	incident.RemediationLog += fmt.Sprintf(" (Execution time: %s)", duration.Round(time.Millisecond))
	return nil
}

func (e *Engine) restartPod(ctx context.Context, incident *types.Incident) error {
	slog.Info("Executing Playbook: Graceful Pod Restart",
		"namespace", incident.Pod.Namespace,
		"pod", incident.Pod.Name,
	)
	if e.client == nil {
		incident.RemediationLog = "Pod restart executed (simulated client)."
		return nil
	}
	err := e.client.DeletePod(ctx, incident.Pod.Namespace, incident.Pod.Name, 15)
	if err != nil {
		return err
	}
	incident.RemediationLog = "Pod gracefully deleted; owner controller is spinning up fresh replacement."
	return nil
}

func (e *Engine) rollbackDeployment(ctx context.Context, incident *types.Incident) error {
	deployName := incident.Pod.OwnerName
	slog.Warn("Executing Playbook: Automated Deployment Rollback",
		"namespace", incident.Pod.Namespace,
		"deployment", deployName,
	)
	if e.client == nil {
		incident.RemediationLog = fmt.Sprintf("Deployment %s rolled back to previous revision (simulated).", deployName)
		return nil
	}
	err := e.client.RollbackDeployment(ctx, incident.Pod.Namespace, deployName)
	if err != nil {
		return err
	}
	incident.RemediationLog = fmt.Sprintf("Deployment '%s' successfully rolled back to previous stable ReplicaSet revision.", deployName)
	return nil
}

func (e *Engine) evictPod(ctx context.Context, incident *types.Incident) error {
	slog.Info("Executing Playbook: Evict Tombstone Pod",
		"namespace", incident.Pod.Namespace,
		"pod", incident.Pod.Name,
	)
	if e.client == nil {
		incident.RemediationLog = "Eviction executed (simulated client)."
		return nil
	}
	err := e.client.DeletePod(ctx, incident.Pod.Namespace, incident.Pod.Name, 0)
	if err != nil {
		return err
	}
	incident.RemediationLog = "Dead/evicted pod purged from node."
	return nil
}

func (e *Engine) suppressFlapping(ctx context.Context, incident *types.Incident) error {
	slog.Warn("Executing Playbook: Flapping Suppression Circuit Breaker",
		"namespace", incident.Pod.Namespace,
		"pod", incident.Pod.Name,
	)
	incident.Status = types.StatusSuppressed
	if e.client != nil {
		annotations := map[string]string{
			"kubehealer.io/flapping":   "suppressed",
			"kubehealer.io/updated_at": time.Now().UTC().Format(time.RFC3339),
		}
		_ = e.client.AnnotatePod(ctx, incident.Pod.Namespace, incident.Pod.Name, annotations)
	}
	incident.RemediationLog = "Circuit breaker triggered: automatic restarts paused to prevent node degradation. Human SRE intervention requested."
	return nil
}

func (e *Engine) quarantinePod(ctx context.Context, incident *types.Incident) error {
	slog.Info("Executing Playbook: Quarantine Pod",
		"namespace", incident.Pod.Namespace,
		"pod", incident.Pod.Name,
	)
	if e.client != nil {
		annotations := map[string]string{
			"kubehealer.io/quarantine": "true",
		}
		_ = e.client.AnnotatePod(ctx, incident.Pod.Namespace, incident.Pod.Name, annotations)
	}
	incident.RemediationLog = "Pod isolated and marked for developer inspection."
	return nil
}

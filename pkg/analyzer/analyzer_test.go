package analyzer

import (
	"testing"
	"time"

	"github.com/olumideadekunle/kubehealer/pkg/types"
)

func TestDiagnose_OOMKilled(t *testing.T) {
	cfg := types.DefaultConfig()
	an := New(cfg)

	pod := types.PodInfo{
		Name:      "redis-cache-0",
		Namespace: "default",
		Container: "redis",
		ExitCode:  137,
		Phase:     "Running",
	}

	reason, severity, action, _ := an.Diagnose(pod, []string{"OOMKilled"}, 0)

	if reason != types.ReasonOOMKilled {
		t.Errorf("expected reason %s, got %s", types.ReasonOOMKilled, reason)
	}
	if severity != types.SeverityCritical {
		t.Errorf("expected critical severity, got %s", severity)
	}
	if action != types.ActionRestartPod {
		t.Errorf("expected action %s, got %s", types.ActionRestartPod, action)
	}
}

func TestDiagnose_CrashLoopRollback(t *testing.T) {
	cfg := types.DefaultConfig()
	cfg.AutoRollbackEnabled = true
	an := New(cfg)

	pod := types.PodInfo{
		Name:         "frontend-v2-abc-123",
		Namespace:    "prod",
		Container:    "app",
		OwnerKind:    "Deployment",
		OwnerName:    "frontend-v2",
		RestartCount: 3,
		ExitCode:     1,
		Phase:        "Running",
	}

	reason, _, action, _ := an.Diagnose(pod, []string{"CrashLoopBackOff"}, 2)

	if reason != types.ReasonCrashLoopBackOff {
		t.Errorf("expected reason %s, got %s", types.ReasonCrashLoopBackOff, reason)
	}
	if action != types.ActionRollbackDeployment {
		t.Errorf("expected action %s, got %s", types.ActionRollbackDeployment, action)
	}
}

func TestDiagnose_FlappingSuppression(t *testing.T) {
	cfg := types.DefaultConfig()
	cfg.FlappingThreshold = 5
	cfg.FlappingWindow = 10 * time.Minute
	an := New(cfg)

	pod := types.PodInfo{
		Name:         "unstable-pod-xyz",
		Namespace:    "staging",
		RestartCount: 8,
		ExitCode:     1,
	}

	_, severity, action, _ := an.Diagnose(pod, []string{"CrashLoopBackOff"}, 6)

	if action != types.ActionSuppressFlapping {
		t.Errorf("expected action %s, got %s", types.ActionSuppressFlapping, action)
	}
	if severity != types.SeverityCritical {
		t.Errorf("expected severity critical on flapping suppression, got %s", severity)
	}
}

func TestDiagnose_ImagePullBackOff(t *testing.T) {
	cfg := types.DefaultConfig()
	an := New(cfg)

	pod := types.PodInfo{
		Name:      "broken-image-pod",
		Namespace: "default",
		Phase:     "Pending",
	}

	reason, _, action, _ := an.Diagnose(pod, []string{"ImagePullBackOff"}, 0)

	if reason != types.ReasonImagePullBackOff {
		t.Errorf("expected reason %s, got %s", types.ReasonImagePullBackOff, reason)
	}
	if action != types.ActionQuarantinePod {
		t.Errorf("expected action %s, got %s", types.ActionQuarantinePod, action)
	}
}

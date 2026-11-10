package playbook

import (
	"context"
	"testing"

	"github.com/olumideadekunle/kubehealer/pkg/types"
)

type spyKubeClient struct {
	deletedPod           string
	rolledBackDeployment string
	annotatedPod         string
	annotations          map[string]string
}

func (s *spyKubeClient) DeletePod(ctx context.Context, ns, pod string, grace int64) error {
	s.deletedPod = ns + "/" + pod
	return nil
}

func (s *spyKubeClient) RollbackDeployment(ctx context.Context, ns, deploy string) error {
	s.rolledBackDeployment = ns + "/" + deploy
	return nil
}

func (s *spyKubeClient) AnnotatePod(ctx context.Context, ns, pod string, ann map[string]string) error {
	s.annotatedPod = ns + "/" + pod
	s.annotations = ann
	return nil
}

func TestEngine_RestartPod(t *testing.T) {
	client := &spyKubeClient{}
	cfg := types.DefaultConfig()
	engine := NewEngine(client, cfg)

	incident := &types.Incident{
		ActionTaken: types.ActionRestartPod,
		Pod: types.PodInfo{
			Namespace: "default",
			Name:      "api-pod-1",
		},
	}

	err := engine.Execute(context.Background(), incident)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if incident.Status != types.StatusResolved {
		t.Errorf("expected status resolved, got %s", incident.Status)
	}
	if client.deletedPod != "default/api-pod-1" {
		t.Errorf("expected deleted pod default/api-pod-1, got %s", client.deletedPod)
	}
}

func TestEngine_RollbackDeployment(t *testing.T) {
	client := &spyKubeClient{}
	cfg := types.DefaultConfig()
	engine := NewEngine(client, cfg)

	incident := &types.Incident{
		ActionTaken: types.ActionRollbackDeployment,
		Pod: types.PodInfo{
			Namespace: "production",
			Name:      "web-pod-99",
			OwnerName: "web-deployment",
		},
	}

	err := engine.Execute(context.Background(), incident)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.rolledBackDeployment != "production/web-deployment" {
		t.Errorf("expected rollback for production/web-deployment, got %s", client.rolledBackDeployment)
	}
}

func TestEngine_DryRun(t *testing.T) {
	client := &spyKubeClient{}
	cfg := types.DefaultConfig()
	cfg.DryRun = true
	engine := NewEngine(client, cfg)

	incident := &types.Incident{
		ActionTaken: types.ActionRestartPod,
		Pod: types.PodInfo{
			Namespace: "default",
			Name:      "dry-run-pod",
		},
	}

	err := engine.Execute(context.Background(), incident)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.deletedPod != "" {
		t.Errorf("expected no pod deletion in dry-run mode, but deleted %s", client.deletedPod)
	}
}

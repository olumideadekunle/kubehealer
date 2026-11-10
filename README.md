# KubeHealer

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go&logoColor=white)](https://golang.org)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-1.28+-326CE5?style=flat&logo=kubernetes&logoColor=white)](https://kubernetes.io)
[![CI](https://img.shields.io/badge/CI-Passing-brightgreen?style=flat&logo=githubactions&logoColor=white)](https://github.com/olumideadekunle/kubehealer/actions)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker&logoColor=white)](Dockerfile)

**KubeHealer** is an enterprise-grade autonomous Site Reliability Engineering (SRE) and auto-remediation controller for Kubernetes clusters.

It bridges the gap between raw cluster metrics and human SRE intervention. When workloads enter failure states (`CrashLoopBackOff`, `OOMKilled`, probe timeouts, or bad release rollouts), KubeHealer **captures diagnostic forensics before container death**, **applies safely bounded self-healing playbooks**, and **dispatches root-cause analysis alerts** to Slack, Webhooks, and Prometheus.

---

## The Problem: The 3:00 AM SRE Firefight

In modern cloud-native environments:
1. **Ephemeral Evidence Destruction**: When an `OOMKilled` or `CrashLoopBackOff` pod restarts or is evicted by the kubelet, temporary stack traces, in-memory heap leaks, and stdout/stderr buffers are destroyed before an on-call engineer can inspect them.
2. **Cascading Deployment Failures**: A bad container release or bad configuration can trigger cascading crash loops across entire replica sets, impacting end users before humans can run `kubectl rollout undo`.
3. **Flapping Storms**: Naive auto-restart loops consume node CPU/memory cycles and throttle Kubernetes API servers when an underlying external database or secret is missing.

---

## How KubeHealer Solves It

```
               ┌─────────────────────────────────────────────────────────────┐
               │                  Kubernetes Cluster API                     │
               │        (Pods, Events, Deployments, Node Conditions)         │
               └──────────────────────────────┬──────────────────────────────┘
                                              │ Real-Time Watch & Informer
                                              ▼
               ┌─────────────────────────────────────────────────────────────┐
               │                    KubeHealer Controller                    │
               │                                                             │
               │   1. Telemetry Ingestion & Anomaly Classifier               │
               │      ├─ CrashLoopBackOff Detection                          │
               │      ├─ OOMKilled (Exit 137 / CGroup Memory Spike)          │
               │      └─ Liveness / Readiness Probe Timeouts                 │
               │                                                             │
               │   2. Pre-Kill Forensic Capture Engine                       │
               │      ├─ Snapshots last 100 stdout/stderr lines              │
               │      ├─ Terminated container logs (previous=true)           │
               │      └─ Node pressure inspection (Memory/Disk)              │
               │                                                             │
               │   3. Self-Healing Playbook Orchestrator                     │
               │      ├─ Safe Eviction & Graceful Pod Restart                │
               │      ├─ Zero-Downtime Deployment Rollback                   │
               │      └─ Anti-Flapping Circuit Breaker                       │
               └───────────────┬───────────────────────────────┬─────────────┘
                               │                               │
                               ▼                               ▼
               ┌───────────────────────────────┐ ┌───────────────────────────┐
               │  Multi-Channel Incident Dispatch│ │ Prometheus Metrics Server │
               │  ├─ Rich Slack Block-Kit Cards│ │  ├─ /metrics (Prometheus) │
               │  ├─ Generic JSON Webhooks     │ │  ├─ /healthz (K8s Probe)  │
               │  └─ REST Audit API            │ │  └─ /api/v1/incidents     │
               └───────────────────────────────┘ └───────────────────────────┘
```

---

## Architectural Highlights

### 1. Pre-Kill Forensic Snapshotting
Before initiating any restart or eviction, KubeHealer automatically takes a diagnostic snapshot:
- Captures recent stdout/stderr lines from the failing container.
- Fetches prior container crash logs (`previous=true`).
- Inspects node memory and disk pressure conditions.
- Attaches the diagnostic analysis directly to the generated Incident Report.

### 2. Flapping Suppression Circuit Breaker
To prevent restart storms, KubeHealer monitors restarts within a sliding window (e.g. 5 restarts in 10 minutes):
- If the threshold is exceeded, KubeHealer **pauses automatic restarts**.
- Annotates the pod with `kubehealer.io/flapping: suppressed`.
- Fires an escalation alert requesting human SRE triage.

### 3. Cascading Deployment Rollback Safeguard
If a newly deployed ReplicaSet suffers multiple immediate container crashes, KubeHealer safely reverts the Deployment to its previous stable revision (`rollout undo`), preventing customer-facing downtime.

### 4. Built-In Prometheus Metrics & Health Probe
Exports OpenMetrics on `:8080/metrics`:
- `kubehealer_incidents_total{reason, severity}`
- `kubehealer_remediations_total{action, status}`
- `kubehealer_flapping_suppressed_total`
- `kubehealer_up 1`

---

## Quickstart: Interactive Chaos Demonstration

Experience KubeHealer's autonomous SRE loop in 5 seconds with **zero Kubernetes cluster prerequisites**:

```bash
# Clone the repository
git clone https://github.com/olumideadekunle/kubehealer.git
cd kubehealer

# Run the live interactive simulation
go run ./cmd/kubehealer -mode=simulate
```

### What You Will See:
1. **Scenario 1**: High-severity `OOMKilled` (Exit Code 137) in a payment gateway. KubeHealer captures the memory stack trace, logs a recommendation to raise memory limits, and gracefully restarts the pod.
2. **Scenario 2**: Newly rolled-out microservice entering `CrashLoopBackOff`. KubeHealer triggers an automated zero-downtime rollback to the previous stable ReplicaSet.
3. **Scenario 3**: Rapidly crashing worker pod. KubeHealer triggers the **Anti-Flapping Circuit Breaker**, pauses restarts, and escalates to human on-call engineers.

---

## Kubernetes Production Deployment

### Option 1: Helm Installation (Recommended)

```bash
helm install kubehealer ./deploy/helm/kubehealer \
  --namespace kube-system \
  --set config.clusterName="prod-us-east-1" \
  --set config.slackWebhookUrl="https://hooks.slack.com/services/..."
```

### Option 2: Standard Manifests

```bash
# 1. Apply RBAC service account & permissions
kubectl apply -f deploy/rbac.yaml

# 2. Deploy KubeHealer controller
kubectl apply -f deploy/deployment.yaml
```

---

## Testing with Synthetic Chaos Pods

Deploy sample failure pods into your test cluster:

```bash
# Test OOMKilled self-healing
kubectl apply -f examples/chaos-oom.yaml

# Test CrashLoopBackOff detection & rollback
kubectl apply -f examples/chaos-crashloop.yaml
```

---

## Prometheus Metrics Reference

| Metric Name | Type | Description |
|---|---|---|
| `kubehealer_up` | Gauge | Controller health status (1 = operational) |
| `kubehealer_incidents_total` | Counter | Total incidents diagnosed by reason and severity |
| `kubehealer_remediations_total` | Counter | Total self-healing actions executed by status |
| `kubehealer_flapping_suppressed_total` | Counter | Workloads paused by anti-flapping circuit breaker |

---

## REST API Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/healthz` | Kubernetes liveness and readiness probe |
| `GET` | `/metrics` | Prometheus metrics exposition format |
| `GET` | `/api/v1/incidents` | Real-time audit log of recent incidents & actions |

---

## Running Automated Tests

```bash
go test -v -race ./...
```

---

## Pushing to GitHub

```bash
cd kubehealer
git init
git add .
git commit -m "feat: Initial commit for KubeHealer autonomous Kubernetes SRE controller"
git branch -M main
git remote add origin https://github.com/olumideadekunle/kubehealer.git
git push -u origin main
```

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

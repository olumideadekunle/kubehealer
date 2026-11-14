package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/olumideadekunle/kubehealer/pkg/alert"
	"github.com/olumideadekunle/kubehealer/pkg/controller"
	"github.com/olumideadekunle/kubehealer/pkg/forensics"
	"github.com/olumideadekunle/kubehealer/pkg/metrics"
	"github.com/olumideadekunle/kubehealer/pkg/playbook"
	"github.com/olumideadekunle/kubehealer/pkg/types"
)

//go:embed web/index.html
var indexHTML []byte

var (
	version = "1.0.0"
	commit  = "dev"
)

func main() {
	var (
		mode       = flag.String("mode", "simulate", "Operating mode: 'simulate' (local chaos demo) or 'daemon' (background controller)")
		port       = flag.Int("port", 8080, "Prometheus metrics and HTTP API port")
		dryRun     = flag.Bool("dry-run", false, "Simulate actions without executing cluster mutations")
		slackURL   = flag.String("slack-webhook", "", "Slack incoming webhook URL for incident alerts")
		flappingTh = flag.Int("flapping-threshold", 4, "Maximum restarts allowed before triggering suppression")
	)
	flag.Parse()

	// Configure structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	printBanner()

	// Honor $PORT when provided by the hosting platform (e.g. Vercel, Cloud Run).
	if p, err := strconv.Atoi(os.Getenv("PORT")); err == nil && p > 0 {
		*port = p
	}

	cfg := types.DefaultConfig()
	cfg.MetricsPort = *port
	cfg.DryRun = *dryRun
	cfg.SlackWebhookURL = *slackURL
	cfg.FlappingThreshold = *flappingTh

	// Initialize components
	reg := metrics.NewRegistry()
	disp := alert.NewDispatcher(cfg)
	collector := forensics.NewCollector(&mockLogProvider{})
	engine := playbook.NewEngine(&mockKubeClient{}, cfg)
	ctrl := controller.New(cfg, collector, engine, disp, reg)

	// Start HTTP Server (Prometheus metrics & REST API)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexHTML)
	})
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"service":   "kubehealer",
			"version":   version,
			"status":    "ok",
			"mode":      *mode,
			"endpoints": []string{"/healthz", "/metrics", "/api/v1/incidents", "/api/v1/demo"},
		})
	})
	// Live demo: inject the three chaos scenarios and return the resulting incidents.
	mux.HandleFunc("/api/v1/demo", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "use POST", http.StatusMethodNotAllowed)
			return
		}
		results := make([]interface{}, 0, 3)
		for _, sc := range demoScenarios() {
			inc, err := ctrl.ProcessPod(r.Context(), sc.pod, sc.recentEvents)
			if err != nil {
				results = append(results, map[string]string{"scenario": sc.name, "error": err.Error()})
				continue
			}
			if inc != nil {
				results = append(results, map[string]interface{}{"scenario": sc.name, "incident": inc})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": len(results), "results": results})
	})
	mux.Handle("/metrics", reg.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK\nversion: %s\n", version)
	})
	mux.HandleFunc("/api/v1/incidents", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		incidents := ctrl.GetRecentIncidents()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"count":     len(incidents),
			"incidents": incidents,
		})
	})

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
	}

	go func() {
		slog.Info("Starting HTTP Metrics and Incident API server", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if *mode == "simulate" {
		runSimulation(ctx, ctrl)
	} else {
		slog.Info("KubeHealer Daemon active. Watching cluster events...", "namespaces", cfg.WatchNamespaces)
	}

	<-sigChan
	slog.Info("Shutdown signal received. Terminating gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func printBanner() {
	banner := `
╔═══════════════════════════════════════════════════════════════════════╗
║   __  __      _          _   _            _                           ║
║  |  \/  |    | |        | | | |          | |                          ║
║  | \  / |_  _| |__   ___| |_| | ___  __ _| | ___ _ __                 ║
║  | |\/| | | | | '_ \ / _ \  _  |/ _ \/ _' | |/ _ \ '__|                ║
║  | |  | | |_| | |_) |  __/ | | |  __/ (_| | |  __/ |                   ║
║  |_|  |_|\__,_|_.__/ \___|_| |_|\___|\__,_|_|\___|_|                   ║
║                                                                       ║
║  Autonomous Kubernetes SRE & Incident Auto-Remediation Controller    ║
╚═══════════════════════════════════════════════════════════════════════╝
`
	fmt.Println(banner)
}

func runSimulation(ctx context.Context, ctrl *controller.Controller) {
	fmt.Println("🚀 RUNNING AUTONOMOUS SRE CHAOS DEMONSTRATION")
	fmt.Println("----------------------------------------------------------------------")

	scenarios := demoScenarios()
	for i, sc := range scenarios {
		fmt.Printf("\n▶ [%d/3] INJECTING: %s\n", i+1, sc.name)
		time.Sleep(sc.delay)

		incident, err := ctrl.ProcessPod(ctx, sc.pod, sc.recentEvents)
		if err != nil {
			fmt.Printf("❌ Failed: %v\n", err)
			continue
		}
		if incident != nil {
			fmt.Printf("✅ AUTO-REMEDIATION SUCCESSFUL!\n")
			fmt.Printf("   ├─ Incident ID:     %s\n", incident.ID)
			fmt.Printf("   ├─ Diagnosed Cause: %s (%s)\n", incident.Reason, incident.Severity)
			fmt.Printf("   ├─ Action Taken:    %s\n", incident.ActionTaken)
			fmt.Printf("   ├─ Forensic Log:    %s\n", incident.Snapshot.RecentLogs)
			fmt.Printf("   ├─ Analysis:        %s\n", incident.Snapshot.DiagnosticAnalysis)
			fmt.Printf("   └─ Result:          %s\n", incident.RemediationLog)
		}
	}
	printSimulationFooter()
}

type demoScenario struct {
	name         string
	pod          types.PodInfo
	recentEvents []string
	delay        time.Duration
}

func demoScenarios() []demoScenario {
	return []demoScenario{
		{
			name: "Scenario 1: High-Severity Out-Of-Memory (OOMKilled) in Payment Service",
			pod: types.PodInfo{
				Name:         "payment-api-7d6f54c9-xk28m",
				Namespace:    "production",
				Container:    "payment-gateway",
				OwnerKind:    "Deployment",
				OwnerName:    "payment-api",
				ExitCode:     137, // SIGKILL
				RestartCount: 1,
				Phase:        "Running",
			},
			recentEvents: []string{
				"Container payment-gateway exceeded memory limit (cgroup killed)",
				"OOMKilled",
			},
			delay: 2 * time.Second,
		},
		{
			name: "Scenario 2: Newly Deployed Release CrashLooping on Startup",
			pod: types.PodInfo{
				Name:         "checkout-service-89bc7-tq9v1",
				Namespace:    "production",
				Container:    "checkout-app",
				OwnerKind:    "Deployment",
				OwnerName:    "checkout-service",
				ExitCode:     1,
				RestartCount: 3,
				Phase:        "Running",
			},
			recentEvents: []string{
				"Back-off restarting failed container checkout-app in pod",
				"CrashLoopBackOff",
			},
			delay: 2 * time.Second,
		},
		{
			name: "Scenario 3: Repeated Flapping Pod Exceeding Circuit Breaker Threshold",
			pod: types.PodInfo{
				Name:         "legacy-auth-worker-0",
				Namespace:    "auth",
				Container:    "auth-worker",
				OwnerKind:    "StatefulSet",
				OwnerName:    "legacy-auth-worker",
				ExitCode:     1,
				RestartCount: 6,
				Phase:        "Running",
			},
			recentEvents: []string{
				"Back-off restarting failed container",
				"CrashLoopBackOff",
			},
			delay: 2 * time.Second,
		},
	}
}

func printSimulationFooter() {
	fmt.Println("\n----------------------------------------------------------------------")
	fmt.Println("📊 Chaos Demonstration complete!")
	fmt.Println("Prometheus Metrics available at: http://localhost:8080/metrics")
	fmt.Println("Incident Audit REST API:         http://localhost:8080/api/v1/incidents")
	fmt.Println("Health Probe:                    http://localhost:8080/healthz")
	fmt.Println("Press Ctrl+C to stop.")
}

// Mock Log Provider for standalone simulation & testing
type mockLogProvider struct{}

func (m *mockLogProvider) GetRecentLogs(ctx context.Context, ns, pod, container string, tail int) (string, error) {
	return "FATAL [2026-11-10T09:22:15Z] java.lang.OutOfMemoryError: Java heap space at io.netty.buffer.PooledByteBufAllocator.allocate(PooledByteBufAllocator.java:312)", nil
}

func (m *mockLogProvider) GetPreviousLogs(ctx context.Context, ns, pod, container string, tail int) (string, error) {
	return "ERROR [2026-11-10T09:21:40Z] Terminated with exit code 137 (OOMKilled by Linux kernel)", nil
}

func (m *mockLogProvider) GetNodeCondition(ctx context.Context, nodeName string) (string, error) {
	return "Ready=True, MemoryPressure=False, DiskPressure=False", nil
}

// Mock Kube Client for standalone simulation & testing
type mockKubeClient struct{}

func (m *mockKubeClient) DeletePod(ctx context.Context, ns, pod string, grace int64) error {
	return nil
}

func (m *mockKubeClient) RollbackDeployment(ctx context.Context, ns, deploy string) error {
	return nil
}

func (m *mockKubeClient) AnnotatePod(ctx context.Context, ns, pod string, ann map[string]string) error {
	return nil
}

// nines-operator reconciles one administrator-bound namespace and tenant.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ishantanu/nines/internal/observability"
	"github.com/ishantanu/nines/internal/operator"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "", "administrator-owned operator configuration YAML")
	health := flag.String("health-addr", "127.0.0.1:8081", "health probe address; use :8081 inside a pod")
	flag.Parse()
	if *path == "" || flag.NArg() != 0 {
		return fmt.Errorf("use nines-operator -config operator.yaml [-health-addr :8081]")
	}
	config, e := operator.LoadConfig(*path)
	if e != nil {
		return e
	}
	api, e := operator.InCluster(config.Namespace)
	if e != nil {
		return e
	}
	if os.Getenv("OTEL_SERVICE_NAME") == "" {
		_ = os.Setenv("OTEL_SERVICE_NAME", "nines-operator")
	}
	setup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	telemetry, e := observability.Setup(setup, os.Stdout)
	cancel()
	if e != nil {
		return e
	}
	defer func() {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		_ = telemetry.Shutdown(ctx)
	}()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200); _, _ = w.Write([]byte("ok\n")) })
	// Readiness checks CRD discovery through a bounded metadata read. It does not
	// require leadership, so standby replicas remain available during a handover.
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		probe, end := context.WithTimeout(r.Context(), 2*time.Second)
		defer end()
		e := api.Probe(probe)
		if e != nil {
			http.Error(w, "Kubernetes resources unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ready\n"))
	})
	server := &http.Server{Addr: *health, Handler: mux, ReadHeaderTimeout: 3 * time.Second}
	failed := make(chan error, 1)
	go func() { failed <- server.ListenAndServe(); stop() }()
	controller := operator.Controller{Config: config, API: api}
	e = controller.Run(ctx, telemetry.Logger)
	shutdown, end := context.WithTimeout(context.Background(), 5*time.Second)
	defer end()
	_ = server.Shutdown(shutdown)
	select {
	case err := <-failed:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	default:
	}
	if errors.Is(e, context.Canceled) {
		return nil
	}
	return e
}

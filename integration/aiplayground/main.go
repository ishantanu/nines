// aiplayground is a disposable synthetic workload, not a production server/client.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("use serve, health, render, setup or verify")
	}
	if args[0] == "serve" {
		if len(args) != 1 {
			return errors.New("serve accepts no arguments")
		}
		workload := newWorkload()
		server := &http.Server{Addr: ":8001", Handler: workload.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					workload.tick()
				}
			}
		}()
		go func() {
			<-ctx.Done()
			shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = server.Shutdown(shutdown)
		}()
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
	if args[0] == "health" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8001/healthz", nil)
		if err != nil {
			return err
		}
		response, err := (&http.Client{Timeout: 2 * time.Second}).Do(req)
		if err != nil {
			return errors.New("sample unavailable")
		}
		defer func() { _ = response.Body.Close() }()
		if response.StatusCode != 200 {
			return errors.New("sample unhealthy")
		}
		return nil
	}
	if args[0] == "render" {
		if len(args) != 1 {
			return errors.New("render accepts no arguments")
		}
		encoder := yaml.NewEncoder(out)
		defer func() { _ = encoder.Close() }()
		for _, d := range definitions() {
			if err := encoder.Encode(d.SLO); err != nil {
				return err
			}
		}
		return encoder.Encode(journey())
	}
	flags := flag.NewFlagSet("ai-playground", flag.ContinueOnError)
	flags.SetOutput(out)
	config := settings{}
	flags.StringVar(&config.nines, "url", "http://127.0.0.1:8080", "Loopback playground Nines origin")
	flags.StringVar(&config.sample, "sample-url", "http://ai-sample:8001", "Synthetic sample origin")
	flags.StringVar(&config.prometheus, "prometheus-url", "http://prometheus:9090", "Playground Prometheus origin")
	flags.StringVar(&config.mimir, "mimir-url", "http://mimir:9009/prometheus", "Playground Mimir query base")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected positional argument")
	}
	if err := config.validate(); err != nil {
		return err
	}
	if value := os.Getenv("NINES_PLAYGROUND_TENANTS"); value == "prometheus" {
		config.tenants = []string{"prometheus"}
	} else if value == "prometheus,mimir" {
		config.tenants = []string{"prometheus", "mimir"}
	} else if value != "" && value != "auto" {
		return errors.New("use auto, prometheus or prometheus,mimir for NINES_PLAYGROUND_TENANTS")
	}

	switch args[0] {
	case "setup":
		return config.setup(ctx, out)
	case "verify":
		return config.verify(ctx, out)
	default:
		return errors.New("use serve, health, render, setup or verify")
	}
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

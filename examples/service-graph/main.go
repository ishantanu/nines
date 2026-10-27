package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ishantanu/nines/examples/service-graph/app"
)

func main() {
	name := flag.String("name", "gateway", "service identity")
	peer := flag.String("dependency", "", "fixed dependency identity")
	next := flag.String("next", "", "fixed dependency request URL")
	addr := flag.String("addr", ":8000", "listen address")
	generate := flag.String("generate", "", "generate bounded requests to a fixed example endpoint")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	if *generate != "" {
		if e := app.Generate(ctx, *generate, 500*time.Millisecond); e != nil {
			log.Error("example generator failed")
			os.Exit(1)
		}
		return
	}
	handler, e := app.New(*name, *peer, *next)
	if e != nil {
		log.Error("invalid example configuration")
		os.Exit(1)
	}
	server := http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Error("example HTTP server failed")
		os.Exit(1)
	}
}

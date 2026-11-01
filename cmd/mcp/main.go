// nines-mcp serves one tenant over stdio or opt-in OAuth-protected HTTP.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ishantanu/nines/internal/mcpserver"
)

func main() {
	log.SetOutput(os.Stderr)
	serverDefault := os.Getenv("NINES_URL")
	if serverDefault == "" {
		serverDefault = "http://127.0.0.1:8080"
	}
	server := flag.String("server", serverDefault, "Nines API origin (or NINES_URL; HTTPS unless loopback)")
	tenant := flag.String("tenant", os.Getenv("NINES_TENANT"), "fixed tenant ID (or NINES_TENANT)")
	writes := flag.Bool("allow-writes", false, "enable direct writes and workflow creation; never enables workflow approval")
	rules := flag.Bool("allow-rule-deployment", false, "also expose recording-rule deployment; requires allow-writes and API owner role")
	withdrawal := flag.Bool("allow-rule-withdrawal", false, "also expose reviewed operator withdrawal; requires both write/deployment flags and API owner role")
	transport := flag.String("transport", "stdio", "stdio or http; remote HTTP requires -remote-config")
	remoteConfig := flag.String("remote-config", "", "private RemoteMCP YAML; never uses the shared API_TOKEN fallback")
	listen := flag.String("listen", "127.0.0.1:8081", "remote HTTP listen address")
	tlsCert := flag.String("tls-cert", "", "remote HTTP TLS certificate file")
	tlsKey := flag.String("tls-key", "", "remote HTTP TLS private-key file")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected positional arguments")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *transport == "http" {
		if *remoteConfig == "" {
			log.Fatal("remote HTTP requires -remote-config")
		}
		conflicting := false
		flag.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "transport", "remote-config", "listen", "tls-cert", "tls-key":
			default:
				conflicting = true
			}
		})
		if conflicting {
			log.Fatal("remote server, tenant and write policy belong in RemoteMCP YAML, not stdio flags")
		}

		config, e := mcpserver.LoadRemoteConfig(*remoteConfig, os.Getenv)
		if e != nil {
			log.Fatal(e)
		}
		handler, e := mcpserver.NewRemote(config)
		if e != nil {
			log.Fatal(e)
		}
		if e = serveRemote(ctx, *listen, *tlsCert, *tlsKey, handler); e != nil {
			log.Fatal(e)
		}
		return
	}
	if *transport != "stdio" || *remoteConfig != "" || *tlsCert != "" || *tlsKey != "" {
		log.Fatal("choose stdio without remote options, or http with -remote-config")
	}
	s, err := mcpserver.New(mcpserver.Config{Server: *server, Tenant: *tenant, Token: apiToken(), AllowWrites: *writes, AllowRuleDeployment: *rules, AllowRuleWithdrawal: *withdrawal})
	if err != nil {
		log.Fatal(err)
	}
	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal("MCP transport stopped")
	}
}

// Keep the CLI and MCP environment contract aligned; API_TOKEN remains a
// compatibility fallback for existing stdio client configurations.
func apiToken() string {
	if token := os.Getenv("NINES_API_KEY"); token != "" {
		return token
	}
	return os.Getenv("API_TOKEN")
}

func serveRemote(ctx context.Context, address, cert, key string, handler http.Handler) error {
	host, _, e := net.SplitHostPort(address)
	if e != nil {
		return errors.New("remote listen address must include a host and port")
	}
	ip := net.ParseIP(host)
	if cert == "" && key == "" && (ip == nil || !ip.IsLoopback()) {
		return errors.New("non-loopback remote listeners require -tls-cert and -tls-key; use a loopback TLS-terminating proxy otherwise")
	}
	if (cert == "") != (key == "") {
		return errors.New("provide both TLS certificate and key")
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 32 << 10}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			stop, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = server.Shutdown(stop)
		case <-done:
		}
	}()
	if cert != "" {
		e = server.ListenAndServeTLS(cert, key)
	} else {
		e = server.ListenAndServe()
	}
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}

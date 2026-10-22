// nines-mcp is a stdio MCP adapter for one explicitly configured tenant.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

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
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected positional arguments")
	}
	s, err := mcpserver.New(mcpserver.Config{Server: *server, Tenant: *tenant, Token: os.Getenv("API_TOKEN"), AllowWrites: *writes, AllowRuleDeployment: *rules})
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := s.Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		log.Fatal("MCP transport stopped")
	}
}

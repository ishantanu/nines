package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"

	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/model"
)

// warnings prints each distinct server or local warning once to stderr, the
// way kubectl does, so it never mixes into stdout that scripts and agents
// parse.
var warnings = struct {
	sync.Mutex
	out  io.Writer
	seen map[string]bool
}{out: os.Stderr, seen: map[string]bool{}}

func warn(message string) {
	warnings.Lock()
	defer warnings.Unlock()
	if warnings.seen[message] {
		return
	}
	warnings.seen[message] = true
	_, _ = fmt.Fprintf(warnings.out, "Warning: %s\n", message)
}

// Every request the CLI makes, directly or through the Go client, uses the
// default transport, so wrapping it once surfaces all server warnings.
func init() {
	http.DefaultTransport = client.WarningTransport(http.DefaultTransport, warn)
}

// warnDefinition reports a deprecated resource version read from a file,
// including offline commands that never reach a server.
func warnDefinition(d model.Definition) {
	if d.SourceAPIVersion == model.APIVersion && (d.SLO != nil || d.Journey != nil || d.Binding != nil || d.DependencyMap != nil) {
		warn(model.V1alpha1Deprecation)
	}
}

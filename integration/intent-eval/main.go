// intent-eval is a development harness, not an installed Nines client command.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/ishantanu/nines/internal/intenteval"
)

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("intent-eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	caseID := flags.String("case", "", "optional case ID for the workflow suite")
	suite := flags.String("suite", "draft", "evaluation suite: draft or workflow")
	live := flags.Bool("live", false, "explicitly contact the configured model using synthetic cases (may incur provider charges)")
	out := flags.String("out", "", "write a private JSON report to a new file instead of stdout")
	runtime := flags.String("runtime-version", "", "operator-supplied model runtime version for the live report")
	timeout := flags.Duration("timeout", 5*time.Minute, "total evaluation timeout (at most 15m)")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if (*caseID != "" && *suite != "workflow") || (*suite != "draft" && *suite != "workflow") || flags.NArg() != 0 || *timeout <= 0 || *timeout > 15*time.Minute || len(*runtime) > 200 || strings.ContainsFunc(*runtime, unicode.IsControl) || !*live && *runtime != "" {
		_, _ = fmt.Fprintln(stderr, "invalid evaluation arguments")
		return 2
	}
	// Reserve the destination before any potentially billable model request.
	var file *os.File
	if *out != "" {
		var err error
		file, err = os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "could not create report; the output must be a new writable file")
			return 1
		}
		defer func() { _ = file.Close() }()
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	var report intenteval.Report
	var err error
	if *suite == "workflow" {
		if *live {
			report, err = intenteval.WorkflowLive(ctx, os.Getenv, *runtime, *caseID)
		} else {
			report, err = intenteval.WorkflowFixtures(ctx, *caseID)
		}
	} else if *live {
		report, err = intenteval.Live(ctx, os.Getenv, *runtime)
	} else {
		report, err = intenteval.Fixtures(ctx)
	}
	data, encodeErr := json.MarshalIndent(report, "", "  ")
	if encodeErr != nil {
		_, _ = fmt.Fprintln(stderr, "could not encode evaluation report")
		return 1
	}
	data = append(data, '\n')
	if file != nil {
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_, _ = fmt.Fprintln(stderr, "could not finish evaluation report")
			return 1
		}
	} else if _, writeErr := stdout.Write(data); writeErr != nil {
		_, _ = fmt.Fprintln(stderr, "could not write evaluation report")
		return 1
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "evaluation incomplete or one or more contract checks failed; inspect the report")
		return 1
	}
	return 0
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	cancel()
	os.Exit(code)
}

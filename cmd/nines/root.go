package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/buildinfo"
)

type options struct {
	Server, Tenant, Token, Output, Context, Config, credentialSource string
	Offline, Agent                                                   bool
	Timeout                                                          time.Duration
	cancel                                                           context.CancelFunc
}

func (o *options) format(fallback string) string {
	if o.Output != "" {
		return o.Output
	}
	if o.Agent {
		return "json"
	}
	return fallback
}
func mark(c *cobra.Command, mode, effect, short string) *cobra.Command {
	c.Short = short
	if prior := c.Args; prior != nil {
		c.Args = func(cmd *cobra.Command, args []string) error {
			if e := prior(cmd, args); e != nil {
				return usage(e.Error())
			}
			return nil
		}
	}
	c.Annotations = map[string]string{"mode": mode, "effect": effect}
	return c
}
func position(c *cobra.Command, use string, min, max int) *cobra.Command {
	c.Use = use
	c.Args = cobra.RangeArgs(min, max)
	return c
}
func (o *options) prepare(c *cobra.Command) error {
	if o.Timeout <= 0 || o.Timeout > 30*time.Minute {
		return usage("timeout must be positive and no more than 30m")
	}
	if o.Output != "" && o.Output != "json" && o.Output != "yaml" && o.Output != "table" && o.Output != "csv" {
		return usage("output must be json, yaml, table or history csv")
	}
	ctx, cancel := context.WithTimeout(c.Context(), o.Timeout)
	o.cancel = cancel
	c.SetContext(ctx)
	if o.Output == "csv" && c.CommandPath() != "nines history" {
		return usage("csv output is supported only by history")
	}
	path := c.CommandPath()
	artifact := strings.HasPrefix(path, "nines init ") || path == "nines rules generate" || strings.HasPrefix(path, "nines operator ") || strings.HasPrefix(path, "nines export ") || strings.HasSuffix(path, "conversations kubernetes") || strings.HasSuffix(path, "conversations rollback-kubernetes")
	if artifact && o.Output != "" && o.Output != "yaml" {
		return usage("this artifact command emits YAML; omit --output or choose yaml")
	}
	mode := c.Annotations["mode"]
	if o.Offline && (mode == "server" || mode == "login") {
		return usage("command requires a server; remove --offline or choose an offline command")
	}
	if mode == "server" || mode == "login" {
		return o.resolve(c)
	}
	if o.Tenant == "" {
		if path == "nines rules generate" || path == "nines operator render" {
			o.Tenant = os.Getenv("NINES_TENANT")
		}
		if o.Tenant == "" {
			o.Tenant = "default"
		}
	}
	return nil
}
func rootCommand(o *options, out, errOut io.Writer) *cobra.Command {
	r := &cobra.Command{Use: "nines", Short: "SLOs and critical user journeys, locally and with Nines", Long: "Validate YAML and generate rules offline, or manage authenticated tenant resources through Nines.\nUse --offline to prohibit server operations. Use --agent for machine-readable results and errors without interactive login.", SilenceUsage: true, SilenceErrors: true}
	r.Version = buildinfo.Version
	r.SetVersionTemplate("{{.Version}}\n")
	r.SetOut(out)
	r.SetErr(errOut)
	r.SetFlagErrorFunc(func(c *cobra.Command, e error) error { return usage(e.Error()) })
	f := r.PersistentFlags()
	f.StringVar(&o.Server, "server", "", "Nines origin (NINES_URL or selected context)")
	f.StringVar(&o.Tenant, "tenant", "", "Tenant namespace (NINES_TENANT or selected context)")
	f.StringVar(&o.Context, "context", "", "Named tenant context (NINES_CONTEXT)")
	f.StringVar(&o.Config, "config", "", "CLI configuration file (NINES_CONFIG)")
	f.StringVarP(&o.Output, "output", "o", "", "Output: json, yaml, table; history also supports csv")
	f.BoolVar(&o.Offline, "offline", false, "Prohibit server commands and credential loading")
	f.BoolVar(&o.Agent, "agent", false, "JSON output/errors; no interactive login or browser launch")
	f.DurationVar(&o.Timeout, "timeout", 2*time.Minute, "Maximum command duration")
	r.PersistentPreRunE = func(c *cobra.Command, args []string) error { return o.prepare(c) }
	group := func(parent *cobra.Command, name, short string) *cobra.Command {
		c := &cobra.Command{Use: name, Short: short}
		parent.AddCommand(c)
		return c
	}
	core := func(parent *cobra.Command, action, kind, use, mode, effect, short string, min, max int) {
		parent.AddCommand(mark(position(coreCommand(action, kind, o), use, min, max), mode, effect, short))
	}
	r.AddCommand(templatesCommand(o), extensionsCommand(o), retirementCommand(o))
	g := group(r, "init", "Create local versioned YAML")
	core(g, "init", "slo", "slo", "offline", "local-write", "Generate an SLO manifest", 0, 0)
	for _, v := range []struct{ a, mode, effect, short string }{{"validate", "offline", "read", "Validate a local YAML definition"}, {"plan", "server", "read", "Review a definition against the server"}, {"apply", "server", "write", "Apply an exact revision-checked definition"}} {
		core(r, v.a, "", v.a, v.mode, v.effect, v.short, 0, 0)
	}
	for _, a := range []string{"get", "export", "describe", "context"} {
		g := group(r, a, map[string]string{"get": "List tenant resources", "export": "Export canonical resource YAML", "describe": "Read resource definition and status", "context": "Read revision-scoped evidence"}[a])
		ks := []string{"slo", "journey", "binding", "dependency-map"}
		if a == "get" {
			ks = []string{"slos", "journeys", "connections", "access"}
		}
		if a == "context" {
			ks = []string{"slo", "journey"}
		}
		for _, k := range ks {
			use, min, max := k+" ID", 1, 1
			if a == "get" {
				use, min, max = k, 0, 0
			}
			core(g, a, k, use, "server", "read", g.Short, min, max)
		}
	}
	for _, a := range []string{"status", "quality", "budget", "integration", "history", "collect", "events", "search"} {
		use, min, max := a+" ID", 1, 1
		effect := "read"
		if a == "events" || a == "search" {
			use, min, max = a, 0, 0
		}
		if a == "quality" {
			effect = "provider-read"
		}
		if a == "collect" {
			effect = "write"
		}
		core(r, a, "", use, "server", effect, map[string]string{"budget": "Read the error-budget response decision; optionally gate CI with --check", "quality": "Explain measurement freshness, coverage, traffic and optional cohort evidence", "integration": "Check definition, collection, rule deployment and external evaluation", "status": "Show current SLO calculation", "history": "Read SLI, SLO and burn-rate history", "collect": "Collect a bounded measurement range", "events": "List alert transitions", "search": "Search tenant resource metadata"}[a], min, max)
	}
	g = group(r, "connections", "Inspect configured data sources")
	core(g, "connections-list", "", "list", "server", "read", "List configured sources", 0, 0)
	core(g, "connections-test", "", "test [NAME]", "server", "provider-read", "Test a configured source", 0, 1)
	g = group(r, "rules", "Generate and review recording and burn-rate rules")
	for _, a := range []string{"generate", "plan", "apply", "withdraw", "pause", "resume"} {
		mode, effect := "server", "read"
		if a == "generate" {
			mode, effect = "offline", "local-write"
		}
		if a == "apply" || a == "withdraw" || a == "pause" || a == "resume" {
			effect = "deploy"
		}
		core(g, "rules-"+a, "", a, mode, effect, map[string]string{"generate": "Generate rules from local YAML", "plan": "Review rule digest and destination dataset", "apply": "Deploy an exact reviewed rule digest", "pause": "Pause automatic deployment without removing loaded rules", "resume": "Resume automatic deployment of the saved SLO revision", "withdraw": "Withdraw a reviewed operator publication and retain its definition"}[a], 0, 0)
	}
	intent := group(r, "intent", "Prepare and review intent-based SLOs")
	for _, a := range []string{"draft", "plan", "backtest"} {
		effect := "provider-read"
		if a == "draft" {
			effect = "model-call"
		}
		intent.AddCommand(mark(intentCommand(a, o), "server", effect, map[string]string{"draft": "Draft using the configured model", "plan": "Compile a reviewed measurement selection", "backtest": "Backtest a selection against provider telemetry"}[a]))
	}
	g = group(intent, "conversations", "Durable preparation, review, apply and verification")
	for _, a := range []string{"list", "show", "create", "message", "run", "confirm", "cancel", "changes", "approve", "reject", "apply", "verify", "observe", "audit", "rollback-plan", "rollback-approve", "rollback-apply", "rollback-reject", "kubernetes", "rollback-kubernetes"} {
		use, min, max := a+" ID", 1, 1
		effect := "write"
		if a == "list" || a == "create" {
			use, min, max = a, 0, 0
		}
		if a == "list" || a == "show" || a == "verify" || a == "kubernetes" || a == "audit" || a == "rollback-kubernetes" {
			effect = "read"
		}
		if a == "run" {
			effect = "model-call"
		}
		g.AddCommand(mark(position(conversationCommand(a, o), use, min, max), "server", effect, map[string]string{"list": "List saved conversations", "show": "Read one conversation", "create": "Create a preparation conversation", "message": "Append a clarification", "run": "Prepare with explicit model consent", "confirm": "Confirm a measurement revision", "cancel": "Cancel preparation", "changes": "Freeze exact SLO and journey changes", "approve": "Approve an exact change-set digest", "reject": "Reject changes or withdraw approval with a reason", "audit": "Read the decision audit log", "rollback-plan": "Prepare a revision-fenced configuration rollback", "rollback-approve": "Approve an exact rollback digest", "rollback-apply": "Apply an approved configuration rollback", "rollback-reject": "Reject or withdraw rollback approval", "rollback-kubernetes": "Export reviewed rollback definitions for GitOps", "apply": "Atomically apply approved definitions", "observe": "Inspect runtime and persist post-change outcome evidence", "verify": "Read applied revisions and recorded outcome evidence", "kubernetes": "Export exact definitions for GitOps review"}[a]))
	}
	for _, name := range []string{"telemetry", "bindings", "dependencies"} {
		g := group(r, name, map[string]string{"telemetry": "Discover configured source measurements", "bindings": "Review and compile measurement bindings", "dependencies": "Discover and read service dependency maps"}[name])
		if name == "dependencies" {
			g.AddCommand(dependencyDiscoveryCommand(o, false), dependencyDiscoveryCommand(o, true))
		}
		actions := map[string][]string{"telemetry": {"names", "inspect", "verify"}, "bindings": {"list", "compile"}, "dependencies": {"list", "show"}}[name]
		for _, a := range actions {
			use, min, max := a+" ID", 1, 1
			if a == "list" || a == "verify" {
				use, min, max = a, 0, 0
			}
			g.AddCommand(mark(position(telemetryCommand(name, a, o), use, min, max), "server", "provider-read", a+" "+name))
		}
	}
	g = group(r, "replays", "Read or ingest consented masked recordings")
	for _, a := range []string{"list", "show", "events", "ingest"} {
		use, min, max := a, 0, 0
		effect := "read"
		if a == "show" || a == "events" {
			use, min, max = a+" ID", 1, 1
		}
		if a == "ingest" {
			effect = "write"
		}
		g.AddCommand(mark(position(replayCommand(a, o), use, min, max), "server", effect, a+" replay recordings"))
	}
	g = group(r, "review-skills", "Read executable review skill metadata")
	for _, a := range []string{"list", "show"} {
		use, min, max := a, 0, 0
		if a == "show" {
			use, min, max = a+" ID", 1, 1
		}
		g.AddCommand(mark(position(reviewSkillCommand(a, o), use, min, max), "server", "read", a+" server review skills"))
	}
	r.AddCommand(auditCommand(o), operatorCommand(o), configCommand(o), authCommand(o), loginCommand(o), skillsCommand(o))
	r.AddCommand(mark(&cobra.Command{Use: "version", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		b, _ := debug.ReadBuildInfo()
		return render(c.OutOrStdout(), map[string]string{"name": "nines", "module": b.Main.Path, "version": buildinfo.Version, "commit": buildinfo.Commit, "builtAt": buildinfo.Date, "goVersion": b.GoVersion}, o.format("json"), "version")
	}}, "offline", "read", "Show CLI build information"))
	r.AddCommand(mark(&cobra.Command{Use: "capabilities", Args: cobra.NoArgs, RunE: func(c *cobra.Command, args []string) error {
		return render(c.OutOrStdout(), commandCatalog(r), o.format("json"), "capabilities")
	}}, "offline", "read", "Discover commands, flags, execution modes and effects"))
	refineFlags(r)
	var requiredFiles func(*cobra.Command)
	requiredFiles = func(c *cobra.Command) {
		if c.Flags().Lookup("file") != nil && c.Annotations["file-alternatives"] == "" {
			_ = c.MarkFlagRequired("file")
		}
		for _, child := range c.Commands() {
			requiredFiles(child)
		}
	}
	requiredFiles(r)
	return r
}
func exposeCoreFlags(c *cobra.Command, fs *pflag.FlagSet, a string) {
	def := "file window step target source lifecycle short-window long-window burn-rate for min-coverage min-events good-query total-query no-policy"
	allowed := map[string]string{"init": "out name project service display-name " + strings.ReplaceAll(def, "file ", ""), "validate": def, "plan": def, "apply": def + " expected-revision", "get": "", "export": "out", "describe": "", "context": "expected-revision at limit cursor", "search": "kind query project limit cursor linked-slo", "status": "", "quality": "cohorts expected-revision", "budget": "check expected-revision", "integration": "", "history": "from to range resolution revision", "collect": "from to recent", "events": "id", "connections-list": "", "connections-test": "", "rules-generate": "out " + def, "rules-plan": "id", "rules-apply": "id expected-digest", "rules-pause": "id expected-revision", "rules-resume": "id expected-revision", "rules-withdraw": "id expected-digest"}[a]
	for _, name := range strings.Fields(allowed) {
		c.Flags().AddFlag(fs.Lookup(name))
	}
}
func normalizeArgs(r *cobra.Command, args []string) []string {
	names := map[string]*pflag.Flag{}
	short := map[string]*pflag.Flag{}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		each := func(f *pflag.Flag) {
			names[f.Name] = f
			if f.Shorthand != "" {
				short[f.Shorthand] = f
			}
		}
		c.Flags().VisitAll(each)
		c.PersistentFlags().VisitAll(each)
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(r)
	out := append([]string(nil), args...)
	value := false
	for i, a := range out {
		if value {
			value = false
			continue
		}
		if a == "--" {
			break
		}
		if !strings.HasPrefix(a, "-") {
			continue
		}
		n, _, assigned := strings.Cut(strings.TrimLeft(a, "-"), "=")
		f := names[n]
		if f == nil {
			f = short[n]
		}
		if f != nil {
			if !strings.HasPrefix(a, "--") && len(n) > 1 {
				out[i] = "-" + a
			}
			value = !assigned && f.NoOptDefVal == ""
		}
	}
	return out
}
func execute(ctx context.Context, args []string, out, errOut io.Writer) error {
	o := &options{}
	r := rootCommand(o, out, errOut)
	r.SetArgs(normalizeArgs(r, args))
	defer func() {
		if o.cancel != nil {
			o.cancel()
		}
	}()
	e := r.ExecuteContext(ctx)
	if e != nil && (strings.HasPrefix(e.Error(), "unknown command ") || strings.HasPrefix(e.Error(), "required flag")) {
		return usage(e.Error())
	}
	return e
}
func run(args []string, out io.Writer) error {
	return execute(context.Background(), args, out, io.Discard)
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var out bytes.Buffer
	if e := execute(ctx, os.Args[1:], &out, os.Stderr); e != nil {
		writeError(os.Stderr, e, agentArg(os.Args[1:]))
		os.Exit(exitCode(e))
	}
	_, _ = out.WriteTo(os.Stdout)
}

type cliError struct {
	Code, Message string
	Exit          int
}

func (e *cliError) Error() string { return e.Message }
func usage(s string) error        { return &cliError{"usage", s, 2} }
func exitCode(e error) int {
	var ce *cliError
	if errors.As(e, &ce) {
		return ce.Exit
	}
	var ae *client.APIError
	if errors.As(e, &ae) {
		switch ae.StatusCode {
		case 401:
			return 3
		case 403:
			return 4
		case 409, 412:
			return 5
		default:
			return 6
		}
	}
	var ne net.Error
	if errors.As(e, &ne) || errors.Is(e, context.DeadlineExceeded) || errors.Is(e, context.Canceled) {
		return 6
	}

	return 1
}
func agentArg(args []string) bool {
	for _, a := range args {
		if a == "--agent" || a == "-agent" || a == "--agent=true" {
			return true
		}
	}
	return false
}
func writeError(w io.Writer, e error, agent bool) {
	if agent {
		_ = json.NewEncoder(w).Encode(map[string]any{"apiVersion": "nines.dev/v1alpha1", "kind": "CLIError", "error": map[string]any{"code": map[int]string{1: "error", 2: "usage", 3: "unauthenticated", 4: "forbidden", 5: "conflict", 6: "unavailable"}[exitCode(e)], "message": e.Error(), "exitCode": exitCode(e)}})
	} else {
		// Reporting already failed; there is no second output channel to retry.
		_, _ = fmt.Fprintf(w, "Error: %s\n", e)
	}
}
func (o *options) httpClient() *http.Client {
	return &http.Client{Timeout: o.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func refineFlags(root *cobra.Command) {
	allowed := map[string]string{
		"nines intent draft": "file out", "nines intent plan": "file binding expected-revision out", "nines intent backtest": "file binding expected-revision from to resolution",
		"nines telemetry verify": "file out", "nines telemetry names": "prefix", "nines telemetry inspect": "metric labels", "nines bindings list": "limit cursor", "nines bindings compile": "expected-revision name display-name target window out", "nines dependencies list": "limit cursor", "nines dependencies show": "expected-revision",
		"nines replays list": "resource-kind resource-id", "nines replays show": "", "nines replays events": "", "nines replays ingest": "file",
		"nines review-skills list": "", "nines review-skills show": "version",
	}
	for _, a := range []string{"list", "show", "create", "message", "run", "confirm", "cancel", "changes", "approve", "reject", "apply", "verify", "audit", "rollback-plan", "rollback-approve", "rollback-apply", "rollback-reject", "kubernetes", "rollback-kubernetes"} {
		f := "file"
		switch a {
		case "list":
			f = "cursor"
		case "audit":
			f = "after through"
		case "show", "verify":
			f = ""
		case "kubernetes", "rollback-kubernetes":
			f = "namespace digest"
		}
		allowed["nines intent conversations "+a] = f
	}
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if fields, ok := allowed[c.CommandPath()]; ok {
			old := c.Flags()
			c.ResetFlags()
			for _, name := range strings.Fields(fields) {
				c.Flags().AddFlag(old.Lookup(name))
			}
		}
		for _, s := range c.Commands() {
			walk(s)
		}
	}
	walk(root)
}
func openInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

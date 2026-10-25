// nines provides tenant-scoped SLO and journey operations.
package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/engine"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/rules"
	"github.com/ishantanu/nines/internal/tenancy"
)

func coreCommand(action, kind string, o *options) *cobra.Command {
	id := ""
	cmd := &cobra.Command{Use: action, Args: cobra.NoArgs}
	flags := pflag.NewFlagSet(action, pflag.ContinueOnError)

	file := flags.StringP("file", "f", "", "YAML resource file")
	linkedSLO := flags.String("linked-slo", "", "find journeys referencing a tenant-local SLO")
	searchKind := flags.String("kind", "", "slo or journey resource search filter")
	searchQuery := flags.String("query", "", "literal resource name search")
	searchLimit := flags.Int("limit", 0, "bounded discovery or context page size")
	searchCursor := flags.String("cursor", "", "opaque next-page cursor")
	contextAt := flags.String("at", "", "context requested wall clock RFC3339")
	server, tenant := &o.Server, &o.Tenant
	revisionUsage := "0 creates; current revision updates"
	if action == "quality" || action == "budget" {
		revisionUsage = "require this positive current definition revision (omit for latest)"
	}
	expected := flags.Int64("expected-revision", -1, revisionUsage)
	digest := flags.String("expected-digest", "", "reviewed rule plan digest")
	flags.StringVar(&id, "id", id, "SLO identifier")
	outputDefault := "json"
	if action == "get" || action == "events" || strings.HasPrefix(action, "connections-") {
		outputDefault = "table"
	}
	output := new(string)
	pathOut := flags.String("out", "", "write generated YAML to a new file")
	budgetCheck := flags.Bool("check", false, "exit 7 unless the configured error-budget decision is proceed")
	cohorts := flags.Bool("cohorts", false, "explicitly query bounded cohort counts from the configured provider")
	recent := flags.Bool("recent", false, "collect a bounded recent range using the server clock")
	from := flags.String("from", "", "range start RFC3339")
	to := flags.String("to", "", "range end RFC3339")
	rangeText := flags.String("range", "", "history duration")
	resolution := flags.String("resolution", "", "history bucket duration")
	historyRevision := flags.Int64("revision", 0, "archived SLO revision for history (omit for current)")
	name := flags.String("name", "", "resource name")
	project := flags.String("project", "production", "project")
	serviceName := flags.String("service", "api", "service")
	display := flags.String("display-name", "", "display name")
	source := flags.String("source", "", "configured connection name")
	target := flags.Float64("target", .999, "SLO target fraction")
	window := flags.String("window", "28d", "SLO rolling window")
	step := flags.String("step", "1m", "measurement interval")
	short := flags.String("short-window", "5m", "short burn window")
	long := flags.String("long-window", "1h", "long burn window")
	threshold := flags.Float64("burn-rate", 14.4, "both-window burn threshold")
	pending := flags.String("for", "2m", "pending duration")
	coverage := flags.Float64("min-coverage", 1, "minimum coverage fraction")
	minEvents := flags.Float64("min-events", 0, "minimum eligible events in each burn window (not statistical confidence)")
	lifecycle := flags.String("lifecycle", "", "experimental, aspirational, committed (default) or deprecated")
	good := flags.String("good-query", "", "good interval-count expression")
	total := flags.String("total-query", "", "total interval-count expression")
	noPolicy := flags.Bool("no-policy", false, "omit burn-rate alert policy")
	exposeCoreFlags(cmd, flags, action)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		if len(args) > 0 {
			id = args[0]
		}
		if action == "connections-test" && id == "" {
			id = "primary"
		}
		*output = o.format(outputDefault)
		if !tenancy.ValidID(*tenant) {
			return fmt.Errorf("invalid tenant")
		}
		if id != "" && !tenancy.ValidID(id) {
			return fmt.Errorf("invalid resource identifier")
		}
		visited := map[string]bool{}
		flags.VisitAll(func(f *pflag.Flag) {
			if f.Changed {
				visited[f.Name] = true
			}
		})
		var definition model.Definition
		switch action {
		case "init":
			if kind != "slo" || *name == "" {
				return fmt.Errorf("init slo requires -name")
			}
			if *display == "" {
				*display = *name
			}
			d := model.SLODefinition(v1.NewSLO(*name, *project, v1.SLOSpec{Source: *source, DisplayName: *display, Service: *serviceName, Target: *target, GoodQuery: *good, TotalQuery: *total}))
			definition.SLO = &d
			for _, key := range []string{"window", "step", "short-window", "long-window", "burn-rate", "for", "min-coverage"} {
				visited[key] = true
			}
		case "validate", "plan", "apply", "rules-generate":
			if *file == "" {
				return fmt.Errorf("provide -f resource.yaml")
			}
			f, err := openInput(*file)
			if err != nil {
				return err
			}
			body, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
			_ = f.Close()
			if err != nil {
				return err
			}
			if action == "validate" {
				var header struct {
					Kind string `yaml:"kind"`
				}
				_ = yaml.Unmarshal(body, &header)
				if header.Kind == v1.IntentKind {
					for key := range visited {
						if key != "file" {
							return usage("intent validation does not accept SLO overrides; edit the intent YAML")
						}
					}
					goal, e := model.ParseIntent(body)
					if e != nil {
						return e
					}
					return render(out, map[string]string{"status": "valid", "apiVersion": goal.APIVersion, "kind": goal.Kind, "name": goal.Metadata.Name, "digest": model.DefinitionDigest(goal), "note": "schema validated; measurement selection and provider evidence require connected planning"}, *output, "validation")
				}
			}
			definition, err = model.ParseDefinition(body)
			if err != nil {
				return err
			}
		}
		if definition.Manifest() != nil && definition.SLO == nil {
			for key := range visited {
				if key != "file" && key != "expected-revision" {
					return usage("SLO override flags require a ServiceLevelObjective")
				}
			}
		}
		if definition.SLO != nil {
			d := definition.SLO
			for _, field := range []struct {
				key, text   string
				destination *int64
			}{{"window", *window, &d.Spec.WindowSeconds}, {"step", *step, &d.Spec.StepSeconds}} {
				if visited[field.key] {
					v, err := rules.DurationSeconds(field.text)
					if err != nil {
						return err
					}
					*field.destination = v
				}
			}
			if visited["lifecycle"] {
				d.Spec.Lifecycle = v1.SLOLifecycle(*lifecycle)
			}
			if visited["target"] {
				d.Spec.Target = *target
			}
			if visited["source"] {
				d.Spec.Source = *source
			}
			if visited["good-query"] {
				d.Spec.GoodQuery = *good
			}
			if visited["total-query"] {
				d.Spec.TotalQuery = *total
			}
			for _, key := range []string{"short-window", "long-window", "burn-rate", "for", "min-coverage", "min-events"} {
				if visited[key] && len(d.Spec.Policies) > 0 {
					return usage("single-policy override flags cannot edit named policies; edit the YAML policies instead")
				}
				if visited[key] && d.Spec.Policy == nil {
					d.Spec.Policy = &model.Policy{ShortWindowSeconds: min(300, d.Spec.WindowSeconds), LongWindowSeconds: min(3600, d.Spec.WindowSeconds), BurnRate: 14.4, MinCoverage: 1}
				}
			}
			if p := d.Spec.Policy; p != nil {
				for _, field := range []struct {
					key, text   string
					destination *int64
				}{{"short-window", *short, &p.ShortWindowSeconds}, {"long-window", *long, &p.LongWindowSeconds}, {"for", *pending, &p.ForSeconds}} {
					if visited[field.key] {
						v, err := rules.DurationSeconds(field.text)
						if err != nil {
							return err
						}
						*field.destination = v
					}
				}
				if visited["burn-rate"] {
					p.BurnRate = *threshold
				}
				if visited["min-events"] {
					p.MinEvents = *minEvents
				}
				if visited["min-coverage"] {
					p.MinCoverage = *coverage
				}
			}
			if *noPolicy {
				d.Spec.Policies = nil
				d.Spec.Policy = nil
			}
			if err := d.Validate(); err != nil {
				return err
			}
		}
		if action == "init" {
			body, _ := yaml.Marshal(definition.Manifest())
			return o.generated(out, *pathOut, body)
		}
		if action == "rules-generate" {
			if definition.SLO == nil {
				return fmt.Errorf("rules generate requires a ServiceLevelObjective")
			}
			g, err := rules.Generate(definition.SLO.SLO(), *tenant)
			if err != nil {
				return err
			}
			body, _ := yaml.Marshal(rules.File{Groups: []rules.Group{g}})
			return o.generated(out, *pathOut, body)
		}
		if action == "validate" {
			note := "schema validated; plan/apply also checks source and SLO references"
			if definition.SLO != nil && definition.SLO.SLO().HasQueries() {
				if _, err := rules.Generate(definition.SLO.SLO(), "validation"); err != nil {
					return err
				}
				note = "schema, PromQL types and generated expressions validated offline; metric availability, cardinality and data semantics require provider evidence"
			}
			return render(out, map[string]string{"status": "valid", "apiVersion": model.APIVersion, "kind": definition.Kind(), "name": definition.Metadata().Name, "digest": model.DefinitionDigest(definition.Manifest()), "note": note}, *output, "validation")
		}
		u, err := url.Parse(*server)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" && u.Path != "/" {
			return fmt.Errorf("server must be an origin without credentials, path or query")
		}
		ip := net.ParseIP(u.Hostname())
		allowedTransport := u.Scheme == "https" || u.Scheme == "http" && (u.Hostname() == "localhost" || ip != nil && ip.IsLoopback())
		if !allowedTransport {
			return fmt.Errorf("use HTTPS, or HTTP on loopback")
		}
		request := func(method, path string, body []byte, media string, revision *int64) ([]byte, error) {
			req, err := http.NewRequestWithContext(cmd.Context(), method, strings.TrimRight(*server, "/")+path, bytes.NewReader(body))
			if err != nil {
				return nil, err
			}
			req.Header.Set("X-Nines-Tenant", *tenant)
			if token := o.Token; token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			if media != "" {
				req.Header.Set("Content-Type", media)
			}
			if revision != nil {
				req.Header.Set("If-Match", fmt.Sprintf("\"%d\"", *revision))
			}
			c := http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			res, err := c.Do(req)
			if err != nil {
				return nil, err
			}
			defer func() {
				_ = res.Body.Close()
			}()
			data, err := io.ReadAll(io.LimitReader(res.Body, (4<<20)+1))
			if err != nil {
				return nil, err
			}
			if len(data) > 4<<20 {
				return nil, fmt.Errorf("response exceeds 4 MiB")
			}
			if res.StatusCode != 200 {
				return nil, &client.APIError{StatusCode: res.StatusCode}
			}
			return data, nil
		}
		method, path, media := http.MethodGet, "", ""
		var body []byte
		var rev *int64
		switch action {
		case "plan", "apply":
			if action == "apply" {
				if *expected < 0 {
					return fmt.Errorf("apply requires -expected-revision")
				}
				rev = expected
			}
			method = http.MethodPost
			path = "/api/resources/" + action
			body, _ = yaml.Marshal(definition.Manifest())
			media = "application/yaml"
		case "get":
			switch kind {
			case "slos", "journeys", "connections", "access":
				path = "/api/" + kind
			default:
				return fmt.Errorf("get expects slos, journeys, connections or access")
			}
		case "search":
			q := url.Values{}
			switch *searchKind {
			case "", v1.SLOKind, v1.JourneyKind:
				q.Set("kind", *searchKind)
			case "slo":
				q.Set("kind", v1.SLOKind)
			case "journey":
				q.Set("kind", v1.JourneyKind)
			default:
				return fmt.Errorf("search kind must be slo or journey")
			}
			if *searchLimit < 0 || *searchLimit > 100 {
				return fmt.Errorf("search limit must be 1–100")
			}
			if visited["project"] {
				q.Set("project", *project)
			}
			if *searchQuery != "" {
				q.Set("query", *searchQuery)
			}
			if *searchLimit > 0 {
				q.Set("limit", strconv.Itoa(*searchLimit))
			}
			if *searchCursor != "" {
				q.Set("cursor", *searchCursor)
			}
			if *linkedSLO != "" {
				if !tenancy.ValidID(*linkedSLO) {
					return fmt.Errorf("invalid linked SLO identifier")
				}
				q.Set("linkedSLO", *linkedSLO)
			}
			path = "/api/resources/search?" + q.Encode()
			kind = "search"
		case "context":
			prefix := "/api/slos/"
			if kind == "journey" {
				prefix = "/api/journeys/"
			} else if kind != "slo" {
				return fmt.Errorf("context expects slo or journey")
			}
			q := url.Values{}
			if visited["expected-revision"] {
				if *expected < 1 {
					return fmt.Errorf("expected revision must be positive")
				}
				q.Set("expectedRevision", strconv.FormatInt(*expected, 10))
			}
			if *contextAt != "" {
				at, e := time.Parse(time.RFC3339Nano, *contextAt)
				if e != nil {
					return fmt.Errorf("at must be RFC3339")
				}
				q.Set("at", at.Format(time.RFC3339Nano))
			}
			if *searchLimit != 0 || *searchCursor != "" {
				if kind != "journey" {
					return fmt.Errorf("link pagination applies only to journey context")
				}
				if *searchLimit < 0 || *searchLimit > 10 {
					return fmt.Errorf("context limit must be 1–10")
				}
				if *searchLimit > 0 {
					q.Set("limit", strconv.Itoa(*searchLimit))
				}
				if *searchCursor != "" {
					q.Set("cursor", *searchCursor)
				}
			}
			path = prefix + id + "/context"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
		case "budget":
			q := url.Values{}
			if visited["expected-revision"] {
				if *expected < 1 {
					return usage("expected-revision must be positive")
				}
				q.Set("expectedRevision", strconv.FormatInt(*expected, 10))
			}
			path = "/api/slos/" + id + "/budget"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
		case "quality":
			q := url.Values{}
			if *cohorts {
				q.Set("cohorts", "true")
			}
			if visited["expected-revision"] {
				if *expected < 1 {
					return usage("expected-revision must be positive")
				}
				q.Set("expectedRevision", strconv.FormatInt(*expected, 10))
			}
			path = "/api/slos/" + id + "/quality"
			if len(q) > 0 {
				path += "?" + q.Encode()
			}
		case "status":
			path = "/api/slos/" + id + "/status"
		case "history":
			q := url.Values{}
			if *historyRevision < 0 {
				return fmt.Errorf("revision must be positive")
			}
			if *historyRevision > 0 {
				q.Set("revision", strconv.FormatInt(*historyRevision, 10))
			}
			if *from != "" {
				q.Set("from", *from)
			}
			if *to != "" {
				q.Set("to", *to)
			}
			if *rangeText != "" {
				n, err := rules.DurationSeconds(*rangeText)
				if err != nil {
					return err
				}
				q.Set("rangeSeconds", strconv.FormatInt(n, 10))
			}
			if *resolution != "" {
				n, err := rules.DurationSeconds(*resolution)
				if err != nil {
					return err
				}
				q.Set("resolutionSeconds", strconv.FormatInt(n, 10))
			}
			path = "/api/slos/" + id + "/history?" + q.Encode()
		case "integration":
			path = "/api/slos/" + id + "/integration"
		case "events":
			path = "/api/events"
		case "connections-list":
			path = "/api/connections"
			kind = "connections"
		case "connections-test":
			method = http.MethodPost
			path = "/api/connections/" + id + "/test"
		case "collect":
			if *recent && (*from != "" || *to != "") {
				return fmt.Errorf("--recent cannot be combined with --from or --to")
			}
			if !*recent && (*from == "" || *to == "") {
				return fmt.Errorf("collect requires --recent or --from and --to RFC3339")
			}
			method = http.MethodPost
			path = "/api/slos/" + id + "/collect"
			if *recent {
				body, _ = json.Marshal(map[string]bool{"recent": true})
			} else {
				body, _ = json.Marshal(map[string]string{"from": *from, "to": *to})
			}
			media = "application/json"
		case "rules-pause", "rules-resume":
			if id == "" || *expected < 1 {
				return fmt.Errorf("provide --id and a positive --expected-revision")
			}
			method = http.MethodPost
			path = "/api/slos/" + id + "/rules/automation"
			media = "application/json"
			body, _ = json.Marshal(map[string]any{"expectedRevision": *expected, "paused": action == "rules-pause"})
		case "rules-plan", "rules-apply", "rules-withdraw":
			if id == "" {
				return fmt.Errorf("rules plan/apply requires -id")
			}
			method = http.MethodPost
			path = "/api/slos/" + id + "/rules/" + strings.TrimPrefix(action, "rules-")
			if action == "rules-apply" || action == "rules-withdraw" {
				if *digest == "" {
					return fmt.Errorf("rules apply/withdraw requires -expected-digest from the reviewed plan")
				}
				body, _ = json.Marshal(map[string]string{"expectedDigest": *digest})
				media = "application/json"
			}
		case "export", "describe":
			if kind != "slo" && kind != "journey" && kind != "binding" && kind != "dependency-map" {
				return fmt.Errorf("use slo, journey, binding or dependency-map")
			}
			if action == "describe" && kind == "slo" {
				data, err := request(http.MethodGet, "/api/slos/"+id, nil, "", nil)
				if err != nil {
					return err
				}
				var view struct {
					SLO    model.SLO      `json:"slo"`
					Status model.Snapshot `json:"status"`
				}
				if err := json.Unmarshal(data, &view); err != nil {
					return err
				}
				if view.SLO.ID != id {
					return fmt.Errorf("API returned an invalid SLO view")
				}
				return render(out, map[string]any{"definition": model.DefinitionFromSLO(view.SLO), "status": view.Status, "revision": model.ConfigRevision(view.SLO)}, *output, kind)
			}
			plural := map[string]string{"slo": "slos", "journey": "journeys", "binding": "bindings", "dependency-map": "dependency-maps"}[kind]
			data, err := request(http.MethodGet, "/api/"+plural+"/"+id+"/manifest", nil, "", nil)
			if err != nil {
				return err
			}
			if action == "export" {
				return o.generated(out, *pathOut, data)
			}
			var manifest any
			if err = yaml.Unmarshal(data, &manifest); err != nil {
				return err
			}
			result := map[string]any{"definition": manifest}
			return render(out, result, *output, kind)
		default:
			return fmt.Errorf("unsupported command %s", action)
		}
		data, err := request(method, path, body, media, rev)
		if err != nil {
			return err
		}
		if action == "history" && *output == "csv" {
			var h engine.History
			if err = json.Unmarshal(data, &h); err != nil {
				return err
			}
			return historyCSV(out, h)
		}
		var value any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err = decoder.Decode(&value); err != nil {
			return err
		}
		if action == "events" && id != "" {
			if list, ok := value.([]any); ok {
				filtered := []any{}
				for _, v := range list {
					m, ok := v.(map[string]any)
					if !ok {
						return fmt.Errorf("invalid alert response")
					}
					e, ok := m["event"].(map[string]any)
					if !ok {
						return fmt.Errorf("invalid alert event")
					}
					if e["sloId"] == id {
						filtered = append(filtered, v)
					}
				}
				value = filtered
			}
		}
		if action == "get" || action == "connections-list" {
			return render(out, value, *output, kind)
		}
		if err := render(out, value, *output, action); err != nil {
			return err
		}
		if action == "budget" && *budgetCheck {
			var decision v1.ErrorBudgetDecision
			if err := json.Unmarshal(data, &decision); err != nil || decision.APIVersion != v1.APIVersion || decision.SLOID != id || decision.Tenant != *tenant || decision.Revision < 1 || (visited["expected-revision"] && decision.Revision != *expected) || decision.DefinitionDigest == "" || decision.PolicyDigest == "" || decision.State != "proceed" || decision.QualityState != "usable" || decision.SourceDigest == "" || decision.Evidence == nil || decision.Evidence.Coverage != 1 || decision.Evidence.Total <= 0 || decision.Evidence.Total < decision.Evidence.MinimumEvents || decision.Evidence.Freshness.State != "fresh" {
				return &cliError{Code: "budget_policy", Message: "error-budget policy does not permit proceeding; review the decision evidence", Exit: 7}
			}
		}
		return nil
	}
	return cmd
}
func generated(out io.Writer, path string, body []byte) error {
	if path == "" {
		_, err := out.Write(body)
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(body)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	_, err = fmt.Fprintln(out, path)
	return err
}
func render(out io.Writer, value any, format, kind string) error {
	if format == "json" {
		e := json.NewEncoder(out)
		e.SetIndent("", "  ")
		return e.Encode(value)
	}
	if format == "yaml" {
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var canonical any
		if err = yaml.Unmarshal(raw, &canonical); err != nil {
			return err
		}
		b, err := yaml.Marshal(canonical)
		if err != nil {
			return err
		}
		_, err = out.Write(b)
		return err
	}
	if format != "table" {
		return fmt.Errorf("unsupported output format %q", format)
	}
	if err := validateTable(value, kind); err != nil {
		return err
	}
	list, ok := value.([]any)
	if !ok {
		if m, yes := value.(map[string]any); yes && kind == "connections" {
			list, _ = m["connections"].([]any)
		}
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	switch kind {
	case "search":
		envelope, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid search response")
		}
		items, _ := envelope["items"].([]any)
		if _, err := fmt.Fprintln(w, "KIND\tID\tPROJECT\tNAME\tREVISION"); err != nil {
			return err
		}
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid resource summary")
			}
			if err := tableRow(w, "%v\t%v\t%v\t%v\t%v\n", m["kind"], m["id"], m["project"], m["displayName"], m["revision"]); err != nil {
				return err
			}
		}
		if next, _ := envelope["nextCursor"].(string); next != "" {
			if err := tableRow(w, "Next cursor: %s\n", next); err != nil {
				return err
			}
		}
	case "slos":
		if _, err := fmt.Fprintln(w, "ID\tSOURCE\tTARGET\tOBSERVED\tCOVERAGE\tBURN"); err != nil {
			return err
		}
		for _, v := range list {
			m := v.(map[string]any)
			s := m["slo"].(map[string]any)
			status := m["status"].(map[string]any)
			source, _ := s["source"].(string)
			if source == "" {
				source = "primary"
			}
			if err := tableRow(w, "%s\t%s\t%s\t%s\t%s\t%s\n", s["id"], source, number(s["target"], 100, "%"), number(status["reliability"], 100, "%"), number(status["coverage"], 100, "%"), number(status["burnRate"], 1, "x")); err != nil {
				return err
			}
		}
	case "journeys":
		if _, err := fmt.Fprintln(w, "ID\tNAME\tHEALTH\tREVISION"); err != nil {
			return err
		}
		for _, v := range list {
			m := v.(map[string]any)
			r := m["resource"].(map[string]any)
			manifest := r["manifest"].(map[string]any)
			if err := tableRow(w, "%s\t%s\t%s\t%v\n", manifest["metadata"].(map[string]any)["name"], manifest["spec"].(map[string]any)["displayName"], m["health"], r["revision"]); err != nil {
				return err
			}
		}
	case "connections":
		if _, err := fmt.Fprintln(w, "NAME\tKIND\tMIMIR TENANT\tENDPOINT"); err != nil {
			return err
		}
		for _, v := range list {
			m := v.(map[string]any)
			if err := tableRow(w, "%v\t%v\t%v\t%v\n", m["id"], m["kind"], m["mimirOrgID"], m["endpoint"]); err != nil {
				return err
			}
		}
	case "events":
		if _, err := fmt.Fprintln(w, "TIME\tSLO\tTRANSITION\tSHORT BURN\tLONG BURN\tDELIVERY"); err != nil {
			return err
		}
		for _, v := range list {
			m := v.(map[string]any)
			e := m["event"].(map[string]any)
			if err := tableRow(w, "%v\t%v\t%v\t%s\t%s\t%v\n", e["time"], e["sloId"], e["kind"], number(e["shortBurn"], 1, "x"), number(e["longBurn"], 1, "x"), m["status"]); err != nil {
				return err
			}
		}
	default:
		return genericTable(out, value)

	}
	return w.Flush()
}
func number(value any, mult float64, suffix string) string {
	if n, ok := value.(json.Number); ok {
		f, e := n.Float64()
		if e == nil {
			value = f
		}
	}
	if f, ok := value.(float64); ok {
		return fmt.Sprintf("%.3f%s", f*mult, suffix)
	}
	return "unknown"
}
func historyCSV(out io.Writer, h engine.History) error {
	w := csv.NewWriter(out)
	if err := w.Write([]string{"time", "interval_sli", "rolling_sli", "target", "coverage", "budget_remaining_fraction", "short_burn", "long_burn", "interval_below_target", "rolling_target_violated", "burn_breached"}); err != nil {
		return err
	}
	n := func(p *float64) string {
		if p == nil {
			return ""
		}
		return strconv.FormatFloat(*p, 'g', -1, 64)
	}
	for _, p := range h.Points {
		if err := w.Write([]string{p.At.Format(time.RFC3339), n(p.IntervalReliability), n(p.Reliability), strconv.FormatFloat(h.Target, 'g', -1, 64), strconv.FormatFloat(p.Coverage, 'g', -1, 64), n(p.BudgetRemainingFraction), n(p.ShortBurnRate), n(p.LongBurnRate), strconv.FormatBool(p.BelowTarget), strconv.FormatBool(p.TargetViolated), strconv.FormatBool(p.BurnThresholdBreached)}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

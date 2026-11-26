package openapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/client"
)

// Every stable request the Go client sends must be a documented operation. Each
// exported method is called with placeholder arguments against a recording
// server; a method that refuses the placeholders before sending must be given
// arguments or, for experimental features only, listed in unchecked.
func TestGoClientCallsOnlyDocumentedOperations(t *testing.T) {
	raw, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err = yaml.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	type operation struct {
		method string
		path   *regexp.Regexp
	}
	var documented []operation
	param := regexp.MustCompile(`\\\{[^}]+\\\}`)
	for path, methods := range spec.Paths {
		pattern := regexp.MustCompile("^" + param.ReplaceAllString(regexp.QuoteMeta(path), "[^/]+") + "$")
		for method := range methods {
			documented = append(documented, operation{strings.ToUpper(method), pattern})
		}
	}

	var mu sync.Mutex
	sent := map[string]bool{}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sent[r.Method+" "+r.URL.Path] = true
		requests++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()
	c, err := client.New(client.Config{URL: server.URL, Tenant: "default", Token: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}

	value := reflect.ValueOf(c)
	silent := []string{}
	for i := 0; i < value.NumMethod(); i++ {
		m := value.Type().Method(i)
		mu.Lock()
		before := requests
		mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		args := make([]reflect.Value, m.Type.NumIn()-1)
		for j := range args {
			args[j] = placeholder(m.Type.In(j+1), ctx, j)
		}
		for j, v := range arguments[m.Name] {
			args[j+1] = reflect.ValueOf(v)
		}
		func() {
			defer func() { _ = recover() }()
			value.Method(i).Call(args)
		}()
		cancel()
		mu.Lock()
		if requests == before {
			silent = append(silent, m.Name)
		}
		mu.Unlock()
	}

	routes := make([]string, 0, len(sent))
	for route := range sent {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		method, path, _ := strings.Cut(route, " ")
		if experimentalRoute(path) {
			continue
		}
		found := false
		for _, op := range documented {
			found = found || op.method == method && op.path.MatchString(path)
		}
		if !found {
			t.Errorf("the Go client sends %s, which the contract does not document", route)
		}
	}
	if len(routes) == 0 {
		t.Fatal("the Go client sent no requests")
	}
	if got, want := strings.Join(silent, ", "), strings.Join(unchecked, ", "); got != want {
		t.Errorf("methods that send nothing with placeholder arguments:\n got %s\nwant %s\ngive a new method arguments, or list an experimental one in unchecked", got, want)
	}
}

// unchecked are experimental-feature methods whose arguments are too
// specific for placeholders. Stable methods are never listed.
var unchecked = []string{"AssessVisual", "ConvertMigration", "ExportIntentKubernetes", "ExportIntentRollbackKubernetes", "IntentContext", "ReviewSkill", "ReviewVisual", "WatchInvestigationRun"}

// arguments override placeholders, after the context, for methods that check
// their arguments before sending.
var arguments = map[string][]any{
	"CreateSLO":       {v1.ServiceLevelObjective{APIVersion: v1.APIVersion, Kind: v1.SLOKind, Metadata: v1.Metadata{Name: "checkout"}}},
	"GitProposalsFor": {"slo", "checkout"},
	"GitDriftFor":     {"slo", "checkout"},
	"ProposeYAML":     {manifest, int64(1), strings.Repeat("a", 64)},
	"PlanRetirement":  {"checkout", "archive"},
	"ApplyRetirement": {"checkout", v1.RetirementInput{Action: "archive", ExpectedRevision: 1, Digest: strings.Repeat("a", 64)}},
	"CompileBinding":  {"checkout", v1.CompileBindingInput{ExpectedRevision: 1}},
}

var manifest = []byte("apiVersion: nines.dev/v1beta1\nkind: ServiceLevelObjective\nmetadata: {name: checkout}\n")

// placeholder returns a valid-looking argument: identifiers for strings, one
// for numbers, a manifest for bytes, increasing times, zero values otherwise.
func placeholder(t reflect.Type, ctx context.Context, position int) reflect.Value {
	if t.Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
		return reflect.ValueOf(ctx)
	}
	if t == reflect.TypeOf(time.Time{}) {
		return reflect.ValueOf(time.Now().Add(time.Duration(position-3) * time.Hour))
	}
	v := reflect.New(t).Elem()
	switch t.Kind() {
	case reflect.String:
		v.SetString("checkout")
	case reflect.Int, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			v.SetBytes(manifest)
		}
	case reflect.Pointer:
		v.Set(reflect.New(t.Elem()))
	}
	return v
}

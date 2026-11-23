package openapi

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/features"
)

func TestContractStructureAndRegisteredRoutes(t *testing.T) {
	data, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	if spec["openapi"] != "3.1.1" {
		t.Fatal("unexpected OpenAPI version")
	}
	info, ok := spec["info"].(map[string]any)
	if !ok || info["version"] != "0.1.0-alpha.4" {
		t.Fatal("unexpected contract version")
	}
	paths, ok := spec["paths"].(map[string]any)
	if !ok || len(paths) != 32 {
		t.Fatal("unexpected contract scope")
	}
	components, ok := spec["components"].(map[string]any)
	if !ok {
		t.Fatal("missing components")
	}
	registered := ""
	for _, f := range []string{"../../internal/httpapi/server.go", "../../internal/intentflow/http.go", "../../internal/intentflow/preparation_evidence_http.go", "../../internal/identity/http.go"} {
		b, e := os.ReadFile(f)
		if e != nil {
			t.Fatal(e)
		}
		registered += string(b) + "\n"
	}
	ids := map[string]bool{}
	for path, item := range paths {
		methods, ok := item.(map[string]any)
		if !ok {
			t.Fatal(path)
		}
		for method, value := range methods {
			operation, ok := value.(map[string]any)
			if !ok {
				t.Fatal(path, method)
			}
			id, ok := operation["operationId"].(string)
			if !ok || id == "" || ids[id] {
				t.Fatal("missing or duplicate operation ID", id)
			}
			ids[id] = true
			needle := `"` + strings.ToUpper(method) + " " + path + `"`
			if !strings.Contains(registered, needle) {
				t.Error("unregistered operation", needle)
			}
			responses, ok := operation["responses"].(map[string]any)
			if !ok || responses["200"] == nil {
				t.Error("missing success response", id)
			}
			params, ok := operation["parameters"].([]any)
			if strings.HasPrefix(path, "/auth/cli/") {
				security, public := operation["security"].([]any)
				if !public || len(security) != 0 {
					t.Error("CLI bootstrap must not require a resource bearer credential", id)
				}
			} else if !ok || len(params) == 0 {
				t.Error("missing tenant parameter", id)
			}
		}
	}
	var checkRefs func(any)
	checkRefs = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, y := range x {
				if k == "$ref" {
					p, ok := y.(string)
					if !ok || !strings.HasPrefix(p, "#/components/") {
						t.Errorf("unsupported ref %v", y)
						continue
					}
					parts := strings.Split(strings.TrimPrefix(p, "#/components/"), "/")
					if len(parts) != 2 {
						t.Errorf("invalid ref %s", p)
						continue
					}
					group, ok := components[parts[0]].(map[string]any)
					if !ok || group[parts[1]] == nil {
						t.Errorf("unresolved ref %s", p)
					}
				} else {
					checkRefs(y)
				}
			}
		case []any:
			for _, y := range x {
				checkRefs(y)
			}
		}
	}
	checkRefs(spec)
}

func TestCriticalSchemaFieldsFollowGoContracts(t *testing.T) {
	data, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	schemas := spec["components"].(map[string]any)["schemas"].(map[string]any)
	for name, value := range map[string]any{"IntentActivation": v1.IntentActivation{}, "IntentResourceVerification": v1.IntentResourceVerification{}, "IntegrationStatus": v1.IntegrationStatus{}, "IntegrationStage": v1.IntegrationStage{}, "MeasurementQuality": v1.MeasurementQuality{}, "IntentReviewCandidateReceipt": v1.IntentReviewCandidateReceipt{}, "IntentReviewExclusions": v1.IntentReviewExclusions{}, "AppendIntentConversation": v1.AppendIntentConversation{}, "IntentReviewPacket": v1.IntentReviewPacket{}, "IntentReviewDestination": v1.IntentReviewDestination{}, "IntentReviewResource": v1.IntentReviewResource{}, "IntentRecommendationReview": v1.IntentRecommendation{}, "IntentChangeSetReview": v1.IntentChangeSet{}, "RecordIntentEvidence": v1.RecordIntentEvidence{}, "IntentPreparationEvidence": v1.IntentPreparationEvidence{}, "IntentPreparationCheck": v1.IntentPreparationCheck{}, "IntentEvidence": v1.IntentEvidence{}, "IntentEvidenceInput": v1.IntentEvidenceInput{}, "IntentEvidenceCurrent": v1.IntentEvidenceCurrent{}, "IntentEvidenceHistory": v1.IntentEvidenceHistory{}, "IntentEvidenceComparison": v1.IntentEvidenceComparison{}, "IntentPolicyTradeoff": v1.IntentPolicyTradeoff{}, "IntentEvidenceAlternative": v1.IntentEvidenceAlternative{}, "IntentEvidenceRange": v1.IntentEvidenceRange{}, "IntentCandidateDiscovery": v1.IntentCandidateDiscovery{}, "IntentCandidateInput": v1.IntentCandidateInput{}, "IntentMeasurementCandidate": v1.IntentMeasurementCandidate{}, "IntentRecipeReference": v1.IntentRecipeReference{}, "IntentContext": v1.IntentContext{}, "IntentContextInput": v1.IntentContextInput{}, "IntentSourceContext": v1.IntentSourceContext{}, "IntentJourneyContext": v1.IntentJourneyContext{}, "CLIAuthenticationCapabilities": v1.CLIAuthenticationCapabilities{}, "CLIDeviceStart": v1.CLIDeviceStart{}, "CLIDeviceAuthorization": v1.CLIDeviceAuthorization{}, "CLIDevicePoll": v1.CLIDevicePoll{}, "CLIDeviceResult": v1.CLIDeviceResult{}, "NativeSLOResults": v1.NativeSLOResults{}, "NativeSLOCatalog": v1.NativeSLOCatalog{}, "NativeSLODraft": v1.NativeSLODraft{}, "ReviewRequest": v1.ReviewIntentChangeSet{}, "WorkflowStatus": v1.IntentWorkflowStatus{}, "ReviewArtifact": v1.IntentReviewArtifact{}, "IntentConversation": v1.IntentConversation{}, "GitInfo": v1.GitInfo{}, "GitPullRequest": v1.GitPullRequest{}, "GitReceipt": v1.GitReceipt{}, "GitProposalResult": v1.GitProposalResult{}, "GitProposalStatus": v1.GitProposalStatus{}, "GitProposalList": v1.GitProposalList{}, "GitDrift": v1.GitDrift{}, "MigrationRequest": v1.MigrationRequest{}, "MigrationNote": v1.MigrationNote{}, "MigrationDefinition": v1.MigrationDefinition{}, "MigrationSkipped": v1.MigrationSkipped{}, "MigrationResult": v1.MigrationResult{}} {
		schema := schemas[name].(map[string]any)
		props := schema["properties"].(map[string]any)
		typ := reflect.TypeOf(value)
		fields := map[string]bool{}
		var visit func(reflect.Type)
		visit = func(typ reflect.Type) {
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if f.Anonymous && f.Tag.Get("json") == "" && f.Type.Kind() == reflect.Struct {
					visit(f.Type)
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")[0]
				if tag != "" && tag != "-" {
					fields[tag] = true
				}
			}
		}
		visit(typ)
		for field := range props {
			if !fields[field] {
				t.Errorf("%s property %s not in Go type", name, field)
			}
		}
		b, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		var encoded map[string]any
		if e = json.Unmarshal(b, &encoded); e != nil {
			t.Fatal(e)
		}
		required, ok := schema["required"].([]any)
		if !ok {
			t.Fatal(name, "has no required fields")
		}
		for _, entry := range required {
			key := entry.(string)
			if _, ok = encoded[key]; !ok {
				t.Errorf("%s requires omitted field %s", name, key)
			}
		}
	}
}

func TestIfMatchRevisionPattern(t *testing.T) {
	data, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec map[string]any
	if err = yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	params := spec["components"].(map[string]any)["parameters"].(map[string]any)
	schema := params["IfMatch"].(map[string]any)["schema"].(map[string]any)
	re := regexp.MustCompile(schema["pattern"].(string))
	for _, v := range []string{`"0"`, `"42"`} {
		if !re.MatchString(v) {
			t.Fatal("valid revision rejected", v)
		}
	}
	for _, v := range []string{"0", `"-1"`, `"01"`} {
		if re.MatchString(v) {
			t.Fatal("invalid revision accepted", v)
		}
	}
}

var routePattern = regexp.MustCompile(`"(GET|POST|PUT|PATCH|DELETE) (/(?:api|auth)/[^"]*)"`)

// registeredRoutes finds every route the server registers, from the
// method-and-path patterns in its Go source.
func registeredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, dir := range []string{"../../internal", "../../cmd"} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range routePattern.FindAllStringSubmatch(string(b), -1) {
				out[m[1]+" "+m[2]] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func experimentalRoute(path string) bool {
	_, gated := features.ForPath(strings.NewReplacer("{", "", "}", "").Replace(path))
	return gated
}

// Every stable route is in the contract, outside it for a stated reason, or a
// known gap that may only shrink; experimental operations in the contract
// are marked as such.
func TestContractCoversTheStableSurface(t *testing.T) {
	data, err := os.ReadFile("v1alpha1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Paths map[string]map[string]map[string]any `yaml:"paths"`
	}
	if err = yaml.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	var scope struct {
		Browser   map[string]string `yaml:"browser"`
		Uncovered []string          `yaml:"uncovered"`
	}
	raw, err := os.ReadFile("scope.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err = yaml.Unmarshal(raw, &scope); err != nil {
		t.Fatal(err)
	}
	documented := map[string]bool{}
	for path, methods := range spec.Paths {
		for method, op := range methods {
			route := strings.ToUpper(method) + " " + path
			documented[route] = true
			if experimentalRoute(path) && op["x-nines-maturity"] != "experimental" {
				t.Errorf("%s is experimental: mark it x-nines-maturity: experimental", route)
			}
		}
	}
	uncovered := map[string]bool{}
	for _, r := range scope.Uncovered {
		uncovered[r] = true
	}
	registered := registeredRoutes(t)
	for route := range registered {
		_, path, _ := strings.Cut(route, " ")
		inBrowser := scope.Browser[route] != ""
		switch {
		case experimentalRoute(path):
		case documented[route] && (uncovered[route] || inBrowser):
			t.Errorf("%s is documented: remove it from scope.yaml", route)
		case !documented[route] && !uncovered[route] && !inBrowser:
			t.Errorf("stable route %s is not in the contract: document it in v1alpha1.yaml", route)
		}
	}
	for route := range uncovered {
		if !registered[route] {
			t.Errorf("scope.yaml lists %s, which the server no longer registers", route)
		}
	}
	for route := range scope.Browser {
		if !registered[route] {
			t.Errorf("scope.yaml lists browser route %s, which the server no longer registers", route)
		}
	}
}

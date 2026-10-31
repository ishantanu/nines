package mcpserver

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ishantanu/nines/internal/model"
)

type historyInput struct {
	ID                string `json:"id" jsonschema:"SLO identifier in the configured tenant"`
	From              string `json:"from,omitempty" jsonschema:"Optional aligned RFC3339 start; cannot combine with rangeSeconds"`
	To                string `json:"to,omitempty" jsonschema:"Optional aligned RFC3339 end; defaults to the current evaluation"`
	RangeSeconds      *int64 `json:"rangeSeconds,omitempty" jsonschema:"Optional positive lookback, at most two retained SLO windows; use instead of from"`
	ResolutionSeconds *int64 `json:"resolutionSeconds,omitempty" jsonschema:"Optional positive multiple of sampling interval; at most 1500 points"`
}
type sloApplyInput struct {
	Manifest         string `json:"manifest" jsonschema:"Exactly one ServiceLevelObjective YAML manifest using nines.dev/v1alpha1"`
	ExpectedRevision int64  `json:"expectedRevision" jsonschema:"Required compare-and-swap revision; 0 creates, current revision updates"`
}
type journeyExportInput struct {
	ID       string `json:"id" jsonschema:"Journey identifier"`
	Revision *int64 `json:"revision,omitempty" jsonschema:"Optional positive historical revision; omission exports latest"`
}
type deployRulesInput struct {
	ID             string `json:"id" jsonschema:"SLO identifier"`
	ExpectedDigest string `json:"expectedDigest" jsonschema:"Exact SHA-256 digest from a reviewed plan_slo_rules result; definition or destination changes invalidate it"`
}

var ruleDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func addOperations(s *mcp.Server, a *apiClient, c Config) {
	addNativeSLOOperations(s, a)
	register(s, a, "get_slo_integration", "Read independent saved-definition, collection, publication, evaluator and recorded-output evidence for an SLO. Stored native definitions and observed attainment do not prove alert delivery. This never deploys or resumes rules.", true, idRead(func(ctx context.Context, path string) (any, error) { return a.request(ctx, http.MethodGet, path, nil) }, "/api/slos/", "/integration"))
	register(s, a, "list_connections", "Read server-configured metric connections and rule destinations. Tools cannot override credentials, Mimir tenant or endpoints.", true, func(ctx context.Context, _ emptyInput) (any, error) {
		return a.request(ctx, http.MethodGet, "/api/connections", nil)
	})
	register(s, a, "get_access", "Read the authenticated tenant role and effective permissions. This does not grant or change access.", true, func(ctx context.Context, _ emptyInput) (any, error) {
		return a.request(ctx, http.MethodGet, "/api/access", nil)
	})
	register(s, a, "slo_history", "Read bounded SLI/target comparisons, budget, both burn windows, coverage, condition spans and recorded alert transitions. History uses the current SLO revision; threshold breaches alone are not alert episodes.", true, func(ctx context.Context, in historyInput) (any, error) {
		path, err := historyPath(in)
		if err != nil {
			return nil, err
		}
		return a.request(ctx, http.MethodGet, path, nil)
	})
	register(s, a, "plan_slo_rules", "Preview generated recording/alert rules, SLO revision, exact configured dataset/namespace and deployment digest. Does not deploy; native publication plans may read provider metadata and existing definitions.", true, idRead(func(ctx context.Context, path string) (any, error) { return a.request(ctx, http.MethodGet, path, nil) }, "/api/slos/", "/rules"))
	register(s, a, "export_slo", "Export the current SLO definition as canonical YAML. Previous definitions and retained measurement evidence are available through the SLO revision APIs.", true, func(ctx context.Context, in idInput) (any, error) {
		if err := validID(in.ID); err != nil {
			return nil, err
		}
		text, err := a.manifest(ctx, "/api/slos/"+in.ID+"/manifest", model.SLOKind)
		if err != nil {
			return nil, err
		}
		return map[string]string{"manifest": text}, nil
	})
	register(s, a, "export_journey", "Export canonical CUJ YAML, optionally at an exact historical revision.", true, func(ctx context.Context, in journeyExportInput) (any, error) {
		if err := validID(in.ID); err != nil {
			return nil, err
		}
		path := "/api/journeys/" + in.ID + "/manifest"
		if in.Revision != nil {
			if *in.Revision <= 0 {
				return nil, errors.New("revision must be positive")
			}
			path += "?revision=" + strconv.FormatInt(*in.Revision, 10)
		}
		text, err := a.manifest(ctx, path, model.JourneyKind)
		if err != nil {
			return nil, err
		}
		return map[string]string{"manifest": text}, nil
	})
	register(s, a, "plan_slo", "Validate a ServiceLevelObjective YAML definition against tenant state and preview its expected revision. No mutation; measurement edits start a new configuration revision with fresh coverage and archived evidence. Inspect measurementChanged and warnings before applying.", true, func(ctx context.Context, in createInput) (any, error) {
		if _, err := model.ParseSLODefinition([]byte(in.Manifest)); err != nil {
			return nil, err
		}
		return a.request(ctx, http.MethodPost, "/api/resources/plan", yamlResource{Manifest: in.Manifest})
	})
	register(s, a, "plan_journey", "Validate a CriticalUserJourney YAML definition against tenant state and preview its expected revision. No mutation.", true, func(ctx context.Context, in manifestInput) (any, error) {
		if _, err := model.ParseJourney([]byte(in.Manifest)); err != nil {
			return nil, errors.New("invalid journey manifest; validate with nines for detailed diagnostics")
		}
		return a.request(ctx, http.MethodPost, "/api/resources/plan", yamlResource{Manifest: in.Manifest})
	})
	if c.AllowWrites {
		register(s, a, "apply_slo", "Directly create/update an SLO using exact revision CAS. Requires API owner or editor role and operator-enabled writes; this bypasses workflow review. Review plan_slo first. Rule deployment is separate.", false, func(ctx context.Context, in sloApplyInput) (any, error) {
			if in.ExpectedRevision < 0 {
				return nil, errors.New("expectedRevision must be nonnegative")
			}
			if _, err := model.ParseSLODefinition([]byte(in.Manifest)); err != nil {
				return nil, err
			}
			return a.request(ctx, http.MethodPost, "/api/resources/apply", yamlResource{in.Manifest, &in.ExpectedRevision})
		})
		register(s, a, "test_connection", "Run a bounded query against one server-configured connection using its fixed credentials and dataset. Stores no samples; requires API owner role.", false, func(ctx context.Context, in idInput) (any, error) {
			if err := validID(in.ID); err != nil {
				return nil, err
			}
			return a.request(ctx, http.MethodPost, "/api/connections/"+in.ID+"/test", nil)
		})
	}
	if c.AllowRuleDeployment {
		register(s, a, "deploy_slo_rules", "Deploy a reviewed rule plan to its server-owned destination using the exact digest. Requires API owner role and both operator write flags. Persistence is separate from activation/evaluation in the returned fields; even HTTP success can include failed reload. Prometheus optionally reloads when the trusted connection enables it. Mimir activates through its ruler. Operator destinations publish a Kubernetes SLO and reconcile asynchronously; submitted is not provider activation. Reconcile before retrying a timed-out write.", false, func(ctx context.Context, in deployRulesInput) (any, error) {
			if err := validID(in.ID); err != nil {
				return nil, err
			}
			if !ruleDigest.MatchString(in.ExpectedDigest) {
				return nil, errors.New("expectedDigest must be the 64-character lowercase SHA-256 from a reviewed rule plan")
			}
			return a.request(ctx, http.MethodPost, "/api/slos/"+in.ID+"/rules/apply", map[string]string{"expectedDigest": in.ExpectedDigest})
		})
		if c.AllowRuleWithdrawal {
			register(s, a, "withdraw_slo_rules", "Withdraw an exact reviewed operator publication while retaining the portal SLO and history. Requires explicit allow-rule-withdrawal, both write/deployment flags and API owner role. Pending finalizer cleanup keeps the destination pinned; inspect, refresh and review before retrying. Direct provider destinations are unsupported.", false, func(ctx context.Context, in deployRulesInput) (any, error) {
				if err := validID(in.ID); err != nil {
					return nil, err
				}
				if !ruleDigest.MatchString(in.ExpectedDigest) {
					return nil, errors.New("expectedDigest must come from the exact reviewed rule plan")
				}
				return a.request(ctx, http.MethodPost, "/api/slos/"+in.ID+"/rules/withdraw", map[string]string{"expectedDigest": in.ExpectedDigest})
			})
		}
	}
}
func historyPath(in historyInput) (string, error) {
	if err := validID(in.ID); err != nil {
		return "", err
	}
	if in.From != "" && in.RangeSeconds != nil {
		return "", errors.New("from and rangeSeconds are mutually exclusive")
	}
	query := url.Values{}
	var from, to time.Time
	for _, value := range []struct {
		key, text string
		parsed    *time.Time
	}{{"from", in.From, &from}, {"to", in.To, &to}} {
		if value.text != "" {
			at, err := time.Parse(time.RFC3339Nano, value.text)
			if err != nil {
				return "", errors.New(value.key + " must be RFC3339")
			}
			*value.parsed = at
			query.Set(value.key, at.Format(time.RFC3339Nano))
		}
	}
	if !from.IsZero() && !to.IsZero() && !to.After(from) {
		return "", errors.New("to must be after from")
	}
	for _, value := range []struct {
		key string
		n   *int64
	}{{"rangeSeconds", in.RangeSeconds}, {"resolutionSeconds", in.ResolutionSeconds}} {
		if value.n != nil {
			if *value.n <= 0 || *value.n > 62*86400 {
				return "", errors.New(value.key + " must be positive and at most 62 days")
			}
			query.Set(value.key, strconv.FormatInt(*value.n, 10))
		}
	}
	path := "/api/slos/" + in.ID + "/history"
	if len(query) > 0 {
		path += "?" + query.Encode()
	}
	return path, nil
}
func (a *apiClient) manifest(ctx context.Context, path, kind string) (string, error) {
	b, err := a.requestBytes(ctx, http.MethodGet, path, nil, "application/yaml")
	if err != nil {
		return "", err
	}
	definition, err := model.ParseDefinition(b)
	if err != nil || definition.Kind() != kind {
		return "", errors.New("API returned an invalid YAML resource")
	}
	return string(b), nil
}

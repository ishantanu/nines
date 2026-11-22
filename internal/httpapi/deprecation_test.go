package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/client"
	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/model"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

// A deprecated resource version is reported by the server as a Warning
// header and reaches Go client users through OnWarning; v1beta1 is silent.
func TestDeprecatedResourceVersionWarnsThroughTheClient(t *testing.T) {
	h, err := NewMulti([]Workspace{{Features: features.All(), ID: "alpha", Name: "Alpha", Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h)
	defer server.Close()
	var warnings []string
	c, err := client.New(client.Config{URL: server.URL, Tenant: "alpha", Token: "owner", OnWarning: func(m string) { warnings = append(warnings, m) }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.PlanYAML(context.Background(), []byte(betaManifest)); err != nil || len(warnings) != 0 {
		t.Fatal("v1beta1 warned", warnings, err)
	}
	alpha := strings.NewReplacer("nines.dev/v1beta1", "nines.dev/v1alpha1", "  window: 7d\n", "  windowSeconds: 604800\n  stepSeconds: 60\n", "  queries:\n    good:", "  goodQuery:", "    total:", "  totalQuery:").Replace(betaManifest)
	if _, err = c.PlanYAML(context.Background(), []byte(alpha)); err != nil {
		t.Fatal(err, alpha)
	}
	if len(warnings) != 1 || warnings[0] != model.V1alpha1Deprecation {
		t.Fatalf("got warnings %q", warnings)
	}
}

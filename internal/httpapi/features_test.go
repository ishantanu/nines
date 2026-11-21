package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/features"
	"github.com/ishantanu/nines/internal/service"
	"github.com/ishantanu/nines/internal/store"
	"github.com/ishantanu/nines/internal/tenancy"
)

// A server with no feature configuration offers only the stable core: gated
// APIs are refused with a stated reason, and the workspace lists no gated
// feature so the portal hides their pages.
func TestGatedFeaturesAreOffByDefault(t *testing.T) {
	for _, gates := range []features.Set{{}, features.All()} {
		h, err := NewMulti([]Workspace{{Features: gates, ID: "alpha", Name: "Alpha", Service: service.New(store.NewMemory(), nil), Grants: []tenancy.Grant{{Token: "owner", Role: "owner"}}}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if err != nil {
			t.Fatal(err)
		}
		call := func(method, path, body string) *httptest.ResponseRecorder {
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			r.Header.Set("Authorization", "Bearer owner")
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			return w
		}
		on := len(gates.List()) > 0
		for _, path := range []string{"/api/intent-conversations", "/api/investigations", "/api/session-replays", "/api/migrations/convert"} {
			w := call("GET", path, "")
			refused := w.Code == 404 && strings.Contains(w.Body.String(), `"code":"feature_disabled"`)
			if refused == on {
				t.Fatalf("gates %v: GET %s = %d %s", gates.List(), path, w.Code, w.Body.String())
			}
		}
		if w := call("GET", "/api/slos", ""); w.Code != 200 {
			t.Fatalf("stable core refused: %d", w.Code)
		}
		var workspace struct{ Features []string }
		if err := json.Unmarshal(call("GET", "/api/workspace", "").Body.Bytes(), &workspace); err != nil {
			t.Fatal(err)
		}
		if (len(workspace.Features) > 0) != on {
			t.Fatalf("workspace features %v with gates %v", workspace.Features, gates.List())
		}
	}
}

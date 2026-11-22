package deprecation

import (
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

var release = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[a-z]+\.[0-9]+)?$`)

func TestListIsComplete(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range List() {
		if d.ID == "" || seen[d.ID] || d.Surface == "" || d.What == "" || d.Instead == "" {
			t.Fatalf("%q needs a unique ID, a surface, what is deprecated and what to use instead", d.ID)
		}
		seen[d.ID] = true
		if !release.MatchString(d.Since) || !release.MatchString(d.RemovalNotBefore) {
			t.Fatalf("%s: since and removal must be release versions", d.ID)
		}
		// Removal of anything stable happens only in a major release.
		if !strings.HasSuffix(d.RemovalNotBefore, ".0.0") {
			t.Fatalf("%s: removal must be a major release", d.ID)
		}
	}
}

// The policy page's table is the list, so the published deprecations and the
// warnings users see cannot drift apart.
func TestPolicyTableMatchesList(t *testing.T) {
	raw, err := os.ReadFile("../../docs/public/compatibility.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "<!-- deprecations:start -->\n")
	end := strings.Index(text, "<!-- deprecations:end -->")
	if start < 0 || end < start {
		t.Fatal("deprecation table markers missing")
	}
	got := text[start+len("<!-- deprecations:start -->\n") : end]
	if got != Table() {
		t.Fatalf("docs/public/compatibility.md table is stale; want:\n%s", Table())
	}
}

func TestWarningHeaderRoundTrips(t *testing.T) {
	d := Deprecation{What: `"quoted" \ things`, Instead: "use the other", RemovalNotBefore: "v2.0.0"}
	text, ok := ParseWarning(d.WarningHeader())
	if !ok || text != d.Message() {
		t.Fatalf("got %q %v", text, ok)
	}
	for _, other := range []string{"", `199 - "misc"`, `299 - unquoted`, `299 - "unterminated`} {
		if _, ok := ParseWarning(other); ok {
			t.Fatalf("accepted %q", other)
		}
	}
	w := httptest.NewRecorder()
	Warn(w, ResourceV1alpha1)
	Warn(w, ResourceV1alpha1)
	Warn(w, "unknown")
	if got := w.Header().Values("Warning"); len(got) != 1 {
		t.Fatalf("want one warning, got %q", got)
	}
}

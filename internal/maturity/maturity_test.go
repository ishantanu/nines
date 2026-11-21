package maturity

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ishantanu/nines/internal/providers"
)

func root(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCatalogIsConsistent(t *testing.T) {
	seen := map[string]bool{}
	known := map[Level]bool{}
	for _, d := range Levels() {
		known[d.Level] = true
	}
	for _, f := range Catalog() {
		if f.ID == "" || f.Name == "" || seen[f.ID] {
			t.Fatalf("feature %q needs a unique ID and a name", f.ID)
		}
		seen[f.ID] = true
		if !known[f.Level] {
			t.Fatalf("%s: unknown level %q", f.ID, f.Level)
		}
		if (f.Level == Experimental) != (f.Remaining != "") {
			t.Fatalf("%s: experimental features, and only they, state what remains before beta", f.ID)
		}
		if _, err := os.Stat(filepath.Join(root(t), "docs/public", f.Docs+".md")); err != nil {
			t.Fatalf("%s: guide %s.md does not exist", f.ID, f.Docs)
		}
	}
}

// Every installed telemetry provider has exactly one maturity entry, so a new
// adapter cannot ship without a stated promise.
func TestEveryProviderHasALevel(t *testing.T) {
	byKind := map[string]int{}
	for _, f := range Catalog() {
		if f.Provider != "" {
			byKind[f.Provider]++
		}
	}
	for _, d := range providers.Default.List() {
		if byKind[d.Kind] != 1 {
			t.Fatalf("provider %s has %d maturity entries, want 1", d.Kind, byKind[d.Kind])
		}
		delete(byKind, d.Kind)
	}
	if len(byKind) != 0 {
		t.Fatalf("maturity entries for providers that are not installed: %v", byKind)
	}
}

// The portal reads a generated copy. Run with UPDATE_MATURITY=1 to rewrite it.
func TestPortalCopyMatchesCatalog(t *testing.T) {
	path := filepath.Join(root(t), "web/src/lib/maturity.json")
	want, err := json.MarshalIndent(Published(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if os.Getenv("UPDATE_MATURITY") == "1" {
		if err := os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("web/src/lib/maturity.json is stale; run UPDATE_MATURITY=1 go test ./internal/maturity")
	}
}

// The maturity guide lists every feature, at its level, in one table.
func TestGuideMatchesCatalog(t *testing.T) {
	text, err := os.ReadFile(filepath.Join(root(t), "docs/public/maturity.md"))
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile(`(?m)^\| \[([^\]]+)\]\(([^)]+)\.md(?:#[^)]*)?\) +\| (Stable|Beta|Experimental) +\|`)
	rows := map[string][2]string{}
	lines := map[string]string{}
	for _, m := range row.FindAllStringSubmatch(string(text), -1) {
		rows[m[1]] = [2]string{m[2], strings.ToLower(m[3])}
	}
	for _, line := range strings.Split(string(text), "\n") {
		if m := row.FindStringSubmatch(line); m != nil {
			lines[m[1]] = line
		}
	}
	for _, f := range Catalog() {
		got, ok := rows[f.Name]
		if !ok {
			t.Fatalf("maturity.md has no row for %q", f.Name)
		}
		if got[0] != f.Docs || got[1] != string(f.Level) {
			t.Fatalf("maturity.md row for %q is %v, want %s.md at %s", f.Name, got, f.Docs, f.Level)
		}
		if gate := "`NINES_FEATURES=" + f.ID + "`"; f.Gated != strings.Contains(lines[f.Name], gate) {
			t.Fatalf("maturity.md row for %q must name %s exactly when the feature is gated", f.Name, gate)
		}
		delete(rows, f.Name)
	}
	if len(rows) != 0 {
		t.Fatalf("maturity.md lists features missing from the catalog: %v", rows)
	}
}

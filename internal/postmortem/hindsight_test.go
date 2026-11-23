package postmortem

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestCheckFlagsHindsightAndBlame(t *testing.T) {
	for text, want := range map[string]string{
		"The on-call engineer should have rolled back sooner.": "should have",
		"Deploy FAILED TO check the canary.":                   "failed to",
		"The root cause was a bad config.":                     "root cause",
		"Classic human error.":                                 "human error",
	} {
		got := Check(text)
		if len(got) == 0 || got[0].Phrase != want {
			t.Fatalf("%q: got %+v, want %q", text, got, want)
		}
	}
	for _, neutral := range []string{"", "The rollback started at 14:05 after the canary alert.", "Failure injection stayed on for three minutes.", "The blameless review found three contributing causes."} {
		if got := Check(neutral); len(got) != 0 {
			t.Fatalf("%q flagged %+v", neutral, got)
		}
	}
}

// The portal flags the same phrases while people type. Run with
// UPDATE_HINDSIGHT=1 to rewrite its copy.
func TestPortalCopyMatches(t *testing.T) {
	want, err := json.MarshalIndent(Phrases, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	path := "../../web/src/lib/hindsight.json"
	if os.Getenv("UPDATE_HINDSIGHT") == "1" {
		if err = os.WriteFile(path, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("web/src/lib/hindsight.json is stale; run UPDATE_HINDSIGHT=1 go test ./internal/postmortem")
	}
}

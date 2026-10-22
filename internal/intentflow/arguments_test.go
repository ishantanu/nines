package intentflow

import (
	"encoding/json"
	"testing"
)

func TestIntentToolArgumentsRequireOneStrictObject(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{} {}`, `{"question":"first","question":"second"}`, `{"question":"first","\u0071uestion":"second"}`, `{"question":"review","tenant":"other"}`, `{"question":"review","apiKey":"private-key"}`} {
		var value struct {
			Question string `json:"question"`
		}
		if err := decodeArgs(json.RawMessage(raw), &value); err == nil {
			t.Fatalf("accepted ambiguous or unsafe model arguments %s", raw)
		}
	}
	var value struct {
		Question string `json:"question"`
	}
	if err := decodeArgs(json.RawMessage(`{"question":"Which population?"}`), &value); err != nil || value.Question != "Which population?" {
		t.Fatal(value, err)
	}
	if err := decodeArgs(json.RawMessage(`{}`), &struct{}{}); err != nil {
		t.Fatal(err)
	}
}

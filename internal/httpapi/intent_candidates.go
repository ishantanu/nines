package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

// Candidate reads may reach upstream providers. Reject ambiguous objects before
// decoding scope, including duplicate nested population keys and case aliases.
func candidateInput(w http.ResponseWriter, r *http.Request, out *v1.IntentCandidateInput) bool {
	return boundedIntentInput(w, r, out, map[string]bool{"service": true, "source": true, "project": true, "population": true, "prefix": true, "metrics": true, "category": true, "outcome": true, "threshold": true, "limit": true})
}
func boundedIntentInput(w http.ResponseWriter, r *http.Request, out any, allowed map[string]bool, required ...string) bool {
	if _, err := parseContextQuery(r); err != nil {
		telemetryError(w, err)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32<<10)
	defer func() { _ = r.Body.Close() }()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		telemetryError(w, model.Invalid("intent request exceeds 32 KiB or cannot be read"))
		return false
	}
	d := json.NewDecoder(bytes.NewReader(body))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 32 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		delimiter, compound := token.(json.Delim)
		if !compound {
			return depth != 0 && token != nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok || seen[name] || (depth == 0 && !allowed[name]) {
					return false
				}
				seen[name] = true
				if !value(depth + 1) {
					return false
				}
			}
			if depth == 0 {
				for _, key := range required {
					if !seen[key] {
						return false
					}
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			if depth == 0 {
				return false
			}
			for d.More() {
				if !value(depth + 1) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		default:
			return false
		}
	}
	if !value(0) {
		telemetryError(w, model.Invalid("provide one unambiguous intent request with canonical root field names"))
		return false
	}
	if _, err = d.Token(); err != io.EOF {
		telemetryError(w, model.Invalid("provide one intent request"))
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(out); err != nil {
		telemetryError(w, model.Invalid("invalid intent request"))
		return false
	}
	return true
}

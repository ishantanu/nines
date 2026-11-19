package intentflow

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
	"github.com/ishantanu/nines/internal/model"
)

func mountPreparationEvidence(m *http.ServeMux, s *Service) {
	m.HandleFunc("POST /api/intent-conversations/{id}/evidence", func(w http.ResponseWriter, r *http.Request) {
		var in v1.RecordIntentEvidence
		typ, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || typ != "application/json" || r.URL.RawQuery != "" {
			fail(w, model.Invalid("send one JSON evidence request without query fields"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		defer func() { _ = r.Body.Close() }()
		body, err := io.ReadAll(r.Body)
		if err != nil || !unambiguousEvidenceJSON(body) || decodeArgs(body, &in) != nil {
			fail(w, model.Invalid("send one unambiguous evidence request (maximum 4 KiB)"))
			return
		}
		out, err := s.RecordPreparationEvidence(r.Context(), r.PathValue("id"), in)
		reply(w, out, err, http.StatusOK)
	})
}

// Evidence requests can cause external reads. Reject duplicate keys at every
// depth and noncanonical root names before interpreting consent or provenance.
func unambiguousEvidenceJSON(body []byte) bool {
	d := json.NewDecoder(bytes.NewReader(body))
	allowed := map[string]bool{"expectedVersion": true, "intentDigest": true, "readTelemetry": true, "history": true, "alternatives": true}
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 16 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		delim, compound := token.(json.Delim)
		if !compound {
			return depth != 0 && token != nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				token, err := d.Token()
				key, ok := token.(string)
				if err != nil || !ok || seen[key] || (depth == 0 && !allowed[key]) {
					return false
				}
				seen[key] = true
				if !value(depth + 1) {
					return false
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
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

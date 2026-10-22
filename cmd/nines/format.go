package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"
)

func validateTable(v any, kind string) error {
	if kind != "slos" && kind != "journeys" && kind != "connections" && kind != "events" {
		return nil
	}
	if kind == "connections" {
		if m, ok := v.(map[string]any); ok {
			v = m["connections"]
		}
	}
	list, ok := v.([]any)
	if !ok {
		return fmt.Errorf("invalid %s response: expected a list", kind)
	}
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid %s row", kind)
		}
		switch kind {
		case "slos":
			if _, ok = m["slo"].(map[string]any); !ok {
				return fmt.Errorf("invalid SLO definition")
			}
			if _, ok = m["status"].(map[string]any); !ok {
				return fmt.Errorf("invalid SLO status")
			}
		case "journeys":
			r, ok := m["resource"].(map[string]any)
			if !ok {
				return fmt.Errorf("invalid journey resource")
			}
			d, ok := r["manifest"].(map[string]any)
			if !ok {
				return fmt.Errorf("invalid journey manifest")
			}
			if _, ok = d["metadata"].(map[string]any); !ok {
				return fmt.Errorf("invalid journey metadata")
			}
			if _, ok = d["spec"].(map[string]any); !ok {
				return fmt.Errorf("invalid journey spec")
			}
		case "events":
			if _, ok = m["event"].(map[string]any); !ok {
				return fmt.Errorf("invalid alert event")
			}
		}
	}
	return nil
}

func safeCell(s string) string {
	if strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsControl(r) || r == '\u202e' || r == '\u202d' || r == '\u2066' || r == '\u2067' || r == '\u2068' || r == '\u2069'
	}) {
		q := strconv.QuoteToASCII(s)
		return q[1 : len(q)-1]
	}
	return s
}
func tableRow(w io.Writer, format string, args ...any) error {
	for i, v := range args {
		if s, ok := v.(string); ok {
			args[i] = safeCell(s)
		}
	}
	_, err := fmt.Fprintf(w, format, args...)
	return err
}
func genericTable(out io.Writer, value any) error {
	b, e := json.Marshal(value)
	if e != nil {
		return e
	}
	var v any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if e = d.Decode(&v); e != nil {
		return e
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	if m, ok := v.(map[string]any); ok {
		if _, err := fmt.Fprintln(w, "FIELD\tVALUE"); err != nil {
			return err
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b, _ := json.Marshal(m[k])
			if s, ok := m[k].(string); ok {
				b = []byte(s)
			}
			if err := tableRow(w, "%s\t%s\n", k, string(b)); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(w, "VALUE"); err != nil {
			return err
		}
		if err := tableRow(w, "%s\n", string(b)); err != nil {
			return err
		}
	}
	return w.Flush()
}

func (o *options) generated(out io.Writer, path string, body []byte) error {
	if path == "" || (!o.Agent && o.Output == "") {
		return generated(out, path, body)
	}
	if e := generated(io.Discard, path, body); e != nil {
		return e
	}
	sum := sha256.Sum256(body)
	return render(out, map[string]any{"apiVersion": "nines.dev/v1alpha1", "kind": "CLIArtifact", "path": path, "bytes": len(body), "sha256": hex.EncodeToString(sum[:])}, o.format("json"), "artifact")
}

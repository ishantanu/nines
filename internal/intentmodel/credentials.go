package intentmodel

import (
	"bytes"
	"encoding/json"
	"strings"
)

// ContainsCredential reports an exact echo of this adapter's configured key,
// including JSON-escaped string values. It never exposes the key to callers.
func (c *Client) ContainsCredential(data []byte) bool {
	if c.key == "" {
		return false
	}
	if bytes.Contains(data, []byte(c.key)) {
		return true
	}
	var value any
	if json.Unmarshal(data, &value) != nil {
		return false
	}
	var contains func(any) bool
	contains = func(v any) bool {
		switch x := v.(type) {
		case string:
			return strings.Contains(x, c.key)
		case []any:
			for _, item := range x {
				if contains(item) {
					return true
				}
			}
		case map[string]any:
			for key, item := range x {
				if strings.Contains(key, c.key) || contains(item) {
					return true
				}
			}
		}
		return false
	}
	return contains(value)
}

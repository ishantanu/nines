package model

import (
	"regexp"
	"sort"
)

// SLO labels classify objectives for roll-ups and filters (team, feature area,
// product). They are deliberately small and plain: they carry no measurement
// meaning, so they must never hold customer identifiers or secrets.
const (
	MaxSLOLabels     = 8
	MaxLabelKeyLen   = 32
	MaxLabelValueLen = 63
)

var (
	labelKey   = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,31}$`)
	labelValue = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._/-]{0,62}$`)
)

// ValidateSLOLabels bounds label count, key and value shape.
func ValidateSLOLabels(labels map[string]string) error {
	if len(labels) > MaxSLOLabels {
		return Invalid("an SLO may have at most %d labels", MaxSLOLabels)
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if !labelKey.MatchString(key) {
			return Invalid("label key %q must start with a lowercase letter and use up to %d lowercase letters, digits, dots, hyphens or underscores", key, MaxLabelKeyLen)
		}
		if !labelValue.MatchString(labels[key]) {
			return Invalid("label %q value must be 1 to %d characters of letters, digits, spaces, dots, slashes, hyphens or underscores, starting with a letter or digit", key, MaxLabelValueLen)
		}
	}
	return nil
}

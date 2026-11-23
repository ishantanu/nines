// Package postmortem holds the blameless writing aids for postmortems.
package postmortem

import (
	"regexp"
	"strings"
)

// Phrase is wording that judges past actions with hindsight, or blames, and
// a neutral way to say what is meant. The portal reads a generated copy.
type Phrase struct {
	Phrase     string `json:"phrase"`
	Suggestion string `json:"suggestion"`
}

// Phrases is the hindsight and blame wording a postmortem flags. Flags are
// advice, never a block: the writer decides.
var Phrases = []Phrase{
	{"should have", "Say what was known at the time and why the action made sense then."},
	{"should've", "Say what was known at the time and why the action made sense then."},
	{"could have", "Describe the options visible at the time, not the ones known now."},
	{"failed to", "Say what happened (\"did not\") and what made that reasonable then."},
	{"neglected", "Describe the conditions that made the step easy to miss."},
	{"forgot", "Describe the conditions that made the step easy to miss."},
	{"careless", "Describe the action and its context neutrally."},
	{"negligence", "Describe the action and its context neutrally."},
	{"mistake", "Describe the action and its context neutrally."},
	{"human error", "Name the conditions and missing safeguards that let a normal action cause harm."},
	{"operator error", "Name the conditions and missing safeguards that let a normal action cause harm."},
	{"root cause", "List contributing causes: incidents rarely have just one."},
	{"obviously", "It was not obvious at the time; describe what was visible."},
	{"clearly", "Describe what was visible at the time, not what is clear now."},
	{"blame", "Describe what happened and why, without assigning fault."},
}

var patterns = func() []*regexp.Regexp {
	out := make([]*regexp.Regexp, len(Phrases))
	for i, p := range Phrases {
		out[i] = regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(p.Phrase) + `\b`)
	}
	return out
}()

// Check returns the flagged phrases in text, once each, in list order.
func Check(text string) []Phrase {
	found := []Phrase{}
	if strings.TrimSpace(text) == "" {
		return found
	}
	for i, re := range patterns {
		if re.MatchString(text) {
			found = append(found, Phrases[i])
		}
	}
	return found
}

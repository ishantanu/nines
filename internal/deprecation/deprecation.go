// Package deprecation is the single source of truth for what Nines has
// deprecated. Warnings at the point of use (CLI, HTTP responses, Go client,
// Kubernetes CRDs) and the table in docs/public/compatibility.md come from it.
package deprecation

import (
	"net/http"
	"strings"
)

// Deprecation is one thing that still works but is scheduled for removal.
type Deprecation struct {
	ID string `json:"id"`
	// Surface is the compatibility surface, as named in the policy.
	Surface string `json:"surface"`
	// What is deprecated, in user terms.
	What string `json:"what"`
	// Instead is what to use, and how to move to it.
	Instead string `json:"instead"`
	// Since is the first release that announced the deprecation.
	Since string `json:"since"`
	// RemovalNotBefore is the earliest release that may remove it.
	RemovalNotBefore string `json:"removalNotBefore"`
	// CRDVersions are Kubernetes CRD versions, as plural.group/version, that
	// the deprecation covers; they are marked deprecated in the CRDs so the
	// API server warns kubectl and other clients that use them.
	CRDVersions []string `json:"crdVersions,omitempty"`
}

// ResourceV1alpha1 is v1alpha1 YAML for the kinds v1beta1 covers.
const ResourceV1alpha1 = "resource-v1alpha1"

var list = []Deprecation{
	{
		ID:               ResourceV1alpha1,
		Surface:          "YAML definitions",
		What:             "nines.dev/v1alpha1 ServiceLevelObjective, CriticalUserJourney, MeasurementBinding and ServiceDependencyMap documents",
		Instead:          "write nines.dev/v1beta1; nines export prints any stored definition in it",
		Since:            "v0.1.0-beta.1",
		RemovalNotBefore: "v2.0.0",
		CRDVersions:      []string{"servicelevelobjectives.nines.dev/v1alpha1", "criticaluserjourneys.nines.dev/v1alpha1", "measurementbindings.nines.dev/v1alpha1"},
	},
}

// List returns every current deprecation.
func List() []Deprecation { return append([]Deprecation(nil), list...) }

// Get returns a deprecation by ID.
func Get(id string) (Deprecation, bool) {
	for _, d := range list {
		if d.ID == id {
			return d, true
		}
	}
	return Deprecation{}, false
}

// ForCRDVersion returns the deprecation covering a CRD version.
func ForCRDVersion(crd, version string) (Deprecation, bool) {
	for _, d := range list {
		for _, v := range d.CRDVersions {
			if v == crd+"/"+version {
				return d, true
			}
		}
	}
	return Deprecation{}, false
}

// Message is the warning shown wherever the deprecated thing is used.
func (d Deprecation) Message() string {
	return d.What + " are deprecated and may be removed in " + d.RemovalNotBefore + " or later; " + d.Instead
}

// WarningHeader is the HTTP Warning value (RFC 7234, code 299) for d, the
// header Kubernetes also uses, so generic HTTP tooling can surface it.
func (d Deprecation) WarningHeader() string {
	return `299 - "` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(d.Message()) + `"`
}

// Warn adds the deprecation's Warning header to a response, once.
func Warn(w http.ResponseWriter, id string) {
	d, ok := Get(id)
	if !ok {
		return
	}
	value := d.WarningHeader()
	for _, existing := range w.Header().Values("Warning") {
		if existing == value {
			return
		}
	}
	w.Header().Add("Warning", value)
}

// ParseWarning extracts the text of a code 299 Warning header value, the
// form Warn writes. Other warnings are ignored.
func ParseWarning(value string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(value), "299 ")
	if !ok {
		return "", false
	}
	_, quoted, ok := strings.Cut(rest, " ")
	if !ok || len(quoted) < 2 || quoted[0] != '"' {
		return "", false
	}
	var b strings.Builder
	for i := 1; i < len(quoted); i++ {
		switch c := quoted[i]; c {
		case '\\':
			if i+1 < len(quoted) {
				i++
				b.WriteByte(quoted[i])
			}
		case '"':
			return b.String(), true
		default:
			b.WriteByte(c)
		}
	}
	return "", false
}

// Table is the Markdown table of current deprecations for the policy page.
func Table() string {
	var b strings.Builder
	b.WriteString("| Deprecated | Surface | Use instead | Since | Removed no earlier than |\n| --- | --- | --- | --- | --- |\n")
	for _, d := range list {
		b.WriteString("| " + d.What + " | " + d.Surface + " | " + d.Instead + " | " + d.Since + " | " + d.RemovalNotBefore + " |\n")
	}
	return b.String()
}

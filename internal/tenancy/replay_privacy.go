package tenancy

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func validateReplayPrivacy(p v1.ReplayPrivacy) error {
	if !utf8.ValidString(p.Contact) || len(p.Contact) > 256 || strings.ContainsAny(p.Contact, "\x00\r\n") {
		return fmt.Errorf("replayPrivacy contact must be plain text of at most 256 bytes")
	}
	if p.PolicyURL == "" {
		return nil
	}
	u, e := url.Parse(p.PolicyURL)
	if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(p.PolicyURL) > 2048 {
		return fmt.Errorf("replayPrivacy policyURL must be an HTTPS URL without credentials, query or fragment")
	}
	return nil
}

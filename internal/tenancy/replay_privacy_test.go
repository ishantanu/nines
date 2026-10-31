package tenancy

import (
	"testing"

	v1 "github.com/ishantanu/nines/api/v1alpha1"
)

func TestReplayPrivacyOnlyAcceptsSafeOperatorNotice(t *testing.T) {
	for _, p := range []v1.ReplayPrivacy{{}, {Disabled: true}, {PolicyURL: "https://example.org/privacy", Contact: "Privacy team"}} {
		if e := validateReplayPrivacy(p); e != nil {
			t.Fatal(e)
		}
	}
	for _, p := range []v1.ReplayPrivacy{{PolicyURL: "javascript:alert(1)"}, {PolicyURL: "https://user:secret@example.org/privacy"}, {PolicyURL: "https://example.org/privacy?token=secret"}, {Contact: "private\nheader"}} {
		if validateReplayPrivacy(p) == nil {
			t.Fatal("unsafe privacy notice accepted")
		}
	}
}

package model

import (
	"strings"
	"testing"
)

func TestCredentialIdentityStableAndRotationSensitive(t *testing.T) {
	a := CredentialIdentity("synthetic-user", "short-password")
	if len(a) != 64 || a != CredentialIdentity("synthetic-user", "short-password") {
		t.Fatal("credential identity is not restart-stable")
	}
	for _, parts := range [][]string{{"synthetic-user", "new-password"}, {"synthetic-other", "short-password"}, {"synthetic-user-short", "password"}} {
		if a == CredentialIdentity(parts...) {
			t.Fatal("rotation or input boundary lost")
		}
	}
	if strings.Contains(a, "short-password") || a == DefinitionDigest([]string{"synthetic-user", "short-password"}) {
		t.Fatal("credentials still use the fast content digest")
	}
}

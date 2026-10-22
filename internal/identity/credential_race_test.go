package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestLocalCredentialRecheckRejectsResetAndDisable(t *testing.T) {
	ctx := context.Background()
	config, err := demoAuthConfig("http://127.0.0.1", "admin", true, "admin")
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(ctx, config, NewMemory(), map[string]bool{"alpha": true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	verified, err := s.authenticateLocal(ctx, "local", "admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if cost, err := bcrypt.Cost([]byte(verified.localCredentialHash)); err != nil || cost != 12 {
		t.Fatal("authentication must retain the bcrypt credential for the transaction check", cost, err)
	}
	if _, _, err := s.login(ctx, "local", verified); err != nil {
		t.Fatal("unchanged verified credential rejected", err)
	}
	id := s.user("local", verified).ID
	rotated, err := bcrypt.GenerateFromPassword([]byte("replacement-password"), 12)
	if err != nil {
		t.Fatal(err)
	}
	for _, credential := range []localCredential{{Hash: rotated}, {Hash: []byte(verified.localCredentialHash), Disabled: true}} {
		if err := s.repo.Update(ctx, "user:"+id, func(tx Tx) error { return put(tx, "local_password", "global", id, credential, time.Time{}) }); err != nil {
			t.Fatal(err)
		}
		if token, _, err := s.login(ctx, "local", verified); !errors.Is(err, ErrForbidden) || token != "" {
			t.Fatal("stale/disabled credential created a session", err)
		}
	}
}

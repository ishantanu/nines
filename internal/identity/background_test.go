package identity

import (
	"context"
	"testing"
	"time"
)

func TestBackgroundAuthorizationRechecksSSOClaimsAndAccountRevocation(t *testing.T) {
	ctx := context.Background()
	s := mappingFixture(t, NewMemory())
	_, alice := mappedLogin(t, s, "alice", evidence([]string{"group-engineers"}, nil))
	if err := s.AuthorizeBackground(ctx, "alpha", alice.ID, "user"); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBackground(ctx, "beta", alice.ID, "user"); err == nil {
		t.Fatal("cross tenant background authorization")
	}
	mappedLogin(t, s, "alice", evidence(nil, []string{"Reliability.Reader"}))
	if err := s.AuthorizeBackground(ctx, "alpha", alice.ID, "user"); err == nil {
		t.Fatal("downgraded identity kept background access")
	}
	_, owner := mappedLogin(t, s, "owner", evidence(nil, []string{"Reliability.Admin"}))
	_, _, err := s.CreateAccount(ctx, owner, "alpha", "runner", "Runner", "agent", s.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AuthorizeBackground(ctx, "alpha", "runner", "service_account"); err != nil {
		t.Fatal(err)
	}
	if err = s.RevokeAccount(ctx, owner, "alpha", "runner"); err != nil {
		t.Fatal(err)
	}
	if err = s.AuthorizeBackground(ctx, "alpha", "runner", "service_account"); err == nil {
		t.Fatal("revoked account retained background access")
	}
	if err = s.AuthorizeBackground(ctx, "alpha", owner.ID, "api_token"); err == nil {
		t.Fatal("unknown actor kind accepted")
	}
	s.Now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if err = s.AuthorizeBackground(ctx, "alpha", owner.ID, "user"); err == nil {
		t.Fatal("expired SSO evidence retained background access")
	}
}

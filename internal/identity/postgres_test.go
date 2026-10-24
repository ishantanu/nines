package identity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func testPostgresContract(t *testing.T, db string) {
	ctx := context.Background()
	base := fmt.Sprintf("identity_fixture_%d", time.Now().UnixNano())
	cfg, err := url.Parse(db)
	if err != nil {
		t.Fatal(err)
	}
	query := cfg.Query()
	query.Set("search_path", base)
	cfg.RawQuery = query.Encode()
	databaseURL := cfg.String()
	repo, err := NewPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(base))
	schema := "waypoint_identity_" + hex.EncodeToString(sum[:6])
	defer func() {
		c, e := pgx.Connect(ctx, db)
		if e == nil {
			defer func() {
				_ = c.Close(ctx)
			}()
			_, e = c.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		}
		if e != nil {
			t.Error(e)
		}
	}()
	s, owner := serviceFixture(t, repo)
	token, _, err := s.login(ctx, "directory", externalUser{Subject: "alice", Name: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	member, err := s.Grant(ctx, owner, "alpha", "directory", "bob", "viewer", 0)
	if err != nil {
		t.Fatal(err)
	}
	team, err := s.SaveTeam(ctx, owner, "alpha", Team{ID: "ops", Name: "Ops", Role: "editor", Members: []string{member.UserID}}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RecordAudit(ctx, "alpha", owner, "POST", "/test", "HTTP 200", "restart", ""); err != nil {
		t.Fatal(err)
	}
	sso := mappingFixture(t, repo)
	ssoToken, _ := mappedLogin(t, sso, "federated-user", evidence([]string{"group-engineers"}, []string{"Reliability.Reader"}))
	repo.Close()
	reopened, err := NewPostgres(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	ssoAfter := mappingFixture(t, reopened)
	mapped, err := ssoAfter.SessionActor(ctx, ssoToken)
	if err != nil || mapped.Grants["alpha"] != "editor" || mapped.Grants["beta"] != "viewer" {
		t.Fatal("federated grants did not survive restart", mapped, err)
	}
	// A persisted session cannot carry direct membership across a client change.
	if err := reopened.Update(ctx, "tenant:alpha", func(tx Tx) error {
		return ssoAfter.saveMember(tx, "alpha", Member{UserID: mapped.ID, Role: "owner", EffectiveRole: "owner", Version: 1})
	}); err != nil {
		t.Fatal(err)
	}
	changed := mappingFixture(t, reopened)
	p := changed.providers["microsoft"]
	p.config.ClientID = "replacement-app"
	changed.providers["microsoft"] = p
	if _, err := changed.SessionActor(ctx, ssoToken); !errors.Is(err, ErrForbidden) {
		t.Fatal("persisted session crossed audience change", err)
	}
	if err := reopened.Update(ctx, "tenant:alpha", func(tx Tx) error { return tx.Delete("grant", mapped.ID, "alpha") }); err != nil {
		t.Fatal(err)
	}
	mappedLogin(t, ssoAfter, "federated-user", evidence(nil, nil))
	mapped, err = ssoAfter.SessionActor(ctx, ssoToken)
	if err != nil || len(mapped.Grants) != 0 {
		t.Fatal("persistent old session retained removed assignment", mapped, err)
	}
	after, err := New(ctx, s.config, reopened, s.tenants)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := after.SessionActor(ctx, token)
	if err != nil || actual.Grants["alpha"] != "owner" {
		t.Fatal(actual, err)
	}
	_, bob, err := after.login(ctx, "directory", externalUser{Subject: "bob", Name: "Bob"})
	if err != nil || bob.Grants["alpha"] != "editor" {
		t.Fatal(bob, err)
	}
	if err = after.RemoveTeam(ctx, actual, "alpha", team.ID, team.Version); err != nil {
		t.Fatal(err)
	}
	events, err := after.AuditPage(ctx, actual, "alpha", "")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, event := range events {
		if event.RequestID == "restart" {
			found++
		}
	}
	if found != 1 {
		t.Fatal("restart audit record missing or duplicated", events)
	}
}
